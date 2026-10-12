//go:build !windows

package update

import (
	"os"
	"syscall"
)

func removeLeftover(string) {}

// The running process keeps the old file's inode, so a rename replaces it in place.
func replace(exe, downloaded string) error { return os.Rename(downloaded, exe) }

// relaunch returns only on failure: exec keeps the PID, the terminal and sudo's root.
func relaunch(exe string, env []string) (int, error) {
	return 1, syscall.Exec(exe, os.Args, env)
}
