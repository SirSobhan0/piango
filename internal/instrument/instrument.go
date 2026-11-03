// Package instrument defines what a playable sound is: a math expression plus
// an envelope and parameters. Instruments come either from the built-in set or
// from a user's JSON config.
package instrument

import (
	"fmt"
	"math"
	"strings"

	"github.com/SirSobhan0/piango/internal/mathexpr"
)

// Envelope is a classic ADSR shape expressed in seconds. Levels are 0..1.
type Envelope struct {
	Attack  float64 `json:"attack"`  // seconds to reach full level
	Decay   float64 `json:"decay"`   // seconds to fall to the sustain level
	Sustain float64 `json:"sustain"` // held level, 0..1
	Release float64 `json:"release"` // seconds to fall silent after release
}

// DefaultEnvelope is a gentle organ-like shape that suits most expressions.
func DefaultEnvelope() Envelope {
	return Envelope{Attack: 0.008, Decay: 0.12, Sustain: 0.75, Release: 0.25}
}

// withDefaults fills in unset fields so a partial JSON envelope still works.
// A zero attack is replaced because an instant jump to full level clicks.
func (e Envelope) withDefaults() Envelope {
	d := DefaultEnvelope()
	if e.Attack <= 0 {
		e.Attack = d.Attack
	}
	if e.Decay < 0 {
		e.Decay = d.Decay
	}
	if e.Sustain <= 0 || e.Sustain > 1 {
		e.Sustain = d.Sustain
	}
	if e.Release <= 0 {
		e.Release = d.Release
	}
	return e
}

// Param is a named knob exposed to the expression as a, b, c or d.
type Param struct {
	Name    string  `json:"name"`
	Value   float64 `json:"value"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Comment string  `json:"comment,omitempty"`
}

// Def is the serializable definition of an instrument, matching the JSON
// schema users write in their config file.
type Def struct {
	Name     string   `json:"name"`
	Expr     string   `json:"expr"`
	Formula  string   `json:"formula,omitempty"` // pretty math notation for display
	About    string   `json:"about,omitempty"`   // what the equation teaches
	Envelope Envelope `json:"envelope"`
	Params   []Param  `json:"params,omitempty"`
	Gain     float64  `json:"gain,omitempty"` // 0 means auto-normalize
}

// Instrument is a compiled, ready-to-play Def.
type Instrument struct {
	Def
	Prog *mathexpr.Program

	// Gain normalizes the waveform to full scale so every instrument is
	// equally loud without hand-tuned constants in the expression.
	gain float64

	// table holds one pre-rendered cycle for phase-only expressions. Sampling
	// a table is far cheaper than interpreting the expression per sample,
	// which is what makes large Fourier sums affordable.
	table []float64
}

// tableSize is the resolution of a pre-rendered cycle. 4096 points keeps
// interpolation error well below the noise floor of 16-bit audio.
const tableSize = 4096

// Compile turns a definition into a playable instrument, validating the
// expression and measuring its level.
func Compile(d Def) (*Instrument, error) {
	if strings.TrimSpace(d.Name) == "" {
		return nil, fmt.Errorf("instrument needs a name")
	}
	if strings.TrimSpace(d.Expr) == "" {
		return nil, fmt.Errorf("instrument %q needs an expr", d.Name)
	}

	prog, err := mathexpr.Compile(d.Expr)
	if err != nil {
		return nil, fmt.Errorf("instrument %q: %w", d.Name, err)
	}

	d.Envelope = d.Envelope.withDefaults()
	inst := &Instrument{Def: d, Prog: prog}

	setup := func(v *mathexpr.Vars) { inst.applyParams(v) }
	if d.Gain > 0 {
		inst.gain = d.Gain
	} else {
		// Target roughly -6 dBFS peak so chords have headroom before the
		// master limiter has to work.
		inst.gain = mathexpr.PeakGain(prog, setup) * 0.5
	}

	if prog.PhaseOnly() {
		inst.buildTable()
	}
	return inst, nil
}

// MustCompile is Compile but panics on error, for the built-in set.
func MustCompile(d Def) *Instrument {
	i, err := Compile(d)
	if err != nil {
		panic(fmt.Sprintf("instrument: %v", err))
	}
	return i
}

func (i *Instrument) applyParams(v *mathexpr.Vars) {
	var vals [4]float64
	for k := range vals {
		if k < len(i.Params) {
			vals[k] = i.Params[k].Value
		}
	}
	v.SetParams(vals[0], vals[1], vals[2], vals[3])
}

func (i *Instrument) buildTable() {
	s := mathexpr.NewSampler(i.Prog)
	v := s.Vars()
	v.Set(mathexpr.VarFreq, 440)
	v.Set(mathexpr.VarEnv, 1)
	v.Set(mathexpr.VarVel, 1)
	v.Set(mathexpr.VarSR, 44100)
	i.applyParams(v)

	i.table = make([]float64, tableSize+1)
	for k := range tableSize {
		i.table[k] = s.At(float64(k) / tableSize)
	}
	// Duplicate the first point so interpolation can read table[k+1] freely.
	i.table[tableSize] = i.table[0]
}

// Tabulated reports whether this instrument plays from a pre-rendered cycle.
func (i *Instrument) Tabulated() bool { return i.table != nil }

// Normalized returns the normalization factor applied to the raw expression
// output.
func (i *Instrument) Normalized() float64 { return i.gain }

// Table exposes the pre-rendered cycle, or nil. The caller must not modify it.
func (i *Instrument) Table() []float64 { return i.table }

// Wave renders one normalized cycle at n points, for the waveform display.
func (i *Instrument) Wave(n int) []float64 {
	out := make([]float64, n)

	if i.table != nil {
		for k := range out {
			out[k] = i.table[k*tableSize/n] * i.gain
		}
		return out
	}

	s := mathexpr.NewSampler(i.Prog)
	v := s.Vars()
	v.Set(mathexpr.VarFreq, 440)
	v.Set(mathexpr.VarEnv, 1)
	v.Set(mathexpr.VarVel, 1)
	v.Set(mathexpr.VarSR, 44100)
	i.applyParams(v)

	for k := range out {
		out[k] = s.At(float64(k)/float64(n)) * i.gain
	}
	return out
}

// Harmonics returns the magnitudes of the first n harmonics, normalized so the
// strongest reads 1. This is the theoretical spectrum of the equation.
func (i *Instrument) Harmonics(n int) []float64 {
	h := mathexpr.Harmonics(i.Prog, n, func(v *mathexpr.Vars) { i.applyParams(v) })
	peak := 0.0
	for _, v := range h {
		if v > peak {
			peak = v
		}
	}
	if peak < 1e-12 {
		return h
	}
	for k := range h {
		h[k] /= peak
	}
	return h
}

// Display returns the formula to show, preferring the pretty form.
func (i *Instrument) Display() string {
	if i.Formula != "" {
		return i.Formula
	}
	return i.Expr
}

// SetParam updates parameter idx, clamping to its declared range and
// re-rendering the wavetable so the change is audible immediately.
func (i *Instrument) SetParam(idx int, val float64) {
	if idx < 0 || idx >= len(i.Params) {
		return
	}
	p := &i.Params[idx]
	if p.Min < p.Max {
		val = math.Max(p.Min, math.Min(p.Max, val))
	}
	p.Value = val

	if i.Prog.PhaseOnly() {
		i.buildTable()
	}
	if i.Def.Gain == 0 {
		i.gain = mathexpr.PeakGain(i.Prog, func(v *mathexpr.Vars) { i.applyParams(v) }) * 0.5
	}
}

// Params4 returns the four parameter values in slot order.
func (i *Instrument) Params4() [4]float64 {
	var out [4]float64
	for k := range out {
		if k < len(i.Params) {
			out[k] = i.Params[k].Value
		}
	}
	return out
}

// Clone returns a deep copy, so edits to one instrument never leak into
// another that shares a slice backing array.
func (i *Instrument) Clone() *Instrument {
	c := *i
	c.Params = append([]Param(nil), i.Params...)
	if i.table != nil {
		c.table = append([]float64(nil), i.table...)
	}
	return &c
}
