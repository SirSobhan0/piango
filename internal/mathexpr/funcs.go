package mathexpr

import (
	"math"
	"math/rand/v2"
)

// fnDef describes a builtin callable. Exactly one of f1/f2/f3 is set.
type fnDef struct {
	name  string
	arity int
	doc   string
	f1    func(float64) float64
	f2    func(a, b float64) float64
	f3    func(a, b, c float64) float64
}

func sign(x float64) float64 {
	switch {
	case x > 0:
		return 1
	case x < 0:
		return -1
	}
	return 0
}

func frac(x float64) float64 { return x - math.Floor(x) }

// saw is a naive (aliasing) sawtooth over a 0..1 input, ramping -1..1.
func saw(x float64) float64 { return 2*frac(x) - 1 }

// tri is a naive triangle over a 0..1 input.
func tri(x float64) float64 { return 2*math.Abs(2*frac(x)-1) - 1 }

// sqr is a naive square over a 0..1 input.
func sqr(x float64) float64 {
	if frac(x) < 0.5 {
		return 1
	}
	return -1
}

// pulse is a naive variable-width pulse over a 0..1 input.
func pulse(x, width float64) float64 {
	if frac(x) < width {
		return 1
	}
	return -1
}

func sinc(x float64) float64 {
	if x == 0 {
		return 1
	}
	return math.Sin(math.Pi*x) / (math.Pi * x)
}

func clamp(x, lo, hi float64) float64 {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

func lerp(a, b, tv float64) float64 { return a + (b-a)*tv }

func step(edge, x float64) float64 {
	if x < edge {
		return 0
	}
	return 1
}

// quantize rounds x to a grid of `steps` divisions, the core of bitcrushing.
func quantize(x, steps float64) float64 {
	if steps < 1 {
		steps = 1
	}
	return math.Round(x*steps) / steps
}

// cheb evaluates the Chebyshev polynomial T_n(x) by recurrence. Feeding it a
// sine produces exactly the nth harmonic, which is why it is handy for
// waveshaping demos.
func cheb(n, x float64) float64 {
	k := int(math.Round(n))
	if k < 0 {
		k = -k
	}
	switch k {
	case 0:
		return 1
	case 1:
		return x
	}
	prev, cur := 1.0, x
	for i := 2; i <= k; i++ {
		prev, cur = cur, 2*x*cur-prev
	}
	return cur
}

// gauss is a unit-height Gaussian bell, useful for formant-ish spectra.
func gauss(x float64) float64 { return math.Exp(-x * x) }

func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

// logSafe avoids -Inf leaking into the audio path.
func logSafe(x float64) float64 {
	if x <= 0 {
		return 0
	}
	return math.Log(x)
}

var builtins = []fnDef{
	{name: "sin", arity: 1, f1: math.Sin, doc: "sine"},
	{name: "cos", arity: 1, f1: math.Cos, doc: "cosine"},
	{name: "tan", arity: 1, f1: math.Tan, doc: "tangent"},
	{name: "asin", arity: 1, f1: math.Asin, doc: "inverse sine"},
	{name: "acos", arity: 1, f1: math.Acos, doc: "inverse cosine"},
	{name: "atan", arity: 1, f1: math.Atan, doc: "inverse tangent"},
	{name: "sinh", arity: 1, f1: math.Sinh, doc: "hyperbolic sine"},
	{name: "cosh", arity: 1, f1: math.Cosh, doc: "hyperbolic cosine"},
	{name: "tanh", arity: 1, f1: math.Tanh, doc: "hyperbolic tangent, a smooth saturator"},
	{name: "abs", arity: 1, f1: math.Abs, doc: "absolute value"},
	{name: "sign", arity: 1, f1: sign, doc: "-1, 0 or 1"},
	{name: "floor", arity: 1, f1: math.Floor, doc: "round down"},
	{name: "ceil", arity: 1, f1: math.Ceil, doc: "round up"},
	{name: "round", arity: 1, f1: math.Round, doc: "round to nearest"},
	{name: "trunc", arity: 1, f1: math.Trunc, doc: "drop the fraction"},
	{name: "frac", arity: 1, f1: frac, doc: "fractional part in 0..1"},
	{name: "sqrt", arity: 1, f1: math.Sqrt, doc: "square root"},
	{name: "exp", arity: 1, f1: math.Exp, doc: "e^x"},
	{name: "log", arity: 1, f1: logSafe, doc: "natural log (0 for x<=0)"},
	{name: "log2", arity: 1, f1: math.Log2, doc: "base-2 log"},
	{name: "log10", arity: 1, f1: math.Log10, doc: "base-10 log"},
	{name: "saw", arity: 1, f1: saw, doc: "naive sawtooth of a 0..1 input"},
	{name: "tri", arity: 1, f1: tri, doc: "naive triangle of a 0..1 input"},
	{name: "sqr", arity: 1, f1: sqr, doc: "naive square of a 0..1 input"},
	{name: "sinc", arity: 1, f1: sinc, doc: "sin(pi x)/(pi x)"},
	{name: "gauss", arity: 1, f1: gauss, doc: "e^(-x^2) bell"},

	{name: "pow", arity: 2, f2: math.Pow, doc: "a to the power b"},
	{name: "mod", arity: 2, f2: math.Mod, doc: "floating point remainder"},
	{name: "min", arity: 2, f2: math.Min, doc: "smaller of two"},
	{name: "max", arity: 2, f2: math.Max, doc: "larger of two"},
	{name: "atan2", arity: 2, f2: math.Atan2, doc: "angle of (y, x)"},
	{name: "step", arity: 2, f2: step, doc: "0 below edge, 1 at or above"},
	{name: "pulse", arity: 2, f2: pulse, doc: "naive pulse of 0..1 input with width"},
	{name: "quant", arity: 2, f2: quantize, doc: "quantize to N steps (bitcrush)"},
	{name: "cheb", arity: 2, f2: cheb, doc: "Chebyshev T_n(x): makes the nth harmonic"},
	{name: "hypot", arity: 2, f2: math.Hypot, doc: "sqrt(a^2+b^2)"},

	{name: "clamp", arity: 3, f3: clamp, doc: "limit x to lo..hi"},
	{name: "lerp", arity: 3, f3: lerp, doc: "blend a to b by t"},
	{name: "if", arity: 3, f3: func(c, a, b float64) float64 {
		if c != 0 {
			return a
		}
		return b
	}, doc: "if(cond, a, b)"},
}

var builtinIndex = func() map[string]int {
	m := make(map[string]int, len(builtins))
	for i, f := range builtins {
		m[f.name] = i
	}
	return m
}()

// Functions returns the builtin function names and their documentation,
// for display in help screens.
func Functions() []struct{ Name, Doc string } {
	out := make([]struct{ Name, Doc string }, 0, len(builtins))
	for _, f := range builtins {
		out = append(out, struct{ Name, Doc string }{f.name, f.doc})
	}
	return out
}

// namedConstants are folded straight into the compiled program.
var namedConstants = map[string]float64{
	"pi":  math.Pi,
	"tau": 2 * math.Pi,
	"e":   math.E,
	"phi": math.Phi,
	"inf": math.Inf(1),
}

func whiteNoise() float64 { return rand.Float64()*2 - 1 }
func uniform() float64    { return rand.Float64() }
