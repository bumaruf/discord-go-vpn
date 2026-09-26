package errtext

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/profile"
	"github.com/bumaruf/discord-go-vpn/internal/profile/profiletest"
)

func TestTranslateKeepsPortugueseAndUnknownErrors(t *testing.T) {
	if got := Translate(nil); got != "" {
		t.Errorf("Translate(nil) = %q, want empty", got)
	}
	err := fmt.Errorf("o Discord não subiu: %w", errors.New("some library failure"))
	if got := Translate(err); got != err.Error() {
		t.Errorf("Translate(%q) = %q, want it unchanged", err, got)
	}
}

func TestTranslateRefusedInvite(t *testing.T) {
	f := profiletest.NewProvisiond(t)
	f.LeaseStatus = http.StatusUnauthorized

	_, err := profile.Open("", profile.Provisioning{Server: f.URL, Token: "vpn_x", Fingerprint: f.Fingerprint()})
	want := "não consegui obter um acesso: o servidor recusou (HTTP 401): convite inválido"
	if got := Translate(err); got != want {
		t.Errorf("Translate(%q) =\n%q\nwant\n%q", err, got, want)
	}
}

func TestTranslateUnreachableServer(t *testing.T) {
	f := profiletest.NewProvisiond(t)
	addr := strings.TrimPrefix(f.URL, "https://")
	f.Close()

	_, err := profile.Open("", profile.Provisioning{Server: f.URL, Token: "vpn_x", Fingerprint: f.Fingerprint()})
	want := "não consegui obter um acesso: não consegui falar com o servidor de acesso: a conexão foi recusada (tcp " + addr + ")"
	if got := Translate(err); got != want {
		t.Errorf("Translate(%q) =\n%q\nwant\n%q", err, got, want)
	}
}

func TestTranslateTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer srv.Close()
	defer close(release)

	_, err := (&http.Client{Timeout: 10 * time.Millisecond}).Get(srv.URL)
	err = fmt.Errorf("não consegui falar com o servidor de acesso: %w", err)
	want := "não consegui falar com o servidor de acesso: o tempo de espera esgotou"
	if got := Translate(err); got != want {
		t.Errorf("Translate(%q) = %q, want %q", err, got, want)
	}
}

func TestTranslateCommandFailures(t *testing.T) {
	exitErr := exec.Command("sh", "-c", "exit 3").Run()
	missing := exec.Command("discord-go-vpn-no-such-program").Run()
	_, notExist := os.Open("/discord-go-vpn-no-such-dir/perfil.conf")

	cases := []struct {
		err  error
		want string
	}{
		{
			fmt.Errorf("%s: %w: %s", "ip netns add discord-vpn", exitErr, "RTNETLINK answers: File exists"),
			"ip netns add discord-vpn: o comando falhou com código 3: RTNETLINK answers: File exists",
		},
		{
			fmt.Errorf("não consegui instalar o WireSock: %w\n  tente à mão", exitErr),
			"não consegui instalar o WireSock: o comando falhou com código 3\n  tente à mão",
		},
		{
			fmt.Errorf("abrindo o Discord: %w", missing),
			`abrindo o Discord: o programa "discord-go-vpn-no-such-program" não foi encontrado`,
		},
		{
			fmt.Errorf("abrindo /x/perfil.conf: %w", notExist),
			"abrindo /x/perfil.conf: arquivo ou diretório não encontrado",
		},
	}
	for _, c := range cases {
		if got := Translate(c.err); got != c.want {
			t.Errorf("Translate(%q) =\n%q\nwant\n%q", c.err, got, c.want)
		}
	}
}

func TestTranslateDNSAndJoinedErrors(t *testing.T) {
	notFound := &net.DNSError{Err: "no such host", Name: "vpn.exemplo.invalid", IsNotFound: true}
	err := fmt.Errorf("resolvendo Endpoint %q: %w", "vpn.exemplo.invalid:51820", notFound)
	want := `resolvendo Endpoint "vpn.exemplo.invalid:51820": o endereço vpn.exemplo.invalid não foi encontrado`
	if got := Translate(err); got != want {
		t.Errorf("Translate(%q) = %q, want %q", err, got, want)
	}

	lost := errors.New("o acesso foi perdido")
	joined := fmt.Errorf("%w: %w", lost, fmt.Errorf("renovando: %w", &os.SyscallError{Syscall: "read", Err: syscall.ECONNRESET}))
	want = "o acesso foi perdido: renovando: a conexão foi interrompida"
	if got := Translate(joined); got != want {
		t.Errorf("Translate(%q) = %q, want %q", joined, got, want)
	}
}
