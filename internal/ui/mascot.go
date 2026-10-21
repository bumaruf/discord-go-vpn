package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"

	"charm.land/lipgloss/v2"
)

// The mascot is Discord's Clyde in pixel art, drawn with half blocks: each text
// row holds two pixel rows. Its mood follows the tunnel, and frame (10 per
// second) drives the animation, so a given frame always draws the same.
type mood int

const (
	moodBooting mood = iota // walking towards the tunnel
	moodHappy
	moodWorried // reconnecting
	moodSad     // Discord back on the normal network
	moodCelebrating
	moodAsleep
)

// sprite is the body; eyes are cut out of it per expression.
var sprite = [spriteH]string{
	".##......##.",
	"############",
	"############",
	"############",
	"############",
	"############",
	".####..####.",
	".##......##.",
}

// stride swaps in for the feet on alternate steps.
const stride = "..##....##.."

const (
	spriteW = 12
	spriteH = 8
	canvasH = spriteH + 2 // room to hop and to step
	margin  = 2           // columns each side for particles

	mascotWidth = spriteW + 2*margin
	mascotRows  = canvasH / 2
)

type eyes int

const (
	eyesOpen  eyes = iota
	eyesShut       // a slit low in the eye: blinking, asleep
	eyesSmile      // a slit high in the eye
	eyesLow        // looking down
)

type pose struct {
	y     int // pixel row of the sprite's top in the canvas
	eyes  eyes
	look  int // -1 left, 1 right
	walk  bool
	color color.Color
}

type particle struct {
	row, col int // col counts from the mascot's left edge, margins included
	glyph    string
	color    color.Color
}

func (m Model) mascot(md mood) string {
	p, parts := animate(md, m.frame, m.theme)
	return renderMascot(p, parts)
}

// animate decides the pose and the particles for a frame.
func animate(md mood, frame int, th theme) (pose, []particle) {
	// Resting one pixel down puts the eyes inside one text row, where they read
	// crisp; a step dips a pixel and a hop rises one.
	p := pose{y: 1, color: th.brand}
	blink := frame%40 >= 38
	if blink {
		p.eyes = eyesShut
	}
	var parts []particle
	right := mascotWidth - margin

	switch md {
	case moodBooting:
		step := frame / 3 % 2
		p.walk = step == 1
		p.y += step
		p.look = 1
		dust := []string{"·", "∙"}[step]
		parts = append(parts, particle{mascotRows - 1, 1 - step, dust, th.dim})

	case moodHappy:
		switch phase := frame % 120; {
		case phase >= 60 && phase < 72:
			p.look = 1
		case phase >= 84 && phase < 96:
			p.look = -1
		}
		if !blink && frame%80 >= 70 {
			p.eyes = eyesSmile
		}
		// A note drifts up every few seconds.
		if t := frame % 60; t < 18 {
			parts = append(parts, particle{2 - t/6, right + t/9, "♪", th.accent})
		}

	case moodWorried:
		p.look = []int{-1, -1, 1, 1}[frame/5%4]
		if frame/4%2 == 0 {
			parts = append(parts, particle{0, right, "?", th.warn})
		}

	case moodSad:
		p.color = th.brandDim
		p.eyes = eyesLow
		if blink {
			p.eyes = eyesShut
		}
		if frame/6%2 == 0 {
			parts = append(parts, particle{0, right, "!", th.err})
		}

	case moodCelebrating:
		p.y -= []int{0, 1, 1, 1, 0, 0}[frame%6]
		p.walk = frame%6 >= 4
		p.eyes = eyesSmile
		glyphs := []string{"✦", "✧"}
		parts = append(parts,
			particle{frame % 3, 0, glyphs[frame/2%2], th.warn},
			particle{(frame + 1) % 3, right + 1, glyphs[(frame/2+1)%2], th.accent},
		)

	case moodAsleep:
		p.eyes = eyesShut
		z := frame / 5 % 3
		parts = append(parts, particle{2 - z, right + z%2, []string{"z", "z", "Z"}[z], th.dim})
	}
	return p, parts
}

func renderMascot(p pose, parts []particle) string {
	var canvas [canvasH][spriteW]bool
	for row, line := range sprite {
		if p.walk && row == spriteH-1 {
			line = stride
		}
		for x, px := range line {
			canvas[p.y+row][x] = px == '#'
		}
	}
	cutEyes(&canvas, p)

	body := lipgloss.NewStyle().Foreground(p.color)
	var b strings.Builder
	for r := range mascotRows {
		var row strings.Builder
		for c := 0; c < mascotWidth; c++ {
			if g, ok := particleAt(parts, r, c); ok {
				row.WriteString(g)
				continue
			}
			x := c - margin
			if x < 0 || x >= spriteW {
				row.WriteByte(' ')
				continue
			}
			row.WriteString(body.Render(halfBlock(canvas[2*r][x], canvas[2*r+1][x])))
		}
		b.WriteString(row.String())
		if r < mascotRows-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// cutEyes clears the eye pixels, two wide and two tall at rows 3 and 4 of the
// sprite, shifted by where the mascot looks.
func cutEyes(canvas *[canvasH][spriteW]bool, p pose) {
	top := p.y + 3
	var rows []int
	switch p.eyes {
	case eyesOpen:
		rows = []int{top, top + 1}
	case eyesShut:
		rows = []int{top + 1}
	case eyesSmile:
		rows = []int{top}
	case eyesLow:
		rows = []int{top + 1, top + 2}
	}
	for _, left := range []int{3, 7} {
		for _, row := range rows {
			canvas[row][left+p.look] = false
			canvas[row][left+1+p.look] = false
		}
	}
}

// MascotPNG draws the resting mascot in c, eyes open, centred on a transparent
// square of size pixels, for an icon.
func MascotPNG(c color.Color, size int) []byte {
	scale := max(1, size/spriteW)
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	left, top := (size-spriteW*scale)/2, (size-spriteH*scale)/2
	for y, line := range sprite {
		for x, px := range line {
			eye := (y == 3 || y == 4) && (x == 3 || x == 4 || x == 7 || x == 8)
			if px != '#' || eye {
				continue
			}
			for dy := range scale {
				for dx := range scale {
					img.Set(left+x*scale+dx, top+y*scale+dy, c)
				}
			}
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func halfBlock(top, bottom bool) string {
	switch {
	case top && bottom:
		return "█"
	case top:
		return "▀"
	case bottom:
		return "▄"
	}
	return " "
}

func particleAt(parts []particle, row, col int) (string, bool) {
	for _, p := range parts {
		if p.row == row && p.col == col {
			return paint(p.color, p.glyph), true
		}
	}
	return "", false
}
