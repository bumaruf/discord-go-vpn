package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"golang.org/x/mod/semver"
)

// Version is set from the release tag by the Makefile; "dev" never updates.
var Version = "dev"

// publicKey verifies update.json.sig. Empty disables updates: docs in README.md → Atualização automática.
const publicKey = "UmfK2XPQGeVCI6k2aF9feMhN32NCnFQ4ogeQqtZ+xL4="

const releasesURL = "https://github.com/bumaruf/discord-go-vpn/releases"

// relaunchedEnv marks the process Relaunch started, so a release whose binary still
// reports the old version cannot loop.
const relaunchedEnv = "VPN_UPDATE_RELAUNCHED"

const (
	checkTimeout    = 10 * time.Second
	downloadTimeout = 5 * time.Minute
	maxManifestSize = 64 << 10
	tempPattern     = ".discord-go-vpn-update-*"
)

var errNotFound = errors.New("o GitHub respondeu HTTP 404")

// errNoManifest is a latest release without update.json, as every release
// published before the updater was.
var errNoManifest = errors.New("release sem " + ManifestName)

type updater struct {
	client      *http.Client
	releasesURL string
	version     string
	publicKey   string
	platform    string
}

// Progress receives the download of a newer release as it goes; total is the
// size the signed manifest promises.
type Progress func(version string, done, total int64)

// Installed is a newer release already written over the running executable.
type Installed struct {
	Version string
	exe     string
}

// Install writes a newer release over the running executable; the caller then
// runs it with Relaunch. Without a newer release, in a dev build, or in the
// process a relaunch started, it returns nil. progress may be nil.
func Install(ctx context.Context, progress Progress) (*Installed, error) {
	if os.Getenv(relaunchedEnv) != "" {
		_ = os.Unsetenv(relaunchedEnv)
		return nil, nil
	}
	u := updater{
		client:      http.DefaultClient,
		releasesURL: releasesURL,
		version:     Version,
		publicKey:   publicKey,
		platform:    Platform(runtime.GOOS, runtime.GOARCH),
	}
	if !u.enabled() {
		return nil, nil
	}

	exe, err := executable()
	if err != nil {
		return nil, err
	}
	removeLeftovers(exe)

	m, asset, err := u.check(ctx)
	if errors.Is(err, errNoManifest) {
		return nil, nil
	}
	if err != nil || asset == nil {
		return nil, err
	}
	if progress != nil {
		progress(m.Version, 0, asset.Size)
	}
	downloaded, err := u.download(ctx, m.Version, *asset, filepath.Dir(exe), progress)
	if err != nil {
		return nil, err
	}
	if err := replace(exe, downloaded); err != nil {
		_ = os.Remove(downloaded)
		return nil, fmt.Errorf("não consegui substituir %s: %w", exe, err)
	}
	return &Installed{Version: m.Version, exe: exe}, nil
}

// Relaunch runs the installed release in this process's place and returns the
// code to exit with; an error means the relaunch failed.
func (i *Installed) Relaunch() (int, error) {
	code, err := relaunch(i.exe, append(os.Environ(), relaunchedEnv+"=1"))
	if err != nil {
		return 1, fmt.Errorf("a versão %s foi instalada, mas não consegui reabrir o programa (abra de novo): %w", i.Version, err)
	}
	return code, nil
}

// Enabled reports whether this build looks for updates at all.
func Enabled() bool {
	return updater{version: Version, publicKey: publicKey}.enabled()
}

func (u updater) enabled() bool {
	return u.publicKey != "" && semver.IsValid(u.version)
}

// check returns the latest manifest and this platform's asset, or a nil asset
// when the release is not newer or has no build for this platform.
func (u updater) check(ctx context.Context) (Manifest, *Asset, error) {
	key, err := parsePublicKey(u.publicKey)
	if err != nil {
		return Manifest{}, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	// latest/download redirects to the newest release without the API's rate limit.
	manifest, err := u.fetch(ctx, u.releasesURL+"/latest/download/"+ManifestName)
	if errors.Is(err, errNotFound) {
		return Manifest{}, nil, errNoManifest
	}
	if err != nil {
		return Manifest{}, nil, err
	}
	signature, err := u.fetch(ctx, u.releasesURL+"/latest/download/"+SignatureName)
	if err != nil {
		return Manifest{}, nil, err
	}
	m, err := Verify(manifest, signature, key)
	if err != nil {
		return Manifest{}, nil, err
	}
	if semver.Compare(m.Version, u.version) <= 0 {
		return m, nil, nil
	}
	asset, ok := m.Assets[u.platform]
	if !ok {
		return m, nil, nil
	}
	return m, &asset, nil
}

func (u updater) fetch(ctx context.Context, url string) ([]byte, error) {
	resp, err := u.get(ctx, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, maxManifestSize))
}

func (u updater) get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := u.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("não consegui falar com o GitHub: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, fmt.Errorf("%w para %s", errNotFound, url)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("o GitHub respondeu HTTP %d para %s", resp.StatusCode, url)
	}
	return resp, nil
}

// download writes the asset into dir, beside the executable, so replacing it is
// a rename on the same file system, and returns the file only if its size and
// SHA-256 match the signed manifest.
func (u updater) download(ctx context.Context, version string, asset Asset, dir string, progress Progress) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	resp, err := u.get(ctx, u.releasesURL+"/download/"+version+"/"+asset.Name)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	f, err := os.CreateTemp(dir, tempPattern)
	if err != nil {
		return "", fmt.Errorf("não consegui gravar a atualização em %s: %w", dir, err)
	}
	hash := sha256.New()
	body := io.LimitReader(resp.Body, asset.Size+1)
	if progress != nil {
		body = &progressReader{r: body, report: func(done int64) { progress(version, done, asset.Size) }}
	}
	n, copyErr := io.Copy(io.MultiWriter(f, hash), body)
	closeErr := f.Close()
	err = errors.Join(copyErr, closeErr)
	if err == nil && (n != asset.Size || hex.EncodeToString(hash.Sum(nil)) != asset.SHA256) {
		err = errors.New("o arquivo baixado não confere com o manifesto assinado")
	}
	if err == nil {
		err = os.Chmod(f.Name(), 0o755)
	}
	if err != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("download da versão %s falhou: %w", version, err)
	}
	return f.Name(), nil
}

// removeLeftovers deletes what an earlier update left beside exe: a download
// interrupted before the rename and, on Windows, the replaced executable.
func removeLeftovers(exe string) {
	removeOld(exe)
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), tempPattern))
	for _, path := range matches {
		_ = os.Remove(path)
	}
}

func executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

type progressReader struct {
	r      io.Reader
	done   int64
	report func(done int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	p.report(p.done)
	return n, err
}
