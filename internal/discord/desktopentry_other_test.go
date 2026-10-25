//go:build !windows

package discord

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeDesktopEntry puts an entry for id in the applications dir of a fresh
// home, which becomes the person's own (no sudo, no XDG_DATA_HOME).
func writeDesktopEntry(t *testing.T, id, body string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SUDO_USER", "")
	t.Setenv("XDG_DATA_HOME", "")

	dir := filepath.Join(home, ".local", "share", "applications")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".desktop"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// launcher writes an executable outside PATH, as a wrapper the person made.
func launcher(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "launch.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFindPrefersTheUsersDesktopEntry(t *testing.T) {
	exe, _ := fakeCommands(t) // a Discord on PATH, found without the entry
	wrapper := launcher(t)
	writeDesktopEntry(t, "discord", "[Desktop Entry]\nName=Discord\nExec=\""+wrapper+"\" %U\nType=Application\n")

	got, err := Find()
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got != wrapper {
		t.Errorf("Find = %q, want the desktop entry's %q over PATH's %q", got, wrapper, exe)
	}
}

func TestUserDesktopEntryIgnoresWhatItCannotRun(t *testing.T) {
	wrapper := launcher(t)
	cases := map[string]string{
		"with arguments":  "[Desktop Entry]\nExec=" + wrapper + " --flag\n",
		"hidden":          "[Desktop Entry]\nExec=" + wrapper + "\nHidden=true\n",
		"outside entry":   "[Desktop Action new]\nExec=" + wrapper + "\n",
		"missing program": "[Desktop Entry]\nExec=/nonexistent/discord\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			writeDesktopEntry(t, "discord", body)
			if got := userDesktopEntry(); got != "" {
				t.Errorf("userDesktopEntry = %q, want none", got)
			}
		})
	}
}

func TestUserDesktopEntryWithoutOneFindsNothing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SUDO_USER", "")
	t.Setenv("XDG_DATA_HOME", "")
	if got := userDesktopEntry(); got != "" {
		t.Errorf("userDesktopEntry = %q, want none", got)
	}
}

func TestSplitExec(t *testing.T) {
	cases := map[string][]string{
		`/usr/bin/discord`:           {"/usr/bin/discord"},
		`discord %U`:                 {"discord"},
		`"/opt/my dir/launch.sh" %u`: {"/opt/my dir/launch.sh"},
		`"/a/\"q\"/x" --flag`:        {`/a/"q"/x`, "--flag"},
		`env FOO=1 discord`:          {"env", "FOO=1", "discord"},
		`discord --name=100%% %f`:    {"discord", "--name=100%"},
		`  discord   --flag  `:       {"discord", "--flag"},
		``:                           nil,
	}
	for value, want := range cases {
		got := splitExec(value)
		if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
			t.Errorf("splitExec(%q) = %q, want %q", value, got, want)
		}
	}
}
