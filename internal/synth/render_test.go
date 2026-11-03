package synth

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/SirSobhan0/piango/internal/dsp"
	"github.com/SirSobhan0/piango/internal/instrument"
)

func renderInst(t *testing.T) *instrument.Instrument {
	t.Helper()
	return instrument.MustCompile(instrument.Def{
		Name: "render",
		Expr: "sin(p)",
		Envelope: instrument.Envelope{
			Attack: 0.005, Decay: 0.05, Sustain: 0.8, Release: 0.1,
		},
	})
}

func TestRenderLengthCoversNotesAndTail(t *testing.T) {
	r := Render{
		Instrument: renderInst(t),
		SampleRate: testSR,
		Notes:      []RenderNote{{Start: 0, Duration: 0.5, Freq: 440, Velocity: 1}},
		Tail:       0.25,
	}

	got := len(r.Samples())
	want := int(0.75 * testSR)
	if math.Abs(float64(got-want)) > 512 {
		t.Errorf("rendered %d samples, want about %d", got, want)
	}
}

func TestRenderIsSilentBeforeAndAfterTheNote(t *testing.T) {
	r := Render{
		Instrument: renderInst(t),
		SampleRate: testSR,
		Notes:      []RenderNote{{Start: 0.2, Duration: 0.2, Freq: 440, Velocity: 1}},
		Tail:       0.3,
	}
	out := r.Samples()

	// Before the note starts.
	if p := peak(out[:int(0.19*testSR)]); p != 0 {
		t.Errorf("output = %v before the note began", p)
	}
	// While it sounds.
	mid := out[int(0.25*testSR):int(0.35*testSR)]
	if p := peak(mid); p < 0.1 {
		t.Errorf("output = %v during the note, want audible", p)
	}
	// Well after the release has finished.
	if p := peak(out[len(out)-1024:]); p > 1e-3 {
		t.Errorf("output = %v after the tail, want silence", p)
	}
}

func TestRenderPitchIsCorrect(t *testing.T) {
	const freq = 440.0
	r := Render{
		Instrument: renderInst(t),
		SampleRate: testSR,
		Notes:      []RenderNote{{Start: 0, Duration: 1, Freq: freq, Velocity: 1}},
	}
	out := r.Samples()

	// Analyze a window from the sustain portion.
	const n = 8192
	window := out[testSR/4 : testSR/4+n]
	spec := dsp.Spectrum(window, dsp.Hann(n))

	best, bestMag := 0, 0.0
	for i, m := range spec {
		if m > bestMag {
			best, bestMag = i, m
		}
	}

	got := dsp.BinFreq(best, n, testSR)
	if math.Abs(got-freq) > testSR/float64(n) {
		t.Errorf("rendered pitch = %.1f Hz, want %.1f Hz", got, freq)
	}
}

func TestRenderHandlesOverlappingNotes(t *testing.T) {
	r := Render{
		Instrument: renderInst(t),
		SampleRate: testSR,
		Notes: []RenderNote{
			{Start: 0, Duration: 0.4, Freq: 261.63, Velocity: 1},
			{Start: 0.1, Duration: 0.4, Freq: 329.63, Velocity: 1},
			{Start: 0.2, Duration: 0.4, Freq: 392.00, Velocity: 1},
		},
		Tail: 0.3,
	}
	out := r.Samples()

	if p := peak(out); p > 1.0 {
		t.Errorf("a chord peaked at %v, which clips", p)
	}
	if p := peak(out); p < 0.2 {
		t.Errorf("a chord peaked at %v, which is too quiet", p)
	}
	for i, v := range out {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("sample %d = %v", i, v)
		}
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	mk := func() Render {
		return Render{
			Instrument: renderInst(t),
			SampleRate: testSR,
			Notes:      []RenderNote{{Start: 0, Duration: 0.2, Freq: 440, Velocity: 1}},
			Tail:       0.1,
		}
	}

	a, b := mk().Samples(), mk().Samples()
	if len(a) != len(b) {
		t.Fatalf("lengths differ: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("renders differ at sample %d: %v vs %v", i, a[i], b[i])
		}
	}
}

func TestRenderWithNoNotesIsEmpty(t *testing.T) {
	r := Render{Instrument: renderInst(t), SampleRate: testSR}
	if got := r.Samples(); len(got) != 0 {
		t.Errorf("got %d samples with no notes", len(got))
	}
}

func TestRenderDefaultsSampleRate(t *testing.T) {
	r := Render{
		Instrument: renderInst(t),
		Notes:      []RenderNote{{Start: 0, Duration: 0.1, Freq: 440, Velocity: 1}},
		Tail:       0.1,
	}
	if got, want := len(r.Samples()), int(0.2*44100); math.Abs(float64(got-want)) > 1024 {
		t.Errorf("got %d samples, want about %d at the default rate", got, want)
	}
}

func TestWriteWAVProducesAPlayableFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.wav")
	r := Render{
		Instrument: renderInst(t),
		SampleRate: testSR,
		Notes:      []RenderNote{{Start: 0, Duration: 0.25, Freq: 440, Velocity: 1}},
		Tail:       0.15,
	}
	if err := r.WriteWAV(path); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 44 {
		t.Fatalf("file is only %d bytes, too small for a WAV header", len(data))
	}

	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		t.Fatalf("not a RIFF/WAVE file: %q %q", data[0:4], data[8:12])
	}

	channels := binary.LittleEndian.Uint16(data[22:24])
	rate := binary.LittleEndian.Uint32(data[24:28])
	bits := binary.LittleEndian.Uint16(data[34:36])

	if channels != 2 {
		t.Errorf("channels = %d, want 2", channels)
	}
	if rate != testSR {
		t.Errorf("sample rate = %d, want %d", rate, testSR)
	}
	if bits != 16 {
		t.Errorf("bit depth = %d, want 16", bits)
	}

	// The payload should be roughly duration * rate * channels * bytes.
	wantBytes := int(0.4 * testSR * 2 * 2)
	if got := len(data) - 44; math.Abs(float64(got-wantBytes)) > 8192 {
		t.Errorf("payload is %d bytes, want about %d", got, wantBytes)
	}
}

func TestWriteWAVRejectsAnEmptyRender(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.wav")
	r := Render{Instrument: renderInst(t), SampleRate: testSR}

	if err := r.WriteWAV(path); err == nil {
		t.Error("expected an error when there is nothing to render")
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("an empty render should not leave a file behind")
	}
}

func TestWriteWAVReportsBadPaths(t *testing.T) {
	r := Render{
		Instrument: renderInst(t),
		SampleRate: testSR,
		Notes:      []RenderNote{{Start: 0, Duration: 0.1, Freq: 440, Velocity: 1}},
	}
	if err := r.WriteWAV(filepath.Join(t.TempDir(), "no", "such", "dir", "x.wav")); err == nil {
		t.Error("expected an error for an unwritable path")
	}
}

// Rendering a built-in must exercise the real instrument set, not just a sine.
func TestRenderEveryBuiltin(t *testing.T) {
	for _, in := range instrument.Builtins() {
		r := Render{
			Instrument: in,
			SampleRate: testSR,
			Notes:      []RenderNote{{Start: 0, Duration: 0.1, Freq: 220, Velocity: 1}},
			Tail:       0.1,
		}
		out := r.Samples()

		if len(out) == 0 {
			t.Errorf("%s rendered nothing", in.Name)
			continue
		}
		if p := peak(out); p < 0.05 {
			t.Errorf("%s rendered at %v, which is inaudible", in.Name, p)
		} else if p > 1.0 {
			t.Errorf("%s rendered at %v, which clips", in.Name, p)
		}
		for i, v := range out {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Errorf("%s produced %v at sample %d", in.Name, v, i)
				break
			}
		}
	}
}
