package wg

import (
	"testing"
	"time"
)

func TestParseTraffic(t *testing.T) {
	dump := "private_key=aa\nlisten_port=51820\npublic_key=bb\nendpoint=1.2.3.4:51820\n" +
		"last_handshake_time_sec=1700000000\nlast_handshake_time_nsec=500\n" +
		"rx_bytes=1024\ntx_bytes=2048\npersistent_keepalive_interval=25\nallowed_ip=0.0.0.0/0\nerrno=0\n"

	got := ParseTraffic(dump)
	want := Traffic{RxBytes: 1024, TxBytes: 2048, LastHandshake: time.Unix(1700000000, 500)}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestParseTrafficBeforeTheFirstHandshake(t *testing.T) {
	got := ParseTraffic("public_key=bb\nlast_handshake_time_sec=0\nlast_handshake_time_nsec=0\nrx_bytes=0\ntx_bytes=92\n")
	if !got.LastHandshake.IsZero() || got.TxBytes != 92 {
		t.Fatalf("got %+v", got)
	}
}
