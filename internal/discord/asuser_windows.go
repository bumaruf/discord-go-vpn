//go:build windows

package discord

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procCreateProcessWithTokenW = windows.NewLazySystemDLL("advapi32.dll").NewProc("CreateProcessWithTokenW")

// RestartAsUser reopens Discord minimized, without this process's elevation,
// which an elevated Discord would inherit, losing file drag-and-drop.
func RestartAsUser(explicit string) (string, error) {
	exe, err := Resolve(explicit)
	if err != nil {
		return "", err
	}

	Stop()

	if err := startAsShellUser(exe, MinimizedFlag); err == nil {
		return exe, nil
	}
	// Explorer also runs as the session user, but passes no arguments, so
	// Discord opens in the foreground. It returns at once and reports nothing
	// about what it opens.
	if err := exec.Command("explorer.exe", exe).Start(); err != nil {
		return "", fmt.Errorf("pedindo ao Explorer para abrir %s: %w", exe, err)
	}
	return exe, nil
}

// startAsShellUser starts argv with the token of the desktop shell's process,
// the unelevated session user, in a minimized window that takes no focus.
func startAsShellUser(argv ...string) error {
	shell := windows.GetShellWindow()
	if shell == 0 {
		return errors.New("não há shell na sessão")
	}
	var pid uint32
	if _, err := windows.GetWindowThreadProcessId(shell, &pid); err != nil {
		return err
	}
	proc, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(proc) }()

	var shellToken, token windows.Token
	if err := windows.OpenProcessToken(proc, windows.TOKEN_DUPLICATE, &shellToken); err != nil {
		return err
	}
	defer func() { _ = shellToken.Close() }()
	access := uint32(windows.TOKEN_QUERY | windows.TOKEN_DUPLICATE | windows.TOKEN_ASSIGN_PRIMARY |
		windows.TOKEN_ADJUST_DEFAULT | windows.TOKEN_ADJUST_SESSIONID)
	if err := windows.DuplicateTokenEx(shellToken, access, nil, windows.SecurityImpersonation, windows.TokenPrimary, &token); err != nil {
		return err
	}
	defer func() { _ = token.Close() }()

	var env *uint16
	if err := windows.CreateEnvironmentBlock(&env, token, false); err != nil {
		return err
	}
	defer func() { _ = windows.DestroyEnvironmentBlock(env) }()

	cmdline, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(argv))
	if err != nil {
		return err
	}
	dir, err := windows.UTF16PtrFromString(filepath.Dir(argv[0]))
	if err != nil {
		return err
	}
	si := windows.StartupInfo{
		Flags:      windows.STARTF_USESHOWWINDOW,
		ShowWindow: windows.SW_SHOWMINNOACTIVE,
	}
	si.Cb = uint32(unsafe.Sizeof(si))
	var pi windows.ProcessInformation

	r, _, err := procCreateProcessWithTokenW.Call(uintptr(token), 0, 0, uintptr(unsafe.Pointer(cmdline)),
		windows.CREATE_UNICODE_ENVIRONMENT, uintptr(unsafe.Pointer(env)), uintptr(unsafe.Pointer(dir)),
		uintptr(unsafe.Pointer(&si)), uintptr(unsafe.Pointer(&pi)))
	if r == 0 {
		return err
	}
	_ = windows.CloseHandle(pi.Thread)
	_ = windows.CloseHandle(pi.Process)
	return nil
}
