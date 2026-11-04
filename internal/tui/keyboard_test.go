package tui

import (
	"math"
	"testing"

	"github.com/SirSobhan0/piango/v2/internal/tuning"
)

func TestHomeRowStartsAtReference(t *testing.T) {
	k := NewKeyboard(tuning.Standard())

	f, ok := k.FreqOf("a")
	if !ok {
		t.Fatal(`the key "a" is not mapped`)
	}
	if math.Abs(f-440) > 1e-9 {
		t.Errorf("home Do = %.4f Hz, want the 440 Hz reference", f)
	}
}

func TestLayoutCoversAllRows(t *testing.T) {
	k := NewKeyboard(tuning.Standard())
	if got := len(k.Keys()); got != 21 {
		t.Errorf("got %d keys, want 21", got)
	}

	for _, char := range []string{"q", "u", "a", "j", "z", "m"} {
		if _, ok := k.Lookup(char); !ok {
			t.Errorf("key %q is missing from the layout", char)
		}
	}
	if _, ok := k.Lookup("1"); ok {
		t.Error(`"1" should not be a note key`)
	}
}

func TestLookupAcceptsShiftedKeys(t *testing.T) {
	k := NewKeyboard(tuning.Standard())
	upper, ok := k.Lookup("A")
	if !ok {
		t.Fatal(`the shifted key "A" was not recognized`)
	}
	lower, _ := k.Lookup("a")
	if upper != lower {
		t.Errorf("shifted lookup gave %+v, want %+v", upper, lower)
	}
}

func TestRowsAreOneOctaveApart(t *testing.T) {
	k := NewKeyboard(tuning.Standard())

	low, _ := k.FreqOf("z")
	mid, _ := k.FreqOf("a")
	high, _ := k.FreqOf("q")

	if r := mid / low; math.Abs(r-2) > 1e-9 {
		t.Errorf("home/bottom ratio = %.6f, want 2", r)
	}
	if r := high / mid; math.Abs(r-2) > 1e-9 {
		t.Errorf("top/home ratio = %.6f, want 2", r)
	}
}

func TestOctaveShiftDoublesFrequency(t *testing.T) {
	k := NewKeyboard(tuning.Standard())
	base, _ := k.FreqOf("a")

	k.ShiftOctave(1)
	up, _ := k.FreqOf("a")
	if r := up / base; math.Abs(r-2) > 1e-9 {
		t.Errorf("one octave up gave a ratio of %.6f, want 2", r)
	}

	k.ShiftOctave(-2)
	down, _ := k.FreqOf("a")
	if r := down / base; math.Abs(r-0.5) > 1e-9 {
		t.Errorf("one octave down gave a ratio of %.6f, want 0.5", r)
	}
}

func TestOctaveShiftIsClamped(t *testing.T) {
	k := NewKeyboard(tuning.Standard())
	for range 20 {
		k.ShiftOctave(1)
	}
	if k.Octave() != 3 {
		t.Errorf("octave clamped to %d, want 3", k.Octave())
	}

	for range 40 {
		k.ShiftOctave(-1)
	}
	if k.Octave() != -3 {
		t.Errorf("octave clamped to %d, want -3", k.Octave())
	}

	// Even at the extremes the pitch must stay audible.
	f, _ := k.FreqOf("z")
	if f <= 0 || f > 20000 {
		t.Errorf("extreme low key = %.3f Hz, want something audible", f)
	}
}

func TestJustIntonationGivesExactRatios(t *testing.T) {
	k := NewKeyboard(tuning.System{Kind: tuning.JustIntonation, Divisions: 12, RefFreq: 440})

	do, _ := k.FreqOf("a")
	sol, _ := k.FreqOf("g")

	if r := sol / do; math.Abs(r-1.5) > 1e-9 {
		t.Errorf("just fifth ratio = %.9f, want exactly 1.5", r)
	}

	mi, _ := k.FreqOf("d")
	if r := mi / do; math.Abs(r-1.25) > 1e-9 {
		t.Errorf("just major third ratio = %.9f, want exactly 1.25", r)
	}
}

func TestEqualTemperamentFifthIsSlightlyNarrow(t *testing.T) {
	k := NewKeyboard(tuning.Standard())
	do, _ := k.FreqOf("a")
	sol, _ := k.FreqOf("g")

	cents := tuning.Cents(sol / do)
	// The 12-TET fifth is 700 cents; the pure fifth is about 701.955.
	if math.Abs(cents-700) > 1e-6 {
		t.Errorf("12-TET fifth = %.4f cents, want 700", cents)
	}
}

func TestNonTwelveToneKeepsOctavesIntact(t *testing.T) {
	for _, div := range []int{19, 24, 31} {
		k := NewKeyboard(tuning.System{Kind: tuning.EqualTemperament, Divisions: div, RefFreq: 440})

		mid, _ := k.FreqOf("a")
		high, _ := k.FreqOf("q")
		if r := high / mid; math.Abs(r-2) > 1e-9 {
			t.Errorf("%d-TET row spacing = %.6f, want an exact octave", div, r)
		}

		// The fifth should land within a few cents of pure in every one of
		// these systems, which is the point of choosing them.
		sol, _ := k.FreqOf("g")
		err := tuning.Cents(sol/mid) - tuning.Cents(1.5)
		if math.Abs(err) > 12 {
			t.Errorf("%d-TET fifth is off by %.2f cents, want under 12", div, err)
		}
	}
}

func TestCycleTuningReturnsToStart(t *testing.T) {
	k := NewKeyboard(tuning.Standard())
	start := k.System()

	seen := map[string]bool{start.Name(): true}
	for range 20 {
		s := k.CycleTuning()
		if s == start {
			if len(seen) < 4 {
				t.Errorf("the cycle only visited %d systems before looping", len(seen))
			}
			return
		}
		seen[s.Name()] = true
	}
	t.Error("cycling never returned to the starting system")
}

func TestIDIsUniquePerSoundingNote(t *testing.T) {
	k := NewKeyboard(tuning.Standard())
	ids := map[int]string{}

	for oct := -3; oct <= 3; oct++ {
		k.ShiftOctave(oct - k.Octave())
		for _, key := range k.Keys() {
			id := k.ID(key)
			if prev, dup := ids[id]; dup {
				t.Fatalf("id %d is shared by %q and %q", id, prev, key.Char)
			}
			ids[id] = key.Char
		}
	}
}

func TestEveryKeyProducesAnAudibleFrequency(t *testing.T) {
	systems := []tuning.System{
		tuning.Standard(),
		{Kind: tuning.JustIntonation, Divisions: 12, RefFreq: 440},
		{Kind: tuning.Pythagorean, Divisions: 12, RefFreq: 440},
		{Kind: tuning.MeantoneQuarterComma, Divisions: 12, RefFreq: 440},
		{Kind: tuning.EqualTemperament, Divisions: 31, RefFreq: 440},
	}

	for _, sys := range systems {
		k := NewKeyboard(sys)
		for oct := -3; oct <= 3; oct++ {
			k.ShiftOctave(oct - k.Octave())
			for _, key := range k.Keys() {
				f := k.Freq(key)
				if f <= 0 || math.IsNaN(f) || math.IsInf(f, 0) {
					t.Fatalf("%s key %q octave %d = %v", sys.Name(), key.Char, oct, f)
				}
				if f > 22050 {
					t.Errorf("%s key %q octave %d = %.1f Hz, above Nyquist", sys.Name(), key.Char, oct, f)
				}
			}
		}
	}
}
