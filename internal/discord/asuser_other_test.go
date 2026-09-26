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
