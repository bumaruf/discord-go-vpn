// Package connectiontoken reads the connection token the panel hands out, which carries
// everything the client needs to lease: the server, its certificate fingerprint and the
// invite. The binary has nothing built in, so it asks for the token in the terminal
// instead of requiring a flag, since the Windows .exe is started by double click.
package connectiontoken

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/bumaruf/discord-go-vpn/internal/profile"
)

// prefix names the format version; panel/lib/connection-token.ts builds the same shape,
// pinned by testdata/contracts/connection-token.json.
const prefix = "dgv1_"

var errMalformed = errors.New("isso não é um token de conexão válido")

type Token struct {
	Server      string `json:"server"`
	Fingerprint string `json:"fingerprint"`
	Invite      string `json:"invite"`
}

// Parse rejects anything that would only fail later at the server, so a typo is told apart
// from a revoked invite.
func Parse(s string) (Token, error) {
	encoded, ok := strings.CutPrefix(strings.TrimSpace(s), prefix)
	if !ok {
		return Token{}, errMalformed
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return Token{}, errMalformed
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var t Token
	if err := decoder.Decode(&t); err != nil {
		return Token{}, errMalformed
	}

	server, err := url.Parse(t.Server)
	if err != nil || server.Scheme != "https" || server.Host == "" {
		return Token{}, errMalformed
	}
	if fp, err := base64.StdEncoding.DecodeString(t.Fingerprint); err != nil || len(fp) != sha256.Size {
		return Token{}, errMalformed
	}
	if t.Invite == "" {
		return Token{}, errMalformed
	}
	return t, nil
}

func (t Token) String() string {
	raw, _ := json.Marshal(t)
	return prefix + base64.RawURLEncoding.EncodeToString(raw)
}

func (t Token) Provisioning() profile.Provisioning {
	return profile.Provisioning{Server: t.Server, Token: t.Invite, Fingerprint: t.Fingerprint}
}
