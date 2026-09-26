//go:build windows

package discord

import (
	"encoding/binary"
	"fmt"
	"slices"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	iphlpapi = windows.NewLazySystemDLL("iphlpapi.dll")

	procGetExtendedTcpTable = iphlpapi.NewProc("GetExtendedTcpTable")
	procSetTcpEntry         = iphlpapi.NewProc("SetTcpEntry")
)

const (
	afInet6                     = 23
	tcpTableOwnerPIDConnections = 4
	tcpStateEstablished         = 5
	tcpStateDeleteTCB           = 12
	errInsufficientBuffer       = 122
)

// pids returns the IDs of Discord's running processes.
func pids() (map[uint32]bool, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = windows.CloseHandle(snap) }()

	found := map[uint32]bool{}
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		name := windows.UTF16ToString(entry.ExeFile[:])
		if slices.ContainsFunc(windowsNames, func(n string) bool { return strings.EqualFold(n, name) }) {
			found[entry.ProcessID] = true
		}
	}
	return found, nil
}

// StartedAt identifies the running Discord by when its oldest process started,
// and reports false when none runs. A restart, by anyone, changes it.
func StartedAt() (time.Time, bool) {
	ids, err := pids()
	if err != nil {
		return time.Time{}, false
	}

	var oldest time.Time
	for pid := range ids {
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if err != nil {
			continue
		}
		var created, exited, kernel, user windows.Filetime
		if windows.GetProcessTimes(h, &created, &exited, &kernel, &user) == nil {
			if t := time.Unix(0, created.Nanoseconds()); oldest.IsZero() || t.Before(oldest) {
				oldest = t
			}
		}
		_ = windows.CloseHandle(h)
	}
	return oldest, !oldest.IsZero()
}

// DropConnections ends Discord's established TCP connections to other hosts,
// so it opens them again over the current route without its window being
// touched. Windows can only end IPv4 connections; the IPv6 ones it had to leave
// are counted in skipped.
func DropConnections() (skipped int, err error) {
	ids, err := pids()
	if err != nil {
		return 0, err
	}

	rows, err := tcpTable(windows.AF_INET, 24)
	if err != nil {
		return 0, err
	}
	for _, row := range rows {
		state, remote, pid := u32(row, 0), u32(row, 12), u32(row, 20)
		// The first byte of the address in memory is its first octet: 127 is loopback.
		if !ids[pid] || state != tcpStateEstablished || byte(remote) == 127 {
			continue
		}
		// MIB_TCPROW is MIB_TCPROW_OWNER_PID without the PID.
		entry := slices.Clone(row[:20])
		binary.LittleEndian.PutUint32(entry, tcpStateDeleteTCB)
		if r, _, _ := procSetTcpEntry.Call(uintptr(unsafe.Pointer(&entry[0]))); r != 0 {
			return 0, fmt.Errorf("encerrando uma conexão do Discord: %w", windows.Errno(r))
		}
	}

	rows6, err := tcpTable(afInet6, 56)
	if err != nil {
		return 0, nil
	}
	for _, row := range rows6 {
		if state, pid := u32(row, 48), u32(row, 52); ids[pid] && state == tcpStateEstablished && !isLoopback6(row[24:40]) {
			skipped++
		}
	}
	return skipped, nil
}

// tcpTable returns the rows of GetExtendedTcpTable, rowSize bytes each.
func tcpTable(family uint32, rowSize int) ([][]byte, error) {
	size := uint32(4096)
	for {
		buf := make([]byte, size)
		r, _, _ := procGetExtendedTcpTable.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0,
			uintptr(family), tcpTableOwnerPIDConnections, 0)
		if r == errInsufficientBuffer {
			continue
		}
		if r != 0 {
			return nil, fmt.Errorf("lendo as conexões TCP: %w", windows.Errno(r))
		}

		n := int(u32(buf, 0))
		rows := make([][]byte, 0, n)
		for i := range n {
			off := 4 + i*rowSize
			rows = append(rows, buf[off:off+rowSize])
		}
		return rows, nil
	}
}

func u32(b []byte, off int) uint32 { return binary.LittleEndian.Uint32(b[off:]) }

func isLoopback6(addr []byte) bool {
	return slices.Equal(addr[:15], make([]byte, 15)) && addr[15] == 1
}
