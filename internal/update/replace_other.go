//go:build !windows

package update

import (
	"os"
	"syscall"
)

func removeOld(string) {}

// The running process keeps the old file's inode, so a rename replaces it in place.
// Under sudo the download belongs to root; it takes the old file's owner so the
// person can still rebuild or delete it.
func replace(exe, downloaded string) error {
	if info, err := os.Stat(exe); err == nil {
		if st, ok := info.Sys().(*syscall.Stat_t); ok {
			_ = os.Chown(downloaded, int(st.Uid), int(st.Gid))
		}
	}
	return os.Rename(downloaded, exe)
}

// relaunch returns only on failure: exec keeps the PID, the terminal and sudo's root.
func relaunch(exe string, env []string) (int, error) {
	return 1, syscall.Exec(exe, os.Args, env)
}
