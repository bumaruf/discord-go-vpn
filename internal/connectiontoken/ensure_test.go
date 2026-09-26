package connectiontoken

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bumaruf/discord-go-vpn/internal/profile/profiletest"
)

const testInvite = "vpn_64wu72y2-4vjhv8yv-hmanr9te"

// isolateConfigDir keeps the saved token away from the developer's own and returns the
// config dir the code under test sees: os.UserConfigDir reads XDG_CONFIG_HOME on Linux,
// APPDATA on Windows and HOME on macOS.
func isolateConfigDir(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	for _, env := range []string{"XDG_CONFIG_HOME", "APPDATA", "HOME"} {
		t.Setenv(env, tmp)
	}
	t.Setenv("VPN_TOKEN", "")
	dir, err := os.UserConfigDir()
	if err != nil || !strings.HasPrefix(dir, tmp) {
		t.Fatalf("os.UserConfigDir() = %q, %v; want a dir inside %q", dir, err, tmp)
	}
	return dir
}

func tokenFor(prov *profiletest.Provisiond) Token {
	return Token{Server: prov.URL, Fingerprint: prov.Fingerprint(), Invite: testInvite}
}

func TestEnsureHandsOverValidationLease(t *testing.T) {
	isolateConfigDir(t)
	prov := profiletest.NewProvisiond(t)
	want := tokenFor(prov)

	token, lease, err := Ensure(want.String())
	if token != want || lease == nil || err != nil {
		t.Fatalf("Ensure = (%+v, %v, %v), want (%+v, a lease, nil)", token, lease, err, want)
	}
	if n := len(prov.Calls("/release")); n != 0 {
		t.Fatalf("validation released its lease (%d /release calls) instead of handing it over", n)
	}

	lease.Close()
	if leases, releases := len(prov.Calls("/lease")), len(prov.Calls("/release")); leases != 1 || releases != 1 {
		t.Errorf("got %d /lease and %d /release calls, want 1 and 1", leases, releases)
	}
	if saved := readSavedToken(); saved != want.String() {
		t.Errorf("saved %q, want the accepted token", saved)
	}
}

func TestEnsureReadsTokenFromEnv(t *testing.T) {
	isolateConfigDir(t)
	prov := profiletest.NewProvisiond(t)
	want := tokenFor(prov)
	t.Setenv("VPN_TOKEN", want.String())

	token, lease, err := Ensure("")
	if lease != nil {
		lease.Close()
	}
	if token != want || err != nil {
		t.Errorf("Ensure = (%+v, %v), want (%+v, nil)", token, err, want)
	}
}

func TestEnsureWithUnavailableServerHandsOverNoLease(t *testing.T) {
	isolateConfigDir(t)
	prov := profiletest.NewProvisiond(t)
	prov.LeaseStatus = http.StatusServiceUnavailable
	want := tokenFor(prov)

	token, lease, err := Ensure(want.String())
	if token != want || lease != nil || err != nil {
		t.Errorf("Ensure = (%+v, %v, %v), want (%+v, nil, nil)", token, lease, err, want)
	}
}

// Only the status code tells a refused token from a server problem: a refusal discards the
// saved token, so a 5xx must never be read as one.
func TestValidateClassifiesByStatusCode(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		body        string
		unreachable bool
		want        validation
	}{
		{name: "accepted", status: http.StatusOK, want: accepted},
		{name: "unauthorized", status: http.StatusUnauthorized, want: refused},
		{name: "forbidden", status: http.StatusForbidden, body: "acesso negado", want: refused},
		{name: "too many requests", status: http.StatusTooManyRequests, want: unavailable},
		{name: "server error mentioning 401", status: http.StatusBadGateway, body: "upstream answered 401", want: unavailable},
		{name: "unreachable", status: http.StatusOK, unreachable: true, want: unavailable},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := profiletest.NewProvisiond(t)
			prov.LeaseStatus = tc.status
			if tc.body != "" {
				prov.LeaseBody = []byte(tc.body)
			}
			token := tokenFor(prov)
			if tc.unreachable {
				prov.Close()
			}

			got, lease := validate(token)
			if lease != nil {
				lease.Close()
			}
			if got != tc.want {
				t.Errorf("validate = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestTokenPersistence(t *testing.T) {
	configDir := isolateConfigDir(t)

	if readSavedToken() != "" {
		t.Fatal("started with a saved token where there should be none")
	}

	token := fixture(t)
	Save(token)

	if got := readSavedToken(); got != token.String() {
		t.Errorf("read %q, saved %q", got, token.String())
	}

	info, err := os.Stat(filepath.Join(configDir, "discord-go-vpn", "token"))
	if err != nil {
		t.Fatalf("file was not created: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("permission %o, want 600", perm)
	}

	Forget()
	if readSavedToken() != "" {
		t.Error("forget did not delete the token")
	}
}
