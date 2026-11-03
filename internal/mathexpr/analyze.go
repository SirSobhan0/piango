package mathexpr

import "math"

// Sampler evaluates a program over one cycle. It is not safe for concurrent
// use; create one per goroutine.
type Sampler struct {
	prog *Program
	vars *Vars
}

// NewSampler binds a program to fresh evaluation state.
func NewSampler(p *Program) *Sampler {
	return &Sampler{prog: p, vars: p.NewVars()}
}

// Vars exposes the underlying variable state so callers can set f, env and
// the a-d parameters before sampling.
func (s *Sampler) Vars() *Vars { return s.vars }

// At evaluates the program at normalized phase x in 0..1, keeping p and x
// consistent with one another.
func (s *Sampler) At(x float64) float64 {
	s.vars.Set(VarX, x)
	s.vars.Set(VarPhase, x*2*math.Pi)
	return s.prog.Eval(s.vars)
}

// Cycle renders n evenly spaced samples of a single cycle.
func (s *Sampler) Cycle(n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = s.At(float64(i) / float64(n))
	}
	return out
}

// Span renders n samples spread over the given number of cycles. Anything that
// is not periodic within one cycle, such as a sub-octave term or an inharmonic
// FM ratio, needs a longer window before its average or peak means anything.
func (s *Sampler) Span(cycles float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		u := cycles * float64(i) / float64(n)
		s.vars.Set(VarX, u-math.Floor(u))
		s.vars.Set(VarPhase, u*2*math.Pi)
		out[i] = s.prog.Eval(s.vars)
	}
	return out
}

// analysisCycles is the window used for level and offset measurements. It is
// wide enough to cover sub-octaves and to average away inharmonic partials.
const analysisCycles = 64

// PeakGain returns the reciprocal of the largest absolute value over the
// analysis window, i.e. the factor that scales the waveform to just touch full
// scale. It returns 1 for a silent waveform.
func PeakGain(p *Program, setup func(*Vars)) float64 {
	s := NewSampler(p)
	defaultSetup(s.vars)
	if setup != nil {
		setup(s.vars)
	}

	peak := 0.0
	for _, v := range s.Span(analysisCycles, 8192) {
		if a := math.Abs(v); a > peak {
			peak = a
		}
	}
	if peak < 1e-9 {
		return 1
	}
	return 1 / peak
}

// RMS measures the root-mean-square level, which tracks perceived loudness far
// better than peak level does.
func RMS(p *Program, setup func(*Vars)) float64 {
	s := NewSampler(p)
	defaultSetup(s.vars)
	if setup != nil {
		setup(s.vars)
	}

	wave := s.Span(analysisCycles, 8192)
	sum := 0.0
	for _, v := range wave {
		sum += v * v
	}
	return math.Sqrt(sum / float64(len(wave)))
}

func defaultSetup(v *Vars) {
	v.Set(VarFreq, 440)
	v.Set(VarEnv, 1)
	v.Set(VarVel, 1)
	v.Set(VarSR, 44100)
	v.Set(VarT, 0)
}

// Harmonics computes the magnitude of the first n harmonics of one cycle by
// direct correlation with sine and cosine. This is the analytic counterpart to
// the live FFT: it shows what an expression contains in theory.
func Harmonics(p *Program, n int, setup func(*Vars)) []float64 {
	const probes = 2048
	s := NewSampler(p)
	defaultSetup(s.vars)
	if setup != nil {
		setup(s.vars)
	}

	wave := s.Cycle(probes)
	out := make([]float64, n)
	for k := 1; k <= n; k++ {
		var re, im float64
		for i, v := range wave {
			ang := 2 * math.Pi * float64(k) * float64(i) / probes
			re += v * math.Cos(ang)
			im += v * math.Sin(ang)
		}
		out[k-1] = 2 * math.Hypot(re, im) / probes
	}
	return out
}

// DCOffset returns the mean level over the analysis window. A nonzero result
// means the waveform is not centered, which wastes headroom and can thump on
// note onset.
func DCOffset(p *Program, setup func(*Vars)) float64 {
	s := NewSampler(p)
	defaultSetup(s.vars)
	if setup != nil {
		setup(s.vars)
	}

	wave := s.Span(analysisCycles, 8192)
	sum := 0.0
	for _, v := range wave {
		sum += v
	}
	return sum / float64(len(wave))
}
