package ui

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

type chartMode int

const (
	chartTraffic chartMode = iota
	chartRTT
)

// chartWindows are the spans z cycles through, in seconds.
var chartWindows = []int{60, 5 * 60, 15 * 60}

// chartState is what the person chose about the chart.
type chartState struct {
	mode   chartMode
	window int // index into chartWindows
	// back is the inspected cell counted from the right edge; -1 follows now.
	back  int
	mouse bool // back came from the mouse, which clears it on leaving
}

// series is one line of the chart: a value per dot column and whether it was
// measured there.
type series struct {
	values []float64
	known  []bool
}

// bucket folds n seconds into cols columns, keeping each column's peak, so a
// short burst still shows at the widest window.
func bucket(samples []sample, cols int, value func(sample) (float64, bool)) series {
	s := series{values: make([]float64, cols), known: make([]bool, cols)}
	n := len(samples)
	for c := range cols {
		from, to := c*n/cols, (c+1)*n/cols
		if to <= from {
			to = from + 1
		}
		for _, sm := range samples[from:min(to, n)] {
			if v, ok := value(sm); ok {
				s.known[c] = true
				s.values[c] = math.Max(s.values[c], v)
			}
		}
	}
	return s
}

func (s series) peak() float64 {
	p := 0.0
	for i, v := range s.values {
		if s.known[i] {
			p = math.Max(p, v)
		}
	}
	return p
}

// niceCeil rounds up to 1, 2 or 5 times a power of ten, so the scale reads
// round numbers.
func niceCeil(v float64) float64 {
	if v <= 0 {
		return 1
	}
	exp := math.Pow10(int(math.Floor(math.Log10(v))))
	for _, m := range []float64{1, 2, 5, 10} {
		if v <= m*exp {
			return m * exp
		}
	}
	return 10 * exp
}

// Braille cells hold 2×4 dots; brailleBit[dy][dx] is a dot's bit.
var brailleBit = [4][2]rune{{0x01, 0x08}, {0x02, 0x10}, {0x04, 0x20}, {0x40, 0x80}}

// area draws s as columns of dots w cells wide and h tall, growing up from the
// bottom or down from the top. A measured non-zero value always shows a dot.
func area(s series, top float64, w, h int, up bool) [][]rune {
	cells := make([][]rune, h)
	for r := range cells {
		cells[r] = make([]rune, w)
	}
	dots := 4 * h
	for col, v := range s.values {
		if !s.known[col] || v <= 0 {
			continue
		}
		n := min(dots, max(1, int(math.Round(v/top*float64(dots)))))
		for k := range n {
			row, dy := k/4, k%4
			if up {
				row, dy = h-1-k/4, 3-k%4
			}
			cells[row][col/2] |= brailleBit[dy][col%2]
		}
	}
	return cells
}

// plotRows paints the cells; the inspected column is drawn in the cursor colour,
// with a rule through its empty cells.
func plotRows(cells [][]rune, c color.Color, cursor int, cursorColor color.Color) []string {
	line := lipgloss.NewStyle().Foreground(c)
	hl := lipgloss.NewStyle().Foreground(cursorColor).Bold(true)
	rows := make([]string, len(cells))
	for r, row := range cells {
		var b strings.Builder
		for x, bits := range row {
			switch {
			case x == cursor && bits == 0:
				b.WriteString(hl.Render("│"))
			case x == cursor:
				b.WriteString(hl.Render(string(0x2800 + bits)))
			case bits == 0:
				b.WriteByte(' ')
			default:
				b.WriteString(line.Render(string(0x2800 + bits)))
			}
		}
		rows[r] = b.String()
	}
	return rows
}

// chartGeom is where the plot landed on screen, for the mouse.
type chartGeom struct {
	x, y, w, h int
}

func (g chartGeom) cell(x, y int) (int, bool) {
	if g.w == 0 || x < g.x || x >= g.x+g.w || y < g.y || y >= g.y+g.h {
		return 0, false
	}
	return x - g.x, true
}

const (
	chartGutter = 12 // the scale's labels
	// chartChrome is the card's rows around the plot: borders, time axis, readout.
	chartChrome  = 4
	minPlotRows  = 2
	maxPlotRows  = 18
	minChartRows = chartChrome + minPlotRows
)

// chartCard draws the chart in a card of width columns and rows lines. It
// returns the plot's place relative to the card's top-left corner.
func (m Model) chartCard(width, rows int) (string, chartGeom) {
	inner := width - 2*cardChrome
	w := inner - chartGutter
	h := min(maxPlotRows, rows-chartChrome)
	seconds := chartWindows[m.chart.window]
	samples := m.hist.window(seconds)
	cursor := -1
	if m.chart.back >= 0 {
		cursor = w - 1 - min(m.chart.back, w-1)
	}

	var plot, labels []string
	var readout string
	mode := m.chartMode()
	switch mode {
	case chartTraffic:
		down := bucket(samples, 2*w, func(s sample) (float64, bool) { return s.down, s.traffic })
		up := bucket(samples, 2*w, func(s sample) (float64, bool) { return s.up, s.traffic })
		// Each half has its own scale: upload is usually a sliver of download.
		topDown, topUp := rateScale(down.peak()), rateScale(up.peak())
		hDown := (h + 1) / 2
		plot = append(plotRows(area(down, topDown, w, hDown, true), m.theme.download, cursor, m.theme.brand),
			plotRows(area(up, topUp, w, h-hDown, false), m.theme.upload, cursor, m.theme.brand)...)
		labels = make([]string, h)
		labels[0] = paint(m.theme.download, "↓ "+formatRate(topDown))
		labels[h-1] = paint(m.theme.upload, "↑ "+formatRate(topUp))
		readout = m.readout(cursor, w, func(col int) string {
			return paint(m.theme.download, "↓ "+formatRate(cellPeak(down, col))) + "   " +
				paint(m.theme.upload, "↑ "+formatRate(cellPeak(up, col)))
		})

	case chartRTT:
		rtt := bucket(samples, 2*w, func(s sample) (float64, bool) { return float64(s.rtt.Milliseconds()), s.rtt > 0 })
		top := math.Max(20, niceCeil(rtt.peak()))
		plot = plotRows(area(rtt, top, w, h, true), m.theme.rtt, cursor, m.theme.brand)
		labels = make([]string, h)
		labels[0] = paint(m.theme.rtt, fmt.Sprintf("%.0f ms", top))
		labels[h-1] = paint(m.theme.dim, "0 ms")
		readout = m.readout(cursor, w, func(col int) string {
			if v := cellPeak(rtt, col); v > 0 {
				return paint(m.theme.rtt, "RTT "+formatRTT(time.Duration(v)*time.Millisecond))
			}
			return paint(m.theme.dim, "RTT —")
		})
	}

	body := make([]string, 0, h+2)
	for r := range h {
		body = append(body, fit(labels[r], chartGutter)+plot[r])
	}
	axis := paint(m.theme.dim, "há "+windowName(seconds))
	now := paint(m.theme.dim, "agora")
	body = append(body,
		strings.Repeat(" ", chartGutter)+axis+strings.Repeat(" ", max(1, w-lipgloss.Width(axis)-lipgloss.Width(now)))+now,
		readout)

	return m.card(m.chartTitle(mode, seconds), body, width),
		chartGeom{x: cardChrome + chartGutter, y: 1, w: w, h: h}
}

func rateScale(peak float64) float64 {
	// At least 8 kbps, so an idle line doesn't blow noise up to full height.
	return niceCeil(math.Max(peak, 1000)*8) / 8
}

// cellPeak is a cell's value: the larger of its two dot columns.
func cellPeak(s series, cell int) float64 {
	return math.Max(s.values[2*cell], s.values[2*cell+1])
}

// readout says the inspected cell's time and values, or the latest ones.
func (m Model) readout(cursor, w int, values func(col int) string) string {
	col, when := w-1, "agora"
	if cursor >= 0 {
		col = cursor
		when = backLabel(time.Duration(float64(w-1-cursor) / float64(w) * float64(chartWindows[m.chart.window]) * float64(time.Second)))
	}
	marker := paint(m.theme.dim, "  ")
	if cursor >= 0 {
		marker = boldPaint(m.theme.brand, "▸ ")
	}
	return strings.Repeat(" ", chartGutter-2) + marker + bold(when) + "   " + values(col)
}

func backLabel(d time.Duration) string {
	if d < time.Second {
		return "agora"
	}
	if d < time.Minute {
		return fmt.Sprintf("há %ds", int(d.Seconds()))
	}
	return fmt.Sprintf("há %dmin %02ds", int(d.Minutes()), int(d.Seconds())%60)
}

func (m Model) chartTitle(mode chartMode, seconds int) string {
	tab := func(name string, on bool) string {
		if on {
			return boldPaint(m.theme.brand, name)
		}
		return paint(m.theme.dim, name)
	}
	title := tab("tráfego", mode == chartTraffic)
	if m.hasTraffic() {
		title += paint(m.theme.dim, " │ ") + tab("RTT", mode == chartRTT)
	} else {
		title = tab("RTT", true)
	}
	return title + paint(m.theme.dim, " · "+windowName(seconds))
}

// chartMode falls back to RTT where the transport reports no traffic.
func (m Model) chartMode() chartMode {
	if !m.hasTraffic() {
		return chartRTT
	}
	return m.chart.mode
}
