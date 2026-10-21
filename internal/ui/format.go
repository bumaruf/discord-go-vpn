package ui

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// The units follow the panel's lib/format.ts: rates in bits per second, base
// 1000; volumes in bytes, base 1024. The decimal comma is Portuguese.
var (
	rateUnits   = []string{"kbps", "Mbps", "Gbps"}
	volumeUnits = []string{"B", "kB", "MB", "GB", "TB"}
)

func rateDecimals(n float64, _ int) int {
	if n < 10 {
		return 1
	}
	return 0
}

func volumeDecimals(n float64, unit int) int {
	switch {
	case unit == 0:
		return 0
	case n < 10:
		return 2
	case n < 100:
		return 1
	}
	return 0
}

// scale picks the unit from the rounded value, so 999.96 kbps reads 1,0 Mbps.
func scale(n, base float64, units []string, decimals func(n float64, unit int) int) string {
	i := 0
	for i < len(units)-1 && roundTo(n, decimals(n, i)) >= base {
		n /= base
		i++
	}
	text := fmt.Sprintf("%.*f", decimals(roundTo(n, decimals(n, i)), i), n)
	return strings.Replace(text, ".", ",", 1) + " " + units[i]
}

func roundTo(n float64, decimals int) float64 {
	p := math.Pow10(decimals)
	return math.Round(n*p) / p
}

// formatRate takes bytes per second.
func formatRate(bytesPerSec float64) string {
	if !(bytesPerSec > 0) {
		return "0 kbps"
	}
	return scale(bytesPerSec*8/1000, 1000, rateUnits, rateDecimals)
}

func formatVolume(bytes uint64) string {
	return scale(float64(bytes), 1024, volumeUnits, volumeDecimals)
}

func formatRTT(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	return fmt.Sprintf("%d ms", max(1, d.Milliseconds()))
}

// ago says how long ago t was, as the panel's formatSince does.
func ago(t, now time.Time) string {
	if t.IsZero() {
		return "nunca"
	}
	d := now.Sub(t)
	switch {
	case d < 5*time.Second:
		return "agora"
	case d < time.Minute:
		return fmt.Sprintf("há %ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("há %dmin", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("há %dh", int(d.Hours()))
	}
	return fmt.Sprintf("há %dd", int(d.Hours()/24))
}

// windowName names a span of the chart, such as "5 min".
func windowName(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	return fmt.Sprintf("%d min", seconds/60)
}
