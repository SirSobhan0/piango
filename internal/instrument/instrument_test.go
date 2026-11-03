package instrument

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirSobhan0/piango/internal/mathexpr"
)

func TestBuiltinsAllCompile(t *testing.T) {
	insts := Builtins()
	if len(insts) == 0 {
		t.Fatal("no built-in instruments")
	}

	seen := map[string]bool{}
	for _, in := range insts {
		if seen[in.Name] {
			t.Errorf("duplicate instrument name %q", in.Name)
		}
		seen[in.Name] = true

		if in.About == "" {
			t.Errorf("%s has no explanation", in.Name)
		}
		if in.Formula == "" {
			t.Errorf("%s has no display formula", in.Name)
		}
	}
}

// Every instrument must produce sound that is audible but never clipping.
func TestBuiltinsAreNormalized(t *testing.T) {
	for _, in := range Builtins() {
		wave := in.Wave(1024)

		peak := 0.0
		for _, v := range wave {
			if a := math.Abs(v); a > peak {
				peak = a
			}
		}

		if peak > 1.0 {
			t.Errorf("%s peaks at %.3f, which clips", in.Name, peak)
		}
		// Noise instruments vary per sample, so only check they are not silent.
		if peak < 0.05 {
			t.Errorf("%s peaks at %.3f, which is inaudibly quiet", in.Name, peak)
		}
	}
}

// Instruments whose output depends only on phase must be tabulated, since that
// is what keeps large Fourier sums affordable at audio rate.
func TestPhaseOnlyInstrumentsAreTabulated(t *testing.T) {
	for _, in := range Builtins() {
		want := in.Prog.PhaseOnly()
		if got := in.Tabulated(); got != want {
			t.Errorf("%s: Tabulated = %v, want %v", in.Name, got, want)
		}
	}
}

func TestTabulatedInstrumentsExist(t *testing.T) {
	n := 0
	for _, in := range Builtins() {
		if in.Tabulated() {
			n++
		}
	}
	if n == 0 {
		t.Fatal("no instrument was tabulated, so the optimization is not working")
	}
}

// The Fourier square series must contain odd harmonics falling off as 1/k and
// essentially no even harmonics.
func TestFourierSquareHasOnlyOddHarmonics(t *testing.T) {
	in := findBuiltin(t, "Fourier Square")
	h := in.Harmonics(8)

	for k := 2; k <= 8; k += 2 {
		if h[k-1] > 0.02 {
			t.Errorf("even harmonic %d has magnitude %.4f, want near 0", k, h[k-1])
		}
	}
	// The third harmonic should be about a third of the fundamental.
	if ratio := h[2] / h[0]; math.Abs(ratio-1.0/3.0) > 0.02 {
		t.Errorf("3rd/1st = %.4f, want 0.333", ratio)
	}
	// And the fifth about a fifth.
	if ratio := h[4] / h[0]; math.Abs(ratio-0.2) > 0.02 {
		t.Errorf("5th/1st = %.4f, want 0.2", ratio)
	}
}

// A sawtooth contains every harmonic at 1/k.
func TestFourierSawHasAllHarmonics(t *testing.T) {
	in := findBuiltin(t, "Fourier Saw")
	h := in.Harmonics(6)

	for k := 2; k <= 6; k++ {
		want := 1.0 / float64(k)
		if got := h[k-1] / h[0]; math.Abs(got-want) > 0.02 {
			t.Errorf("harmonic %d = %.4f of fundamental, want %.4f", k, got, want)
		}
	}
}

// A triangle's harmonics fall off as 1/k², much faster than a square's.
func TestFourierTriangleFallsOffQuadratically(t *testing.T) {
	in := findBuiltin(t, "Fourier Triangle")
	h := in.Harmonics(6)

	if got, want := h[2]/h[0], 1.0/9.0; math.Abs(got-want) > 0.02 {
		t.Errorf("3rd/1st = %.4f, want %.4f", got, want)
	}
	if got, want := h[4]/h[0], 1.0/25.0; math.Abs(got-want) > 0.02 {
		t.Errorf("5th/1st = %.4f, want %.4f", got, want)
	}
}

// The Chebyshev shaper's whole point is producing exactly one harmonic.
func TestChebyshevShaperIsolatesOneHarmonic(t *testing.T) {
	in := findBuiltin(t, "Chebyshev Shaper").Clone()

	for _, order := range []float64{2, 3, 5} {
		in.SetParam(0, order)
		h := in.Harmonics(8)

		want := int(order)
		for k, mag := range h {
			if k+1 == want {
				if mag < 0.9 {
					t.Errorf("order %v: harmonic %d = %.3f, want near 1", order, k+1, mag)
				}
			} else if mag > 0.05 {
				t.Errorf("order %v: unexpected harmonic %d = %.3f", order, k+1, mag)
			}
		}
	}
}

// A ring modulator outputs sum and difference frequencies, not the inputs.
func TestRingModulatorSuppressesCarrier(t *testing.T) {
	in := findBuiltin(t, "Ring Modulator").Clone()
	in.SetParam(0, 3) // integer ratio keeps the products on the harmonic grid

	// sin(p)*sin(3p) = [cos(2p) - cos(4p)]/2, so only bins 2 and 4 have energy.
	h := in.Harmonics(6)
	if h[0] > 0.05 {
		t.Errorf("fundamental leaked through at %.3f", h[0])
	}
	if h[1] < 0.9 || h[3] < 0.9 {
		t.Errorf("expected strong 2nd and 4th harmonics, got %.3f and %.3f", h[1], h[3])
	}
}

// Odd-symmetric saturation adds odd harmonics only.
func TestTanhSaturatorAddsOddHarmonics(t *testing.T) {
	in := findBuiltin(t, "Tanh Saturator")
	h := in.Harmonics(6)

	if h[2] < 0.05 {
		t.Errorf("expected a third harmonic from saturation, got %.4f", h[2])
	}
	for k := 2; k <= 6; k += 2 {
		if h[k-1] > 0.02 {
			t.Errorf("even harmonic %d = %.4f, want near 0 for an odd-symmetric shaper", k, h[k-1])
		}
	}
}

// Raising the FM index must add sidebands and therefore brightness.
func TestFMIndexControlsBrightness(t *testing.T) {
	in := findBuiltin(t, "FM Bell").Clone()

	energy := func() float64 {
		h := in.Harmonics(12)
		sum := 0.0
		for k := 3; k < len(h); k++ {
			sum += h[k]
		}
		return sum
	}

	in.SetParam(0, 0.5)
	low := energy()
	in.SetParam(0, 8)
	high := energy()

	if high <= low {
		t.Errorf("raising the FM index did not add upper harmonics: %.4f then %.4f", low, high)
	}
}

func TestSetParamClampsToRange(t *testing.T) {
	in := findBuiltin(t, "Wavefolder").Clone()
	p := in.Params[0]

	in.SetParam(0, 1e6)
	if in.Params[0].Value != p.Max {
		t.Errorf("value = %v, want the max of %v", in.Params[0].Value, p.Max)
	}
	in.SetParam(0, -1e6)
	if in.Params[0].Value != p.Min {
		t.Errorf("value = %v, want the min of %v", in.Params[0].Value, p.Min)
	}

	// Out-of-range indices must be ignored rather than panic.
	in.SetParam(99, 1)
	in.SetParam(-1, 1)
}

// Changing a parameter must actually change the sound.
func TestSetParamRebuildsTable(t *testing.T) {
	in := findBuiltin(t, "Wavefolder").Clone()
	if !in.Tabulated() {
		t.Skip("instrument is not tabulated")
	}

	before := in.Wave(64)
	in.SetParam(0, 9)
	after := in.Wave(64)

	same := true
	for i := range before {
		if math.Abs(before[i]-after[i]) > 1e-9 {
			same = false
			break
		}
	}
	if same {
		t.Error("the wavetable did not change after a parameter change")
	}
}

func TestCloneIsIndependent(t *testing.T) {
	orig := findBuiltin(t, "FM Bell")
	clone := orig.Clone()

	origVal := orig.Params[0].Value
	clone.SetParam(0, origVal+2)

	if orig.Params[0].Value != origVal {
		t.Errorf("editing the clone changed the original to %v", orig.Params[0].Value)
	}
}

func TestCompileRejectsBadDefinitions(t *testing.T) {
	cases := []struct {
		name string
		def  Def
	}{
		{"no name", Def{Expr: "sin(p)"}},
		{"blank name", Def{Name: "   ", Expr: "sin(p)"}},
		{"no expr", Def{Name: "X"}},
		{"blank expr", Def{Name: "X", Expr: "  "}},
		{"bad syntax", Def{Name: "X", Expr: "sin("}},
		{"unknown variable", Def{Name: "X", Expr: "zzz"}},
	}

	for _, tc := range cases {
		if _, err := Compile(tc.def); err == nil {
			t.Errorf("%s: expected an error", tc.name)
		}
	}
}

func TestEnvelopeDefaultsFillGaps(t *testing.T) {
	in, err := Compile(Def{Name: "X", Expr: "sin(p)"})
	if err != nil {
		t.Fatal(err)
	}

	// A zero attack would click, so it must be replaced.
	if in.Envelope.Attack <= 0 {
		t.Errorf("attack = %v, want a positive default", in.Envelope.Attack)
	}
	if in.Envelope.Release <= 0 {
		t.Errorf("release = %v, want a positive default", in.Envelope.Release)
	}
	if in.Envelope.Sustain <= 0 || in.Envelope.Sustain > 1 {
		t.Errorf("sustain = %v, want 0..1", in.Envelope.Sustain)
	}
}

func TestExplicitGainOverridesNormalization(t *testing.T) {
	in, err := Compile(Def{Name: "X", Expr: "sin(p)", Gain: 0.25})
	if err != nil {
		t.Fatal(err)
	}
	if in.Normalized() != 0.25 {
		t.Errorf("gain = %v, want 0.25", in.Normalized())
	}
}

func TestDisplayPrefersFormula(t *testing.T) {
	in := MustCompile(Def{Name: "X", Expr: "sin(p)", Formula: "y = sin(p)"})
	if in.Display() != "y = sin(p)" {
		t.Errorf("Display = %q, want the formula", in.Display())
	}

	in2 := MustCompile(Def{Name: "Y", Expr: "sin(p)"})
	if in2.Display() != "sin(p)" {
		t.Errorf("Display = %q, want the expression", in2.Display())
	}
}

func TestLoadFileMissingIsNotAnError(t *testing.T) {
	insts, problems, err := LoadFile(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Errorf("missing file gave error %v, want nil", err)
	}
	if len(insts) != 0 || len(problems) != 0 {
		t.Errorf("missing file yielded %d instruments and %d problems", len(insts), len(problems))
	}
}

func TestLoadFileParsesInstruments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instruments.json")
	content := `{
	  "instruments": [
	    {
	      "name": "Test Organ",
	      "expr": "sum(k, 1, 3, sin(k*p)/k)",
	      "about": "test",
	      "envelope": { "attack": 0.01, "decay": 0.1, "sustain": 0.8, "release": 0.2 },
	      "params": [{ "name": "x", "value": 1, "min": 0, "max": 2 }]
	    }
	  ]
	}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	insts, problems, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 0 {
		t.Errorf("unexpected problems: %v", problems)
	}
	if len(insts) != 1 {
		t.Fatalf("got %d instruments, want 1", len(insts))
	}
	if insts[0].Name != "Test Organ" {
		t.Errorf("name = %q", insts[0].Name)
	}
	if insts[0].Envelope.Attack != 0.01 {
		t.Errorf("attack = %v, want 0.01", insts[0].Envelope.Attack)
	}
}

// One broken instrument must not prevent the others from loading.
func TestLoadFileSkipsBrokenInstruments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instruments.json")
	content := `{
	  "instruments": [
	    { "name": "Good", "expr": "sin(p)" },
	    { "name": "Broken", "expr": "sin(" },
	    { "name": "AlsoGood", "expr": "sin(2p)" }
	  ]
	}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	insts, problems, err := LoadFile(path)
	if err != nil {
		t.Fatalf("a broken entry should not fail the whole load: %v", err)
	}
	if len(insts) != 2 {
		t.Errorf("got %d instruments, want 2", len(insts))
	}
	if len(problems) != 1 {
		t.Errorf("got %d problems, want 1", len(problems))
	}
	if len(problems) > 0 && !strings.Contains(problems[0], "Broken") {
		t.Errorf("problem should name the instrument, got %q", problems[0])
	}
}

func TestLoadFileRejectsMalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instruments.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadFile(path); err == nil {
		t.Error("expected an error for malformed JSON")
	}
}

func TestExampleConfigIsValid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instruments.json")
	if err := os.WriteFile(path, []byte(ExampleConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	insts, problems, err := LoadFile(path)
	if err != nil {
		t.Fatalf("the shipped example config does not load: %v", err)
	}
	if len(problems) != 0 {
		t.Errorf("the example config has problems: %v", problems)
	}
	if len(insts) == 0 {
		t.Error("the example config defines no instruments")
	}
}

// A Def must survive a round trip through JSON so saving and reloading works.
func TestDefRoundTripsThroughJSON(t *testing.T) {
	orig := Def{
		Name:     "RT",
		Expr:     "sin(p + a*sin(2p))",
		Formula:  "y = sin(p + a sin 2p)",
		About:    "round trip",
		Envelope: Envelope{Attack: 0.01, Decay: 0.2, Sustain: 0.5, Release: 0.3},
		Params:   []Param{{Name: "idx", Value: 2, Min: 0, Max: 8}},
	}

	data, err := json.Marshal(file{Instruments: []Def{orig}})
	if err != nil {
		t.Fatal(err)
	}

	var back file
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Instruments) != 1 {
		t.Fatalf("got %d instruments back", len(back.Instruments))
	}

	got := back.Instruments[0]
	if got.Name != orig.Name || got.Expr != orig.Expr || got.Envelope != orig.Envelope {
		t.Errorf("round trip changed the definition:\n got %+v\nwant %+v", got, orig)
	}
	if len(got.Params) != 1 || got.Params[0].Value != 2 {
		t.Errorf("params did not round trip: %+v", got.Params)
	}
}

func TestWaveIsDeterministicForTabulated(t *testing.T) {
	in := findBuiltin(t, "Fourier Saw")
	a := in.Wave(128)
	b := in.Wave(128)

	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("wave differs between calls at %d: %v vs %v", i, a[i], b[i])
		}
	}
}

func TestParams4PadsMissingSlots(t *testing.T) {
	in := MustCompile(Def{
		Name:   "P",
		Expr:   "a*sin(p)",
		Params: []Param{{Name: "gain", Value: 0.5, Min: 0, Max: 1}},
	})

	got := in.Params4()
	if got[0] != 0.5 {
		t.Errorf("slot a = %v, want 0.5", got[0])
	}
	for i := 1; i < 4; i++ {
		if got[i] != 0 {
			t.Errorf("unused slot %d = %v, want 0", i, got[i])
		}
	}
}

// Instruments that read t must not be tabulated, since their output changes
// over time rather than repeating every cycle.
func TestTimeVaryingInstrumentIsNotTabulated(t *testing.T) {
	in := findBuiltin(t, "Tremolo Sine")
	if in.Tabulated() {
		t.Error("a time-varying instrument was incorrectly tabulated")
	}
	if !in.Prog.UsesTime() {
		t.Error("expected the tremolo to read t")
	}
}

func TestNoiseInstrumentIsNotTabulated(t *testing.T) {
	in := findBuiltin(t, "White Noise")
	if in.Tabulated() {
		t.Error("a noise instrument was incorrectly tabulated")
	}
}

// Every built-in should stay near zero DC, since an offset wastes headroom.
func TestBuiltinsAreRoughlyCentered(t *testing.T) {
	for _, in := range Builtins() {
		if in.Prog.UsesNoise() {
			continue
		}
		if dc := math.Abs(mathexpr.DCOffset(in.Prog, func(v *mathexpr.Vars) { in.applyParams(v) })); dc > 0.25 {
			t.Errorf("%s has a DC offset of %.3f", in.Name, dc)
		}
	}
}

func findBuiltin(t *testing.T, name string) *Instrument {
	t.Helper()
	for _, in := range Builtins() {
		if in.Name == name {
			return in
		}
	}
	t.Fatalf("no built-in instrument named %q", name)
	return nil
}
