package connectiontoken

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bumaruf/discord-go-vpn/internal/profile"
)

// What the person reads when a typed token fails, here and in the Android app.
const (
	MalformedHelp = "Isso não parece um token de conexão. Ele começa com dgv1_ e vem inteiro, numa linha só."
	RefusedHelp   = "Esse token não foi aceito: o acesso pode ter sido revogado."
)

var errNoTerminal = errors.New("sem token de conexão e sem terminal para pedir um: use -token dgv1_... ou VPN_TOKEN")

// Prompter is how Ensure talks to the person: the terminal, or the screen.
type Prompter interface {
	// Checking says a token from origin is being validated with the server.
	Checking(origin string)
	// Tell shows what happened to the last token.
	Tell(msg string)
	// Ask returns a typed token; an error ends Ensure.
	Ask() (string, error)
}

// Where a candidate token came from, as the screen names it.
const (
	OriginFlag  = "do -token"
	OriginEnv   = "de VPN_TOKEN"
	OriginSaved = "salvo"
	OriginTyped = "colado agora"
)

// UnavailableHelp is told when the server didn't answer about a token.
const UnavailableHelp = "O servidor de acesso não respondeu agora; vou tentar conectar assim mesmo."

// Ensure returns the connection token to lease with, prompting until the server accepts one,
// and the lease acquired while validating it, or nil; the caller owns the lease. It fails
// only when Ask does.
func Ensure(flagToken string, p Prompter) (Token, *profile.Source, error) {
	type candidate struct{ token, origin string }
	candidates := []candidate{{flagToken, OriginFlag}, {os.Getenv("VPN_TOKEN"), OriginEnv}, {Saved(), OriginSaved}}

	for {
		var input, origin string
		for len(candidates) > 0 && input == "" {
			input, origin = strings.TrimSpace(candidates[0].token), candidates[0].origin
			candidates = candidates[1:]
		}

		if input == "" {
			var err error
			if input, err = p.Ask(); err != nil {
				return Token{}, nil, err
			}
			origin = OriginTyped
		}

		token, err := Parse(input)
		if err != nil {
			p.Tell(MalformedHelp)
			continue
		}

		p.Checking(origin)
		result, lease := Validate(token.Provisioning())
		switch result {
		case Accepted:
			Save(token)
			return token, lease, nil

		case Refused:
			p.Tell(RefusedHelp)
			Forget()

		case Unavailable:
			// Says nothing about the token, so keep it and start anyway: the supervisor
			// retries with backoff.
			p.Tell(UnavailableHelp)
			Save(token)
			return token, nil, nil
		}
	}
}

// Terminal prompts on stdin, for consoles that can't draw the screen.
func Terminal() Prompter { return &terminal{reader: bufio.NewReader(os.Stdin)} }

type terminal struct{ reader *bufio.Reader }

func (t *terminal) Checking(string) {}

func (t *terminal) Tell(msg string) { fmt.Println("\n" + msg) }

func (t *terminal) Ask() (string, error) {
	fmt.Println("Cole o token de conexão que você recebeu (começa com dgv1_):")
	fmt.Print("> ")

	line, err := t.reader.ReadString('\n')
	if err != nil && line == "" {
		return "", errNoTerminal
	}
	return strings.TrimSpace(line), nil
}

type Validation int

const (
	Accepted Validation = iota
	Refused
	Unavailable
)

// Validate checks a token by acquiring a real lease. On Accepted the caller owns the
// returned lease, so startup does not lease a second time.
func Validate(prov profile.Provisioning) (Validation, *profile.Source) {
	src, err := profile.Open("", prov)
	if err == nil {
		return Accepted, src
	}

	// A refusal discards the saved token, so a 429, a 5xx or no answer at all must never
	// count as one.
	if profile.IsRefusal(err) {
		return Refused, nil
	}
	return Unavailable, nil
}

// tokenPath is in the user config dir rather than beside the executable, which on Windows
// often sits in a folder the program cannot write to.
func tokenPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "discord-go-vpn", "token"), nil
}

func Saved() string {
	path, err := tokenPath()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// Save keeps the token for the next run. A token replaced while the client runs
// is saved right away, like one the server couldn't answer about at startup: a
// refusal on the next run discards it and asks again.
func Save(token Token) {
	path, err := tokenPath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	// A failed write only means prompting again on the next run.
	_ = os.WriteFile(path, []byte(token.String()+"\n"), 0o600)
}

// Forget drops the saved token, so the next run asks for one.
func Forget() {
	if path, err := tokenPath(); err == nil {
		_ = os.Remove(path)
	}
}
