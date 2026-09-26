//go:build windows

package osuser

import (
	"os/user"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// x/sys/windows wraps WTSFreeMemory but not WTSQuerySessionInformationW.
var procWTSQuerySessionInformation = windows.NewLazySystemDLL("wtsapi32.dll").NewProc("WTSQuerySessionInformationW")

const (
	wtsCurrentServerHandle = 0
	wtsCurrentSession      = 0xFFFFFFFF
	wtsUserName            = 5
)

// Name returns the user of this process's desktop session, who stays the same
// when an administrator account approved the elevation, or else the process's
// user without its domain; "" when neither is known.
func Name() string {
	if name := sessionUser(); name != "" {
		return name
	}
	u, err := user.Current()
	if err != nil {
		return ""
	}
	if i := strings.LastIndexByte(u.Username, '\\'); i >= 0 {
		return u.Username[i+1:]
	}
	return u.Username
}

func sessionUser() string {
	if procWTSQuerySessionInformation.Find() != nil {
		return ""
	}
	var buf *uint16
	var size uint32
	ok, _, _ := procWTSQuerySessionInformation.Call(
		wtsCurrentServerHandle, wtsCurrentSession, wtsUserName,
		uintptr(unsafe.Pointer(&buf)), uintptr(unsafe.Pointer(&size)))
	if ok == 0 || buf == nil {
		return ""
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(buf)))
	return windows.UTF16PtrToString(buf)
}
