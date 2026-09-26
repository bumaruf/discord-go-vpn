// Package routing names the ways Discord is put inside the tunnel.
package routing

import "runtime"

// Mode is how Discord is put inside the tunnel. Every mode captures the whole
// process, so call media goes through the tunnel too.
type Mode string

const (
	// ModeNetns runs Discord in a network namespace whose only exit is the
	// tunnel. Linux only; needs root.
	ModeNetns Mode = "netns"

	// ModeWiresock captures Discord's processes with WireSock's WFP filter.
	// Windows only; needs administrator.
	ModeWiresock Mode = "wiresock"
)

// Default returns this system's mode, or "" where no mechanism can capture a
// whole process.
func Default() Mode {
	switch runtime.GOOS {
	case "linux":
		return ModeNetns
	case "windows":
		return ModeWiresock
	}
	return ""
}
