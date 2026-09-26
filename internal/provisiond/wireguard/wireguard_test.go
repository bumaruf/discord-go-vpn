package wireguard

import (
	"net/netip"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseDump(t *testing.T) {
	dump := strings.Join([]string{
		"privkey\tpubkey\t51820\toff",
		"peerA\t(none)\t198.51.100.7:40000\t10.9.0.10/32\t1789300000\t1500\t2500\t25",
		"peerB\t(none)\t(none)\t10.9.0.11/32,10.9.0.12/32\t0\t0\t0\toff",
		"peerC\t(none)\t(none)\t(none)\t0\t0\t0\toff",
		"truncated\tline",
	}, "\n") + "\n"

	peers := parseDump(dump)
	if len(peers) != 3 {
		t.Fatalf("parsed %d peers, want 3: %+v", len(peers), peers)
	}

	want := PeerStat{
		PubKey:     "peerA",
		Endpoint:   "198.51.100.7:40000",
		AllowedIPs: []netip.Prefix{netip.MustParsePrefix("10.9.0.10/32")},
		RxBytes:    1500,
		TxBytes:    2500,
		Handshake:  time.Unix(1789300000, 0),
	}
	if a := peers["peerA"]; !reflect.DeepEqual(a, want) {
		t.Errorf("peerA = %+v, want %+v", a, want)
	}

	b := peers["peerB"]
	if !b.Handshake.IsZero() || b.Endpoint != "(none)" {
		t.Errorf("peerB = %+v, want zero handshake and endpoint (none)", b)
	}
	wantIPs := []netip.Prefix{netip.MustParsePrefix("10.9.0.11/32"), netip.MustParsePrefix("10.9.0.12/32")}
	if !reflect.DeepEqual(b.AllowedIPs, wantIPs) {
		t.Errorf("peerB allowed IPs = %v, want %v", b.AllowedIPs, wantIPs)
	}

	if c := peers["peerC"]; len(c.AllowedIPs) != 0 {
		t.Errorf("peerC allowed IPs = %v, want none", c.AllowedIPs)
	}

	for _, output := range []string{"", "privkey\tpubkey\t51820\toff\n"} {
		if got := parseDump(output); len(got) != 0 {
			t.Errorf("parseDump(%q) = %+v, want no peers", output, got)
		}
	}
}
