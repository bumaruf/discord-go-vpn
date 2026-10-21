//go:build !windows

// Package instance keeps one copy of a window per desktop session, with a lock
// file in the runtime dir that the kernel releases when the holder exits.
package instance

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// ErrHeld is another process holding the lock.
var ErrHeld = errors.New("já está aberto")

// Hold takes the lock called name until release, or fails with ErrHeld.
func Hold(name string) (release func(), err error) {
	f, err := os.OpenFile(path(name), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, ErrHeld
	}
	return func() { _ = f.Close() }, nil
}

// Held reports whether another process holds the lock called name.
func Held(name string) bool {
	release, err := Hold(name)
	if err != nil {
		return errors.Is(err, ErrHeld)
	}
	release()
	return false
}

func path(name string) string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "discord-go-vpn-"+name+".lock")
}
