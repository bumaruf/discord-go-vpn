package wg

import (
	"bufio"
	"strconv"
	"strings"
	"time"
)

// Traffic is the peer's counters as wireguard-go reports them, summed over
// every peer in the device.
type Traffic struct {
	RxBytes       uint64
	TxBytes       uint64
	LastHandshake time.Time // zero before the first handshake
}

// ParseTraffic reads the counters out of a UAPI "get" dump.
func ParseTraffic(uapi string) Traffic {
	var t Traffic
	var sec, nsec int64
	scanner := bufio.NewScanner(strings.NewReader(uapi))
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if !ok {
			continue
		}
		switch key {
		case "rx_bytes":
			n, _ := strconv.ParseUint(value, 10, 64)
			t.RxBytes += n
		case "tx_bytes":
			n, _ := strconv.ParseUint(value, 10, 64)
			t.TxBytes += n
		case "last_handshake_time_sec":
			sec, _ = strconv.ParseInt(value, 10, 64)
		case "last_handshake_time_nsec":
			nsec, _ = strconv.ParseInt(value, 10, 64)
			if sec > 0 {
				if at := time.Unix(sec, nsec); at.After(t.LastHandshake) {
					t.LastHandshake = at
				}
			}
		}
	}
	return t
}
