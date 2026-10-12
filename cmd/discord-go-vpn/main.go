// Command discord-go-vpn runs Discord inside a WireGuard tunnel without
// modifying the client.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/bumaruf/discord-go-vpn/internal/app"
	"github.com/bumaruf/discord-go-vpn/internal/connectiontoken"
	"github.com/bumaruf/discord-go-vpn/internal/errtext"
	"github.com/bumaruf/discord-go-vpn/internal/platform/console"
	"github.com/bumaruf/discord-go-vpn/internal/profile"
	"github.com/bumaruf/discord-go-vpn/internal/routing"
	"github.com/bumaruf/discord-go-vpn/internal/update"
)

func main() {
	configPath := flag.String("c", "", "perfil .conf do WireGuard (o padrão é procurar na pasta do programa)")
	discordPath := flag.String("discord", "", "caminho do executável do Discord (quando a busca automática não acha)")
	tokenFlag := flag.String("token", "", "atalho: o token de conexão dgv1_... (o padrão é pedir no terminal)")
	noUpdate := flag.Bool("no-update", false, "não procurar uma versão nova ao abrir")
	flag.Usage = func() { printUsage(flag.CommandLine) }
	flag.Parse()

	mode := routing.Default()
	if mode == "" {
		fmt.Fprintf(os.Stderr, "não há modo disponível em %s: o túnel por processo existe só no Linux (netns) e no Windows (wiresock)\n", runtime.GOOS)
		exitWith(1)
	}

	if *configPath != "" {
		if _, err := os.Stat(*configPath); err != nil {
			fmt.Fprintf(os.Stderr, "não consegui ler %s\n", *configPath)
			exitWith(1)
		}
	}

	// The .exe manifest already requests elevation; this covers a build without it.
	if mode == routing.ModeWiresock {
		console.EnsureElevated()
	}

	// Before the tunnel exists, so updating never drops a call.
	if !*noUpdate {
		code, replaced, err := update.Run(context.Background(), os.Stdout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "não consegui atualizar: %s\n", errtext.Translate(err))
		}
		if replaced {
			exitWith(code)
		}
	}

	// A .conf needs no server, so the token is asked for only without one.
	profilePath := pickProfile(*configPath)
	var provision profile.Provisioning
	var lease *profile.Source
	if profilePath == "" {
		token, validated, err := connectiontoken.Ensure(*tokenFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n%v\n", err)
			exitWith(1)
		}
		provision, lease = token.Provisioning(), validated
	}

	exitWith(app.Run(app.Options{
		ConfigPath:  profilePath,
		DiscordPath: *discordPath,
		Mode:        mode,
		Provision:   provision,
		Source:      lease,
	}))
}

// printUsage is the flag package's default usage with the header in Portuguese.
func printUsage(fs *flag.FlagSet) {
	fmt.Fprintf(fs.Output(), "Uso de %s:\n", fs.Name())
	fs.PrintDefaults()
}

func exitWith(code int) {
	console.HoldWindow()
	os.Exit(code)
}

// pickProfile returns "" when there is no file; access then comes from the connection token.
func pickProfile(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if profiles := profile.Discover(); len(profiles) > 0 {
		return profiles[0]
	}
	return ""
}
