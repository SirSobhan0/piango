package synth

import (
	"math"
	"sync"
	"testing"

	"github.com/SirSobhan0/piango/v2/internal/instrument"
)

const testSR = 44100

func testEngine(t *testing.T, expr string, env instrument.Envelope) *Engine {
	t.Helper()
	in, err := instrument.Compile(instrument.Def{
		Name:     "test",
		Expr:     expr,
		Envelope: env,
	})
	if err != nil {
		t.Fatal(err)
	}
	return NewEngine(in, testSR)
}

// render pulls n samples and returns the left channel.
func render(e *Engine, n int) []float64 {
	buf := make([][2]float64, n)
	e.Stream(buf)

	out := make([]float64, n)
	for i, s := range buf {
		out[i] = s[0]
	}
	return out
}

func peak(xs []float64) float64 {
	p := 0.0
	for _, x := range xs {
		if a := math.Abs(x); a > p {
			p = a
		}
	}
	return p
}

func TestSilenceWithNoNotes(t *testing.T) {
	e := testEngine(t, "sin(p)", instrument.DefaultEnvelope())
	if p := peak(render(e, 1024)); p != 0 {
		t.Errorf("idle engine produced %v, want silence", p)
	}
}

func TestStreamAlwaysReportsAlive(t *testing.T) {
	e := testEngine(t, "sin(p)", instrument.DefaultEnvelope())
	buf := make([][2]float64, 64)

	n, ok := e.Stream(buf)
	if !ok {
		t.Error("Stream reported dead, which would stop the speaker")
	}
	if n != len(buf) {
		t.Errorf("filled %d of %d samples", n, len(buf))
	}
}

func TestNoteProducesSound(t *testing.T) {
	e := testEngine(t, "sin(p)", instrument.DefaultEnvelope())
	e.NoteOn(1, 440, 1)

	if p := peak(render(e, testSR/10)); p < 0.1 {
		t.Errorf("peak = %v, want an audible note", p)
	}
}

func TestOutputIsStereoIdentical(t *testing.T) {
	e := testEngine(t, "sin(p)", instrument.DefaultEnvelope())
	e.NoteOn(1, 440, 1)

	buf := make([][2]float64, 512)
	e.Stream(buf)
	for i, s := range buf {
		if s[0] != s[1] {
			t.Fatalf("channels differ at %d: %v vs %v", i, s[0], s[1])
		}
	}
}

// The envelope must be defined in seconds. The old per-sample version reached
// full level in a quarter of a millisecond and clicked.
func TestAttackTakesTheStatedTime(t *testing.T) {
	const attack = 0.05
	e := testEngine(t, "sin(p)", instrument.Envelope{
		Attack: attack, Decay: 0.1, Sustain: 1, Release: 0.1,
	})
	e.NoteOn(1, 440, 1)

	// Halfway through the attack the envelope should be near half.
	render(e, int(math.Round(attack*testSR/2)))
	if lvl := e.Voices()[0].Level; math.Abs(lvl-0.5) > 0.1 {
		t.Errorf("level at half attack = %.3f, want about 0.5", lvl)
	}

	// After the full attack it should be at the top.
	render(e, int(math.Round(attack*testSR/2))+16)
	if lvl := e.Voices()[0].Level; lvl < 0.95 {
		t.Errorf("level after attack = %.3f, want about 1", lvl)
	}
}

// A slow attack must not jump: consecutive samples should stay close.
func TestAttackHasNoDiscontinuity(t *testing.T) {
	e := testEngine(t, "sin(p)", instrument.Envelope{
		Attack: 0.02, Decay: 0.1, Sustain: 1, Release: 0.1,
	})
	e.NoteOn(1, 440, 1)

	out := render(e, 2048)
	if out[0] > 0.05 {
		t.Errorf("first sample = %v, want a near-zero onset", out[0])
	}

	maxStep := 0.0
	for i := 1; i < len(out); i++ {
		if d := math.Abs(out[i] - out[i-1]); d > maxStep {
			maxStep = d
		}
	}
	// At 440 Hz a full-scale sine steps by about 0.063 per sample.
	if maxStep > 0.15 {
		t.Errorf("largest sample-to-sample jump = %v, which would click", maxStep)
	}
}

func TestReleaseFadesToSilence(t *testing.T) {
	e := testEngine(t, "sin(p)", instrument.Envelope{
		Attack: 0.001, Decay: 0.01, Sustain: 1, Release: 0.05,
	})
	e.NoteOn(1, 440, 1)
	render(e, testSR/20)

	e.NoteOff(1)
	render(e, int(0.06*testSR))

	if n := e.ActiveVoices(); n != 0 {
		t.Errorf("%d voices still alive after the release finished", n)
	}
	if p := peak(render(e, 512)); p != 0 {
		t.Errorf("output = %v after release, want silence", p)
	}
}

func TestReleaseIsGradual(t *testing.T) {
	e := testEngine(t, "sin(p)", instrument.Envelope{
		Attack: 0.001, Decay: 0.01, Sustain: 1, Release: 0.2,
	})
	e.NoteOn(1, 440, 1)
	render(e, testSR/20)

	before := e.Voices()[0].Level
	e.NoteOff(1)
	render(e, 64) // a millisecond and a half

	after := e.Voices()[0].Level
	if after >= before {
		t.Errorf("level did not fall after release: %v then %v", before, after)
	}
	if after < before*0.5 {
		t.Errorf("level collapsed from %v to %v, which would click", before, after)
	}
}

func TestDecayReachesSustain(t *testing.T) {
	e := testEngine(t, "sin(p)", instrument.Envelope{
		Attack: 0.001, Decay: 0.05, Sustain: 0.3, Release: 0.1,
	})
	e.NoteOn(1, 440, 1)
	render(e, int(0.08*testSR))

	if lvl := e.Voices()[0].Level; math.Abs(lvl-0.3) > 0.02 {
		t.Errorf("level after decay = %.3f, want the sustain of 0.3", lvl)
	}
}

func TestSustainHolds(t *testing.T) {
	e := testEngine(t, "sin(p)", instrument.Envelope{
		Attack: 0.001, Decay: 0.01, Sustain: 0.5, Release: 0.1,
	})
	e.NoteOn(1, 440, 1)
	render(e, int(0.05*testSR))

	first := e.Voices()[0].Level
	render(e, testSR) // a whole second
	second := e.Voices()[0].Level

	if math.Abs(first-second) > 1e-9 {
		t.Errorf("sustain drifted from %v to %v", first, second)
	}
}

func TestStaccatoDecaysOnItsOwn(t *testing.T) {
	e := testEngine(t, "sin(p)", instrument.Envelope{
		Attack: 0.001, Decay: 0.01, Sustain: 1, Release: 0.1,
	})
	e.NoteOnStaccato(1, 440, 1, 8)

	render(e, int(1.5*testSR))
	if n := e.ActiveVoices(); n != 0 {
		t.Errorf("%d staccato voices still alive, want 0", n)
	}
}

func TestRetriggerReplacesVoice(t *testing.T) {
	e := testEngine(t, "sin(p)", instrument.DefaultEnvelope())
	for range 5 {
		e.NoteOn(1, 440, 1)
	}
	render(e, 64)

	held := 0
	for _, v := range e.Voices() {
		if !v.Releasing {
			held++
		}
	}
	if held != 1 {
		t.Errorf("%d held voices for one key, want 1", held)
	}
}

func TestPolyphonyIsLimited(t *testing.T) {
	e := testEngine(t, "sin(p)", instrument.DefaultEnvelope())
	e.SetMaxVoices(4)

	for i := range 20 {
		e.NoteOn(i, 220+float64(i)*20, 1)
	}
	if n := e.ActiveVoices(); n > 4 {
		t.Errorf("%d voices alive, want at most 4", n)
	}
}

func TestSetMaxVoicesRefusesZero(t *testing.T) {
	e := testEngine(t, "sin(p)", instrument.DefaultEnvelope())
	e.SetMaxVoices(0)
	e.NoteOn(1, 440, 1)

	if n := e.ActiveVoices(); n != 1 {
		t.Errorf("%d voices, want 1 even with a zero limit", n)
	}
}

// A full chord must not clip, which is the whole point of the master limiter.
func TestChordDoesNotClip(t *testing.T) {
	e := testEngine(t, "sin(p)", instrument.Envelope{
		Attack: 0.001, Decay: 0.01, Sustain: 1, Release: 0.1,
	})
	for i := range 16 {
		e.NoteOn(i, 110*math.Pow(2, float64(i)/12), 1)
	}

	out := render(e, testSR/4)
	if p := peak(out); p > 1.0 {
		t.Errorf("a 16-note chord peaked at %v, which clips", p)
	}
	if p := peak(out); p < 0.3 {
		t.Errorf("a 16-note chord peaked at %v, which is too quiet", p)
	}
}

func TestSoftClipIsTransparentWhenQuiet(t *testing.T) {
	for _, x := range []float64{0, 0.1, -0.25, 0.5, -0.7, 0.7} {
		if got := softClip(x); got != x {
			t.Errorf("softClip(%v) = %v, want it untouched", x, got)
		}
	}
}

func TestSoftClipBoundsLoudSignals(t *testing.T) {
	for _, x := range []float64{1, -1, 5, -5, 100, -100} {
		got := softClip(x)
		if math.Abs(got) > 1 {
			t.Errorf("softClip(%v) = %v, which exceeds full scale", x, got)
		}
		if math.Signbit(got) != math.Signbit(x) {
			t.Errorf("softClip(%v) = %v, which flipped sign", x, got)
		}
	}
}

func TestSoftClipIsMonotonic(t *testing.T) {
	prev := softClip(-4)
	for x := -4.0; x <= 4; x += 0.01 {
		got := softClip(x)
		if got < prev-1e-12 {
			t.Fatalf("softClip decreased at x=%v: %v then %v", x, prev, got)
		}
		prev = got
	}
}

// Nothing the expression language can produce should reach the sound card as
// NaN or infinity.
func TestOutputIsAlwaysFinite(t *testing.T) {
	exprs := []string{
		"sin(p)",
		"1/sin(p)",
		"log(sin(p))",
		"tan(p)",
		"sqrt(sin(p))",
		"noise",
		"p^p^p",
	}

	for _, expr := range exprs {
		e := testEngine(t, expr, instrument.DefaultEnvelope())
		e.NoteOn(1, 440, 1)

		for i, v := range render(e, 4096) {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Errorf("%q produced %v at sample %d", expr, v, i)
				break
			}
			if math.Abs(v) > 1 {
				t.Errorf("%q produced %v at sample %d, which exceeds full scale", expr, v, i)
				break
			}
		}
	}
}

func TestPanicSilencesEverything(t *testing.T) {
	e := testEngine(t, "sin(p)", instrument.DefaultEnvelope())
	for i := range 8 {
		e.NoteOn(i, 220+float64(i)*30, 1)
	}
	render(e, 256)

	e.Panic()
	if n := e.ActiveVoices(); n != 0 {
		t.Errorf("%d voices survived a panic", n)
	}
	if p := peak(render(e, 512)); p != 0 {
		t.Errorf("output = %v after a panic, want silence", p)
	}
}

func TestAllNotesOffReleasesRatherThanCuts(t *testing.T) {
	e := testEngine(t, "sin(p)", instrument.Envelope{
		Attack: 0.001, Decay: 0.01, Sustain: 1, Release: 0.3,
	})
	e.NoteOn(1, 440, 1)
	render(e, 2048)

	e.AllNotesOff()
	if n := e.ActiveVoices(); n != 1 {
		t.Errorf("%d voices, want the note to still be fading", n)
	}
	for _, v := range e.Voices() {
		if !v.Releasing {
			t.Error("voice was not marked as releasing")
		}
	}
}

func TestRejectsOutOfRangeFrequencies(t *testing.T) {
	e := testEngine(t, "sin(p)", instrument.DefaultEnvelope())

	e.NoteOn(1, 0, 1)
	e.NoteOn(2, -100, 1)
	e.NoteOn(3, testSR, 1) // above Nyquist, would alias

	if n := e.ActiveVoices(); n != 0 {
		t.Errorf("%d voices from invalid frequencies, want 0", n)
	}
}

func TestVelocityScalesLevel(t *testing.T) {
	loud := testEngine(t, "sin(p)", instrument.Envelope{Attack: 0.001, Decay: 0.01, Sustain: 1, Release: 0.1})
	soft := testEngine(t, "sin(p)", instrument.Envelope{Attack: 0.001, Decay: 0.01, Sustain: 1, Release: 0.1})

	loud.NoteOn(1, 440, 1.0)
	soft.NoteOn(1, 440, 0.25)

	pl := peak(render(loud, testSR/10))
	ps := peak(render(soft, testSR/10))

	if ps >= pl {
		t.Errorf("soft note (%v) was not quieter than loud (%v)", ps, pl)
	}
	if ratio := ps / pl; math.Abs(ratio-0.25) > 0.05 {
		t.Errorf("level ratio = %.3f, want about 0.25", ratio)
	}
}

func TestVelocityIsClamped(t *testing.T) {
	e := testEngine(t, "sin(p)", instrument.DefaultEnvelope())
	e.NoteOn(1, 440, 99)
	e.NoteOn(2, 550, -5)

	if p := peak(render(e, testSR/10)); p > 1 {
		t.Errorf("peak = %v with an absurd velocity", p)
	}
}

// A tabulated instrument and a directly evaluated one must sound the same,
// otherwise the wavetable optimization changes the timbre.
func TestWavetableMatchesDirectEvaluation(t *testing.T) {
	const expr = "sin(p) + 0.5*sin(3p) + 0.25*sin(5p)"

	tabbed := instrument.MustCompile(instrument.Def{
		Name: "tab", Expr: expr,
		Envelope: instrument.Envelope{Attack: 0.0001, Decay: 0.001, Sustain: 1, Release: 0.1},
	})
	if !tabbed.Tabulated() {
		t.Fatal("expected a phase-only instrument to be tabulated")
	}

	// Adding a term that is always zero but reads t defeats tabulation while
	// leaving the sound identical.
	direct := instrument.MustCompile(instrument.Def{
		Name: "direct", Expr: expr + " + 0*t",
		Gain:     tabbed.Normalized(),
		Envelope: instrument.Envelope{Attack: 0.0001, Decay: 0.001, Sustain: 1, Release: 0.1},
	})
	if direct.Tabulated() {
		t.Fatal("expected the time-dependent instrument to skip the table")
	}

	ea := NewEngine(tabbed, testSR)
	eb := NewEngine(direct, testSR)
	ea.NoteOn(1, 440, 1)
	eb.NoteOn(1, 440, 1)

	a := render(ea, 4096)
	b := render(eb, 4096)

	worst := 0.0
	for i := range a {
		if d := math.Abs(a[i] - b[i]); d > worst {
			worst = d
		}
	}
	// Linear interpolation over 4096 points leaves a small error.
	if worst > 0.01 {
		t.Errorf("wavetable differs from direct evaluation by %v", worst)
	}
}

func TestScopeCapturesOutput(t *testing.T) {
	e := testEngine(t, "sin(p)", instrument.DefaultEnvelope())
	e.NoteOn(1, 440, 1)
	render(e, 4096)

	snap := e.Scope().Snapshot()
	if len(snap) == 0 {
		t.Fatal("the scope captured nothing")
	}
	if peak(snap) < 0.1 {
		t.Error("the scope captured silence while a note was sounding")
	}
}

func TestPeakMeterResetsAfterReading(t *testing.T) {
	e := testEngine(t, "sin(p)", instrument.DefaultEnvelope())
	e.NoteOn(1, 440, 1)
	render(e, testSR/10)

	if p := e.Peak(); p <= 0 {
		t.Fatalf("peak = %v while sounding", p)
	}
	e.Panic()
	render(e, 256)

	if p := e.Peak(); p != 0 {
		t.Errorf("peak = %v after silence, want 0", p)
	}
}

// Switching instruments must not cut off a sounding note.
func TestInstrumentSwitchKeepsSoundingVoices(t *testing.T) {
	e := testEngine(t, "sin(p)", instrument.DefaultEnvelope())
	e.NoteOn(1, 440, 1)
	render(e, 1024)

	e.SetInstrument(instrument.MustCompile(instrument.Def{Name: "other", Expr: "saw(x)"}))
	render(e, 1024)

	if n := e.ActiveVoices(); n != 1 {
		t.Errorf("%d voices after an instrument change, want 1", n)
	}
}

func TestVoicesSnapshotIsACopy(t *testing.T) {
	e := testEngine(t, "sin(p)", instrument.DefaultEnvelope())
	e.NoteOn(7, 440, 1)
	render(e, 64)

	snap := e.Voices()
	if len(snap) != 1 || snap[0].Key != 7 {
		t.Fatalf("snapshot = %+v", snap)
	}

	snap[0].Key = 999
	if again := e.Voices(); again[0].Key != 7 {
		t.Error("mutating the snapshot changed engine state")
	}
}

func TestMasterLevelIsClamped(t *testing.T) {
	e := testEngine(t, "sin(p)", instrument.DefaultEnvelope())

	e.SetMaster(-1)
	if g := e.Master(); g != 0 {
		t.Errorf("master = %v, want 0", g)
	}
	e.SetMaster(100)
	if g := e.Master(); g > 1.5 {
		t.Errorf("master = %v, want at most 1.5", g)
	}
}

func TestMasterAtZeroIsSilent(t *testing.T) {
	e := testEngine(t, "sin(p)", instrument.DefaultEnvelope())
	e.SetMaster(0)
	e.NoteOn(1, 440, 1)

	if p := peak(render(e, 2048)); p != 0 {
		t.Errorf("output = %v with the master at zero", p)
	}
}

// The audio callback and the UI run on different goroutines, so every public
// method has to be safe to call while Stream is running. Run with -race.
func TestConcurrentControlIsSafe(t *testing.T) {
	e := testEngine(t, "sin(p) + a*sin(2p)", instrument.DefaultEnvelope())
	other := instrument.MustCompile(instrument.Def{Name: "other", Expr: "tri(x)"})

	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([][2]float64, 512)
		for {
			select {
			case <-stop:
				return
			default:
				e.Stream(buf)
			}
		}
	}()

	for i := range 2000 {
		e.NoteOn(i%12, 220+float64(i%12)*20, 0.8)
		e.NoteOff((i + 6) % 12)
		e.Voices()
		e.ActiveVoices()
		e.Peak()
		e.Scope().Snapshot()
		switch i % 4 {
		case 0:
			e.SetInstrument(other)
		case 1:
			e.SetMaster(0.5)
		case 2:
			e.AllNotesOff()
		case 3:
			e.Panic()
		}
	}

	close(stop)
	wg.Wait()
}

func BenchmarkStreamTabulatedChord(b *testing.B) {
	in := instrument.MustCompile(instrument.Def{
		Name: "bench", Expr: "sum(k, 0, 24, sin((2k+1)*p)/(2k+1))",
	})
	e := NewEngine(in, testSR)
	for i := range 10 {
		e.NoteOn(i, 110*math.Pow(2, float64(i)/12), 1)
	}

	buf := make([][2]float64, 512)
	b.ResetTimer()
	for range b.N {
		e.Stream(buf)
	}
}

func BenchmarkStreamDirectChord(b *testing.B) {
	in := instrument.MustCompile(instrument.Def{
		Name: "bench", Expr: "sum(k, 0, 24, sin((2k+1)*p)/(2k+1)) + 0*t",
	})
	e := NewEngine(in, testSR)
	for i := range 10 {
		e.NoteOn(i, 110*math.Pow(2, float64(i)/12), 1)
	}

	buf := make([][2]float64, 512)
	b.ResetTimer()
	for range b.N {
		e.Stream(buf)
	}
}
