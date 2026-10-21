module github.com/bumaruf/discord-go-vpn

go 1.26.0

// O painel é um app Next.js, e o node_modules dele carrega pacotes Go de
// terceiros que nada têm a ver com este módulo. Sem isto, "go build ./..."
// desce lá e compila código que ninguém pediu.
ignore ./panel

exclude golang.zx2c4.com/wireguard/tun/netstack v0.0.0-20220703234212-c31a7b1ab478

require (
	charm.land/bubbletea/v2 v2.1.0
	charm.land/lipgloss/v2 v2.0.6
	github.com/charmbracelet/x/ansi v0.11.9
	github.com/charmbracelet/x/term v0.2.2
	go.mongodb.org/mongo-driver/v2 v2.9.0
	golang.org/x/crypto v0.58.0
	golang.org/x/mod v0.42.0
	golang.org/x/sys v0.49.0
	golang.zx2c4.com/wireguard v0.0.0-20260522210424-ecfc5a8d5446
)

require (
	github.com/charmbracelet/colorprofile v0.4.3 // indirect
	github.com/charmbracelet/ultraviolet v0.0.0-20261008173134-6b8d4baf91b4 // indirect
	github.com/charmbracelet/x/termios v0.1.1 // indirect
	github.com/charmbracelet/x/windows v0.2.2 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/klauspost/compress v1.19.2 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.1 // indirect
	github.com/mattn/go-runewidth v0.0.30 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.2.0 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/xo/terminfo v1.0.0 // indirect
	github.com/youmark/pkcs8 v0.0.0-20240726163527-a2c0da244d78 // indirect
	golang.org/x/mobile v0.0.0-20261011034528-197770444867 // indirect
	golang.org/x/net v0.61.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	golang.org/x/tools v0.52.0 // indirect
	golang.zx2c4.com/wintun v0.0.0-20230126152724-0fa3db229ce2 // indirect
)

tool (
	golang.org/x/mobile/cmd/gobind
	golang.org/x/mobile/cmd/gomobile
)
