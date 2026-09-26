package main

import (
	"bytes"
	"flag"
	"strings"
	"testing"
)

func TestUsageIsInPortuguese(t *testing.T) {
	var out bytes.Buffer
	fs := flag.NewFlagSet("discord-go-vpn", flag.ContinueOnError)
	fs.SetOutput(&out)
	fs.String("c", "", "perfil .conf do WireGuard")
	fs.String("token", "", "atalho: o token de conexão")
	fs.Usage = func() { printUsage(fs) }

	if err := fs.Parse([]string{"-h"}); err != flag.ErrHelp {
		t.Fatalf("Parse(-h) = %v, want flag.ErrHelp", err)
	}
	help := out.String()
	if !strings.HasPrefix(help, "Uso de discord-go-vpn:") {
		t.Errorf("help lacks the Portuguese header:\n%s", help)
	}
	if !strings.Contains(help, "-token") || !strings.Contains(help, "-c") {
		t.Errorf("help misses -token or -c:\n%s", help)
	}
}
