package mathexpr

import (
	"math"
	"testing"
)

func evalAt(t *testing.T, src string, setup func(*Vars)) float64 {
	t.Helper()
	p, err := Compile(src)
	if err != nil {
		t.Fatalf("Compile(%q) failed: %v", src, err)
	}
	v := p.NewVars()
	if setup != nil {
		setup(v)
	}
	return p.Eval(v)
}

func TestArithmeticAndPrecedence(t *testing.T) {
	cases := []struct {
		src  string
		want float64
	}{
		{"1+2*3", 7},
		{"(1+2)*3", 9},
		{"2^3^2", 512}, // right associative
		{"-2^2", -4},   // unary minus binds looser than power
		{"2^-1", 0.5},
		{"10-3-2", 5}, // left associative
		{"7%3", 1},
		{"1/0", 0}, // division guard
		{"2 < 3", 1},
		{"3 <= 3", 1},
		{"2 == 3", 0},
		{"2 != 3", 1},
		{"1 && 0", 0},
		{"1 || 0", 1},
		{"!0", 1},
		{"1 ? 5 : 9", 5},
		{"0 ? 5 : 9", 9},
		{"pi", math.Pi},
		{"tau", 2 * math.Pi},
	}

	for _, tc := range cases {
		if got := evalAt(t, tc.src, nil); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%s = %v, want %v", tc.src, got, tc.want)
		}
	}
}

func TestImplicitMultiplication(t *testing.T) {
	setup := func(v *Vars) { v.Set(VarPhase, 2) }

	cases := []struct {
		src  string
		want float64
	}{
		{"2p", 4},
		{"3(1+1)", 6},
		{"2p+1", 5},
		{"2sin(0)", 0},
		{"2 p 2", 8},
	}

	for _, tc := range cases {
		if got := evalAt(t, tc.src, setup); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%s = %v, want %v", tc.src, got, tc.want)
		}
	}
}

func TestBuiltinFunctions(t *testing.T) {
	cases := []struct {
		src  string
		want float64
	}{
		{"sin(0)", 0},
		{"cos(0)", 1},
		{"abs(-3)", 3},
		{"sign(-2)", -1},
		{"floor(2.7)", 2},
		{"ceil(2.1)", 3},
		{"frac(2.25)", 0.25},
		{"clamp(5, 0, 1)", 1},
		{"lerp(0, 10, 0.25)", 2.5},
		{"min(2, 3)", 2},
		{"max(2, 3)", 3},
		{"quant(0.37, 4)", 0.25},
		{"step(1, 0.5)", 0},
		{"if(1, 2, 3)", 2},
		{"log(0)", 0}, // guarded against -Inf
		{"sinc(0)", 1},
	}

	for _, tc := range cases {
		if got := evalAt(t, tc.src, nil); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%s = %v, want %v", tc.src, got, tc.want)
		}
	}
}

// Chebyshev polynomials fed a cosine must reproduce the nth harmonic exactly:
// T_n(cos θ) = cos(nθ). This is the identity the waveshaping instruments rely on.
func TestChebyshevProducesNthHarmonic(t *testing.T) {
	for n := 1; n <= 6; n++ {
		for _, theta := range []float64{0.1, 0.7, 1.3, 2.9, 4.4} {
			got := cheb(float64(n), math.Cos(theta))
			want := math.Cos(float64(n) * theta)
			if math.Abs(got-want) > 1e-9 {
				t.Errorf("T_%d(cos %v) = %v, want %v", n, theta, got, want)
			}
		}
	}
}

func TestSumAndProduct(t *testing.T) {
	if got := evalAt(t, "sum(k, 1, 4, k)", nil); got != 10 {
		t.Errorf("sum(k,1,4,k) = %v, want 10", got)
	}
	if got := evalAt(t, "prod(k, 1, 4, k)", nil); got != 24 {
		t.Errorf("prod(k,1,4,k) = %v, want 24", got)
	}
	if got := evalAt(t, "sum(k, 1, 3, k^2)", nil); got != 14 {
		t.Errorf("sum(k,1,3,k^2) = %v, want 14", got)
	}
	// Empty range contributes nothing.
	if got := evalAt(t, "sum(k, 5, 1, k)", nil); got != 0 {
		t.Errorf("empty sum = %v, want 0", got)
	}
	// Nested aggregates reuse distinct slots.
	if got := evalAt(t, "sum(i, 1, 3, sum(j, 1, 2, i*j))", nil); got != 18 {
		t.Errorf("nested sum = %v, want 18", got)
	}
	// Sums compose with surrounding arithmetic.
	if got := evalAt(t, "1 + sum(k, 1, 3, k) * 2", nil); got != 13 {
		t.Errorf("sum in context = %v, want 13", got)
	}
}

// A truncated Fourier square series must approach the square wave's amplitude
// and overshoot near the discontinuity (the Gibbs phenomenon).
func TestFourierSquareSeriesConverges(t *testing.T) {
	p := MustCompile("(4/pi)*sum(k, 0, 40, sin((2k+1)*p)/(2k+1))")
	s := NewSampler(p)

	if got := s.At(0.25); math.Abs(got-1) > 0.05 {
		t.Errorf("series at quarter cycle = %v, want near 1", got)
	}
	if got := s.At(0.75); math.Abs(got+1) > 0.05 {
		t.Errorf("series at three-quarter cycle = %v, want near -1", got)
	}

	peak := 0.0
	for i := range 4096 {
		if v := math.Abs(s.At(float64(i) / 4096)); v > peak {
			peak = v
		}
	}
	if peak <= 1.0 {
		t.Errorf("expected Gibbs overshoot above 1, got peak %v", peak)
	}
}

func TestHarmonicsMatchesKnownSpectrum(t *testing.T) {
	// A pure second harmonic should show energy only in bin 2.
	h := Harmonics(MustCompile("sin(2p)"), 5, nil)
	if math.Abs(h[1]-1) > 1e-6 {
		t.Errorf("second harmonic magnitude = %v, want 1", h[1])
	}
	for i, v := range h {
		if i != 1 && v > 1e-6 {
			t.Errorf("unexpected energy %v in harmonic %d", v, i+1)
		}
	}
}

func TestPeakGainNormalizes(t *testing.T) {
	g := PeakGain(MustCompile("0.25*sin(p)"), nil)
	if math.Abs(g-4) > 1e-3 {
		t.Errorf("PeakGain = %v, want ~4", g)
	}
	if g := PeakGain(MustCompile("0"), nil); g != 1 {
		t.Errorf("silent PeakGain = %v, want 1", g)
	}
}

func TestDCOffsetDetected(t *testing.T) {
	if d := DCOffset(MustCompile("sin(p)"), nil); math.Abs(d) > 1e-6 {
		t.Errorf("sine DC = %v, want 0", d)
	}
	if d := DCOffset(MustCompile("sin(p)+0.5"), nil); math.Abs(d-0.5) > 1e-6 {
		t.Errorf("offset DC = %v, want 0.5", d)
	}
}

// Analysis must honour parameter values, or an expression like cheb(a, cos p)
// gets measured with a = 0 and looks like a constant.
func TestAnalysisRespectsParameters(t *testing.T) {
	p := MustCompile("cheb(a, cos(p))")
	setup := func(v *Vars) { v.SetParams(3, 0, 0, 0) }

	h := Harmonics(p, 6, setup)
	if h[2] < 0.9 {
		t.Errorf("third harmonic = %v, want near 1 with a=3", h[2])
	}
	if d := DCOffset(p, setup); math.Abs(d) > 1e-6 {
		t.Errorf("DC = %v, want 0 with a=3", d)
	}
}

func TestPhaseOnlyDetection(t *testing.T) {
	cases := []struct {
		src  string
		want bool
	}{
		{"sin(p)", true},
		{"sin(p)*a", true},
		{"sin(p)*t", false},
		{"sin(p)*env", false},
		{"noise", false},
		{"sin(p)+0.1*noise", false},
		{"sin(p)*vel", false},
	}

	for _, tc := range cases {
		p := MustCompile(tc.src)
		if got := p.PhaseOnly(); got != tc.want {
			t.Errorf("%q PhaseOnly = %v, want %v", tc.src, got, tc.want)
		}
	}
}

func TestUsedParams(t *testing.T) {
	p := MustCompile("a*sin(p) + c")
	got := p.UsedParams()
	want := [4]bool{true, false, true, false}
	if got != want {
		t.Errorf("UsedParams = %v, want %v", got, want)
	}
}

func TestNaNAndInfAreSuppressed(t *testing.T) {
	for _, src := range []string{"0/0", "log(-1)", "sqrt(-1)", "inf", "0*inf", "tan(pi/2)*inf"} {
		if got := evalAt(t, src, nil); math.IsNaN(got) || math.IsInf(got, 0) {
			t.Errorf("%q leaked %v into the audio path", src, got)
		}
	}
}

func TestRuntimeStaysBounded(t *testing.T) {
	// A huge aggregate must be capped rather than stalling the audio thread.
	p := MustCompile("sum(k, 1, 100000000, sin(k*p))")
	v := p.NewVars()
	v.Set(VarPhase, 1)
	_ = p.Eval(v) // must return promptly
}

func TestCompileErrors(t *testing.T) {
	cases := []string{
		"",
		"sin(",
		"sin(1,2)",
		"nosuchfunc(1)",
		"zzz",
		"1 +",
		"(1",
		"1)",
		"1 $ 2",
		"sum(k, 1, 2)",
		"sum(pi, 1, 2, pi)",
		"sum(p, 1, 2, p)",
		"1 ? 2",
	}

	for _, src := range cases {
		if _, err := Compile(src); err == nil {
			t.Errorf("Compile(%q) should have failed", src)
		}
	}
}

func TestErrorCarriesPosition(t *testing.T) {
	_, err := Compile("sin(p) + zzz")
	if err == nil {
		t.Fatal("expected an error")
	}
	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("error is %T, want *Error", err)
	}
	if e.Pos != 9 {
		t.Errorf("error position = %d, want 9", e.Pos)
	}
}

func TestVarsAreIndependent(t *testing.T) {
	p := MustCompile("sin(p)")
	v1, v2 := p.NewVars(), p.NewVars()
	v1.Set(VarPhase, 0)
	v2.Set(VarPhase, math.Pi/2)

	if got := p.Eval(v1); math.Abs(got) > 1e-9 {
		t.Errorf("v1 = %v, want 0", got)
	}
	if got := p.Eval(v2); math.Abs(got-1) > 1e-9 {
		t.Errorf("v2 = %v, want 1", got)
	}
}

func TestEvalIsRepeatable(t *testing.T) {
	p := MustCompile("sum(k, 1, 8, sin(k*p)/k)")
	v := p.NewVars()
	v.Set(VarPhase, 1.234)

	first := p.Eval(v)
	for range 100 {
		if got := p.Eval(v); got != first {
			t.Fatalf("Eval drifted: %v then %v", first, got)
		}
	}
}

func BenchmarkEvalSimple(b *testing.B) {
	p := MustCompile("sin(p)")
	v := p.NewVars()
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		v.Set(VarPhase, float64(i)*0.001)
		_ = p.Eval(v)
	}
}

func BenchmarkEvalFourierSeries(b *testing.B) {
	p := MustCompile("sum(k, 1, 16, sin(k*p)/k)")
	v := p.NewVars()
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		v.Set(VarPhase, float64(i)*0.001)
		_ = p.Eval(v)
	}
}
