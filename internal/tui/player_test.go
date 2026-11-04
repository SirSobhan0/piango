package tui

import (
	"testing"
	"time"

	"github.com/SirSobhan0/piango/v2/internal/instrument"
	"github.com/SirSobhan0/piango/v2/internal/synth"
	"github.com/SirSobhan0/piango/v2/internal/tuning"
)

// testPlayer returns a player driven by a manually advanced clock, so the
// hold-detection logic can be tested without sleeping.
func testPlayer(t *testing.T) (*Player, *synth.Engine, *time.Time) {
	t.Helper()

	inst := instrument.MustCompile(instrument.Def{Name: "test", Expr: "sin(p)"})
	e := synth.NewEngine(inst, 44100)
	p := NewPlayer(e, NewKeyboard(tuning.Standard()))

	clock := time.Now()
	p.now = func() time.Time { return clock }
	return p, e, &clock
}

func TestPressStartsANote(t *testing.T) {
	p, e, _ := testPlayer(t)

	if !p.Press("a", false) {
		t.Fatal("Press rejected a valid note key")
	}
	if e.ActiveVoices() != 1 {
		t.Errorf("got %d voices, want 1", e.ActiveVoices())
	}
	if !p.Down("a") {
		t.Error("the key should register as held")
	}
}

func TestPressIgnoresNonNoteKeys(t *testing.T) {
	p, e, _ := testPlayer(t)

	if p.Press("1", false) {
		t.Error(`"1" should not be treated as a note`)
	}
	if e.ActiveVoices() != 0 {
		t.Errorf("a non-note key started %d voices", e.ActiveVoices())
	}
}

func TestAutoRepeatDoesNotRetrigger(t *testing.T) {
	p, e, clock := testPlayer(t)
	p.Press("a", false)

	// Simulate the terminal's key-repeat stream.
	for range 10 {
		*clock = clock.Add(30 * time.Millisecond)
		p.Press("a", false)
		p.Tick()
	}

	if got := e.ActiveVoices(); got != 1 {
		t.Errorf("auto-repeat produced %d voices, want 1 sustained note", got)
	}
	if !p.Down("a") {
		t.Error("the key should still be held during auto-repeat")
	}
}

func TestSilenceReleasesTheNote(t *testing.T) {
	p, e, clock := testPlayer(t)
	p.Press("a", false)

	*clock = clock.Add(holdTimeout + 10*time.Millisecond)
	p.Tick()

	if p.Down("a") {
		t.Error("the key should have been released after the repeats stopped")
	}
	// The voice enters its release stage rather than vanishing instantly.
	voices := e.Voices()
	if len(voices) != 1 || !voices[0].Releasing {
		t.Errorf("voices after timeout = %+v, want one releasing voice", voices)
	}
}

func TestStaccatoReleasesSooner(t *testing.T) {
	p, _, clock := testPlayer(t)
	p.Press("a", true)

	*clock = clock.Add(staccatoTimeout + 10*time.Millisecond)
	p.Tick()

	if p.Down("a") {
		t.Error("a staccato note should release on the shorter timeout")
	}
}

func TestSustainedNoteOutlivesTheStaccatoTimeout(t *testing.T) {
	p, _, clock := testPlayer(t)
	p.Press("a", false)

	*clock = clock.Add(staccatoTimeout + 10*time.Millisecond)
	p.Tick()

	if !p.Down("a") {
		t.Error("a sustained note was cut off at the staccato timeout")
	}
}

func TestDeliberateRetriggerRestartsTheNote(t *testing.T) {
	p, e, clock := testPlayer(t)
	p.Press("a", false)

	// A gap longer than the repeat interval but shorter than the hold
	// timeout means the player struck the key again.
	*clock = clock.Add(retriggerGap + 5*time.Millisecond)
	p.Press("a", false)

	// The old voice is released and a new one starts, so both exist briefly.
	if got := e.ActiveVoices(); got < 1 {
		t.Errorf("got %d voices after a retrigger, want at least the new one", got)
	}

	var sustained int
	for _, v := range e.Voices() {
		if !v.Releasing {
			sustained++
		}
	}
	if sustained != 1 {
		t.Errorf("got %d sustained voices after a retrigger, want 1", sustained)
	}
}

func TestChordHoldsEveryKey(t *testing.T) {
	p, e, clock := testPlayer(t)

	for _, k := range []string{"a", "d", "g"} {
		p.Press(k, false)
	}
	if e.ActiveVoices() != 3 {
		t.Fatalf("got %d voices, want a 3-note chord", e.ActiveVoices())
	}

	// Release only the middle key by letting its repeats lapse.
	for range 6 {
		*clock = clock.Add(30 * time.Millisecond)
		p.Press("a", false)
		p.Press("g", false)
		p.Tick()
	}
	*clock = clock.Add(holdTimeout)
	p.Press("a", false)
	p.Press("g", false)
	p.Tick()

	if p.Down("d") {
		t.Error("the un-repeated key should have been released")
	}
	if !p.Down("a") || !p.Down("g") {
		t.Error("the repeated keys should still be held")
	}
}

func TestReleaseAllClearsEverything(t *testing.T) {
	p, e, _ := testPlayer(t)
	for _, k := range []string{"a", "s", "d", "f"} {
		p.Press(k, false)
	}

	p.ReleaseAll()

	if p.HeldCount() != 0 {
		t.Errorf("%d keys still held after ReleaseAll", p.HeldCount())
	}
	if e.ActiveVoices() != 0 {
		t.Errorf("%d voices still sounding after ReleaseAll", e.ActiveVoices())
	}
}

func TestHeldReportsSoundingFrequencies(t *testing.T) {
	p, _, _ := testPlayer(t)
	p.Press("a", false)
	p.Press("g", false)

	got := p.Held()
	if len(got) != 2 {
		t.Fatalf("Held returned %d frequencies, want 2", len(got))
	}
	for _, f := range got {
		if f < 20 || f > 20000 {
			t.Errorf("held frequency %.2f is out of the audible range", f)
		}
	}
}

func TestOctaveShiftDoesNotOrphanHeldNotes(t *testing.T) {
	p, e, clock := testPlayer(t)
	p.Press("a", false)

	// Shifting octaves changes the key's ID; the held entry must still be
	// the one that gets released, or the note would hang forever.
	p.board.ShiftOctave(1)

	*clock = clock.Add(holdTimeout + 10*time.Millisecond)
	p.Tick()

	if p.Down("a") {
		t.Error("the key is still marked held after the timeout")
	}
	for _, v := range e.Voices() {
		if !v.Releasing {
			t.Errorf("voice %+v survived the release; an octave shift orphaned it", v)
		}
	}
}

func TestUppercasePressTracksTheSameKey(t *testing.T) {
	p, _, _ := testPlayer(t)
	p.Press("A", true)

	if !p.Down("a") {
		t.Error("a shifted press should register under the lowercase key")
	}
	if p.HeldCount() != 1 {
		t.Errorf("got %d held keys, want 1", p.HeldCount())
	}
}
