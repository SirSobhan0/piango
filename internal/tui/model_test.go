package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SirSobhan0/piango/internal/instrument"
	"github.com/SirSobhan0/piango/internal/synth"
)

func testModel(t *testing.T) Model {
	t.Helper()

	insts := []*instrument.Instrument{
		instrument.MustCompile(instrument.Def{Name: "One", Expr: "sin(p)"}),
		instrument.MustCompile(instrument.Def{Name: "Two", Expr: "sin(2p)"}),
		instrument.MustCompile(instrument.Def{
			Name: "Three", Expr: "sin(p + a*sin(p))",
			Params: []instrument.Param{{Name: "index", Value: 1, Min: 0, Max: 8}},
		}),
	}
	m := New(synth.NewEngine(insts[0], 44100), insts)
	m.width, m.height = 120, 44
	m.ready = true
	return m
}

// press feeds a key to the model and returns the updated copy.
func press(m Model, s string) Model {
	var msg tea.KeyMsg
	switch s {
	case "tab":
		msg = tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		msg = tea.KeyMsg{Type: tea.KeyShiftTab}
	case "left":
		msg = tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		msg = tea.KeyMsg{Type: tea.KeyRight}
	case "up":
		msg = tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEscape}
	case "space":
		msg = tea.KeyMsg{Type: tea.KeySpace}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}

	next, _ := m.Update(msg)
	return next.(Model)
}

func TestTabCyclesInstruments(t *testing.T) {
	m := testModel(t)
	if m.inst().Name != "One" {
		t.Fatalf("started on %q, want One", m.inst().Name)
	}

	m = press(m, "tab")
	if m.inst().Name != "Two" {
		t.Errorf("after TAB the instrument is %q, want Two", m.inst().Name)
	}

	// Cycling past the end wraps around.
	m = press(m, "tab")
	m = press(m, "tab")
	if m.inst().Name != "One" {
		t.Errorf("TAB did not wrap, landed on %q", m.inst().Name)
	}

	m = press(m, "shift+tab")
	if m.inst().Name != "Three" {
		t.Errorf("SHIFT+TAB should wrap backwards, landed on %q", m.inst().Name)
	}
}

func TestInstrumentChangeReachesTheEngine(t *testing.T) {
	m := testModel(t)
	m = press(m, "tab")

	if got := m.engine.Instrument().Name; got != "Two" {
		t.Errorf("the engine is playing %q, want Two", got)
	}
}

func TestNoteKeyStartsAVoice(t *testing.T) {
	m := testModel(t)
	m = press(m, "a")

	if m.engine.ActiveVoices() != 1 {
		t.Errorf("got %d voices, want 1", m.engine.ActiveVoices())
	}
}

func TestSpacePanics(t *testing.T) {
	m := testModel(t)
	for _, k := range []string{"a", "s", "d"} {
		m = press(m, k)
	}
	m = press(m, "space")

	if m.engine.ActiveVoices() != 0 {
		t.Errorf("%d voices survived the panic key", m.engine.ActiveVoices())
	}
}

func TestOctaveKeysAreClamped(t *testing.T) {
	m := testModel(t)
	for range 10 {
		m = press(m, "right")
	}
	if m.board.Octave() != 3 {
		t.Errorf("octave = %d, want it clamped to 3", m.board.Octave())
	}
}

func TestPresetSaveAndLoad(t *testing.T) {
	m := testModel(t)
	m = press(m, "tab") // move to Two
	m = press(m, "!")   // save to slot 1
	m = press(m, "tab") // move away
	m = press(m, "1")   // load slot 1

	if m.inst().Name != "Two" {
		t.Errorf("loaded %q, want the saved Two", m.inst().Name)
	}
}

func TestEmptyPresetDoesNotChangeInstrument(t *testing.T) {
	m := testModel(t)
	before := m.inst().Name
	m = press(m, "7")

	if m.inst().Name != before {
		t.Errorf("an empty preset changed the instrument to %q", m.inst().Name)
	}
	if !strings.Contains(m.notice, "empty") {
		t.Errorf("notice = %q, want it to mention the empty slot", m.notice)
	}
}

func TestVisualizerCyclesThroughEveryView(t *testing.T) {
	m := testModel(t)
	seen := map[view]bool{m.view: true}

	for range int(numViews) {
		m = press(m, "]")
		seen[m.view] = true
	}
	if len(seen) != int(numViews) {
		t.Errorf("cycling visited %d views, want %d", len(seen), numViews)
	}
	if m.view != viewScope {
		t.Errorf("a full cycle ended on %v, want to be back at the start", m.view)
	}
}

func TestTuningKeyChangesTheSystem(t *testing.T) {
	m := testModel(t)
	before := m.board.System().Name()
	m = press(m, "\\")

	if m.board.System().Name() == before {
		t.Error("the tuning system did not change")
	}
}

func TestParameterKeysAdjustTheInstrument(t *testing.T) {
	m := testModel(t)
	m = press(m, "tab")
	m = press(m, "tab") // instrument Three, which has a parameter

	before := m.inst().Params[0].Value
	m = press(m, "up")
	after := m.inst().Params[0].Value

	if after <= before {
		t.Errorf("the parameter went from %v to %v, want an increase", before, after)
	}

	m = press(m, "down")
	m = press(m, "down")
	if m.inst().Params[0].Value >= after {
		t.Error("the down key did not lower the parameter")
	}
}

func TestParameterIsClampedByTheInstrument(t *testing.T) {
	m := testModel(t)
	m = press(m, "tab")
	m = press(m, "tab")

	for range 200 {
		m = press(m, "down")
	}
	if got := m.inst().Params[0].Value; got != 0 {
		t.Errorf("parameter bottomed out at %v, want its minimum of 0", got)
	}
}

func TestEditorAcceptsAValidEquation(t *testing.T) {
	m := testModel(t)
	before := len(m.insts)

	m = press(m, "=")
	if m.mode != modeEdit {
		t.Fatal("the editor did not open")
	}

	m.editor.SetValue("sin(p) + 0.3 sin(3p)")
	m = press(m, "enter")

	if m.mode != modePlay {
		t.Error("the editor stayed open after a valid equation")
	}
	if len(m.insts) != before+1 {
		t.Errorf("got %d instruments, want %d", len(m.insts), before+1)
	}
	if m.inst().Expr != "sin(p) + 0.3 sin(3p)" {
		t.Errorf("the new instrument is %q", m.inst().Expr)
	}
}

func TestEditorRejectsABadEquation(t *testing.T) {
	m := testModel(t)
	before := len(m.insts)

	m = press(m, "=")
	m.editor.SetValue("sin(p")
	m = press(m, "enter")

	if m.mode != modeEdit {
		t.Error("the editor closed on an invalid equation")
	}
	if m.editErr == "" {
		t.Error("no error was reported for an unbalanced parenthesis")
	}
	if len(m.insts) != before {
		t.Error("an invalid equation was added to the instrument list")
	}
}

func TestEditorReportsErrorsWhileTyping(t *testing.T) {
	m := testModel(t)
	m = press(m, "=")

	for _, r := range "sin(q)" {
		m = press(m, string(r))
	}
	if m.editErr == "" {
		t.Error("an unknown variable should be flagged as it is typed")
	}

	m = press(m, "=") // typing continues inside the editor
	m.editor.SetValue("sin(p)")
	m = press(m, "x") // trigger a re-check
	m.editor.SetValue("sin(p)")
	m = press(m, tea.KeyBackspace.String())

	if m.editErr != "" && strings.Contains(m.editErr, "q") {
		t.Errorf("a stale error survived the correction: %q", m.editErr)
	}
}

func TestEditorEscapeCancels(t *testing.T) {
	m := testModel(t)
	before := len(m.insts)

	m = press(m, "=")
	m.editor.SetValue("sin(p)*2")
	m = press(m, "esc")

	if m.mode != modePlay {
		t.Error("escape did not close the editor")
	}
	if len(m.insts) != before {
		t.Error("escape still added the instrument")
	}
}

func TestNoteKeysDoNotPlayWhileEditing(t *testing.T) {
	m := testModel(t)
	m = press(m, "=")
	m = press(m, "a")

	if m.engine.ActiveVoices() != 0 {
		t.Error("typing in the editor played a note")
	}
	if !strings.Contains(m.editor.Value(), "a") {
		t.Errorf("the letter did not reach the editor, value = %q", m.editor.Value())
	}
}

func TestHelpScreenToggles(t *testing.T) {
	m := testModel(t)
	m = press(m, "/")
	if m.mode != modeHelp {
		t.Fatal("the help screen did not open")
	}
	if !strings.Contains(m.View(), "Variables") {
		t.Error("the help screen is missing the variable reference")
	}

	m = press(m, "/")
	if m.mode != modePlay {
		t.Error("the help screen did not close")
	}
}

func TestMasterVolumeKeys(t *testing.T) {
	m := testModel(t)
	before := m.engine.Master()

	m = press(m, "-")
	if m.engine.Master() >= before {
		t.Error("the minus key did not lower the volume")
	}

	m = press(m, "+")
	m = press(m, "+")
	if m.engine.Master() <= before {
		t.Error("the plus key did not raise the volume")
	}
}

func TestViewRendersInEveryMode(t *testing.T) {
	m := testModel(t)
	m = press(m, "a") // give the visualizers something to show

	for v := view(0); v < numViews; v++ {
		m.view = v
		out := m.View()
		if strings.TrimSpace(out) == "" {
			t.Errorf("the %v view rendered nothing", v)
		}
	}

	m.mode = modeEdit
	if strings.TrimSpace(m.View()) == "" {
		t.Error("the editor rendered nothing")
	}
}

func TestViewSurvivesATinyTerminal(t *testing.T) {
	m := testModel(t)
	m.width, m.height = 20, 8

	for v := view(0); v < numViews; v++ {
		m.view = v
		// Must not panic on a cramped layout.
		_ = m.View()
	}
}

func TestViewShowsTheFormula(t *testing.T) {
	m := testModel(t)
	out := m.View()

	if !strings.Contains(out, "sin(p)") {
		t.Error("the equation is not shown on screen")
	}
	if !strings.Contains(out, "One") {
		t.Error("the instrument name is not shown on screen")
	}
}

func TestWindowSizeMakesTheModelReady(t *testing.T) {
	insts := []*instrument.Instrument{
		instrument.MustCompile(instrument.Def{Name: "One", Expr: "sin(p)"}),
	}
	m := New(synth.NewEngine(insts[0], 44100), insts)

	if strings.TrimSpace(m.View()) == "" {
		t.Error("the pre-size view should still show something")
	}

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if !next.(Model).ready {
		t.Error("a window size message should mark the model ready")
	}
}

func TestTickKeepsTheTimerAlive(t *testing.T) {
	m := testModel(t)
	_, cmd := m.Update(tickMsg{})
	if cmd == nil {
		t.Error("the frame tick did not reschedule itself")
	}
}

func TestKeyboardRowsAligned(t *testing.T) {
	m := testModel(t)
	kb := m.keyboardRows()
	lines := strings.Split(kb, "\n")
	if len(lines) < 12 {
		t.Fatalf("expected at least 12 lines for keyboard rows, got %d", len(lines))
	}
	// All rows should be aligned at the left margin without staggered indentation
	for r := 0; r < 12; r++ {
		if strings.HasPrefix(lines[r], " ") {
			t.Errorf("line %d (%q) should not have leading spaces, buttons should be aligned", r, lines[r])
		}
	}
}

func TestHoldTwoButtons(t *testing.T) {
	p, e, clock := testPlayer(t)

	// User presses 'a' and 's'
	p.Press("a", false)
	p.Press("s", false)

	if !p.Down("a") || !p.Down("s") {
		t.Fatal("both keys should be held initially")
	}

	// On a real keyboard, only the last key 's' receives auto-repeats.
	// Advance clock past holdTimeout (e.g. 1 second).
	for range 20 {
		*clock = clock.Add(50 * time.Millisecond)
		p.Press("s", false) // only 's' repeats!
		p.Tick()
	}

	// BOTH 'a' and 's' must still be held!
	if !p.Down("a") {
		t.Error("key 'a' was released while 's' was still being held!")
	}
	if !p.Down("s") {
		t.Error("key 's' should still be held")
	}
	if e.ActiveVoices() != 2 {
		t.Errorf("expected 2 active voices, got %d", e.ActiveVoices())
	}

	// Now user releases both keys (no more repeats).
	*clock = clock.Add(holdTimeout + 50*time.Millisecond)
	p.Tick()

	if p.Down("a") || p.Down("s") {
		t.Error("all keys should be released once the keyboard goes quiet")
	}
}

func TestParamLineSingleLine(t *testing.T) {
	m := testModel(t)
	m.selectInstrument(2) // instrument Three has params
	line := m.paramLine()
	if strings.Contains(line, "\n") {
		t.Errorf("paramLine should be a single line, got:\n%s", line)
	}
	if strings.Contains(line, "╭") || strings.Contains(line, "│") {
		t.Errorf("paramLine should not contain box-drawing border characters, got:\n%s", line)
	}
}

func TestFooterFitsStandardTerminal(t *testing.T) {
	m := testModel(t)
	footer := m.footer()
	for _, l := range strings.Split(footer, "\n") {
		// Visible rune count should be <= 80
		if len([]rune(l)) > 80 {
			t.Errorf("footer line exceeds 80 characters (%d): %q", len([]rune(l)), l)
		}
	}
}

func TestCenteredView(t *testing.T) {
	m := testModel(t)
	m.width, m.height = 100, 35
	m.ready = true
	t.Logf("\n=== CENTERED VIEW 100x35 ===\n%s\n", m.View())
}

func TestOscilloscopeStreaming(t *testing.T) {
	m := testModel(t)
	m.width, m.height = 100, 35
	m.ready = true

	// Play note 'a' (440 Hz)
	m = press(m, "a")
	streamBuf := make([][2]float64, 512)
	for i := 0; i < 4; i++ {
		m.engine.Stream(streamBuf)
	}

	scopeOutput := m.scopePlot(m.vizSize())
	t.Logf("\n=== OSCILLOSCOPE STREAMING ===\n%s\n", scopeOutput)

	// Release note
	m = press(m, "space")
	for i := 0; i < 3; i++ {
		m.engine.Stream(streamBuf)
	}

	silentScope := m.scopePlot(m.vizSize())
	t.Logf("\n=== OSCILLOSCOPE SILENT ===\n%s\n", silentScope)
}
















