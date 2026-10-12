package update

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testPlatform = "linux/amd64"

type release struct {
	manifest, signature []byte
	binaries            map[string][]byte // path under /download/ → contents
}

func newRelease(t *testing.T, version string, binary []byte, key ed25519.PrivateKey) release {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "discord-go-vpn"), binary, 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := BuildManifest(version, dir, map[string]string{testPlatform: "discord-go-vpn"})
	if err != nil {
		t.Fatal(err)
	}
	manifest, signature, err := Sign(m, key)
	if err != nil {
		t.Fatal(err)
	}
	return release{manifest, signature, map[string][]byte{version + "/discord-go-vpn": binary}}
}

func serve(t *testing.T, r release) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch path := req.URL.Path; {
		case path == "/latest/download/"+ManifestName:
			_, _ = w.Write(r.manifest)
		case path == "/latest/download/"+SignatureName:
			_, _ = w.Write(r.signature)
		case strings.HasPrefix(path, "/download/") && r.binaries[strings.TrimPrefix(path, "/download/")] != nil:
			_, _ = w.Write(r.binaries[strings.TrimPrefix(path, "/download/")])
		default:
			http.NotFound(w, req)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func newKey(t *testing.T) (string, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(pub), priv
}

func testUpdater(url, version, pub string) updater {
	return updater{client: http.DefaultClient, releasesURL: url, version: version, publicKey: pub, platform: testPlatform}
}

func TestCheckFindsNewerRelease(t *testing.T) {
	pub, priv := newKey(t)
	url := serve(t, newRelease(t, "v1.2.0", []byte("new"), priv))

	m, asset, err := testUpdater(url, "v1.1.9", pub).check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if asset == nil || m.Version != "v1.2.0" || asset.Name != "discord-go-vpn" {
		t.Fatalf("got %+v, %+v; want v1.2.0's asset", m, asset)
	}
}

func TestCheckIgnoresSameOrOlderRelease(t *testing.T) {
	pub, priv := newKey(t)
	url := serve(t, newRelease(t, "v1.2.0", []byte("new"), priv))

	for _, current := range []string{"v1.2.0", "v1.10.0"} {
		if _, asset, err := testUpdater(url, current, pub).check(context.Background()); err != nil || asset != nil {
			t.Errorf("current %s: got asset %+v, err %v; want neither", current, asset, err)
		}
	}
}

func TestCheckIgnoresMissingPlatform(t *testing.T) {
	pub, priv := newKey(t)
	url := serve(t, newRelease(t, "v1.2.0", []byte("new"), priv))
	u := testUpdater(url, "v1.0.0", pub)
	u.platform = "windows/arm64"

	if _, asset, err := u.check(context.Background()); err != nil || asset != nil {
		t.Fatalf("got asset %+v, err %v; want neither", asset, err)
	}
}

func TestCheckRejectsForeignSignature(t *testing.T) {
	pub, _ := newKey(t)
	_, otherPriv := newKey(t)
	url := serve(t, newRelease(t, "v9.0.0", []byte("evil"), otherPriv))

	if _, _, err := testUpdater(url, "v1.0.0", pub).check(context.Background()); err == nil {
		t.Fatal("a manifest signed by another key was accepted")
	}
}

func TestCheckRejectsEditedManifest(t *testing.T) {
	pub, priv := newKey(t)
	r := newRelease(t, "v1.2.0", []byte("new"), priv)
	r.manifest = []byte(strings.Replace(string(r.manifest), "v1.2.0", "v1.3.0", 1))

	if _, _, err := testUpdater(serve(t, r), "v1.0.0", pub).check(context.Background()); err == nil {
		t.Fatal("an edited manifest was accepted")
	}
}

func TestDownloadVerifiesHash(t *testing.T) {
	pub, priv := newKey(t)
	r := newRelease(t, "v1.2.0", []byte("new"), priv)
	u := testUpdater(serve(t, r), "v1.0.0", pub)
	m, asset, err := u.check(context.Background())
	if err != nil || asset == nil {
		t.Fatalf("check: %+v, %v", asset, err)
	}

	dir := t.TempDir()
	path, err := u.download(context.Background(), m.Version, *asset, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "new" {
		t.Fatalf("downloaded %q, want %q", got, "new")
	}

	r.binaries["v1.2.0/discord-go-vpn"] = []byte("bad")
	if _, err := testUpdater(serve(t, r), "v1.0.0", pub).download(context.Background(), m.Version, *asset, dir); err == nil {
		t.Fatal("a binary that doesn't match the manifest was accepted")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("the rejected download was left in %s: %v", dir, entries)
	}
}

func TestReplace(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "discord-go-vpn")
	downloaded := filepath.Join(dir, ".update")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(downloaded, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := replace(exe, downloaded); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "new" {
		t.Fatalf("executable holds %q, want %q", got, "new")
	}
}

func TestEnabled(t *testing.T) {
	pub, _ := newKey(t)
	for _, tc := range []struct {
		version, key string
		want         bool
	}{
		{"v1.0.0", pub, true},
		{"dev", pub, false},
		{"v1.0.0", "", false},
	} {
		if got := testUpdater("", tc.version, tc.key).enabled(); got != tc.want {
			t.Errorf("version %q, key %q: enabled = %v, want %v", tc.version, tc.key, got, tc.want)
		}
	}
}
