package instrument

// Builtins returns the shipped instrument set. Each one is a small piece of
// synthesis theory written as an equation you can read, edit and hear.
//
// Where a naive waveform would alias, the definition uses a truncated Fourier
// series instead: summing a finite number of harmonics keeps every partial
// below Nyquist, so the sound stays clean and the maths stays visible.
func Builtins() []*Instrument {
	defs := []Def{
		{
			Name:    "Pure Sine",
			Expr:    "sin(p)",
			Formula: "y = sin(p)",
			About:   "The only waveform with a single harmonic. Every other sound here is built from these.",
		},
		{
			Name:    "Harmonic Stack",
			Expr:    "sum(k, 1, 6, sin(k*p)/k^a)",
			Formula: "y = Σ(k=1..6) sin(kp) / k^a",
			About:   "Additive synthesis. The exponent a controls how fast the harmonics fall off: 1 is bright, 3 is mellow.",
			Params: []Param{
				{Name: "rolloff", Value: 1.6, Min: 0.5, Max: 4, Comment: "harmonic decay exponent"},
			},
		},
		{
			Name:    "Fourier Square",
			Expr:    "(4/pi) * sum(k, 0, 24, sin((2k+1)*p)/(2k+1))",
			Formula: "y = (4/π) Σ sin((2k+1)p) / (2k+1)",
			About:   "A square wave is odd harmonics falling off as 1/k. Truncating the series band-limits it and shows Gibbs ringing at the edges.",
		},
		{
			Name:    "Fourier Saw",
			Expr:    "(2/pi) * sum(k, 1, 32, sin(k*p)/k)",
			Formula: "y = (2/π) Σ(k=1..32) sin(kp) / k",
			About:   "Every harmonic at 1/k. The richest simple spectrum, and the classic subtractive-synthesis starting point.",
		},
		{
			Name:    "Fourier Triangle",
			Expr:    "(8/pi^2) * sum(k, 0, 16, (-1)^k * sin((2k+1)*p)/(2k+1)^2)",
			Formula: "y = (8/π²) Σ (-1)^k sin((2k+1)p) / (2k+1)²",
			About:   "Odd harmonics again, but falling as 1/k² with alternating sign. Much softer than a square.",
		},
		{
			Name:    "FM Bell",
			Expr:    "sin(p + a*sin(b*p))",
			Formula: "y = sin(p + I·sin(r·p))",
			About:   "Chowning FM. Index I sets brightness, ratio r sets the timbre. Non-integer ratios give inharmonic, bell-like spectra.",
			Params: []Param{
				{Name: "index", Value: 3.0, Min: 0, Max: 12, Comment: "modulation depth I"},
				{Name: "ratio", Value: 1.41, Min: 0.25, Max: 8, Comment: "modulator/carrier ratio r"},
			},
			Envelope: Envelope{Attack: 0.003, Decay: 0.9, Sustain: 0.25, Release: 0.5},
		},
		{
			Name:    "FM Brass",
			Expr:    "sin(p + a*env*sin(p))",
			Formula: "y = sin(p + I·env·sin(p))",
			About:   "The same FM, but the envelope drives the index, so the sound gets brighter as it swells. That is what brass does.",
			Params: []Param{
				{Name: "index", Value: 5.0, Min: 0, Max: 12, Comment: "peak modulation depth"},
			},
			Envelope: Envelope{Attack: 0.06, Decay: 0.2, Sustain: 0.8, Release: 0.2},
		},
		{
			Name:    "Wavefolder",
			Expr:    "sin(a*sin(p))",
			Formula: "y = sin(A·sin(p))",
			About:   "Folding a sine through a sine. The Jacobi-Anger identity expands this into odd harmonics weighted by Bessel functions J(A).",
			Params: []Param{
				{Name: "fold", Value: 3.0, Min: 0.5, Max: 10, Comment: "fold amount A"},
			},
		},
		{
			Name:    "Chebyshev Shaper",
			Expr:    "cheb(a, cos(p))",
			Formula: "y = T_n(cos p) = cos(np)",
			About:   "Chebyshev polynomials turn a cosine into exactly its nth harmonic. Set n and watch a single spectral line move.",
			Params: []Param{
				{Name: "order", Value: 3, Min: 1, Max: 12, Comment: "polynomial order n"},
			},
		},
		{
			Name:    "Ring Modulator",
			Expr:    "sin(p) * sin(a*p)",
			Formula: "y = sin(p)·sin(r·p)",
			About:   "A product of sines is a sum of sines: sum and difference frequencies only, no original tones. Inharmonic r sounds metallic.",
			Params: []Param{
				{Name: "ratio", Value: 2.73, Min: 0.25, Max: 8, Comment: "modulator ratio r"},
			},
		},
		{
			Name:    "Tanh Saturator",
			Expr:    "tanh(a*sin(p))",
			Formula: "y = tanh(A·sin(p))",
			About:   "Soft clipping. The odd-symmetric curve adds only odd harmonics, which is why it sounds warm rather than harsh.",
			Params: []Param{
				{Name: "drive", Value: 4.0, Min: 1, Max: 20, Comment: "input gain A"},
			},
		},
		{
			Name:    "Bitcrusher",
			Expr:    "quant(sin(p), a)",
			Formula: "y = round(sin(p)·N) / N",
			About:   "Amplitude quantization. Fewer steps means more quantization error, and that error is the crunchy distortion you hear.",
			Params: []Param{
				{Name: "steps", Value: 5, Min: 2, Max: 64, Comment: "quantization levels N"},
			},
		},
		{
			Name:     "Glass Bell",
			Expr:     "sum(k, 1, 5, sin(k^1.4 * p) / k^1.6)",
			Formula:  "y = Σ sin(k^1.4·p) / k^1.6",
			About:    "Stretching harmonics to k^1.4 makes them inharmonic. Struck metal and glass behave this way, which is why it reads as a bell.",
			Envelope: Envelope{Attack: 0.002, Decay: 1.4, Sustain: 0.1, Release: 0.8},
		},
		{
			Name:    "Formant Vowel",
			Expr:    "sum(k, 1, 20, sin(k*p) * gauss((k*f - a)/b) / k)",
			Formula: "y = Σ sin(kp)·exp(-((k·f - F)/w)²) / k",
			About:   "A Gaussian bump over the harmonic series makes a formant, the resonance that gives vowels their identity.",
			Params: []Param{
				{Name: "formant", Value: 900, Min: 200, Max: 3000, Comment: "formant center F in Hz"},
				{Name: "width", Value: 500, Min: 100, Max: 2000, Comment: "formant width w in Hz"},
			},
		},
		{
			Name:    "Sub Bass",
			Expr:    "tanh(2*sin(p) + a*sin(p/2))",
			Formula: "y = tanh(2·sin(p) + A·sin(p/2))",
			About:   "Adding a half-frequency term puts energy one octave below the note, then tanh glues it together.",
			Params: []Param{
				{Name: "suboct", Value: 1.2, Min: 0, Max: 3, Comment: "sub-octave amount A"},
			},
			Envelope: Envelope{Attack: 0.006, Decay: 0.25, Sustain: 0.85, Release: 0.18},
		},
		{
			Name:    "PWM Pulse",
			Expr:    "sum(k, 1, 24, sin(pi*k*a)*cos(k*p)/k)",
			Formula: "y = Σ sin(πkd)·cos(kp) / k",
			About:   "A band-limited pulse from its Fourier coefficients. Duty cycle d mutes every harmonic where kd is a whole number.",
			Params: []Param{
				{Name: "duty", Value: 0.32, Min: 0.05, Max: 0.95, Comment: "duty cycle d"},
			},
		},
		{
			Name:    "Detuned Supersaw",
			Expr:    "sum(k, 1, 16, (sin(k*p) + sin(k*p*(1+a)) + sin(k*p*(1-a)))/k)",
			Formula: "y = Σ [sin(kp) + sin(kp(1+δ)) + sin(kp(1-δ))] / k",
			About:   "Three saws a hair apart. The tiny frequency offset δ makes them drift in and out of phase, which the ear hears as width.",
			Params: []Param{
				{Name: "detune", Value: 0.008, Min: 0, Max: 0.05, Comment: "detune fraction δ"},
			},
		},
		{
			Name:    "Breathy Flute",
			Expr:    "sin(p) + 0.25*sin(2p) + a*noise",
			Formula: "y = sin(p) + 0.25·sin(2p) + n·noise",
			About:   "Nearly a pure tone plus filtered air. Noise is what separates a flute from a sine.",
			Params: []Param{
				{Name: "breath", Value: 0.12, Min: 0, Max: 0.5, Comment: "noise amount n"},
			},
			Envelope: Envelope{Attack: 0.05, Decay: 0.1, Sustain: 0.9, Release: 0.15},
		},
		{
			Name:     "White Noise",
			Expr:     "noise",
			Formula:  "y = U(-1, 1)",
			About:    "Equal power at every frequency, with no pitch at all. Useful as a reference for what a flat spectrum looks like.",
			Envelope: Envelope{Attack: 0.001, Decay: 0.05, Sustain: 0.6, Release: 0.08},
		},
		{
			Name:    "Tremolo Sine",
			Expr:    "sin(p) * (1 - a + a*sin(tau*b*t))",
			Formula: "y = sin(p)·(1 - d + d·sin(2π·r·t))",
			About:   "Amplitude modulation driven by real time t rather than phase, so the wobble rate stays fixed no matter which note you play.",
			Params: []Param{
				{Name: "depth", Value: 0.5, Min: 0, Max: 1, Comment: "modulation depth d"},
				{Name: "rate", Value: 5.5, Min: 0.1, Max: 20, Comment: "modulation rate r in Hz"},
			},
		},
	}

	out := make([]*Instrument, 0, len(defs))
	for _, d := range defs {
		out = append(out, MustCompile(d))
	}
	return out
}
