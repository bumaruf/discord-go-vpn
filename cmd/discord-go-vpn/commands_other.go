//go:build !linux || android

package main

import (
	"fmt"
	"os"
)

func runService([]string) int {
	fmt.Fprintln(os.Stderr, "service: only on Linux")
	return 2
}

func writeIcon([]string) int {
	fmt.Fprintln(os.Stderr, "icon: only on Linux")
	return 2
}

func runTray() int {
	fmt.Fprintln(os.Stderr, "a bandeja separada só existe no Linux; no Windows ela abre junto com o programa")
	return 2
}
