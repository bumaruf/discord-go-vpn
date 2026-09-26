package auth

import (
	"net/http/httptest"
	"testing"
)

func TestBearerToken(t *testing.T) {
	cases := []struct {
		header string
		want   string
	}{
		{"", ""},
		{"Bearer abc", "abc"},
		{"Bearer   abc  ", "abc"},
		{"bearer abc", ""},
		{"Basic abc", ""},
		{"Bearer", ""},
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", "/", nil)
		if c.header != "" {
			req.Header.Set("Authorization", c.header)
		}
		if got := BearerToken(req); got != c.want {
			t.Errorf("BearerToken(%q) = %q, want %q", c.header, got, c.want)
		}
	}
}
