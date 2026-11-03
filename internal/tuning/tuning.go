// Package tuning turns note positions into frequencies under several
// historical and mathematical tuning systems, and describes the intervals
// between them as exact ratios.
package tuning

import (
	"fmt"
	"math"
)

// Kind identifies a tuning system.
type Kind int

const (
	// EqualTemperament divides the octave into N equal ratio steps, so every
	// semitone is 2^(1/N). Standard Western tuning is N=12.
	EqualTemperament Kind = iota
	// JustIntonation uses small whole-number frequency ratios, which beat
	// perfectly but only in one key.
	JustIntonation
	// Pythagorean stacks pure 3:2 fifths, leaving the "wolf" comma behind.
	Pythagorean
	// MeantoneQuarterComma narrows each fifth by a quarter of the syntonic
	// comma to make the major thirds pure.
	MeantoneQuarterComma
)

// System is a fully specified tuning: a kind, a reference pitch, and for
// equal temperament the number of divisions per octave.
type System struct {
	Kind      Kind
	Divisions int     // steps per octave; only used by EqualTemperament
	RefFreq   float64 // frequency of the reference note, e.g. 440
}

// Standard is 12-tone equal temperament at A=440.
func Standard() System {
	return System{Kind: EqualTemperament, Divisions: 12, RefFreq: 440}
}

// justRatios are the 5-limit ratios for the twelve chromatic degrees.
var justRatios = [12]struct {
	num, den int
	name     string
}{
	{1, 1, "unison"},
	{16, 15, "minor second"},
	{9, 8, "major second"},
	{6, 5, "minor third"},
	{5, 4, "major third"},
	{4, 3, "perfect fourth"},
	{45, 32, "tritone"},
	{3, 2, "perfect fifth"},
	{8, 5, "minor sixth"},
	{5, 3, "major sixth"},
	{9, 5, "minor seventh"},
	{15, 8, "major seventh"},
}

// pythagoreanExponents gives the number of 3:2 fifths to stack for each
// chromatic degree; the result is then folded back into one octave.
var pythagoreanExponents = [12]int{0, -5, 2, -3, 4, -1, 6, 1, -4, 3, -2, 5}

// Freq returns the frequency of the note `steps` chromatic steps away from the
// reference pitch. Negative values go down.
func (s System) Freq(steps int) float64 {
	ref := s.RefFreq
	if ref <= 0 {
		ref = 440
	}

	switch s.Kind {
	case EqualTemperament:
		n := s.Divisions
		if n <= 0 {
			n = 12
		}
		return ref * math.Pow(2, float64(steps)/float64(n))

	case JustIntonation:
		octave, degree := divMod12(steps)
		r := justRatios[degree]
		return ref * float64(r.num) / float64(r.den) * math.Pow(2, float64(octave))

	case Pythagorean:
		octave, degree := divMod12(steps)
		e := pythagoreanExponents[degree]
		ratio := math.Pow(1.5, float64(e))
		// Fold the stacked fifths back into a single octave.
		ratio /= math.Pow(2, math.Floor(math.Log2(ratio)))
		return ref * ratio * math.Pow(2, float64(octave))

	case MeantoneQuarterComma:
		octave, degree := divMod12(steps)
		// A quarter-comma meantone fifth is the fourth root of 5.
		fifth := math.Pow(5, 0.25)
		e := pythagoreanExponents[degree]
		ratio := math.Pow(fifth, float64(e))
		ratio /= math.Pow(2, math.Floor(math.Log2(ratio)))
		return ref * ratio * math.Pow(2, float64(octave))
	}

	return ref
}

// divMod12 splits a step count into an octave and a 0..11 degree, flooring
// toward negative infinity so downward intervals behave correctly.
func divMod12(steps int) (octave, degree int) {
	octave = int(math.Floor(float64(steps) / 12))
	return octave, steps - octave*12
}

// Name returns a short human label for the system.
func (s System) Name() string {
	switch s.Kind {
	case EqualTemperament:
		n := s.Divisions
		if n <= 0 {
			n = 12
		}
		return fmt.Sprintf("%d-TET", n)
	case JustIntonation:
		return "Just"
	case Pythagorean:
		return "Pythagorean"
	case MeantoneQuarterComma:
		return "1/4 Meantone"
	}
	return "unknown"
}

// Formula describes how the system computes frequencies, for display in the UI.
func (s System) Formula() string {
	switch s.Kind {
	case EqualTemperament:
		n := s.Divisions
		if n <= 0 {
			n = 12
		}
		return fmt.Sprintf("f = %.0f * 2^(n/%d)", s.RefFreq, n)
	case JustIntonation:
		return "f = ref * small whole-number ratios (3/2, 5/4, ...)"
	case Pythagorean:
		return "f = ref * (3/2)^k, folded into one octave"
	case MeantoneQuarterComma:
		return "f = ref * (5^(1/4))^k, pure major thirds"
	}
	return ""
}

// Cycle returns the next tuning system, for cycling through with a key press.
func (s System) Cycle() System {
	switch {
	case s.Kind == EqualTemperament && s.Divisions == 12:
		return System{Kind: JustIntonation, Divisions: 12, RefFreq: s.RefFreq}
	case s.Kind == JustIntonation:
		return System{Kind: Pythagorean, Divisions: 12, RefFreq: s.RefFreq}
	case s.Kind == Pythagorean:
		return System{Kind: MeantoneQuarterComma, Divisions: 12, RefFreq: s.RefFreq}
	case s.Kind == MeantoneQuarterComma:
		return System{Kind: EqualTemperament, Divisions: 19, RefFreq: s.RefFreq}
	case s.Kind == EqualTemperament && s.Divisions == 19:
		return System{Kind: EqualTemperament, Divisions: 24, RefFreq: s.RefFreq}
	case s.Kind == EqualTemperament && s.Divisions == 24:
		return System{Kind: EqualTemperament, Divisions: 31, RefFreq: s.RefFreq}
	default:
		return Standard()
	}
}

// Cents converts a frequency ratio to cents, the logarithmic unit where an
// equal-tempered semitone is exactly 100.
func Cents(ratio float64) float64 {
	if ratio <= 0 {
		return 0
	}
	return 1200 * math.Log2(ratio)
}

// Interval describes the relationship between two frequencies as the nearest
// simple rational approximation plus the error in cents.
type Interval struct {
	Num, Den int
	Cents    float64
	ErrCents float64 // how far the real ratio sits from Num/Den
	Name     string
}

// String renders the interval as "3:2 major fifth (+2.0c)".
func (iv Interval) String() string {
	s := fmt.Sprintf("%d:%d", iv.Num, iv.Den)
	if iv.Name != "" {
		s += " " + iv.Name
	}
	if math.Abs(iv.ErrCents) >= 0.05 {
		s += fmt.Sprintf(" (%+.1fc)", iv.ErrCents)
	}
	return s
}

// Analyze describes the interval between two frequencies. The lower frequency
// becomes the denominator so the ratio reads as at least 1.
func Analyze(f1, f2 float64) Interval {
	if f1 <= 0 || f2 <= 0 {
		return Interval{Num: 1, Den: 1}
	}
	lo, hi := math.Min(f1, f2), math.Max(f1, f2)
	ratio := hi / lo

	num, den := Rationalize(ratio, 64)
	iv := Interval{
		Num:      num,
		Den:      den,
		Cents:    Cents(ratio),
		ErrCents: Cents(ratio / (float64(num) / float64(den))),
		Name:     intervalName(num, den),
	}
	return iv
}

// intervalName labels the common consonances by their exact ratio.
func intervalName(num, den int) string {
	switch [2]int{num, den} {
	case [2]int{1, 1}:
		return "unison"
	case [2]int{2, 1}:
		return "octave"
	case [2]int{3, 2}:
		return "fifth"
	case [2]int{4, 3}:
		return "fourth"
	case [2]int{5, 4}:
		return "major third"
	case [2]int{6, 5}:
		return "minor third"
	case [2]int{5, 3}:
		return "major sixth"
	case [2]int{8, 5}:
		return "minor sixth"
	case [2]int{9, 8}:
		return "major second"
	case [2]int{16, 15}:
		return "minor second"
	case [2]int{15, 8}:
		return "major seventh"
	case [2]int{7, 4}:
		return "harmonic seventh"
	case [2]int{7, 5}:
		return "septimal tritone"
	}
	return ""
}

// Rationalize finds the best rational approximation p/q to x with q <= maxDen,
// using the continued fraction expansion. This is what lets the UI show
// "3:2" instead of "1.4983".
func Rationalize(x float64, maxDen int) (num, den int) {
	if x <= 0 {
		return 0, 1
	}
	// Continued fraction convergents: h/k, tracking the previous pair.
	h0, k0 := 0, 1
	h1, k1 := 1, 0
	v := x

	for range 32 {
		a := math.Floor(v)
		h2 := int(a)*h1 + h0
		k2 := int(a)*k1 + k0

		if k2 > maxDen {
			break
		}
		h0, k0 = h1, k1
		h1, k1 = h2, k2

		frac := v - a
		if frac < 1e-12 {
			break
		}
		v = 1 / frac
	}

	if k1 == 0 {
		return int(math.Round(x)), 1
	}
	return h1, k1
}

// SyntonicComma is the gap between four pure fifths and a pure major third,
// 81/80, about 21.5 cents. It is the reason no tuning can be pure everywhere.
var SyntonicComma = 81.0 / 80.0

// PythagoreanComma is the gap left after twelve pure fifths fail to close the
// circle, 3^12 / 2^19, about 23.5 cents.
var PythagoreanComma = math.Pow(3, 12) / math.Pow(2, 19)
