// Command updatesign makes the update key pair and signs a release's manifest.
//
//	updatesign keygen
//	UPDATE_SIGNING_KEY=... updatesign sign -version v1.2.3 -dir dist linux/amd64=discord-go-vpn ...
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bumaruf/discord-go-vpn/internal/update"
)

func main() {
	if len(os.Args) < 2 {
		fail(fmt.Errorf("usage: updatesign keygen | updatesign sign -version vX.Y.Z -dir DIR GOOS/GOARCH=FILE [...]"))
	}
	switch os.Args[1] {
	case "keygen":
		fail(update.GenerateKey(os.Stdout))
	case "sign":
		fail(sign(os.Args[2:]))
	default:
		fail(fmt.Errorf("unknown command %q", os.Args[1]))
	}
}

func sign(args []string) error {
	fs := flag.NewFlagSet("sign", flag.ExitOnError)
	version := fs.String("version", "", "release tag, such as v1.2.3")
	dir := fs.String("dir", "dist", "folder with the binaries; the manifest and signature are written there")
	_ = fs.Parse(args)

	key, err := update.ParsePrivateKey(os.Getenv("UPDATE_SIGNING_KEY"))
	if err != nil {
		return err
	}
	files := make(map[string]string, fs.NArg())
	for _, arg := range fs.Args() {
		platform, name, ok := strings.Cut(arg, "=")
		if !ok {
			return fmt.Errorf("%q is not GOOS/GOARCH=FILE", arg)
		}
		files[platform] = name
	}

	m, err := update.BuildManifest(*version, *dir, files)
	if err != nil {
		return err
	}
	manifest, signature, err := update.Sign(m, key)
	if err != nil {
		return err
	}
	if err := update.VerifyRelease(manifest, signature); err != nil {
		return fmt.Errorf("UPDATE_SIGNING_KEY doesn't match publicKey in internal/update: %w", err)
	}
	if err := os.WriteFile(filepath.Join(*dir, update.ManifestName), manifest, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(*dir, update.SignatureName), signature, 0o644)
}

func fail(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
