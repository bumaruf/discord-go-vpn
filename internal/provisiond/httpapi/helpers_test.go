package httpapi

import (
	"strings"
	"testing"
)

func TestCleanLabel(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", ""},
		{"plain", "notebook da ana", "notebook da ana"},
		{"control characters dropped", "note\x00book\t\n\x7f", "notebook"},
		{"surrounding spaces trimmed", "  pc  ", "pc"},
		{"cut at 32 bytes", strings.Repeat("a", 40), strings.Repeat("a", 32)},
		// The limit is checked after each rune is written, in bytes.
		{"a multibyte rune may cross the limit", strings.Repeat("a", 31) + "éé", strings.Repeat("a", 31) + "é"},
		// Spaces count toward the limit because trimming happens after the cut.
		{"leading spaces use up the limit", strings.Repeat(" ", 30) + "abcdef", "ab"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := cleanLabel(c.raw); got != c.want {
				t.Errorf("cleanLabel(%q) = %q, want %q", c.raw, got, c.want)
			}
		})
	}
}
