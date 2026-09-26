//go:build !windows

package console

// Only wiresock mode elevates; on Linux the netns error asks for sudo instead.
func IsElevated() bool { return false }
func Elevate() bool    { return false }
func HoldWindow()      {}
