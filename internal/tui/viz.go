package tui

import (
	"math"
	"strings"
)

// Braille cells pack a 2x4 grid of dots into one character, which gives the
// scope and Lissajous plots four times the vertical resolution of block
// characters.
const brailleBase = 0x2800

// dotBits maps a (column, row) position inside a cell to its bit. The odd
// ordering is part of the Unicode braille layout, not a mistake.
var dotBits = [2][4]rune{
	{0x01, 0x02, 0x04, 0x40},
	{0x08, 0x10, 0x20, 0x80},
}

// Canvas is a monochrome plotting surface drawn with braille characters.
type Canvas struct {
	w, h  int // in characters
	cells []rune
}

// NewCanvas creates a canvas w characters wide and h characters tall, giving a
// dot resolution of 2w by 4h.
func NewCanvas(w, h int) *Canvas {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return &Canvas{w: w, h: h, cells: make([]rune, w*h)}
}

// Dots returns the canvas resolution in dots.
func (c *Canvas) Dots() (int, int) { return c.w * 2, c.h * 4 }

// Set lights the dot at (x, y), with y counted downward from the top. Points
// outside the canvas are ignored so callers do not have to clip.
func (c *Canvas) Set(x, y int) {
	dw, dh := c.Dots()
	if x < 0 || y < 0 || x >= dw || y >= dh {
		return
	}
	c.cells[(y/4)*c.w+(x/2)] |= dotBits[x%2][y%4]
}

// Line draws a straight line between two dots using Bresenham's algorithm, so
// a waveform reads as a continuous trace rather than a dotted one.
func (c *Canvas) Line(x0, y0, x1, y1 int) {
	dx := abs(x1 - x0)
	dy := -abs(y1 - y0)
	sx, sy := -1, -1
	if x0 < x1 {
		sx = 1
	}
	if y0 < y1 {
		sy = 1
	}

	err := dx + dy
	for {
		c.Set(x0, y0)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

// Rows renders the canvas to one string per character row.
func (c *Canvas) Rows() []string {
	out := make([]string, c.h)
	var b strings.Builder
	for y := range c.h {
		b.Reset()
		for x := range c.w {
			b.WriteRune(brailleBase + c.cells[y*c.w+x])
		}
		out[y] = b.String()
	}
	return out
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// Plot draws a signal as a centered waveform trace. Values are expected in
// -1..1 and are clipped to the canvas.
func Plot(c *Canvas, samples []float64) {
	if len(samples) < 2 {
		return
	}
	dw, dh := c.Dots()
	mid := float64(dh-1) / 2

	toY := func(v float64) int {
		y := int(math.Round(mid - v*mid))
		return min(max(y, 0), dh-1)
	}

	prevX, prevY := 0, toY(sampleAt(samples, 0))
	for x := 1; x < dw; x++ {
		u := float64(x) / float64(dw-1)
		y := toY(sampleAt(samples, u))
		c.Line(prevX, prevY, x, y)
		prevX, prevY = x, y
	}
}

// PlotXY draws one signal against another, which turns two simultaneous tones
// into a Lissajous figure whose shape is their frequency ratio.
func PlotXY(c *Canvas, xs, ys []float64) {
	n := min(len(xs), len(ys))
	if n < 2 {
		return
	}
	dw, dh := c.Dots()

	// Keep the figure square so a 1:1 ratio reads as a circle, not an ellipse.
	size := min(dw, dh)
	offX := (dw - size) / 2
	offY := (dh - size) / 2
	half := float64(size-1) / 2

	toX := func(v float64) int {
		return offX + min(max(int(math.Round(half+v*half)), 0), size-1)
	}
	toY := func(v float64) int {
		return offY + min(max(int(math.Round(half-v*half)), 0), size-1)
	}

	px, py := toX(xs[0]), toY(ys[0])
	for i := 1; i < n; i++ {
		x, y := toX(xs[i]), toY(ys[i])
		c.Line(px, py, x, y)
		px, py = x, y
	}
}

// sampleAt reads a signal at a normalized position with linear interpolation,
// so a trace looks the same whatever the terminal width.
func sampleAt(s []float64, u float64) float64 {
	if len(s) == 0 {
		return 0
	}
	pos := u * float64(len(s)-1)
	i := int(pos)
	if i >= len(s)-1 {
		return s[len(s)-1]
	}
	return s[i] + (pos-float64(i))*(s[i+1]-s[i])
}

// blocks are the eighth-height bar characters, from empty to full.
var blocks = []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// Bars renders values in 0..1 as vertical bars, returning one string per row
// from top to bottom.
func Bars(values []float64, height int) []string {
	if height < 1 {
		height = 1
	}
	rows := make([]string, height)

	var b strings.Builder
	for r := range height {
		b.Reset()
		fromBottom := height - 1 - r
		for _, v := range values {
			eighths := int(math.Round(clamp01(v) * float64(height) * 8))
			cell := eighths - fromBottom*8
			b.WriteRune(blocks[min(max(cell, 0), 8)])
		}
		rows[r] = b.String()
	}
	return rows
}

// Meter renders a horizontal level bar of the given width.
func Meter(level float64, width int) string {
	if width < 1 {
		return ""
	}
	filled := int(math.Round(clamp01(level) * float64(width)))
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func clamp01(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	return min(max(v, 0), 1)
}

// Smoother applies an asymmetric filter to a set of values: quick to rise so
// transients are visible, slow to fall so the display does not flicker.
type Smoother struct {
	values []float64
	rise   float64
	fall   float64
}

// NewSmoother creates a smoother with the given rise and fall coefficients,
// each in 0..1 where 1 means no smoothing.
func NewSmoother(rise, fall float64) *Smoother {
	return &Smoother{rise: rise, fall: fall}
}

// Update blends the new values into the smoothed set and returns it. The
// returned slice is owned by the Smoother and is overwritten on the next call.
func (s *Smoother) Update(next []float64) []float64 {
	if len(s.values) != len(next) {
		s.values = make([]float64, len(next))
		copy(s.values, next)
		return s.values
	}
	for i, v := range next {
		if v > s.values[i] {
			s.values[i] += (v - s.values[i]) * s.rise
		} else {
			s.values[i] += (v - s.values[i]) * s.fall
		}
	}
	return s.values
}
