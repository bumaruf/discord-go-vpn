//go:build linux && !android

package main

import (
	"context"
	"flag"
	"fmt"
	"image/color"
	"os"
	"os/signal"
	"syscall"

	"github.com/bumaruf/discord-go-vpn/internal/errtext"
	"github.com/bumaruf/discord-go-vpn/internal/routing"
	"github.com/bumaruf/discord-go-vpn/internal/service"
	"github.com/bumaruf/discord-go-vpn/internal/trayapp"
	"github.com/bumaruf/discord-go-vpn/internal/ui"
	"github.com/bumaruf/discord-go-vpn/internal/update"
)

// runService is what systemd runs; its output lands in the journal.
func runService(args []string) int {
	fs := flag.NewFlagSet("service", flag.ExitOnError)
	owner := fs.String("user", "", "desktop user whose Discord the tunnel carries (required)")
	discordPath := fs.String("discord", "", "path to the Discord executable, when the search misses it")
	_ = fs.Parse(args)
	if *owner == "" {
		fmt.Fprintln(os.Stderr, "service: -user is required")
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer cancel()

	restart, err := service.Run(ctx, service.Options{
		User:        *owner,
		DiscordPath: *discordPath,
		Mode:        routing.ModeNetns,
		Version:     update.Version,
		CheckUpdate: true,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "service:", errtext.Translate(err))
		return 1
	}
	if restart != nil {
		fmt.Fprintln(os.Stderr, "service: relaunching into", restart.Version)
		code, err := restart.Relaunch()
		if err != nil {
			fmt.Fprintln(os.Stderr, "service:", errtext.Translate(err))
		}
		return code
	}
	return 0
}

// writeIcon writes the menu entry's icon, so the installer ships no image.
func writeIcon(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: discord-go-vpn icon PATH")
		return 2
	}
	blurple := color.NRGBA{0x58, 0x65, 0xF2, 0xFF}
	if err := os.WriteFile(args[0], ui.MascotPNG(blurple, 256), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "icon:", err)
		return 1
	}
	return 0
}

func runTray() int {
	if err := trayapp.Run(); err != nil {
		fmt.Fprintln(os.Stderr, errtext.Translate(err))
		return 1
	}
	return 0
}
