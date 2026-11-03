// Package dsp provides the signal analysis behind the visualizers: a radix-2
// FFT, windowing, and a lock-free-ish ring buffer for tapping live audio.
package dsp

import (
	"math"
	"math/cmplx"
	"sync"
)

// FFT computes the discrete Fourier transform in place using the iterative
// radix-2 Cooley-Tukey algorithm. len(x) must be a power of two.
func FFT(x []complex128) {
	n := len(x)
	if n <= 1 {
		return
	}

	// Bit-reversal permutation.
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j |= bit
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}

	// Butterfly stages over successively larger halves.
	for length := 2; length <= n; length <<= 1 {
		ang := -2 * math.Pi / float64(length)
		wl := cmplx.Rect(1, ang)
		for i := 0; i < n; i += length {
			w := complex(1, 0)
			for j := range length / 2 {
				u := x[i+j]
				v := x[i+j+length/2] * w
				x[i+j] = u + v
				x[i+j+length/2] = u - v
				w *= wl
			}
		}
	}
}

// NextPow2 returns the smallest power of two at or above n.
func NextPow2(n int) int {
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}

// Hann returns a Hann window of length n. Windowing tapers the ends of the
// analysis block so the FFT does not see a discontinuity, which would smear
// energy across every bin.
func Hann(n int) []float64 {
	w := make([]float64, n)
	if n == 1 {
		w[0] = 1
		return w
	}
	for i := range w {
		w[i] = 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(n-1)))
	}
	return w
}

// Spectrum computes the magnitude spectrum of a real signal. The result has
// len(samples)/2 bins, covering DC up to the Nyquist frequency.
func Spectrum(samples []float64, window []float64) []float64 {
	n := NextPow2(len(samples))
	buf := make([]complex128, n)
	for i, s := range samples {
		if window != nil && i < len(window) {
			s *= window[i]
		}
		buf[i] = complex(s, 0)
	}

	FFT(buf)

	half := n / 2
	out := make([]float64, half)
	for i := range out {
		// Scale so a full-scale sine reads 1.0 in its bin.
		out[i] = 2 * cmplx.Abs(buf[i]) / float64(n)
	}
	return out
}

// BinFreq returns the center frequency of FFT bin i.
func BinFreq(i, fftSize int, sampleRate float64) float64 {
	return float64(i) * sampleRate / float64(fftSize)
}

// ToDB converts a linear magnitude to decibels, clamped at floorDB so silence
// does not become negative infinity.
func ToDB(mag, floorDB float64) float64 {
	if mag <= 0 {
		return floorDB
	}
	db := 20 * math.Log10(mag)
	if db < floorDB {
		return floorDB
	}
	return db
}

// LogBands collapses a linear FFT spectrum into n bands spaced logarithmically
// between minHz and maxHz, which is how pitch is actually perceived. Each band
// takes the peak of the bins that fall inside it.
func LogBands(spec []float64, sampleRate float64, n int, minHz, maxHz float64) []float64 {
	out := make([]float64, n)
	if len(spec) == 0 || maxHz <= minHz {
		return out
	}

	fftSize := len(spec) * 2
	logMin := math.Log2(minHz)
	logMax := math.Log2(maxHz)

	for i, mag := range spec {
		f := BinFreq(i, fftSize, sampleRate)
		if f < minHz || f > maxHz {
			continue
		}
		pos := (math.Log2(f) - logMin) / (logMax - logMin)
		b := int(pos * float64(n))
		if b < 0 {
			b = 0
		}
		if b >= n {
			b = n - 1
		}
		if mag > out[b] {
			out[b] = mag
		}
	}
	return out
}

// Ring is a fixed-size circular buffer of recent audio, written by the audio
// callback and read by the UI. It keeps the newest Size() samples.
type Ring struct {
	mu   sync.Mutex
	buf  []float64
	pos  int
	full bool
}

// NewRing allocates a ring holding n samples.
func NewRing(n int) *Ring {
	return &Ring{buf: make([]float64, n)}
}

// Size reports the ring capacity.
func (r *Ring) Size() int { return len(r.buf) }

// Write appends samples, overwriting the oldest data once full.
func (r *Ring) Write(samples []float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, s := range samples {
		r.buf[r.pos] = s
		r.pos++
		if r.pos >= len(r.buf) {
			r.pos = 0
			r.full = true
		}
	}
}

// Snapshot copies the buffer in chronological order, oldest sample first.
func (r *Ring) Snapshot() []float64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]float64, 0, len(r.buf))
	if r.full {
		out = append(out, r.buf[r.pos:]...)
	}
	out = append(out, r.buf[:r.pos]...)
	return out
}

// Peak returns the largest absolute sample currently held.
func (r *Ring) Peak() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	peak := 0.0
	for _, s := range r.buf {
		if a := math.Abs(s); a > peak {
			peak = a
		}
	}
	return peak
}

// FindZeroCrossing locates a rising zero crossing near the start of the buffer
// so an oscilloscope trace stands still instead of sliding across the screen.
// It returns 0 when no crossing is found.
func FindZeroCrossing(samples []float64, searchLen int) int {
	if searchLen > len(samples)-1 {
		searchLen = len(samples) - 1
	}
	for i := 1; i < searchLen; i++ {
		if samples[i-1] <= 0 && samples[i] > 0 {
			return i
		}
	}
	return 0
}

// Resample linearly resamples src to exactly n points, for fitting a waveform
// to a fixed number of terminal columns.
func Resample(src []float64, n int) []float64 {
	out := make([]float64, n)
	if len(src) == 0 || n == 0 {
		return out
	}
	if len(src) == 1 {
		for i := range out {
			out[i] = src[0]
		}
		return out
	}

	for i := range out {
		pos := float64(i) * float64(len(src)-1) / float64(n-1)
		lo := int(pos)
		hi := lo + 1
		if hi >= len(src) {
			out[i] = src[len(src)-1]
			continue
		}
		frac := pos - float64(lo)
		out[i] = src[lo]*(1-frac) + src[hi]*frac
	}
	return out
}
