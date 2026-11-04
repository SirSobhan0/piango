package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/SirSobhan0/piango/v2/internal/dsp"
	"github.com/SirSobhan0/piango/v2/internal/instrument"
	"github.com/SirSobhan0/piango/v2/internal/mathexpr"
	"github.com/SirSobhan0/piango/v2/internal/synth"
	"github.com/SirSobhan0/piango/v2/internal/tuning"
)

const (
	frameRate    = 30 * time.Millisecond
	scopeWidth   = 512
	fftSize      = 1024
	spectrumBars = 48
)

// mode is which panel currently owns the keyboard.
type mode int

const (
	modePlay mode = iota
	modeEdit      // typing a new equation
	modeHelp      // the function reference
)

// view selects which visualizer is showing.
type view int

const (
	viewScope view = iota
	viewSpectrum
	viewLissajous
	viewHarmonics
	numViews
)

func (v view) String() string {
	switch v {
	case viewScope:
		return "Oscilloscope"
	case viewSpectrum:
		return "Spectrum"
	case viewLissajous:
		return "Lissajous"
	case viewHarmonics:
		return "Harmonics"
	}
	return ""
}

type tickMsg time.Time

// Model is the Bubble Tea program state.
type Model struct {
	engine *synth.Engine
	board  *Keyboard
	player *Player

	insts   []*instrument.Instrument
	current int
	presets map[string]int

	mode     mode
	view     view
	editor   textinput.Model
	editErr  string
	paramSel int

	spectrum *Smoother
	window   []float64

	notice     string
	noticeTill time.Time

	width, height int
	ready         bool
}

// New builds a model over an engine and an instrument list. The list must not
// be empty.
func New(e *synth.Engine, insts []*instrument.Instrument) Model {
	board := NewKeyboard(tuning.Standard())

	in := textinput.New()
	in.Placeholder = "sin(p) + 0.5 sin(2p)"
	in.CharLimit = 240
	in.Width = 60

	e.SetInstrument(insts[0])

	return Model{
		engine:   e,
		board:    board,
		player:   NewPlayer(e, board),
		insts:    insts,
		presets:  map[string]int{},
		editor:   in,
		spectrum: NewSmoother(0.6, 0.12),
		window:   dsp.Hann(fftSize),
	}
}

// Init starts the frame timer.
func (m Model) Init() tea.Cmd { return tick() }

func tick() tea.Cmd {
	return tea.Tick(frameRate, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) notify(format string, args ...any) {
	m.notice = fmt.Sprintf(format, args...)
	m.noticeTill = time.Now().Add(2 * time.Second)
}

func (m *Model) inst() *instrument.Instrument { return m.insts[m.current] }

func (m *Model) selectInstrument(i int) {
	n := len(m.insts)
	m.current = ((i % n) + n) % n
	m.engine.SetInstrument(m.inst())
	m.paramSel = 0
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ready = true
		return m, nil

	case tickMsg:
		m.player.Tick()
		if m.notice != "" && time.Now().After(m.noticeTill) {
			m.notice = ""
		}
		return m, tick()

	case tea.KeyMsg:
		if m.mode == modeEdit {
			return m.updateEditor(msg)
		}
		return m.updatePlay(msg)
	}
	return m, nil
}

func (m Model) updatePlay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEscape:
		if m.mode == modeHelp {
			m.mode = modePlay
			return m, nil
		}
		m.engine.Panic()
		return m, tea.Quit

	case tea.KeySpace:
		m.player.ReleaseAll()
		m.notify("All notes off")
		return m, nil

	case tea.KeyTab:
		m.selectInstrument(m.current + 1)
		return m, nil

	case tea.KeyShiftTab:
		m.selectInstrument(m.current - 1)
		return m, nil

	case tea.KeyLeft:
		m.board.ShiftOctave(-1)
		m.notify("Octave %+d", m.board.Octave())
		return m, nil

	case tea.KeyRight:
		m.board.ShiftOctave(1)
		m.notify("Octave %+d", m.board.Octave())
		return m, nil

	case tea.KeyUp:
		m.nudgeParam(+1)
		return m, nil

	case tea.KeyDown:
		m.nudgeParam(-1)
		return m, nil
	}

	input := msg.String()

	// Shifted digits save a preset.
	if slot, ok := shiftedDigits[input]; ok {
		m.presets[slot] = m.current
		m.notify("Saved %s to %s", m.inst().Name, slot)
		return m, nil
	}

	// Plain digits load one.
	if len(input) == 1 && input[0] >= '0' && input[0] <= '9' {
		if id, ok := m.presets[input]; ok && id < len(m.insts) {
			m.selectInstrument(id)
			m.notify("Loaded %s", m.inst().Name)
		} else {
			m.notify("Preset %s is empty", input)
		}
		return m, nil
	}

	// Commands that would otherwise collide with note keys are placed on
	// characters the layout does not use.
	switch input {
	case "[":
		m.view = (m.view + numViews - 1) % numViews
		return m, nil
	case "]":
		m.view = (m.view + 1) % numViews
		return m, nil
	case "\\":
		sys := m.board.CycleTuning()
		m.notify("Tuning: %s", sys.Name())
		return m, nil
	case ";":
		m.paramSel = (m.paramSel + 1) % 4
		return m, nil
	case "/":
		if m.mode == modeHelp {
			m.mode = modePlay
		} else {
			m.mode = modeHelp
		}
		return m, nil
	case "=":
		m.mode = modeEdit
		m.editor.SetValue(m.inst().Expr)
		m.editor.CursorEnd()
		m.editErr = ""
		return m, m.editor.Focus()
	case "-":
		m.engine.SetMaster(m.engine.Master() - 0.05)
		m.notify("Master %.0f%%", m.engine.Master()*100)
		return m, nil
	case "+":
		m.engine.SetMaster(m.engine.Master() + 0.05)
		m.notify("Master %.0f%%", m.engine.Master()*100)
		return m, nil
	}

	if m.mode == modeHelp {
		return m, nil
	}

	// Anything left is a note. An uppercase letter means shift was held,
	// which plays the note staccato.
	staccato := len(input) == 1 && input[0] >= 'A' && input[0] <= 'Z'
	m.player.Press(input, staccato)
	return m, nil
}

func (m Model) updateEditor(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape, tea.KeyCtrlC:
		m.mode = modePlay
		m.editor.Blur()
		return m, nil

	case tea.KeyEnter:
		expr := strings.TrimSpace(m.editor.Value())
		in, err := instrument.Compile(instrument.Def{
			Name:     "Custom",
			Expr:     expr,
			Envelope: m.inst().Envelope,
		})
		if err != nil {
			m.editErr = err.Error()
			return m, nil
		}
		m.insts = append(m.insts, in)
		m.selectInstrument(len(m.insts) - 1)
		m.mode = modePlay
		m.editor.Blur()
		m.notify("Added custom instrument")
		return m, nil

	case tea.KeyCtrlS:
		// Save the edited equation to the user's config file.
		expr := strings.TrimSpace(m.editor.Value())
		if _, err := mathexpr.Compile(expr); err != nil {
			m.editErr = err.Error()
			return m, nil
		}
		name := fmt.Sprintf("Custom %d", len(m.insts)+1)
		if err := instrument.Append(instrument.Def{Name: name, Expr: expr}); err != nil {
			m.editErr = err.Error()
			return m, nil
		}
		m.notify("Saved %s to the config file", name)
		return m, nil
	}

	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)

	// Re-check the expression on every keystroke so errors appear as typed.
	if expr := strings.TrimSpace(m.editor.Value()); expr != "" {
		if _, err := mathexpr.Compile(expr); err != nil {
			m.editErr = err.Error()
		} else {
			m.editErr = ""
		}
	} else {
		m.editErr = ""
	}
	return m, cmd
}

// nudgeParam moves the selected parameter by one step of its range.
func (m *Model) nudgeParam(dir int) {
	in := m.inst()
	if m.paramSel >= len(in.Params) {
		m.notify("This instrument has no parameter %c", 'a'+rune(m.paramSel))
		return
	}
	p := in.Params[m.paramSel]
	step := (p.Max - p.Min) / 40
	if step == 0 {
		step = 0.05
	}
	in.SetParam(m.paramSel, p.Value+float64(dir)*step)
	m.notify("%s = %.3f", p.Name, in.Params[m.paramSel].Value)
}

var shiftedDigits = map[string]string{
	"!": "1", "@": "2", "#": "3", "$": "4", "%": "5",
	"^": "6", "&": "7", "*": "8", "(": "9", ")": "0",
}
