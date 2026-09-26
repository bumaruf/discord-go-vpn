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

var errNoTerminal = errors.New("sem token de conexão e sem terminal para pedir um: use -token dgv1_... ou VPN_TOKEN")

// Ensure returns the connection token to lease with, prompting until the server accepts one,
// and the lease acquired while validating it, or nil; the caller owns the lease. It fails
// only when it must prompt and stdin is closed.
func Ensure(flagToken string) (Token, *profile.Source, error) {
	candidates := []string{flagToken, os.Getenv("VPN_TOKEN"), readSavedToken()}
	reader := bufio.NewReader(os.Stdin)

	for {
		var input string
		for len(candidates) > 0 && input == "" {
			input, candidates = strings.TrimSpace(candidates[0]), candidates[1:]
		}

		if input == "" {
			var err error
			if input, err = prompt(reader); err != nil {
				return Token{}, nil, err
			}
		}

		token, err := Parse(input)
		if err != nil {
			fmt.Println("\nIsso não parece um token de conexão. Ele começa com dgv1_ e vem inteiro, numa linha só.")
			continue
		}

		result, lease := validate(token)
		switch result {
		case accepted:
			Save(token)
			return token, lease, nil

		case refused:
			fmt.Println("\nEsse token não foi aceito: o acesso pode ter sido revogado.")
			Forget()

		case unavailable:
			// Says nothing about the token, so keep it and start anyway: the supervisor
			// retries with backoff.
			fmt.Fprintln(os.Stderr, "O servidor de acesso não respondeu agora; vou tentar conectar assim mesmo.")
			Save(token)
			return token, nil, nil
		}
	}
}

func prompt(reader *bufio.Reader) (string, error) {
	fmt.Println("Cole o token de conexão que você recebeu (começa com dgv1_):")
	fmt.Print("> ")

	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return "", errNoTerminal
	}
	return strings.TrimSpace(line), nil
}

type validation int

const (
	accepted validation = iota
	refused
	unavailable
)

// validate checks the token by acquiring a real lease. On accepted the caller owns the
// returned lease, so startup does not lease a second time.
func validate(token Token) (validation, *profile.Source) {
	src, err := profile.Open("", token.Provisioning())
	if err == nil {
		return accepted, src
	}

	// A refusal discards the saved token, so a 429, a 5xx or no answer at all must never
	// count as one.
	if profile.IsRefusal(err) {
		return refused, nil
	}
	return unavailable, nil
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

func readSavedToken() string {
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
