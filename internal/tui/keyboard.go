package tui

import (
	"math"
	"strings"

	"github.com/SirSobhan0/piango/v2/internal/tuning"
)

// Key is one playable key on the computer keyboard.
type Key struct {
	Char  string // the key the user presses
	Name  string // solfege name, e.g. "Do"
	Steps int    // scale steps from the reference pitch
}

// Keyboard maps computer keys to pitches under a chosen tuning system.
type Keyboard struct {
	Rows   [3][]Key
	byChar map[string]Key
	system tuning.System
	octave int
}

// layout lists the three rows in the same arrangement the original used: the
// home row is the middle octave, with the row above and below one octave out.
var layout = [3][]Key{
	{
		{"q", "Do", 12}, {"w", "Re", 14}, {"e", "Mi", 16}, {"r", "Fa", 17},
		{"t", "Sol", 19}, {"y", "La", 21}, {"u", "Si", 23},
	},
	{
		{"a", "Do", 0}, {"s", "Re", 2}, {"d", "Mi", 4}, {"f", "Fa", 5},
		{"g", "Sol", 7}, {"h", "La", 9}, {"j", "Si", 11},
	},
	{
		{"z", "Do", -12}, {"x", "Re", -10}, {"c", "Mi", -8}, {"v", "Fa", -7},
		{"b", "Sol", -5}, {"n", "La", -3}, {"m", "Si", -1},
	},
}

// NewKeyboard builds a keyboard using the given tuning system. The home row's
// Do sits on the system's reference pitch.
func NewKeyboard(sys tuning.System) *Keyboard {
	k := &Keyboard{Rows: layout, system: sys, byChar: map[string]Key{}}
	for _, row := range layout {
		for _, key := range row {
			k.byChar[key.Char] = key
		}
	}
	return k
}

// System returns the active tuning system.
func (k *Keyboard) System() tuning.System { return k.system }

// SetSystem changes the tuning. Sounding notes are unaffected; new ones use the
// new system.
func (k *Keyboard) SetSystem(s tuning.System) { k.system = s }

// CycleTuning advances to the next tuning system and returns it.
func (k *Keyboard) CycleTuning() tuning.System {
	k.system = k.system.Cycle()
	return k.system
}

// Octave returns the current octave offset.
func (k *Keyboard) Octave() int { return k.octave }

// ShiftOctave moves the keyboard by whole octaves, clamped to a useful range.
func (k *Keyboard) ShiftOctave(d int) {
	k.octave = min(max(k.octave+d, -3), 3)
}

// Lookup finds the key for a pressed character, accepting either case so that
// shift-modified presses still identify a note.
func (k *Keyboard) Lookup(char string) (Key, bool) {
	key, ok := k.byChar[strings.ToLower(char)]
	return key, ok
}

// Freq returns the sounding frequency of a key in Hz, under the current tuning
// system and octave offset.
func (k *Keyboard) Freq(key Key) float64 {
	return k.system.Freq(k.steps(key))
}

// steps converts a key's twelve-tone position into the active system's own
// step count, so a row still spans one octave in 19-, 24-, or 31-TET.
func (k *Keyboard) steps(key Key) int {
	div := k.system.Divisions
	if div <= 0 {
		div = 12
	}
	scaled := key.Steps
	if div != 12 {
		scaled = int(math.Round(float64(key.Steps) * float64(div) / 12))
	}
	return scaled + k.octave*div
}

// FreqOf returns the frequency for a pressed character.
func (k *Keyboard) FreqOf(char string) (float64, bool) {
	key, ok := k.Lookup(char)
	if !ok {
		return 0, false
	}
	return k.Freq(key), true
}

// ID returns a stable identifier for a key, used as the engine's voice key.
// The octave is folded in so the same letter at different octaves counts as a
// different note.
func (k *Keyboard) ID(key Key) int {
	return key.Steps + k.octave*1000
}

// Keys returns every key in layout order.
func (k *Keyboard) Keys() []Key {
	out := make([]Key, 0, 21)
	for _, row := range k.Rows {
		out = append(out, row...)
	}
	return out
}
