package profile

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeConf(t *testing.T, dir, name string, modTime time.Time) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("[Interface]\n"), 0o600); err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("setting the time of %s: %v", name, err)
	}
	return path
}

// inDir makes dir the working directory. Discover also looks beside the test
// binary, which holds no .conf files.
func inDir(t *testing.T, dir string) {
	t.Helper()

	prevWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("reading the working directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("entering %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevWd) })
}

func TestDiscoverSortsNewestFirst(t *testing.T) {
	dir := t.TempDir()
	inDir(t, dir)

	now := time.Now()
	writeConf(t, dir, "old.conf", now.Add(-48*time.Hour))
	writeConf(t, dir, "newest.conf", now)
	writeConf(t, dir, "middle.conf", now.Add(-2*time.Hour))

	found := Discover()
	if len(found) != 3 {
		t.Fatalf("found %d profiles, want 3: %v", len(found), found)
	}

	wantOrder := []string{"newest.conf", "middle.conf", "old.conf"}
	for i, name := range wantOrder {
		if got := filepath.Base(found[i]); got != name {
			t.Errorf("position %d = %q, want %q", i, got, name)
		}
	}
}

func TestDiscoverSkipsExample(t *testing.T) {
	dir := t.TempDir()
	inDir(t, dir)

	writeConf(t, dir, "example.conf", time.Now())
	writeConf(t, dir, "real.conf", time.Now().Add(-time.Hour))

	found := Discover()
	for _, match := range found {
		if filepath.Base(match) == "example.conf" {
			t.Errorf("the example file was listed: %v", found)
		}
	}
	if len(found) != 1 {
		t.Errorf("found %d profiles, want 1: %v", len(found), found)
	}
}

func TestDiscoverWithoutProfilesReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	inDir(t, dir)

	if found := Discover(); len(found) != 0 {
		t.Errorf("found profiles in an empty folder: %v", found)
	}
}

func TestDiscoverIgnoresOtherExtensions(t *testing.T) {
	dir := t.TempDir()
	inDir(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("nothing"), 0o600); err != nil {
		t.Fatalf("creating file: %v", err)
	}
	writeConf(t, dir, "profile.conf", time.Now())

	found := Discover()
	if len(found) != 1 || filepath.Base(found[0]) != "profile.conf" {
		t.Errorf("unexpected list: %v", found)
	}
}
