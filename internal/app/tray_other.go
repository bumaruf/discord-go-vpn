//go:build !windows

package app

import "github.com/bumaruf/discord-go-vpn/internal/supervisor"

func startTray(*supervisor.Supervisor) (func(), error) { return nil, nil }
