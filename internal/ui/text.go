package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// wrap breaks text into rows of at most width runes, at spaces where it can.
// Each line keeps its indentation on the rows it wraps into.
func wrap(text string, width int) []string {
	var rows []string
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		// Capped, so every row keeps room for at least one rune of a word.
		indent := line[:min(len(line)-len(strings.TrimLeft(line, " ")), width-1)]
		row := indent
		for _, word := range strings.Fields(line) {
			for {
				room := width - len([]rune(row))
				if row != indent {
					room-- // the space before the word
				}
				if len([]rune(word)) <= room {
					break
				}
				if row != indent {
					rows = append(rows, row)
					row = indent
					continue
				}
				// A word wider than a whole row is split where the row ends.
				r := []rune(word)
				cut := max(1, width-len(indent))
				rows = append(rows, indent+string(r[:cut]))
				word = string(r[cut:])
			}
			if row != indent {
				row += " "
			}
			row += word
		}
		rows = append(rows, row)
	}
	return rows
}

// elide keeps the first n rows, ending the last one in "…" when it drops any.
func elide(rows []string, n, width int) []string {
	if len(rows) <= n {
		return rows
	}
	last := []rune(rows[n-1])
	rows[n-1] = string(last[:min(len(last), width-1)]) + "…"
	return rows[:n]
}

// truncate cuts by rune, so an accented letter is never split.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:max(0, n-1)]) + "…"
}

// fit pads or cuts styled text to exactly width columns.
func fit(s string, width int) string {
	if w := lipgloss.Width(s); w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return ansi.Truncate(s, width, "…")
}

func lines(s string) []string { return strings.Split(strings.TrimRight(s, "\n"), "\n") }
