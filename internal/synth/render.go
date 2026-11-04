package synth

import (
	"fmt"
	"os"
	"sort"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/wav"

	"github.com/SirSobhan0/piango/v2/internal/instrument"
)

// Render plays a sequence of notes offline. It drives the same engine as live
// playback, so a rendered file sounds exactly like what you would hear.
type Render struct {
	Instrument *instrument.Instrument
	SampleRate float64
	Notes      []RenderNote
	Tail       float64 // extra seconds recorded after the last release
}

// RenderNote is one note in an offline render, timed in seconds.
type RenderNote struct {
	Start    float64
	Duration float64
	Freq     float64
	Velocity float64
}

type renderEvent struct {
	sample int
	on     bool
	idx    int
}

// Samples renders the sequence to mono samples.
func (r Render) Samples() []float64 {
	if len(r.Notes) == 0 {
		return nil
	}

	sr := r.SampleRate
	if sr <= 0 {
		sr = 44100
	}
	tail := r.Tail
	if tail <= 0 {
		tail = r.Instrument.Envelope.Release + 0.2
	}

	end := 0.0
	for _, n := range r.Notes {
		if t := n.Start + n.Duration; t > end {
			end = t
		}
	}
	total := int((end + tail) * sr)
	if total <= 0 {
		return nil
	}

	events := make([]renderEvent, 0, 2*len(r.Notes))
	for i, n := range r.Notes {
		events = append(events,
			renderEvent{int(n.Start * sr), true, i},
			renderEvent{int((n.Start + n.Duration) * sr), false, i},
		)
	}
	// Off events fire before on events at the same instant, so a repeated
	// pitch releases its old voice first.
	sort.SliceStable(events, func(a, b int) bool {
		if events[a].sample != events[b].sample {
			return events[a].sample < events[b].sample
		}
		return !events[a].on && events[b].on
	})

	e := NewEngine(r.Instrument, sr)
	e.SetMaxVoices(len(r.Notes) + 8)

	out := make([]float64, 0, total)
	buf := make([][2]float64, 512)
	ev := 0

	for pos := 0; pos < total; {
		for ev < len(events) && events[ev].sample <= pos {
			n := r.Notes[events[ev].idx]
			vel := n.Velocity
			if vel <= 0 {
				vel = 1
			}
			if events[ev].on {
				e.NoteOn(events[ev].idx, n.Freq, vel)
			} else {
				e.NoteOff(events[ev].idx)
			}
			ev++
		}

		// Render only as far as the next event, so note timing stays sample
		// accurate instead of snapping to the buffer size.
		chunk := total - pos
		if chunk > len(buf) {
			chunk = len(buf)
		}
		if ev < len(events) {
			if d := events[ev].sample - pos; d > 0 && d < chunk {
				chunk = d
			}
		}

		frame := buf[:chunk]
		e.Stream(frame)
		for _, s := range frame {
			out = append(out, s[0])
		}
		pos += chunk
	}

	return out
}

// WriteWAV renders the sequence and writes it as a 16-bit stereo WAV file.
func (r Render) WriteWAV(path string) error {
	samples := r.Samples()
	if len(samples) == 0 {
		return fmt.Errorf("nothing to render")
	}

	sr := r.SampleRate
	if sr <= 0 {
		sr = 44100
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}

	format := beep.Format{
		SampleRate:  beep.SampleRate(int(sr)),
		NumChannels: 2,
		Precision:   2,
	}
	if err := wav.Encode(f, &sliceStreamer{data: samples}, format); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// sliceStreamer replays finished samples, which is the shape the WAV encoder
// pulls from.
type sliceStreamer struct {
	data []float64
	pos  int
}

func (s *sliceStreamer) Stream(samples [][2]float64) (int, bool) {
	if s.pos >= len(s.data) {
		return 0, false
	}
	i := 0
	for ; i < len(samples) && s.pos < len(s.data); i++ {
		v := s.data[s.pos]
		samples[i][0] = v
		samples[i][1] = v
		s.pos++
	}
	return i, true
}

func (s *sliceStreamer) Err() error { return nil }

var (
	_ beep.Streamer = (*sliceStreamer)(nil)
	_ beep.Streamer = (*Engine)(nil)
)
