//go:build windows

package console

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// OwnsConsole reports whether this process is its console's only user, which
// means a double click created the console and Windows closes it on exit.
func OwnsConsole() bool {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GetConsoleProcessList")
	if proc.Find() != nil {
		return false
	}

	var pids [4]uint32
	n, _, _ := proc.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	return n == 1
}

func IsElevated() bool {
	var sid *windows.SID
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY, 2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0, &sid)
	if err != nil {
		return false
	}
	defer func() { _ = windows.FreeSid(sid) }()

	member, err := windows.Token(0).IsMember(sid)
	return err == nil && member
}

// Elevate relaunches this program through the UAC prompt and reports whether
// the elevated copy started; the caller must then exit.
func Elevate() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}

	// Elevated, Windows would start in system32 and miss the profile beside the
	// executable.
	dir := filepath.Dir(exe)
	args := strings.Join(os.Args[1:], " ")

	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exe)
	workDir, _ := syscall.UTF16PtrFromString(dir)
	var params *uint16
	if args != "" {
		params, _ = syscall.UTF16PtrFromString(args)
	}

	// SW_NORMAL: the elevated copy's window carries on the work.
	if err := windows.ShellExecute(0, verb, file, params, workDir, windows.SW_NORMAL); err != nil {
		return false
	}
	return true
}

// HoldWindow waits for Enter when the console is ours, so the final message
// stays readable.
func HoldWindow() {
	if !OwnsConsole() {
		return
	}
	fmt.Print("\npressione Enter para fechar…")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}
