// Package errtext turns an error into the text a person reads. The client's own
// messages are already Brazilian Portuguese; what Go, the operating system and
// external commands append in English is replaced where its type is known.
package errtext

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/bumaruf/discord-go-vpn/internal/profile"
)

// Translate returns err's message in Brazilian Portuguese, keeping the technical
// detail a person can act on, such as an address or an exit code.
func Translate(err error) string {
	if err == nil {
		return ""
	}
	return translate(err, err.Error())
}

// translate replaces, inside text, the message of each outermost error in err's
// tree that describe knows. fmt.Errorf copies a wrapped message verbatim, so it
// is found inside the whole.
func translate(err error, text string) string {
	if own, ok := describe(err); ok {
		return strings.Replace(text, err.Error(), own, 1)
	}
	switch e := err.(type) {
	case interface{ Unwrap() error }:
		if inner := e.Unwrap(); inner != nil {
			return translate(inner, text)
		}
	case interface{ Unwrap() []error }:
		for _, inner := range e.Unwrap() {
			text = translate(inner, text)
		}
	}
	return text
}

// describe matches err itself, never its chain: matching a wrapper would replace
// the Portuguese context around the cause too.
func describe(err error) (string, bool) {
	switch e := err.(type) {
	case *profile.StatusError:
		// Go's status line, such as "401 Unauthorized"; the reason after it
		// comes from provisiond, already in Portuguese.
		status := fmt.Sprintf("%d %s", e.StatusCode, http.StatusText(e.StatusCode))
		return strings.Replace(e.Error(), status, fmt.Sprintf("HTTP %d", e.StatusCode), 1), true
	case *url.Error:
		// Drops the method and URL: the caller already names the server.
		return Translate(e.Err), true
	case *net.OpError:
		if e.Addr == nil {
			return Translate(e.Err), true
		}
		return fmt.Sprintf("%s (%s %s)", Translate(e.Err), e.Net, e.Addr), true
	case *net.DNSError:
		switch {
		case e.IsNotFound:
			return fmt.Sprintf("o endereço %s não foi encontrado", e.Name), true
		case e.IsTimeout:
			return fmt.Sprintf("a consulta de DNS de %s não respondeu a tempo", e.Name), true
		}
		return fmt.Sprintf("não consegui resolver o endereço %s", e.Name), true
	case *os.SyscallError:
		return Translate(e.Err), true
	case *os.PathError:
		// Every caller already names the path.
		return Translate(e.Err), true
	case *exec.ExitError:
		if code := e.ExitCode(); code >= 0 {
			return fmt.Sprintf("o comando falhou com código %d", code), true
		}
		return "o comando foi interrompido", true
	case *exec.Error:
		if e.Err == exec.ErrNotFound {
			return fmt.Sprintf("o programa %q não foi encontrado", e.Name), true
		}
		return fmt.Sprintf("não consegui executar %q: %s", e.Name, Translate(e.Err)), true
	case syscall.Errno:
		// On Windows these constants never match: its system messages come
		// already in the language Windows is set to.
		if text, ok := errnoText[e]; ok {
			return text, true
		}
	}

	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return "a conexão foi encerrada no meio da resposta", true
	}
	if t, ok := err.(interface{ Timeout() bool }); ok && t.Timeout() {
		return "o tempo de espera esgotou", true
	}
	return "", false
}

var errnoText = map[syscall.Errno]string{
	syscall.ECONNREFUSED: "a conexão foi recusada",
	syscall.ECONNRESET:   "a conexão foi interrompida",
	syscall.ENETUNREACH:  "a rede está inacessível",
	syscall.EHOSTUNREACH: "o servidor está inacessível",
	syscall.ETIMEDOUT:    "o tempo de espera esgotou",
	syscall.EADDRINUSE:   "o endereço já está em uso",
	syscall.EACCES:       "permissão negada",
	syscall.EPERM:        "operação não permitida",
	syscall.ENOENT:       "arquivo ou diretório não encontrado",
	syscall.EEXIST:       "já existe",
}
