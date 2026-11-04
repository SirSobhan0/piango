// Package synth turns instruments into sound. It owns voice allocation, the
// time-based ADSR envelope, and the master bus that mixes and limits the
// result before it reaches the sound card.
package synth

import (
	"math"
	"sync"

	"github.com/SirSobhan0/piango/v2/internal/dsp"
	"github.com/SirSobhan0/piango/v2/internal/instrument"
	"github.com/SirSobhan0/piango/v2/internal/mathexpr"
)

// stage tracks where a voice is in its envelope.
type stage int

const (
	stageAttack stage = iota
	stageDecay
	stageSustain
	stageRelease
	stageDone
)

// Voice renders one note. Each voice keeps its own evaluation state so voices
// never share mutable memory with one another.
type Voice struct {
	inst *instrument.Instrument
	vars *mathexpr.Vars

	id    int64
	key   int     // caller's note identifier, used to find the voice again
	freq  float64 // Hz
	vel   float64 // 0..1
	decay float64 // extra decay rate for staccato, 0 means use the envelope

	phase    float64 // radians, wrapped to 0..2pi
	phaseInc float64 // radians per sample
	elapsed  float64 // seconds since note-on
	sr       float64

	stage stage
	env   float64 // current envelope level
	relFr float64 // envelope level at the moment of release

	table []float64 // nil when the instrument must be evaluated per sample
	gain  float64
}

// newVoice prepares a voice to start sounding immediately.
func newVoice(in *instrument.Instrument, key int, freq, vel, sr float64) *Voice {
	v := &Voice{
		inst:     in,
		vars:     in.Prog.NewVars(),
		key:      key,
		freq:     freq,
		vel:      clamp01(vel),
		phaseInc: 2 * math.Pi * freq / sr,
		sr:       sr,
		table:    in.Table(),
		gain:     in.Normalized(),
		stage:    stageAttack,
	}

	p := in.Params4()
	v.vars.SetParams(p[0], p[1], p[2], p[3])
	v.vars.Set(mathexpr.VarFreq, freq)
	v.vars.Set(mathexpr.VarVel, v.vel)
	v.vars.Set(mathexpr.VarSR, sr)
	return v
}

// Key reports the note identifier this voice was started with.
func (v *Voice) Key() int { return v.key }

// Freq reports the voice's fundamental in Hz.
func (v *Voice) Freq() float64 { return v.freq }

// Active reports whether the voice still contributes sound.
func (v *Voice) Active() bool { return v.stage != stageDone }

// Releasing reports whether the key has been let go.
func (v *Voice) Releasing() bool { return v.stage >= stageRelease }

// Level reports the current envelope level, for meters and displays.
func (v *Voice) Level() float64 { return v.env }

// release starts the release stage from wherever the envelope currently is,
// which avoids the click you get from jumping to a fixed level.
func (v *Voice) release() {
	if v.stage >= stageRelease {
		return
	}
	v.stage = stageRelease
	v.relFr = v.env
	v.elapsed = 0
}

// sample advances the voice one sample and returns its contribution.
func (v *Voice) sample() float64 {
	if v.stage == stageDone {
		return 0
	}

	v.advanceEnvelope()
	if v.stage == stageDone {
		return 0
	}

	var raw float64
	if v.table != nil {
		raw = v.lookup()
	} else {
		v.vars.Set(mathexpr.VarPhase, v.phase)
		v.vars.Set(mathexpr.VarX, v.phase/(2*math.Pi))
		v.vars.Set(mathexpr.VarT, v.elapsed)
		v.vars.Set(mathexpr.VarEnv, v.env)
		raw = v.inst.Prog.Eval(v.vars)
	}

	v.phase += v.phaseInc
	if v.phase >= 2*math.Pi {
		v.phase -= 2 * math.Pi
	}

	return raw * v.gain * v.env * v.vel
}

// lookup reads the pre-rendered wavetable with linear interpolation. Without
// interpolation the table's steps would add their own harmonics.
func (v *Voice) lookup() float64 {
	n := len(v.table) - 1 // the table repeats its first point at the end
	pos := v.phase / (2 * math.Pi) * float64(n)
	i := int(pos)
	if i >= n {
		i = n - 1
	}
	frac := pos - float64(i)
	return v.table[i] + frac*(v.table[i+1]-v.table[i])
}

// advanceEnvelope moves the ADSR forward by one sample. Every stage is defined
// in seconds, so the envelope sounds identical at any sample rate.
func (v *Voice) advanceEnvelope() {
	dt := 1 / v.sr
	v.elapsed += dt

	e := v.inst.Envelope
	switch v.stage {
	case stageAttack:
		if e.Attack <= 0 {
			v.env = 1
			v.stage = stageDecay
			v.elapsed = 0
			break
		}
		v.env = v.elapsed / e.Attack
		if v.env >= 1 {
			v.env = 1
			v.stage = stageDecay
			v.elapsed = 0
		}

	case stageDecay:
		if e.Decay <= 0 {
			v.env = e.Sustain
			v.stage = stageSustain
			break
		}
		// Exponential decay toward sustain, which is how real bodies lose
		// energy and what the ear expects.
		v.env = e.Sustain + (1-e.Sustain)*math.Exp(-3*v.elapsed/e.Decay)
		if v.elapsed >= e.Decay {
			v.env = e.Sustain
			v.stage = stageSustain
		}

	case stageSustain:
		v.env = e.Sustain
		if v.decay > 0 {
			// Staccato: keep falling instead of holding.
			v.env *= math.Exp(-v.decay * v.elapsed)
			if v.env < silenceFloor {
				v.stage = stageDone
				v.env = 0
			}
		}

	case stageRelease:
		if e.Release <= 0 {
			v.stage = stageDone
			v.env = 0
			break
		}
		v.env = v.relFr * math.Exp(-5*v.elapsed/e.Release)
		if v.env < silenceFloor || v.elapsed >= e.Release {
			v.stage = stageDone
			v.env = 0
		}
	}
}

// silenceFloor is roughly -80 dB, below which a voice is inaudible and can be
// freed.
const silenceFloor = 1e-4

func clamp01(x float64) float64 {
	switch {
	case x < 0:
		return 0
	case x > 1:
		return 1
	default:
		return x
	}
}

// Engine mixes voices into a stereo stream. It implements beep.Streamer, and
// all of its state lives behind one mutex so the audio callback and the UI
// goroutine cannot race.
type Engine struct {
	mu        sync.Mutex
	voices    []*Voice
	inst      *instrument.Instrument
	sr        float64
	maxVoices int
	master    float64
	nextID    int64

	scope *dsp.Ring
	peak  float64
}

// NewEngine builds an engine for the given sample rate and starting
// instrument.
func NewEngine(in *instrument.Instrument, sampleRate float64) *Engine {
	return &Engine{
		inst:      in,
		sr:        sampleRate,
		maxVoices: 24,
		master:    0.8,
		scope:     dsp.NewRing(4096),
	}
}

// SampleRate reports the engine's sample rate in Hz.
func (e *Engine) SampleRate() float64 { return e.sr }

// Scope exposes the ring buffer of recent output for the visualizers.
func (e *Engine) Scope() *dsp.Ring { return e.scope }

// SetInstrument changes the instrument used for new notes. Sounding voices
// keep their original instrument so switching never cuts a note off.
func (e *Engine) SetInstrument(in *instrument.Instrument) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.inst = in
}

// Instrument returns the instrument that new notes will use.
func (e *Engine) Instrument() *instrument.Instrument {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.inst
}

// SetMaster sets the output level, clamped to a sane range.
func (e *Engine) SetMaster(g float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.master = math.Max(0, math.Min(1.5, g))
}

// Master returns the output level.
func (e *Engine) Master() float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.master
}

// SetMaxVoices limits polyphony. At least one voice is always allowed.
func (e *Engine) SetMaxVoices(n int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if n < 1 {
		n = 1
	}
	e.maxVoices = n
}

// NoteOn starts a note. Retriggering the same key replaces the previous voice
// so a held key cannot stack up into distortion.
func (e *Engine) NoteOn(key int, freq, vel float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.noteOnLocked(key, freq, vel, 0)
}

// NoteOnStaccato starts a note that decays on its own at the given rate in
// nepers per second, for keyboards that cannot report key release.
func (e *Engine) NoteOnStaccato(key int, freq, vel, decayRate float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.noteOnLocked(key, freq, vel, decayRate)
}

func (e *Engine) noteOnLocked(key int, freq, vel, decayRate float64) {
	if freq <= 0 || freq >= e.sr/2 {
		return // outside the audible or representable range
	}

	e.releaseKeyLocked(key)
	e.makeRoomLocked()

	v := newVoice(e.inst, key, freq, vel, e.sr)
	v.decay = decayRate
	e.nextID++
	v.id = e.nextID
	e.voices = append(e.voices, v)
}

// makeRoomLocked steals a voice when polyphony is exhausted, preferring one
// that is already releasing, then the quietest, then the oldest.
func (e *Engine) makeRoomLocked() {
	if len(e.voices) < e.maxVoices {
		return
	}

	best, bestScore := -1, math.Inf(1)
	for i, v := range e.voices {
		score := v.env
		if v.Releasing() {
			score -= 10 // strongly prefer stealing a released note
		}
		score += float64(v.id) * 1e-9 // break ties toward the oldest
		if score < bestScore {
			best, bestScore = i, score
		}
	}
	if best >= 0 {
		e.voices = append(e.voices[:best], e.voices[best+1:]...)
	}
}

// NoteOff releases every voice playing the given key.
func (e *Engine) NoteOff(key int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.releaseKeyLocked(key)
}

func (e *Engine) releaseKeyLocked(key int) {
	for _, v := range e.voices {
		if v.key == key && !v.Releasing() {
			v.release()
		}
	}
}

// AllNotesOff releases everything, which is the gentle version of panic.
func (e *Engine) AllNotesOff() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, v := range e.voices {
		v.release()
	}
}

// Panic drops every voice immediately. Use it when something has gone wrong
// and silence matters more than a clean tail.
func (e *Engine) Panic() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.voices = e.voices[:0]
	e.peak = 0
}

// ActiveVoices reports how many voices are sounding.
func (e *Engine) ActiveVoices() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.voices)
}

// VoiceInfo is a snapshot of a sounding voice for display purposes.
type VoiceInfo struct {
	Key       int
	Freq      float64
	Level     float64
	Releasing bool
}

// Voices returns a snapshot of the sounding voices. The returned slice is a
// copy, so the caller may hold it without blocking audio.
func (e *Engine) Voices() []VoiceInfo {
	e.mu.Lock()
	defer e.mu.Unlock()

	out := make([]VoiceInfo, 0, len(e.voices))
	for _, v := range e.voices {
		out = append(out, VoiceInfo{
			Key:       v.key,
			Freq:      v.freq,
			Level:     v.env,
			Releasing: v.Releasing(),
		})
	}
	return out
}

// Peak returns the loudest sample seen since the last call, for a level meter.
func (e *Engine) Peak() float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	p := e.peak
	e.peak = 0
	return p
}

// Stream fills the buffer with mixed audio. It satisfies beep.Streamer and
// always reports itself as alive so the speaker keeps pulling even when no
// note is sounding.
func (e *Engine) Stream(samples [][2]float64) (int, bool) {
	e.mu.Lock()

	mono := make([]float64, len(samples))
	for i := range samples {
		sum := 0.0
		for _, v := range e.voices {
			sum += v.sample()
		}

		out := softClip(sum * e.master)
		mono[i] = out
		samples[i][0] = out
		samples[i][1] = out

		if a := math.Abs(out); a > e.peak {
			e.peak = a
		}
	}

	e.reapLocked()
	e.mu.Unlock()

	e.scope.Write(mono)
	return len(samples), true
}

// Err satisfies beep.Streamer. The engine has no failure mode that should stop
// playback.
func (e *Engine) Err() error { return nil }

// reapLocked drops finished voices, preserving order so voice stealing stays
// predictable.
func (e *Engine) reapLocked() {
	live := e.voices[:0]
	for _, v := range e.voices {
		if v.Active() {
			live = append(live, v)
		}
	}
	// Clear the tail so finished voices can be collected.
	for i := len(live); i < len(e.voices); i++ {
		e.voices[i] = nil
	}
	e.voices = live
}

// softClip keeps the mix inside full scale without the harsh harmonics of hard
// clipping. Below the threshold it is exactly unity, so quiet playing is
// untouched.
func softClip(x float64) float64 {
	const threshold = 0.7
	a := math.Abs(x)
	if a <= threshold {
		return x
	}
	// tanh-shaped knee above the threshold, asymptotic to 1.
	over := a - threshold
	shaped := threshold + (1-threshold)*math.Tanh(over/(1-threshold))
	return math.Copysign(shaped, x)
}
