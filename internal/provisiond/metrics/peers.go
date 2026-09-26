package metrics

import (
	"cmp"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// recentHandshake reports whether the handshake is recent enough for its peer to count as online
// at now. WireGuard only renews the handshake every two minutes, and keepalives don't update it.
func recentHandshake(handshake, now time.Time) bool {
	const window = 3 * time.Minute
	return !handshake.IsZero() && now.Sub(handshake) < window
}

// maskOrigin keeps the first two octets of the client's address, which still tell clients
// and networks apart.
func maskOrigin(endpoint string) string {
	host := endpointHost(endpoint)
	if host == "" {
		return ""
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return "?"
	}
	if v4 := ip.To4(); v4 != nil {
		return fmt.Sprintf("%d.%d.x.x", v4[0], v4[1])
	}
	groups := strings.Split(ip.String(), ":")
	if len(groups) >= 2 {
		return groups[0] + ":" + groups[1] + ":…"
	}
	return "?"
}

// endpointHost is the client's address without the port, or empty before its first handshake.
func endpointHost(endpoint string) string {
	if endpoint == "" || endpoint == "(none)" {
		return ""
	}
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		return endpoint
	}
	return host
}

// compareIP compares the last octet, which is what varies in the tunnel range.
func compareIP(a, b string) int {
	return cmp.Compare(lastOctet(a), lastOctet(b))
}

func lastOctet(ip string) int {
	i := strings.LastIndexByte(ip, '.')
	if i < 0 {
		return 0
	}
	n, _ := strconv.Atoi(ip[i+1:])
	return n
}
