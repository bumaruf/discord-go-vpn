package metrics

import "testing"

func TestMaskOrigin(t *testing.T) {
	cases := []struct {
		endpoint string
		want     string
	}{
		{"", ""},
		{"(none)", ""},
		{"198.51.100.7:40000", "198.51.x.x"},
		{"198.51.100.7", "198.51.x.x"},
		{"[2001:db8:1:2::7]:40000", "2001:db8:…"},
		{"not-an-ip:40000", "?"},
	}
	for _, c := range cases {
		if got := maskOrigin(c.endpoint); got != c.want {
			t.Errorf("maskOrigin(%q) = %q, want %q", c.endpoint, got, c.want)
		}
	}
}

func TestEndpointHost(t *testing.T) {
	cases := []struct {
		endpoint string
		want     string
	}{
		{"", ""},
		{"(none)", ""},
		{"198.51.100.7:40000", "198.51.100.7"},
		{"198.51.100.7", "198.51.100.7"},
		{"[2001:db8:1:2::7]:40000", "2001:db8:1:2::7"},
	}
	for _, c := range cases {
		if got := endpointHost(c.endpoint); got != c.want {
			t.Errorf("endpointHost(%q) = %q, want %q", c.endpoint, got, c.want)
		}
	}
}
