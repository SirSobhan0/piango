package tuning

import (
	"math"
	"testing"
)

func TestEqualTemperamentMatchesReference(t *testing.T) {
	s := Standard()

	cases := []struct {
		steps int
		want  float64
	}{
		{0, 440},       // A4
		{12, 880},      // A5
		{-12, 220},     // A3
		{3, 523.2511},  // C5
		{-9, 261.6256}, // C4, middle C
	}

	for _, tc := range cases {
		if got := s.Freq(tc.steps); math.Abs(got-tc.want) > 0.001 {
			t.Errorf("Freq(%d) = %.4f, want %.4f", tc.steps, got, tc.want)
		}
	}
}

func TestEqualTemperamentOctavesAreExact(t *testing.T) {
	for _, div := range []int{12, 19, 24, 31} {
		s := System{Kind: EqualTemperament, Divisions: div, RefFreq: 440}
		if got := s.Freq(div); math.Abs(got-880) > 1e-9 {
			t.Errorf("%d-TET octave = %v, want 880", div, got)
		}
	}
}

func TestJustIntonationRatiosAreExact(t *testing.T) {
	s := System{Kind: JustIntonation, RefFreq: 440}

	// A perfect fifth in just intonation is exactly 3:2.
	if got, want := s.Freq(7), 440*1.5; math.Abs(got-want) > 1e-9 {
		t.Errorf("just fifth = %v, want %v", got, want)
	}
	// A major third is exactly 5:4.
	if got, want := s.Freq(4), 440*1.25; math.Abs(got-want) > 1e-9 {
		t.Errorf("just major third = %v, want %v", got, want)
	}
	// Octaves remain exact.
	if got := s.Freq(12); math.Abs(got-880) > 1e-9 {
		t.Errorf("just octave = %v, want 880", got)
	}
}

// The equal-tempered major third is famously about 14 cents sharp of pure.
func TestEqualTemperedThirdIsSharpOfJust(t *testing.T) {
	et := Standard().Freq(4)
	just := System{Kind: JustIntonation, RefFreq: 440}.Freq(4)

	diff := Cents(et / just)
	if diff < 13 || diff > 15 {
		t.Errorf("ET third is %.2f cents from just, expected about 13.7", diff)
	}
}

// Pythagorean tuning stacks pure fifths, so its fifth must be exactly 3:2.
func TestPythagoreanFifthIsPure(t *testing.T) {
	s := System{Kind: Pythagorean, RefFreq: 440}
	if got, want := s.Freq(7), 440*1.5; math.Abs(got-want) > 1e-9 {
		t.Errorf("Pythagorean fifth = %v, want %v", got, want)
	}
}

// Quarter-comma meantone exists to make the major third pure.
func TestMeantoneThirdIsPure(t *testing.T) {
	s := System{Kind: MeantoneQuarterComma, RefFreq: 440}
	if got, want := s.Freq(4), 440*1.25; math.Abs(got-want) > 1e-6 {
		t.Errorf("meantone major third = %v, want %v", got, want)
	}
}

func TestNegativeStepsDescend(t *testing.T) {
	for _, k := range []Kind{EqualTemperament, JustIntonation, Pythagorean, MeantoneQuarterComma} {
		s := System{Kind: k, Divisions: 12, RefFreq: 440}
		if got := s.Freq(-12); math.Abs(got-220) > 1e-6 {
			t.Errorf("%s octave down = %v, want 220", s.Name(), got)
		}
		if s.Freq(-7) >= s.Freq(0) {
			t.Errorf("%s: descending interval did not descend", s.Name())
		}
	}
}

func TestCents(t *testing.T) {
	if got := Cents(2); math.Abs(got-1200) > 1e-9 {
		t.Errorf("octave = %v cents, want 1200", got)
	}
	if got := Cents(math.Pow(2, 1.0/12)); math.Abs(got-100) > 1e-9 {
		t.Errorf("semitone = %v cents, want 100", got)
	}
	if got := Cents(1); got != 0 {
		t.Errorf("unison = %v cents, want 0", got)
	}
}

func TestRationalize(t *testing.T) {
	cases := []struct {
		x        float64
		maxDen   int
		num, den int
	}{
		{1.5, 64, 3, 2},
		{1.25, 64, 5, 4},
		{2.0, 64, 2, 1},
		{1.0, 64, 1, 1},
		{1.3333333333, 64, 4, 3},
		{0.5, 64, 1, 2},
	}

	for _, tc := range cases {
		n, d := Rationalize(tc.x, tc.maxDen)
		if n != tc.num || d != tc.den {
			t.Errorf("Rationalize(%v) = %d/%d, want %d/%d", tc.x, n, d, tc.num, tc.den)
		}
	}
}

func TestRationalizeRespectsMaxDenominator(t *testing.T) {
	// Pi has no small rational form; the denominator must stay in bounds.
	_, d := Rationalize(math.Pi, 10)
	if d > 10 {
		t.Errorf("denominator %d exceeds the limit of 10", d)
	}
}

func TestAnalyzeIdentifiesIntervals(t *testing.T) {
	cases := []struct {
		f1, f2   float64
		num, den int
		name     string
	}{
		{440, 660, 3, 2, "fifth"},
		{440, 550, 5, 4, "major third"},
		{440, 880, 2, 1, "octave"},
		{440, 440, 1, 1, "unison"},
		{660, 440, 3, 2, "fifth"}, // order should not matter
	}

	for _, tc := range cases {
		iv := Analyze(tc.f1, tc.f2)
		if iv.Num != tc.num || iv.Den != tc.den {
			t.Errorf("Analyze(%v,%v) = %d:%d, want %d:%d", tc.f1, tc.f2, iv.Num, iv.Den, tc.num, tc.den)
		}
		if iv.Name != tc.name {
			t.Errorf("Analyze(%v,%v) named %q, want %q", tc.f1, tc.f2, iv.Name, tc.name)
		}
	}
}

// The equal-tempered fifth is about 2 cents flat of the pure 3:2.
func TestAnalyzeReportsTemperamentError(t *testing.T) {
	s := Standard()
	iv := Analyze(s.Freq(0), s.Freq(7))

	if iv.Num != 3 || iv.Den != 2 {
		t.Fatalf("expected a 3:2 fifth, got %d:%d", iv.Num, iv.Den)
	}
	if iv.ErrCents > -1.5 || iv.ErrCents < -2.5 {
		t.Errorf("ET fifth error = %.2f cents, want about -1.96", iv.ErrCents)
	}
}

func TestAnalyzeHandlesInvalidInput(t *testing.T) {
	if iv := Analyze(0, 440); iv.Num != 1 || iv.Den != 1 {
		t.Errorf("zero frequency should yield 1:1, got %d:%d", iv.Num, iv.Den)
	}
	if iv := Analyze(-100, 440); iv.Num != 1 || iv.Den != 1 {
		t.Errorf("negative frequency should yield 1:1, got %d:%d", iv.Num, iv.Den)
	}
}

func TestIntervalString(t *testing.T) {
	iv := Analyze(440, 660)
	if got := iv.String(); got == "" {
		t.Error("interval string should not be empty")
	}
}

// Twelve pure fifths overshoot seven octaves by the Pythagorean comma.
func TestPythagoreanCommaValue(t *testing.T) {
	if got := Cents(PythagoreanComma); math.Abs(got-23.46) > 0.01 {
		t.Errorf("Pythagorean comma = %.2f cents, want 23.46", got)
	}
}

func TestSyntonicCommaValue(t *testing.T) {
	if got := Cents(SyntonicComma); math.Abs(got-21.51) > 0.01 {
		t.Errorf("syntonic comma = %.2f cents, want 21.51", got)
	}
}

func TestCycleVisitsAllSystemsAndReturns(t *testing.T) {
	start := Standard()
	seen := map[string]bool{}

	s := start
	for range 20 {
		seen[s.Name()] = true
		s = s.Cycle()
		if s.Name() == start.Name() && s.Kind == start.Kind && s.Divisions == start.Divisions {
			break
		}
	}

	for _, want := range []string{"12-TET", "19-TET", "24-TET", "31-TET", "Just", "Pythagorean", "1/4 Meantone"} {
		if !seen[want] {
			t.Errorf("cycling never reached %s", want)
		}
	}
}

func TestFreqHandlesZeroConfiguration(t *testing.T) {
	// A zero-valued System must not divide by zero or return a bad pitch.
	var s System
	if got := s.Freq(0); got != 440 {
		t.Errorf("zero System reference = %v, want the 440 fallback", got)
	}
	if got := s.Freq(12); math.Abs(got-880) > 1e-9 {
		t.Errorf("zero System octave = %v, want 880", got)
	}
}
