//go:build windows

package update

import (
	"os"
	"os/exec"
	"os/signal"
)

// Windows refuses to overwrite a running executable but lets it be renamed, so
// the old one moves aside and the next start deletes it.
func leftover(exe string) string { return exe + ".old" }

func removeOld(exe string) { _ = os.Remove(leftover(exe)) }

func replace(exe, downloaded string) error {
	removeOld(exe)
	if err := os.Rename(exe, leftover(exe)); err != nil {
		return err
	}
	if err := os.Rename(downloaded, exe); err != nil {
		_ = os.Rename(leftover(exe), exe)
		return err
	}
	return nil
}

// relaunch has no exec on Windows: the new version runs as a child sharing this
// console, and this process waits to pass on its exit code. Ctrl+C reaches both,
// and only the child should act on it.
func relaunch(exe string, env []string) (int, error) {
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = env
	signal.Ignore(os.Interrupt)
	if err := cmd.Start(); err != nil {
		return 1, err
	}
	_ = cmd.Wait()
	return cmd.ProcessState.ExitCode(), nil
}
