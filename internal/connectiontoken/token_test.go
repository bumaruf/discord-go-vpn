package connectiontoken

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/bumaruf/discord-go-vpn/internal/contracttest"
)

func fixture(t *testing.T) Token {
	t.Helper()
	var token Token
	if err := json.Unmarshal(contracttest.Read(t, "connection-token.json"), &token); err != nil {
		t.Fatal(err)
	}
	return token
}

func TestContract(t *testing.T) {
	contracttest.AssertRoundTrip[Token](t, "connection-token.json")

	want := fixture(t)
	got, err := Parse(want.String())
	if err != nil || got != want {
		t.Errorf("Parse(String()) = %+v, %v; want %+v", got, err, want)
	}
}

func TestParseRejects(t *testing.T) {
	valid := fixture(t)
	encode := func(v any) string {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return prefix + base64.RawURLEncoding.EncodeToString(raw)
	}
	with := func(change func(*Token)) string {
		token := valid
		change(&token)
		return token.String()
	}

	tests := map[string]string{
		"empty":               "",
		"old invite code":     valid.Invite,
		"another version":     "dgv2_" + valid.String()[len(prefix):],
		"not base64url":       prefix + "!!!",
		"padded":              valid.String() + "=",
		"not json":            prefix + base64.RawURLEncoding.EncodeToString([]byte("server")),
		"unknown key":         encode(map[string]string{"server": valid.Server, "fingerprint": valid.Fingerprint, "invite": valid.Invite, "extra": "x"}),
		"http server":         with(func(t *Token) { t.Server = "http://203.0.113.10:51821" }),
		"server without host": with(func(t *Token) { t.Server = "https://" }),
		"short fingerprint":   with(func(t *Token) { t.Fingerprint = "c2hvcnQ=" }),
		"no invite":           with(func(t *Token) { t.Invite = "" }),
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if got, err := Parse(input); err == nil {
				t.Errorf("Parse(%q) = %+v, want an error", input, got)
			}
		})
	}

	t.Run("surrounding whitespace is fine", func(t *testing.T) {
		if _, err := Parse("  " + valid.String() + "\r\n"); err != nil {
			t.Errorf("Parse = %v, want nil", err)
		}
	})
}
