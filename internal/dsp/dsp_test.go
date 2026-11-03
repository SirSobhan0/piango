package dsp

import (
	"math"
	"math/cmplx"
	"testing"
)

// naiveDFT is the O(n^2) definition of the transform, used as ground truth.
func naiveDFT(x []complex128) []complex128 {
	n := len(x)
	out := make([]complex128, n)
	for k := range n {
		var sum complex128
		for t := range n {
			ang := -2 * math.Pi * float64(k) * float64(t) / float64(n)
			sum += x[t] * cmplx.Rect(1, ang)
		}
		out[k] = sum
	}
	return out
}

// The fast transform must agree with the textbook definition.
func TestFFTMatchesNaiveDFT(t *testing.T) {
	for _, n := range []int{2, 4, 8, 16, 64, 256} {
		in := make([]complex128, n)
		for i := range in {
			// A deterministic but non-trivial signal.
			in[i] = complex(math.Sin(float64(i)*0.7)+0.3*math.Cos(float64(i)*2.1), 0)
		}

		want := naiveDFT(in)
		got := make([]complex128, n)
		copy(got, in)
		FFT(got)

		for i := range got {
			if cmplx.Abs(got[i]-want[i]) > 1e-9 {
				t.Fatalf("n=%d bin %d: FFT gave %v, DFT gave %v", n, i, got[i], want[i])
			}
		}
	}
}

func TestFFTHandlesTrivialSizes(t *testing.T) {
	FFT(nil)
	FFT([]complex128{})
	one := []complex128{complex(5, 0)}
	FFT(one)
	if one[0] != complex(5, 0) {
		t.Errorf("single-element FFT changed the value to %v", one[0])
	}
}

// A pure sine at an exact bin frequency must put all its energy in that bin.
func TestSpectrumLocatesPureTone(t *testing.T) {
	const n = 1024
	const sampleRate = 44100.0
	const bin = 8

	freq := BinFreq(bin, n, sampleRate)
	samples := make([]float64, n)
	for i := range samples {
		samples[i] = math.Sin(2 * math.Pi * freq * float64(i) / sampleRate)
	}

	spec := Spectrum(samples, nil)

	if math.Abs(spec[bin]-1.0) > 0.01 {
		t.Errorf("peak magnitude = %v, want 1.0", spec[bin])
	}
	for i, m := range spec {
		if i != bin && m > 0.01 {
			t.Errorf("leakage of %v into bin %d", m, i)
		}
	}
}

func TestSpectrumFindsHarmonics(t *testing.T) {
	const n = 1024
	const sampleRate = 44100.0

	f1 := BinFreq(4, n, sampleRate)
	samples := make([]float64, n)
	for i := range samples {
		tt := float64(i) / sampleRate
		samples[i] = math.Sin(2*math.Pi*f1*tt) + 0.5*math.Sin(2*math.Pi*2*f1*tt)
	}

	spec := Spectrum(samples, nil)

	if math.Abs(spec[4]-1.0) > 0.02 {
		t.Errorf("fundamental = %v, want 1.0", spec[4])
	}
	if math.Abs(spec[8]-0.5) > 0.02 {
		t.Errorf("second harmonic = %v, want 0.5", spec[8])
	}
}

func TestSpectrumPadsToPowerOfTwo(t *testing.T) {
	// A non-power-of-two input must not panic and must produce n/2 bins.
	spec := Spectrum(make([]float64, 100), nil)
	if len(spec) != 64 {
		t.Errorf("got %d bins, want 64 after padding to 128", len(spec))
	}
}

func TestNextPow2(t *testing.T) {
	cases := map[int]int{1: 1, 2: 2, 3: 4, 5: 8, 100: 128, 1024: 1024, 1025: 2048}
	for in, want := range cases {
		if got := NextPow2(in); got != want {
			t.Errorf("NextPow2(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestHannWindow(t *testing.T) {
	w := Hann(64)

	if w[0] > 1e-12 || w[63] > 1e-12 {
		t.Errorf("window should taper to zero, got ends %v and %v", w[0], w[63])
	}
	// The peak sits in the middle and reaches 1.
	mid := w[32]
	if mid < 0.99 {
		t.Errorf("window center = %v, want near 1", mid)
	}
	// Hann is symmetric.
	for i := range 32 {
		if math.Abs(w[i]-w[63-i]) > 1e-12 {
			t.Errorf("window is not symmetric at %d", i)
		}
	}
	if got := Hann(1); len(got) != 1 || got[0] != 1 {
		t.Errorf("Hann(1) = %v, want [1]", got)
	}
}

// Windowing should reduce spectral leakage for a tone that falls between bins.
func TestWindowReducesLeakage(t *testing.T) {
	const n = 512
	const sampleRate = 44100.0

	// A frequency sitting halfway between two bins, the worst case for leakage.
	freq := BinFreq(10, n, sampleRate) + 0.5*sampleRate/n
	samples := make([]float64, n)
	for i := range samples {
		samples[i] = math.Sin(2 * math.Pi * freq * float64(i) / sampleRate)
	}

	bare := Spectrum(samples, nil)
	windowed := Spectrum(samples, Hann(n))

	// Measure energy far from the tone, where only leakage lives.
	farEnergy := func(spec []float64) float64 {
		sum := 0.0
		for i, m := range spec {
			if i > 40 {
				sum += m * m
			}
		}
		return sum
	}

	if farEnergy(windowed) >= farEnergy(bare) {
		t.Error("the Hann window did not reduce spectral leakage")
	}
}

func TestToDB(t *testing.T) {
	if got := ToDB(1, -80); math.Abs(got) > 1e-9 {
		t.Errorf("full scale = %v dB, want 0", got)
	}
	if got := ToDB(0.5, -80); math.Abs(got+6.0206) > 0.001 {
		t.Errorf("half scale = %v dB, want -6.02", got)
	}
	if got := ToDB(0, -80); got != -80 {
		t.Errorf("silence = %v dB, want the -80 floor", got)
	}
	if got := ToDB(1e-12, -80); got != -80 {
		t.Errorf("tiny signal = %v dB, want the -80 floor", got)
	}
}

func TestLogBands(t *testing.T) {
	const n = 1024
	const sampleRate = 44100.0

	spec := make([]float64, n/2)
	// Put a spike at roughly 1 kHz.
	target := int(math.Round(1000.0 * float64(n) / sampleRate))
	spec[target] = 1.0

	bands := LogBands(spec, sampleRate, 32, 50, 10000)

	if len(bands) != 32 {
		t.Fatalf("got %d bands, want 32", len(bands))
	}

	loud := -1
	for i, v := range bands {
		if v > 0.5 {
			if loud >= 0 {
				t.Errorf("energy landed in multiple bands: %d and %d", loud, i)
			}
			loud = i
		}
	}
	if loud < 0 {
		t.Fatal("the spike did not land in any band")
	}

	// A 1 kHz spike between 50 Hz and 10 kHz sits a bit past the middle.
	if loud < 10 || loud > 26 {
		t.Errorf("1 kHz landed in band %d of 32, which looks wrong", loud)
	}
}

func TestLogBandsHandlesEmptyInput(t *testing.T) {
	if got := LogBands(nil, 44100, 16, 50, 10000); len(got) != 16 {
		t.Errorf("got %d bands, want 16", len(got))
	}
	if got := LogBands(make([]float64, 8), 44100, 16, 10000, 50); len(got) != 16 {
		t.Errorf("inverted range should still return 16 bands, got %d", len(got))
	}
}

func TestRingKeepsNewestSamples(t *testing.T) {
	r := NewRing(4)

	r.Write([]float64{1, 2})
	if got := r.Snapshot(); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("partial snapshot = %v, want [1 2]", got)
	}

	r.Write([]float64{3, 4, 5, 6})
	got := r.Snapshot()
	want := []float64{3, 4, 5, 6}
	if len(got) != 4 {
		t.Fatalf("snapshot length = %d, want 4", len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("snapshot = %v, want %v", got, want)
			break
		}
	}
}

func TestRingChronologicalOrderAfterWrap(t *testing.T) {
	r := NewRing(4)
	r.Write([]float64{1, 2, 3, 4, 5})

	got := r.Snapshot()
	want := []float64{2, 3, 4, 5}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("after wrap snapshot = %v, want %v", got, want)
		}
	}
}

func TestRingPeak(t *testing.T) {
	r := NewRing(8)
	r.Write([]float64{0.1, -0.9, 0.3})
	if got := r.Peak(); math.Abs(got-0.9) > 1e-12 {
		t.Errorf("Peak = %v, want 0.9", got)
	}
}

func TestRingIsSafeUnderConcurrency(t *testing.T) {
	r := NewRing(1024)
	done := make(chan struct{})

	go func() {
		defer close(done)
		block := make([]float64, 64)
		for range 200 {
			r.Write(block)
		}
	}()

	for range 200 {
		_ = r.Snapshot()
		_ = r.Peak()
	}
	<-done
}

func TestFindZeroCrossing(t *testing.T) {
	samples := make([]float64, 100)
	for i := range samples {
		samples[i] = math.Sin(2 * math.Pi * float64(i) / 50)
	}

	// Start mid-cycle so the first rising crossing is not at index 0.
	shifted := samples[10:]
	idx := FindZeroCrossing(shifted, len(shifted))
	if idx == 0 {
		t.Fatal("expected to find a rising crossing")
	}
	if shifted[idx-1] > 0 || shifted[idx] <= 0 {
		t.Errorf("index %d is not a rising zero crossing", idx)
	}
}

func TestFindZeroCrossingFallsBackToZero(t *testing.T) {
	flat := make([]float64, 32)
	if got := FindZeroCrossing(flat, 32); got != 0 {
		t.Errorf("silent input = %d, want 0", got)
	}
	if got := FindZeroCrossing([]float64{1}, 99); got != 0 {
		t.Errorf("short input = %d, want 0", got)
	}
}

func TestResample(t *testing.T) {
	src := []float64{0, 1, 2, 3, 4}

	got := Resample(src, 5)
	for i := range src {
		if math.Abs(got[i]-src[i]) > 1e-12 {
			t.Errorf("identity resample changed %v to %v", src, got)
			break
		}
	}

	up := Resample(src, 9)
	if len(up) != 9 {
		t.Fatalf("got %d points, want 9", len(up))
	}
	if math.Abs(up[0]-0) > 1e-12 || math.Abs(up[8]-4) > 1e-12 {
		t.Errorf("endpoints not preserved: %v", up)
	}
	if math.Abs(up[4]-2) > 1e-12 {
		t.Errorf("midpoint = %v, want 2", up[4])
	}
}

func TestResampleEdgeCases(t *testing.T) {
	if got := Resample(nil, 4); len(got) != 4 {
		t.Errorf("nil input should still give 4 points, got %d", len(got))
	}
	if got := Resample([]float64{7}, 3); len(got) != 3 || got[0] != 7 || got[2] != 7 {
		t.Errorf("single-sample resample = %v, want [7 7 7]", got)
	}
	if got := Resample([]float64{1, 2}, 0); len(got) != 0 {
		t.Errorf("zero-length output = %v, want empty", got)
	}
}

func BenchmarkSpectrum1024(b *testing.B) {
	samples := make([]float64, 1024)
	for i := range samples {
		samples[i] = math.Sin(float64(i) * 0.1)
	}
	w := Hann(1024)
	b.ResetTimer()
	for b.Loop() {
		_ = Spectrum(samples, w)
	}
}
