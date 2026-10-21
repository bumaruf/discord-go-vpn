//go:build !windows

package discord

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeCommands puts pkill, pgrep, sudo and a Discord executable on PATH that
// only record how they were run, so the test touches no real process.
func fakeCommands(t *testing.T) (exe, record string) {
	t.Helper()
	dir := t.TempDir()
	record = filepath.Join(dir, "record")

	script := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	script("pkill", "exit 0")
	script("pgrep", "exit 1")
	script("sudo", `echo "sudo $*" > '`+record+`'`)
	exe = script("Discord", `echo "direct" > '`+record+`'`)

	t.Setenv("PATH", dir)
	return exe, record
}

func readRecord(t *testing.T, record string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(record); err == nil && strings.HasSuffix(string(data), "\n") {
			return strings.TrimSpace(string(data))
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the relaunched command never ran")
	return ""
}

func TestStopSendsOnlySIGKILL(t *testing.T) {
	dir := t.TempDir()
	record := filepath.Join(dir, "record")
	if err := os.WriteFile(filepath.Join(dir, "pkill"), []byte("#!/bin/sh\necho \"$*\" >> '"+record+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pgrep"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	Stop()

	data, _ := os.ReadFile(record)
	calls := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(calls) != len(unixNames) {
		t.Fatalf("pkill ran %d times, want once per name: %q", len(calls), calls)
	}
	for _, call := range calls {
		// SIGTERM crashes Discord with a core dump the desktop reports.
		if !strings.HasPrefix(call, "-9 -x ") {
			t.Errorf("pkill %s: Discord must be stopped with SIGKILL only", call)
		}
	}
}

func TestRestartAsUserRunsDiscordAsTheSudoUser(t *testing.T) {
	exe, record := fakeCommands(t)
	t.Setenv("SUDO_USER", "alice")

	if _, err := RestartAsUser(exe); err != nil {
		t.Fatalf("RestartAsUser: %v", err)
	}

	got := strings.Fields(readRecord(t, record))
	if len(got) < 5 || strings.Join(got[:4], " ") != "sudo -u alice env" || got[len(got)-1] != exe {
		t.Errorf("Discord was relaunched as %q, want sudo -u alice env ... %s", got, exe)
	}
}

func TestRestartAsUserRunsDiscordDirectlyWithoutSudo(t *testing.T) {
	exe, record := fakeCommands(t)
	t.Setenv("SUDO_USER", "")

	if _, err := RestartAsUser(exe); err != nil {
		t.Fatalf("RestartAsUser: %v", err)
	}

	if got := readRecord(t, record); got != "direct" {
		t.Errorf("Discord was relaunched as %q, want it run directly", got)
	}
}

func TestGraphicalEnvRecoversTheWaylandSessionSudoCleared(t *testing.T) {
	runtimeDir := t.TempDir()
	for _, name := range []string{"wayland-1", "wayland-1.lock", "bus"} {
		if err := os.WriteFile(filepath.Join(runtimeDir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	for _, name := range []string{"WAYLAND_DISPLAY", "XDG_SESSION_TYPE", "DBUS_SESSION_BUS_ADDRESS"} {
		t.Setenv(name, "")
	}

	got := strings.Join(graphicalEnv("alice"), " ")
	for _, want := range []string{
		"WAYLAND_DISPLAY=wayland-1",
		"XDG_SESSION_TYPE=wayland",
		"DBUS_SESSION_BUS_ADDRESS=unix:path=" + filepath.Join(runtimeDir, "bus"),
	} {
		if !strings.Contains(got, want) {
			t.Errorf("graphicalEnv = %q, missing %s", got, want)
		}
	}
}

func TestGraphicalEnvKeepsTheSessionItWasGiven(t *testing.T) {
	runtimeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(runtimeDir, "wayland-0"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Setenv("WAYLAND_DISPLAY", "wayland-1")
	t.Setenv("XDG_SESSION_TYPE", "x11")

	got := strings.Join(graphicalEnv("alice"), " ")
	if strings.Count(got, "WAYLAND_DISPLAY=") != 1 || !strings.Contains(got, "WAYLAND_DISPLAY=wayland-1") {
		t.Errorf("graphicalEnv = %q, want only the WAYLAND_DISPLAY it was given", got)
	}
	if strings.Count(got, "XDG_SESSION_TYPE=") != 1 || !strings.Contains(got, "XDG_SESSION_TYPE=x11") {
		t.Errorf("graphicalEnv = %q, want only the XDG_SESSION_TYPE it was given", got)
	}
}
