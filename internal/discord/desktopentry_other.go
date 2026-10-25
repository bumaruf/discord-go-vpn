//go:build !windows

package discord

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// desktopEntryIDs are the desktop entries a person may override, one per name
// in namesInPath.
var desktopEntryIDs = []string{"discord", "discord-canary", "discord-ptb", "vesktop"}

// userDesktopEntry returns the executable the person's own desktop entry for
// Discord runs, or "" when there is none. An entry in ~/.local/share/applications
// is how the desktop menu opens Discord when the person wants it opened some
// other way: a wrapper script, a Flatpak, an LD_PRELOAD hook. Discord is
// reopened the same way here, so it keeps working inside the tunnel.
//
// Only an entry whose Exec is a bare executable counts: Find returns one path,
// and a command with arguments has nowhere to carry them.
func userDesktopEntry() string {
	dir := filepath.Join(userDataHome(), "applications")
	for _, id := range desktopEntryIDs {
		if exe := execOfEntry(filepath.Join(dir, id+".desktop")); exe != "" {
			return exe
		}
	}
	return ""
}

// userDataHome is XDG_DATA_HOME for the person Discord opens as. Under sudo,
// the variable belongs to root and the person's own is unknown, so the default
// of their home is used.
func userDataHome() string {
	if username := os.Getenv("SUDO_USER"); username != "" {
		return filepath.Join(homeOf(username), ".local", "share")
	}
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share")
}

// execOfEntry returns the executable the entry's Exec line runs, resolved
// through PATH when it is a bare name, or "" when the entry is missing,
// hidden, has arguments, or its executable isn't there.
func execOfEntry(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	var execLine string
	inEntry := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "["):
			inEntry = line == "[Desktop Entry]"
		case !inEntry:
		case strings.HasPrefix(line, "Hidden=") && strings.TrimSpace(strings.TrimPrefix(line, "Hidden=")) == "true":
			return ""
		case strings.HasPrefix(line, "Exec=") && execLine == "":
			execLine = strings.TrimPrefix(line, "Exec=")
		}
	}

	argv := splitExec(execLine)
	if len(argv) != 1 {
		return ""
	}
	exe, err := exec.LookPath(argv[0])
	if err != nil {
		return ""
	}
	return exe
}

// splitExec splits an Exec value as the Desktop Entry spec reads it: words
// separated by spaces, quoted with double quotes where a backslash escapes the
// next character, with the %-field codes (the files or URLs a launcher would
// pass) dropped.
func splitExec(value string) []string {
	var argv []string
	var word strings.Builder
	inWord, quoted := false, false

	flush := func() {
		if inWord {
			argv = append(argv, word.String())
			word.Reset()
			inWord = false
		}
	}

	runes := []rune(value)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case quoted && r == '\\' && i+1 < len(runes):
			i++
			word.WriteRune(runes[i])
		case r == '"':
			quoted = !quoted
			inWord = true
		case !quoted && r == ' ':
			flush()
		case !quoted && r == '%' && i+1 < len(runes):
			i++
			if runes[i] == '%' {
				word.WriteRune('%')
				inWord = true
			}
			// Any other field code expands to nothing here: there is no file
			// or URL to open.
		default:
			word.WriteRune(r)
			inWord = true
		}
	}
	flush()
	return argv
}
