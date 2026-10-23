package instance

import "errors"

// ErrHeld is another process holding the lock.
var ErrHeld = errors.New("já está aberto")

// Hold never fails on Windows, where the screen and the tray share one process.
func Hold(string) (release func(), err error) { return func() {}, nil }

func Held(string) bool { return false }
