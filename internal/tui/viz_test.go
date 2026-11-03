package tui

import (
	"math"
	"strings"
	"testing"
)

func TestCanvasResolution(t *testing.T) {
	c := NewCanvas(10, 5)
	w, h := c.Dots()
	if w != 20 || h != 20 {
		t.Errorf("dots = %dx%d, want 20x20", w, h)
	}
}

func TestCanvasClampsSize(t *testing.T) {
	c := NewCanvas(0, -3)
	if w, h := c.Dots(); w != 2 || h != 4 {
		t.Errorf("dots = %dx%d, want a minimum 2x4", w, h)
	}
}

func TestCanvasSetLightsOneCell(t *testing.T) {
	c := NewCanvas(2, 1)
	c.Set(0, 0)

	rows := c.Rows()
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	// The top-left dot is bit 0x01 above the braille base.
	got := []rune(rows[0])
	if got[0] != brailleBase+0x01 {
		t.Errorf("first cell = %U, want %U", got[0], brailleBase+0x01)
	}
	if got[1] != brailleBase {
		t.Errorf("second cell = %U, want the empty braille cell", got[1])
	}
}

func TestCanvasIgnoresOutOfBounds(t *testing.T) {
	c := NewCanvas(2, 1)
	// None of these should panic or set anything.
	c.Set(-1, 0)
	c.Set(0, -1)
	c.Set(100, 0)
	c.Set(0, 100)

	for _, r := range c.Rows() {
		for _, ch := range r {
			if ch != brailleBase {
				t.Fatalf("an out-of-bounds Set lit a cell: %U", ch)
			}
		}
	}
}

func TestCanvasLineIsConnected(t *testing.T) {
	c := NewCanvas(20, 20)
	dw, dh := c.Dots()
	c.Line(0, 0, dw-1, dh-1)

	// Every row of the diagonal should have at least one lit dot.
	rows := c.Rows()
	for i, r := range rows {
		if strings.Trim(r, string(rune(brailleBase))) == "" {
			t.Errorf("row %d of the diagonal is empty", i)
		}
	}
}

func TestPlotCentersAtZero(t *testing.T) {
	const rows = 8
	c := NewCanvas(20, rows)
	flat := make([]float64, 64) // all zeros
	Plot(c, flat)

	// findLitRange reports character rows, so compare against the middle row.
	mid := rows / 2
	lo, hi := findLitRange(c)
	if lo > mid || hi < mid-1 {
		t.Errorf("zero signal plotted at rows %d..%d, want it near the center %d", lo, hi, mid)
	}
}

func TestPlotFollowsAmplitude(t *testing.T) {
	const rows = 12
	c := NewCanvas(40, rows)
	sine := make([]float64, 256)
	for i := range sine {
		sine[i] = math.Sin(2 * math.Pi * float64(i) / float64(len(sine)))
	}
	Plot(c, sine)

	lo, hi := findLitRange(c)

	// A full-scale sine should reach the first and last character rows.
	if lo != 0 {
		t.Errorf("sine peak reached only row %d, want row 0", lo)
	}
	if hi != rows-1 {
		t.Errorf("sine trough reached only row %d, want row %d", hi, rows-1)
	}
}

func TestPlotHandlesTinyInput(t *testing.T) {
	c := NewCanvas(10, 4)
	Plot(c, nil)
	Plot(c, []float64{0.5}) // one sample, cannot draw a line
	// Both should be no-ops rather than panics.
	if lo, hi := findLitRange(c); lo != -1 || hi != -1 {
		t.Error("expected nothing to be drawn for degenerate input")
	}
}

func TestPlotXYDrawsACircleForEqualFrequencies(t *testing.T) {
	c := NewCanvas(30, 15)
	n := 512
	xs := make([]float64, n)
	ys := make([]float64, n)
	for i := range n {
		a := 2 * math.Pi * float64(i) / float64(n)
		xs[i] = math.Cos(a)
		ys[i] = math.Sin(a)
	}
	PlotXY(c, xs, ys)

	// A circle should light the middle rows near both horizontal edges but
	// leave the exact center empty.
	if !anyLitInRow(c, 0) {
		t.Error("the top of the circle is missing")
	}
}

func TestBarsHeightTracksValue(t *testing.T) {
	rows := Bars([]float64{0, 0.5, 1}, 4)
	if len(rows) != 4 {
		t.Fatalf("got %d rows, want 4", len(rows))
	}

	col := func(rowStr string, i int) rune { return []rune(rowStr)[i] }

	// The zero column is blank in every row.
	for _, r := range rows {
		if col(r, 0) != ' ' {
			t.Errorf("the zero-value column should be blank, got %q", col(r, 0))
		}
	}
	// The full column is a solid block in the bottom row.
	if col(rows[3], 2) != '█' {
		t.Errorf("the full column's bottom cell = %q, want a full block", col(rows[3], 2))
	}
	// The full column reaches the top row too.
	if col(rows[0], 2) != '█' {
		t.Errorf("the full column's top cell = %q, want a full block", col(rows[0], 2))
	}
}

func TestBarsClampsOutOfRange(t *testing.T) {
	// Values outside 0..1 must not panic or index past the block runes.
	rows := Bars([]float64{-5, 2, math.NaN()}, 3)
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
}

func TestMeterFillsProportionally(t *testing.T) {
	if got := Meter(0, 10); strings.Count(got, "█") != 0 {
		t.Errorf("empty meter = %q", got)
	}
	if got := Meter(1, 10); strings.Count(got, "█") != 10 {
		t.Errorf("full meter = %q", got)
	}
	if got := Meter(0.5, 10); strings.Count(got, "█") != 5 {
		t.Errorf("half meter = %q", got)
	}
}

func TestMeterWidthIsExact(t *testing.T) {
	for _, lvl := range []float64{0, 0.3, 0.7, 1, 5, -1} {
		if got := len([]rune(Meter(lvl, 12))); got != 12 {
			t.Errorf("meter width for %v = %d, want 12", lvl, got)
		}
	}
}

func TestSmootherRisesFastAndFallsSlow(t *testing.T) {
	s := NewSmoother(1.0, 0.1) // instant rise, gentle fall

	// A jump up should be tracked immediately.
	up := s.Update([]float64{1, 1})
	if up[0] < 0.99 {
		t.Errorf("value after an instant-rise update = %v, want about 1", up[0])
	}

	// A jump down should lag.
	down := s.Update([]float64{0, 0})
	if down[0] > 0.95 || down[0] < 0.85 {
		t.Errorf("value after one fall step = %v, want about 0.9", down[0])
	}
}

func TestSmootherAdaptsToLengthChange(t *testing.T) {
	s := NewSmoother(0.5, 0.5)
	s.Update([]float64{1, 1, 1})

	got := s.Update([]float64{0.2, 0.2}) // different length
	if len(got) != 2 {
		t.Fatalf("got length %d, want 2", len(got))
	}
	// On a resize it should snap to the new values rather than blend.
	if math.Abs(got[0]-0.2) > 1e-9 {
		t.Errorf("value after resize = %v, want the raw 0.2", got[0])
	}
}

// findLitRange returns the first and last character rows that contain any lit
// dot, or -1, -1 if the canvas is blank.
func findLitRange(c *Canvas) (lo, hi int) {
	lo, hi = -1, -1
	for i, r := range c.Rows() {
		lit := false
		for _, ch := range r {
			if ch != brailleBase {
				lit = true
				break
			}
		}
		if lit {
			if lo == -1 {
				lo = i
			}
			hi = i
		}
	}
	return
}

func anyLitInRow(c *Canvas, row int) bool {
	rows := c.Rows()
	if row < 0 || row >= len(rows) {
		return false
	}
	for _, ch := range rows[row] {
		if ch != brailleBase {
			return true
		}
	}
	return false
}
