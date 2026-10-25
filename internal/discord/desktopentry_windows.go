//go:build windows

package discord

// userDesktopEntry has no counterpart on Windows: Discord is found by its
// installed path.
func userDesktopEntry() string { return "" }
