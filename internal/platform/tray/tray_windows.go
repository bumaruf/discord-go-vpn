//go:build windows

// Package tray sends the console window to the notification area when it is
// minimised. It calls the Win32 API directly because tray libraries need cgo,
// which would break cross-compiling the .exe from Linux.
package tray

import (
	"os"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procGetConsoleWindow = kernel32.NewProc("GetConsoleWindow")
	procShowWindow       = user32.NewProc("ShowWindow")
	procIsWindowVisible  = user32.NewProc("IsWindowVisible")
	procIsIconic         = user32.NewProc("IsIconic")
	procSetForegroundWin = user32.NewProc("SetForegroundWindow")
	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")
	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procGetMessageW      = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")
	procLoadIconW        = user32.NewProc("LoadIconW")
	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procExtractIconExW   = shell32.NewProc("ExtractIconExW")
	procDestroyIcon      = user32.NewProc("DestroyIcon")
)

const (
	swHide    = 0
	swRestore = 9

	nimAdd    = 0x0
	nimModify = 0x1
	nimDelete = 0x2

	nifMessage = 0x1
	nifIcon    = 0x2
	nifTip     = 0x4

	wmUser         = 0x0400
	wmTrayCallback = wmUser + 1
	wmLButtonUp    = 0x0202
	wmRButtonUp    = 0x0205
	wmDestroy      = 0x0002

	idiApplication = 32512
)

type notifyIconData struct {
	CbSize           uint32
	HWnd             syscall.Handle
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            syscall.Handle
	SzTip            [128]uint16
	// The struct continues in newer Windows versions; CbSize below declares the
	// classic size, which every version still accepts.
}

type point struct{ X, Y int32 }

type msg struct {
	HWnd    syscall.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type wndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     syscall.Handle
	HIcon         syscall.Handle
	HCursor       syscall.Handle
	HbrBackground syscall.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       syscall.Handle
}

// Tray owns the icon and the hidden window that receives its clicks.
type Tray struct {
	hwnd    syscall.Handle
	console syscall.Handle
	data    notifyIconData
	hidden  bool

	// ownIcon is false for the system default icon, which is shared and must not
	// be destroyed.
	ownIcon bool
}

// New creates the icon. It must run on a locked OS thread, because the message
// loop and the window have to live on the same thread.
func New(tooltip string) (*Tray, error) {
	console, _, _ := procGetConsoleWindow.Call()

	hinst, _, _ := procGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString("discord-go-vpn-tray")

	t := &Tray{console: syscall.Handle(console)}

	class := wndClassEx{
		CbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		LpfnWndProc:   syscall.NewCallback(t.wndProc),
		HInstance:     syscall.Handle(hinst),
		LpszClassName: className,
	}
	if atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
		return nil, err
	}

	// A message-only window: it is never shown, it exists to receive the icon's
	// click notifications.
	hwnd, _, err := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(className)), 0, 0,
		0, 0, 0, 0, 0, 0, uintptr(hinst), 0,
	)
	if hwnd == 0 {
		return nil, err
	}
	t.hwnd = syscall.Handle(hwnd)

	icon, ownIcon := executableIcon()
	t.ownIcon = ownIcon

	t.data = notifyIconData{
		CbSize:           uint32(unsafe.Sizeof(notifyIconData{})),
		HWnd:             t.hwnd,
		UID:              1,
		UFlags:           nifMessage | nifIcon | nifTip,
		UCallbackMessage: wmTrayCallback,
		HIcon:            icon,
	}
	copyTip(&t.data, tooltip)

	// Not an error: while Explorer is busy the call can time out and still add the icon.
	_, _, _ = procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&t.data)))
	return t, nil
}

func (t *Tray) SetTooltip(s string) {
	copyTip(&t.data, s)
	_, _, _ = procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&t.data)))
}

// Hide sends the console to the tray; Show brings it back.
func (t *Tray) Hide() {
	if t.console != 0 {
		_, _, _ = procShowWindow.Call(uintptr(t.console), swHide)
		t.hidden = true
	}
}

func (t *Tray) Show() {
	if t.console != 0 {
		_, _, _ = procShowWindow.Call(uintptr(t.console), swRestore)
		_, _, _ = procSetForegroundWin.Call(uintptr(t.console))
		t.hidden = false
	}
}

func (t *Tray) toggle() {
	visible, _, _ := procIsWindowVisible.Call(uintptr(t.console))
	if visible != 0 {
		t.Hide()
	} else {
		t.Show()
	}
}

// Close removes the icon, which would otherwise linger in the tray until
// hovered, as if the program had not exited.
func (t *Tray) Close() {
	_, _, _ = procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&t.data)))
	if t.hwnd != 0 {
		_, _, _ = procDestroyWindow.Call(uintptr(t.hwnd))
	}
	if t.ownIcon && t.data.HIcon != 0 {
		_, _, _ = procDestroyIcon.Call(uintptr(t.data.HIcon))
	}
}

// executableIcon extracts the icon embedded in the .exe and reports whether it
// is ours to destroy. It reads the file rather than calling LoadImage with a
// resource ID because goversioninfo picks that ID when it generates the .syso.
func executableIcon() (syscall.Handle, bool) {
	exe, err := os.Executable()
	if err != nil {
		return defaultIcon(), false
	}

	file, err := syscall.UTF16PtrFromString(exe)
	if err != nil {
		return defaultIcon(), false
	}

	// The small icon is the tray's size; Windows would blur a scaled-down large one.
	var small syscall.Handle
	_, _, _ = procExtractIconExW.Call(
		uintptr(unsafe.Pointer(file)),
		0, // first icon group
		0, // no large icon
		uintptr(unsafe.Pointer(&small)),
		1,
	)

	if small == 0 {
		// The .exe was built without the .syso files.
		return defaultIcon(), false
	}
	return small, true
}

func defaultIcon() syscall.Handle {
	icon, _, _ := procLoadIconW.Call(0, uintptr(idiApplication))
	return syscall.Handle(icon)
}

const minimizePollInterval = 300 * time.Millisecond

// WatchMinimize sends the console to the tray once it is minimised. The console
// window belongs to conhost, which sends us no message, so it polls IsIconic.
func (t *Tray) WatchMinimize(stop <-chan struct{}) {
	if t.console == 0 {
		return
	}

	tick := time.NewTicker(minimizePollInterval)
	defer tick.Stop()

	for {
		select {
		case <-stop:
			return
		case <-tick.C:
			iconic, _, _ := procIsIconic.Call(uintptr(t.console))
			if iconic != 0 {
				// Restore first: a hidden-while-minimised window comes back
				// minimised, and the click on the icon would seem to do nothing.
				_, _, _ = procShowWindow.Call(uintptr(t.console), swRestore)
				t.Hide()
			}
		}
	}
}

// Loop pumps window messages until Close. Runs on its own locked thread.
func (t *Tray) Loop() {
	var m msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		_, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		_, _, _ = procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func (t *Tray) wndProc(hwnd syscall.Handle, message uint32, wparam, lparam uintptr) uintptr {
	switch message {
	case wmTrayCallback:
		// Either button restores the window: a tray icon whose click does
		// nothing reads as broken.
		if lparam == wmLButtonUp || lparam == wmRButtonUp {
			t.toggle()
		}
	case wmDestroy:
		_, _, _ = procPostQuitMessage.Call(0)
	}

	ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(message), wparam, lparam)
	return ret
}

func copyTip(d *notifyIconData, s string) {
	tip, err := syscall.UTF16FromString(s)
	if err != nil {
		return
	}
	for i := range d.SzTip {
		d.SzTip[i] = 0
	}
	copy(d.SzTip[:len(d.SzTip)-1], tip)
}
