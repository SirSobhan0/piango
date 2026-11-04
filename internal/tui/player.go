package tui

import (
	"strings"
	"time"

	"github.com/SirSobhan0/piango/v2/internal/synth"
)

// Terminals report key presses but never key releases, so a held note is seen
// as a burst of repeated presses. The player infers "still held" from the gap
// between repeats and releases a note once that gap goes quiet.
const (
	// retriggerGap is how long a silence must last before a repeated press
	// counts as a fresh strike rather than auto-repeat.
	retriggerGap = 75 * time.Millisecond
	// holdTimeout is how long after the last press a sustained note is
	// released. It must exceed the slowest key-repeat interval.
	holdTimeout = 600 * time.Millisecond
	// staccatoTimeout is shorter because a staccato note is already fading.
	staccatoTimeout = 120 * time.Millisecond
)

type held struct {
	id       int
	lastSeen time.Time
	staccato bool
}

// Player turns key presses into engine notes, tracking which keys are still
// down. It owns no locks of its own; the engine is already safe for concurrent
// use and the player is only touched from the UI goroutine.
type Player struct {
	engine *synth.Engine
	board  *Keyboard
	down   map[string]*held
	now    func() time.Time
}

// NewPlayer connects a keyboard to an engine.
func NewPlayer(e *synth.Engine, b *Keyboard) *Player {
	return &Player{engine: e, board: b, down: map[string]*held{}, now: time.Now}
}

// Press handles one key press. staccato notes decay on their own, as if
// plucked. It reports whether the character was a note key at all.
func (p *Player) Press(char string, staccato bool) bool {
	key, ok := p.board.Lookup(char)
	if !ok {
		return false
	}

	char = strings.ToLower(char)
	now := p.now()

	if h, sounding := p.down[char]; sounding {
		if now.Sub(h.lastSeen) < retriggerGap {
			// Auto-repeat: the key is still down, so just keep it alive.
			h.lastSeen = now
			return true
		}
		// A real second strike. Release the old voice before the new one.
		p.engine.NoteOff(h.id)
	}

	id := p.board.ID(key)
	freq := p.board.Freq(key)

	if staccato {
		p.engine.NoteOnStaccato(id, freq, 0.9, 6)
	} else {
		p.engine.NoteOn(id, freq, 0.9)
	}
	p.down[char] = &held{id: id, lastSeen: now, staccato: staccato}
	return true
}

// Tick releases any key whose repeats have stopped. Call it from the UI's
// frame timer.
func (p *Player) Tick() {
	now := p.now()

	// Count how many keys were seen very recently (e.g. within 200ms).
	recentCount := 0
	hasActive := false
	for _, h := range p.down {
		if !h.staccato && now.Sub(h.lastSeen) <= holdTimeout {
			hasActive = true
			if now.Sub(h.lastSeen) <= 200*time.Millisecond {
				recentCount++
			}
		}
	}

	for char, h := range p.down {
		limit := holdTimeout
		if h.staccato {
			limit = staccatoTimeout
		}
		// On real terminals, holding multiple keys only auto-repeats one key.
		// If only one key is repeating/recent, keep other sustained keys held.
		if !h.staccato && hasActive && recentCount <= 1 {
			continue
		}
		if now.Sub(h.lastSeen) > limit {
			p.engine.NoteOff(h.id)
			delete(p.down, char)
		}
	}
}

// Down reports whether a key is currently considered held.
func (p *Player) Down(char string) bool {
	_, ok := p.down[strings.ToLower(char)]
	return ok
}

// HeldCount returns how many keys are currently held.
func (p *Player) HeldCount() int { return len(p.down) }

// ReleaseAll drops every held key and silences the engine.
func (p *Player) ReleaseAll() {
	clear(p.down)
	p.engine.Panic()
}

// Held returns the frequencies of the keys currently down, in no particular
// order. Used by the Lissajous display, which needs two pitches.
func (p *Player) Held() []float64 {
	out := make([]float64, 0, len(p.down))
	for _, h := range p.down {
		for _, v := range p.engine.Voices() {
			if v.Key == h.id && !v.Releasing {
				out = append(out, v.Freq)
				break
			}
		}
	}
	return out
}
