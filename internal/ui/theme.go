package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// theme holds the screen's colours for the terminal's background, which the
// program asks for at start; dark until the answer arrives.
type theme struct {
	brand, brandDim color.Color // Discord's blurple, for the mascot and accents
	accent          color.Color // a second brand colour, for sparkles and the bar's end
	ok, warn, err   color.Color
	dim, border     color.Color

	// The panel's chart colours, so both draw a series alike.
	download, upload, rtt color.Color
}

func newTheme(dark bool) theme {
	ld := lipgloss.LightDark(dark)
	c := lipgloss.Color
	return theme{
		brand:    ld(c("#4752C4"), c("#5865F2")),
		brandDim: ld(c("#8E95D8"), c("#3C45A5")),
		accent:   ld(c("#C42B7E"), c("#EB459E")),
		ok:       ld(c("#248046"), c("#57F287")),
		warn:     ld(c("#A86A00"), c("#FEE75C")),
		err:      ld(c("#C9302C"), c("#ED4245")),
		dim:      ld(c("#6D6F78"), c("#80848E")),
		border:   ld(c("#C4C9CE"), c("#3F4147")),
		download: ld(c("#0B6BCB"), c("#52A8FF")),
		upload:   ld(c("#B26B00"), c("#F5A623")),
		rtt:      ld(c("#C42B7E"), c("#EB459E")),
	}
}

func paint(c color.Color, s string) string {
	return lipgloss.NewStyle().Foreground(c).Render(s)
}

func bold(s string) string { return lipgloss.NewStyle().Bold(true).Render(s) }

func boldPaint(c color.Color, s string) string {
	return lipgloss.NewStyle().Foreground(c).Bold(true).Render(s)
}

func italic(c color.Color, s string) string {
	return lipgloss.NewStyle().Foreground(c).Italic(true).Render(s)
}
