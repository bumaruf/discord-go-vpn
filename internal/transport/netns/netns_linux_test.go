//go:build linux

package netns

import (
	"reflect"
	"testing"
)

func TestIptablesPutsTableBeforeOperation(t *testing.T) {
	for _, tc := range []struct {
		op   string
		rule []string
		want []string
	}{
		{"-I", fallbackRules[0], []string{"iptables", "-t", "nat", "-I", "POSTROUTING", "-s", vethSubnet, "-j", "MASQUERADE"}},
		{"-D", fallbackRules[1], []string{"iptables", "-D", "FORWARD", "-i", hostVeth, "-j", "ACCEPT"}},
	} {
		if got := iptables(tc.op, tc.rule); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("iptables(%q, %q) = %q, want %q", tc.op, tc.rule, got, tc.want)
		}
	}
}
