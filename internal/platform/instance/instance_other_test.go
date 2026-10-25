//go:build !windows

package instance

import (
	"errors"
	"os"
	"os/exec"
	"testing"
)

// The lock is per open file, so a second Hold in this process sees it held the
// way another process would.
func TestHoldLetsOneHolderIn(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	release, err := Hold("panel")
	if err != nil {
		t.Fatalf("first Hold: %v", err)
	}
	if !Held("panel") {
		t.Error("Held must see the lock taken")
	}
	if _, err := Hold("panel"); !errors.Is(err, ErrHeld) {
		t.Errorf("second Hold = %v, want ErrHeld", err)
	}
	if Held("tray") {
		t.Error("locks with other names are separate")
	}

	release()
	if Held("panel") {
		t.Error("the lock outlived its release")
	}
}

func TestHoldIsReleasedWhenTheHolderExits(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	if _, err := exec.LookPath("flock"); err != nil {
		t.Skip("flock(1) not installed")
	}
	// Another process takes the lock and exits without releasing it.
	if err := exec.Command("flock", "-n", path("panel"), "true").Run(); err != nil {
		t.Fatal(err)
	}
	if Held("panel") {
		t.Error("a lock whose holder exited still reads as held")
	}
	_ = os.Remove(path("panel"))
}
