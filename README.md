# Piango

> **The terminal synthesizer where every instrument is an equation.**

Piango is a low-latency, polyphonic synthesizer that runs entirely in your terminal. Instruments are not hardcoded waveforms: each one is a mathematical expression that Piango compiles and evaluates. Type a Fourier series and hear it. Change one exponent and hear the timbre move.

![piango gif](assets/piango.gif)

## Features

* **Instruments are equations.** `sin(p)` is a pure tone. `(4/pi) sum(k,0,24, sin((2k+1)p)/(2k+1))` is a band-limited square wave. Both are one line of text.
* **Live equation editor.** Press `=`, type an expression, and hear it immediately. Errors are reported as you type, with the column marked.
* **20 built-in instruments**, each chosen to demonstrate one idea: additive synthesis, Fourier series, FM, wave folding, Chebyshev shaping, ring modulation, formant peaks.
* **Custom instruments** loaded from a JSON config file, so your own equations survive restarts.
* **Real analysis, not decoration.** The spectrum display is an actual FFT of the audio output; the harmonic view is measured from the equation itself.
* **Seven tunings:** 12-TET, just intonation, Pythagorean, quarter-comma meantone, and 19-, 24-, and 31-tone equal temperament. Held intervals are named as exact ratios with their error in cents.
* **Lissajous figures** that plot two held notes against each other, making consonance visible as a closed curve.
* **Offline rendering** to WAV, so you can render an equation without opening the UI.
* **3 full octaves** on the letter rows, shiftable ±3 octaves.
* **True ADSR envelopes** specified in seconds, so they sound identical at any sample rate.
* **Polyphonic** with voice stealing and a soft limiter, so chords never clip.

## Installation

### Prerequisites

You need **Go 1.25+** installed.

**Linux Users:**
Because this project uses low-level audio drivers, you must install the ALSA development headers:

```bash
sudo apt-get update
sudo apt-get install libasound2-dev
```
On Fedora:
```bash
sudo dnf install alsa-lib-devel
```

### Installation
Download from the [release page](https://github.com/SirSobhan0/piango/releases)

||

```bash
go install github.com/SirSobhan0/piango@latest
```

||

Build from Source
```bash
# Clone the repository
git clone https://github.com/SirSobhan0/piango.git
cd piango

# Run directly
go run .

# Or build a binary
go build -o piango .
./piango
```

## Controls

The keys are mapped to your physical keyboard rows in an aligned 3×7 grid mimicking a piano keyboard. Each column represents the same scale degree (Do through Si) across three octaves:

| Row    | Keys          | Octave | Scale Degrees (Notes)        |
|--------|---------------|--------|------------------------------|
| Top    | Q W E R T Y U | High   | Do  Re  Mi  Fa  Sol  La  Si  |
| Home   | A S D F G H J | Mid    | Do  Re  Mi  Fa  Sol  La  Si  |
| Bottom | Z X C V B N M | Low    | Do  Re  Mi  Fa  Sol  La  Si  |

Keys align vertically so pressing Q, A, or Z triggers the root note ("Do") in high, mid, or low octaves.

### Chords & Multi-Key Holding

Terminals typically only send auto-repeat events for the most recently pressed key. Piango includes intelligent chord-sustain detection: when holding multiple keys simultaneously (e.g. playing chords or inspecting intervals in the Lissajous view), all actively held notes are sustained together without cutting out early while key repeat continues. Releasing all keys allows the notes to enter their release envelope, or press `SPACE` anytime to panic/silence immediately.

Holding **SHIFT** while pressing a note plays it staccato, letting it decay on its own.

### Special Controls

| Key           | Action                                            |
|---------------|---------------------------------------------------|
| `TAB`         | Next instrument                                   |
| `SHIFT+TAB`   | Previous instrument                               |
| `1`–`0`       | Load preset                                       |
| `SHIFT+1`–`0` | Save the current instrument to that preset slot   |
| `←` `→`       | Shift the keyboard down or up an octave           |
| `↑` `↓`       | Adjust the selected parameter                     |
| `;`           | Select the next parameter (`a` → `b` → `c` → `d`) |
| `[` `]`       | Previous / next visualizer                        |
| `\`           | Cycle the tuning system                           |
| `=`           | Open the equation editor                          |
| `-` `+`       | Master volume                                     |
| `/`           | Function reference                                |
| `SPACE`       | Panic (silence everything instantly)              |
| `ESC`         | Quit                                              |

Inside the equation editor: `ENTER` plays the equation, `CTRL+S` also saves it to your config file, and `ESC` cancels.

### Visualizers

Cycle with `[` and `]`:

| View         | Shows                                                              |
|--------------|--------------------------------------------------------------------|
| Oscilloscope | The live output waveform, zero-crossing aligned with minimal latency |
| Spectrum     | An FFT of the live output in logarithmically spaced bands           |
| Lissajous    | Two held notes plotted against each other                           |
| Harmonics    | Harmonic amplitudes measured from the equation                      |

## The equation language

An instrument is one expression, evaluated once per sample. It should return roughly −1 to 1; Piango normalizes the result anyway.

### Variables

| Name               | Meaning                                              |
|--------------------|------------------------------------------------------|
| `p`, `phase`       | Phase in radians, sweeping 0 to 2π over one cycle    |
| `x`                | The same phase normalized to 0–1                     |
| `t`, `time`        | Seconds since the note started                       |
| `f`, `freq`        | Note frequency in Hz                                 |
| `env`              | Envelope level, 0–1                                  |
| `vel`              | Key velocity, 0–1                                    |
| `sr`               | Sample rate                                          |
| `a`, `b`, `c`, `d` | Tweakable parameters, bound to the `↑` / `↓` keys    |
| `noise`            | Uniform random in −1 to 1                            |

The constants `pi`, `tau`, `e`, `phi`, and `inf` are also available.

### Operators

`+ - * / % ^` with the usual precedence, comparisons `< <= > >= == !=`, logic `&& ||`, and a ternary `cond ? a : b`.

Multiplication may be implicit, so `2p`, `3(1+1)`, and `sin(2p)cos(p)` all work. Exponentiation is right-associative and binds tighter than unary minus, so `-2^2` is −4.

### Functions

One argument: `sin cos tan asin acos atan sinh cosh tanh abs sign floor ceil round trunc frac sqrt exp log log2 log10 saw tri sqr sinc gauss`

Two arguments: `pow mod min max atan2 step pulse quant cheb hypot`

Three arguments: `clamp lerp if`

Run `piango -functions` for the full list with descriptions, or press `/` in the app.

### Series

`sum(k, from, to, body)` and `prod(k, from, to, body)` let you write a series directly:

```
(4/pi) sum(k, 0, 24, sin((2k+1)p) / (2k+1))              # square
(2/pi) sum(k, 1, 32, sin(k p) / k)                       # sawtooth
(8/pi^2) sum(k, 0, 16, (-1)^k sin((2k+1)p) / (2k+1)^2)   # triangle
```

Truncating the series band-limits the waveform, which is why these stay clean where a naive `sqr(x)` aliases.

## Custom instruments

Piango also loads instruments from `~/.config/piango/instruments.json` (respecting `$XDG_CONFIG_HOME`). Write a starter file with:

```bash
piango -write-example
```

The format is:

```json
{
  "instruments": [
    {
      "name": "My Bell",
      "expr": "sin(p + a*sin(b*p))",
      "formula": "sin(p + a·sin(b·p))",
      "about": "Frequency modulation. a is the index, b is the ratio.",
      "envelope": { "attack": 0.002, "decay": 0.8, "sustain": 0.0, "release": 0.6 },
      "params": [
        { "name": "index", "value": 3.0, "min": 0, "max": 12 },
        { "name": "ratio", "value": 1.41, "min": 0.25, "max": 8 }
      ]
    }
  ]
}
```

Only `name` and `expr` are required. Envelope times are in seconds and `sustain` is a level from 0 to 1. A broken instrument is reported and skipped rather than preventing startup.

## Command line

```bash
piango                                        # play live
piango -eval "sin(p) + 0.3 sin(3p)"           # start on a custom equation
piango -eval "saw(x)" -render out.wav         # render one note to a file
piango -render bell.wav -duration 3 -freq 220 # control the rendered note
piango -list                                  # list every instrument
piango -functions                             # print the language reference
piango -write-example                         # create a starter config file
```

## How it works

**The expression engine** compiles each equation to a small stack-machine program rather than walking a syntax tree at audio rate. Evaluation never returns NaN or infinity: a pathological expression collapses to silence instead of blasting your speakers.

**Wavetables.** If an expression depends only on phase, Piango renders one cycle into a 4096-point table at load time and plays back with linear interpolation. Benchmarked on a ten-voice chord of a 25-term Fourier series, that is about 123× faster than evaluating per sample — 0.6% of one core instead of 71%. Expressions using `t`, `env`, or `noise` cannot be tabulated and are evaluated directly.

**Envelopes** are specified in seconds and advanced against elapsed time, so an 8 ms attack is 8 ms at any sample rate. A release starts from the voice's current level rather than jumping to it, which is what keeps note-offs from clicking.

**The audio engine** is a single streamer guarded by one mutex. The UI runs on its own goroutine and touches the engine only through its methods, so there is no shared mixer to race on. The whole suite passes under `-race`.

**Key release detection & chord sustain.** Terminals report key presses but never releases, so a held note arrives as a burst of auto-repeats. Piango infers "still held" from the gap between repeats and releases the note once they stop. When multiple keys are held simultaneously (chords/intervals), keyboard hardware and terminal emulators only auto-repeat the latest pressed key; Piango keeps concurrent notes alive alongside the actively repeating key so chords sustain naturally.

**The TUI** is [Bubble Tea](https://github.com/charmbracelet/bubbletea). Visualizers are drawn with braille characters, giving 2×4 addressable dots per terminal cell.

## License

This project is licensed under the GPL-3.0-or-later License, see the `COPYING` file for details.
