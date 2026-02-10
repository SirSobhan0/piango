# Piango

> **The Terminal Synthesizer written in Go.**

Piango is a low-latency, polyphonic synthesizer that runs entirely in your terminal. It features real-time audio generation, multiple instrument presets, and a reactive TUI (Text User Interface) built with [Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Features

* **3 Full Octaves:** Play Low (`Z-M`), Mid (`A-J`), and High (`Q-U`) ranges.
* **Polyphonic Engine:** Play chords and overlapping notes seamlessly using additive synthesis.
* **8 Unique Instruments:**
    * Electric Piano
    * 8-Bit Square (NES Style)
    * Synth Saw
    * Church Organ
    * *...and more!*
* **Zero Latency:** Optimized audio buffer for instant response.
* **Reactive Visuals:** Keys light up in real-time as you play.

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

Download from [relaese page](https://github.com/SirSobhan0/piango/releases)

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
go run main.go

# Or build a binary
go build -o piango main.go
./piango
```

## Controls
The Keyboard layout

The keys are mapped to your physical keyboard rows to mimic a piano layout.
|Row|	Keys|	Octave|
-----------------------
|Top|	Q W E R T Y U|	High (C5 - B5)|
|Home|	A S D F G H J|	Mid (C4 - B4)|
|Bottom|	Z X C V B N M|	Low (C3 - B3)|

### Special Controls
|Key|	Action|
---------------
|TAB|	Cycle Instruments (Piano -> 8-Bit -> Saw -> ...)|
|SPACE|	Panic Button (Silence all sounds instantly)|
|ESC|	Quit|

## How it Works

Piango is built on two main pillars:

    Audio Engine (Beep):

        Uses Additive Synthesis (math functions) to generate waves on the fly. No sample files are used!

        Implements a custom ADSR Envelope to handle attack and decay.

        Features a "Watchdog Timer" to detect key releases in the terminal environment.

    TUI Engine (Bubble Tea):

        The UI runs on a separate thread from the audio.

        Handles keyboard events and renders the visual state at 60 FPS.

## License

This project is licensed under the GPL-3.0-or-later License, see the `COPYING` file for details.
