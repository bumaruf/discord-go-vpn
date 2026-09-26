package routing

import (
	"runtime"
	"testing"
)

func TestDefaultPerPlatform(t *testing.T) {
	want := map[string]Mode{
		"linux":   ModeNetns,
		"windows": ModeWiresock,
	}[runtime.GOOS]

	if got := Default(); got != want {
		t.Errorf("Default() = %q on %s, want %q", got, runtime.GOOS, want)
	}
}
