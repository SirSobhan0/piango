package tui

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/SirSobhan0/piango/v2/internal/dsp"
	"github.com/SirSobhan0/piango/v2/internal/mathexpr"
	"github.com/SirSobhan0/piango/v2/internal/tuning"
)

// View renders the whole screen.
func (m Model) View() string {
	if !m.ready {
		return "Starting piango..."
	}
	if m.mode == modeHelp {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.helpScreen())
	}

	body := lipgloss.JoinVertical(lipgloss.Center,
		m.header(),
		m.visualizer(),
		m.readout(),
		m.keyboardRows(),
		m.footer(),
	)

	if m.mode == modeEdit {
		body = lipgloss.JoinVertical(lipgloss.Center, body, m.editorPanel())
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, body)
}

func (m Model) header() string {
	in := m.inst()

	title := titleStyle.Render("piango")
	name := instStyle.Render(fmt.Sprintf("%d/%d  %s", m.current+1, len(m.insts), in.Name))

	items := []string{title, "  ", name}
	if m.notice != "" {
		items = append(items, "  ", okStyle.Render(m.notice))
	}
	line := lipgloss.JoinHorizontal(lipgloss.Center, items...)

	return lipgloss.JoinVertical(lipgloss.Left,
		line,
		formulaStyle.Render("  y = "+in.Display()),
		aboutStyle.Render("  "+in.About),
	)
}

// vizSize picks a plot size that fits the terminal without crowding the rest
// of the layout.
func (m Model) vizSize() (w, h int) {
	w = m.width - 6
	if w < 20 {
		w = 20
	}
	if w > 100 {
		w = 100
	}

	h = (m.height - 26) / 2
	if h < 4 {
		h = 4
	}
	if h > 10 {
		h = 10
	}
	return w, h
}

func (m Model) visualizer() string {
	w, h := m.vizSize()

	var body, style = "", waveStyle
	switch m.view {
	case viewScope:
		body = m.scopePlot(w, h)
	case viewSpectrum:
		body, style = m.spectrumPlot(w, h), specStyle
	case viewLissajous:
		body, style = m.lissajousPlot(w, h), lissStyle
	case viewHarmonics:
		body, style = m.harmonicPlot(w, h), specStyle
	}

	head := panelTitleStyle.Render(m.view.String())
	return panelStyle.Width(w + 2).Render(
		lipgloss.JoinVertical(lipgloss.Left, head, style.Render(body)),
	)
}

// scopePlot draws the live output waveform, aligned on a zero crossing so the
// trace holds still instead of scrolling.
func (m Model) scopePlot(w, h int) string {
	buf := m.engine.Scope().Snapshot()
	if len(buf) < 2 {
		return blank(w, h)
	}

	start := 0
	if len(buf) > scopeWidth {
		searchLen := min(len(buf)-scopeWidth, 1024)
		tail := buf[len(buf)-scopeWidth-searchLen:]
		offset := dsp.FindZeroCrossing(tail, searchLen)
		start = len(buf) - scopeWidth - searchLen + offset
	}
	end := min(start+scopeWidth, len(buf))
	seg := buf[start:end]

	c := NewCanvas(w, h)
	Plot(c, seg)
	return strings.Join(c.Rows(), "\n")
}

// spectrumPlot shows the real measured spectrum of the audio output. The
// original build faked this from note frequencies; this is an actual FFT.
func (m Model) spectrumPlot(w, h int) string {
	buf := m.engine.Scope().Snapshot()
	if len(buf) < fftSize {
		return blank(w, h)
	}

	frame := buf[len(buf)-fftSize:]
	spec := dsp.Spectrum(frame, m.window)

	bands := dsp.LogBands(spec, m.engine.SampleRate(), w, 40, 12000)

	// Convert to decibels so quiet harmonics stay visible.
	for i, v := range bands {
		bands[i] = clamp01((dsp.ToDB(v, -60) + 60) / 60)
	}

	return strings.Join(Bars(m.spectrum.Update(bands), h), "\n")
}

// lissajousPlot draws one held note against another. Simple frequency ratios
// trace closed figures, which makes consonance visible.
func (m Model) lissajousPlot(w, h int) string {
	freqs := m.player.Held()
	if len(freqs) < 2 {
		return centered(w, h, "hold two notes")
	}

	fx, fy := freqs[0], freqs[1]
	const n = 1024
	xs := make([]float64, n)
	ys := make([]float64, n)

	// Sweep enough cycles of the first note to close the figure. A p:q ratio
	// closes after q cycles.
	iv := tuning.Analyze(math.Min(fx, fy), math.Max(fx, fy))
	cycles := float64(min(max(iv.Den, 1), 8))
	ratio := fy / fx

	for i := range n {
		u := float64(i) / float64(n-1) * cycles
		xs[i] = math.Sin(2 * math.Pi * u)
		ys[i] = math.Sin(2 * math.Pi * u * ratio)
	}

	c := NewCanvas(w, h)
	PlotXY(c, xs, ys)
	return strings.Join(c.Rows(), "\n")
}

// harmonicPlot shows the theoretical harmonic series of the current equation,
// computed by analysis rather than measured from audio.
func (m Model) harmonicPlot(w, h int) string {
	n := w / 2
	if n < 1 {
		n = 1
	}
	h2 := m.inst().Harmonics(n)

	// Widen each harmonic to two columns so the comb is readable.
	wide := make([]float64, 0, w)
	for _, v := range h2 {
		wide = append(wide, v, 0)
	}
	for len(wide) < w {
		wide = append(wide, 0)
	}
	return strings.Join(Bars(wide, h), "\n")
}

func (m Model) readout() string {
	in := m.inst()
	sys := m.board.System()

	left := []string{
		row("Tuning", sys.Name()),
		row("Formula", sys.Formula()),
		row("Octave", fmt.Sprintf("%+d", m.board.Octave())),
		row("Voices", fmt.Sprintf("%d", m.engine.ActiveVoices())),
	}

	synthesis := "direct"
	if in.Tabulated() {
		synthesis = "wavetable"
	}
	env := in.Envelope
	right := []string{
		row("Synthesis", synthesis),
		row("ADSR", fmt.Sprintf("%.0f/%.0f/%.0f%%/%.0f ms",
			env.Attack*1000, env.Decay*1000, env.Sustain*100, env.Release*1000)),
		row("Master", Meter(m.engine.Master(), 12)),
		row("Output", Meter(m.engine.Peak(), 12)),
	}

	cols := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(38).Render(strings.Join(left, "\n")),
		lipgloss.NewStyle().Width(40).Render(strings.Join(right, "\n")),
	)

	return lipgloss.JoinVertical(lipgloss.Left, cols, m.paramLine(), m.intervalLine())
}

// paramLine shows the four tweakable coefficients, marking the selected one.
func (m Model) paramLine() string {
	in := m.inst()
	if len(in.Params) == 0 {
		return labelStyle.Render("  params    ") + aboutStyle.Render("this equation has none")
	}

	parts := make([]string, 0, len(in.Params))
	for i, p := range in.Params {
		txt := fmt.Sprintf("%s=%.3f", p.Name, p.Value)
		if i == m.paramSel {
			txt = activeParamStyle.Render(" " + txt + " ")
		} else {
			txt = valueStyle.Render(" " + txt + " ")
		}
		parts = append(parts, txt)
	}
	return labelStyle.Render("  params    ") + strings.Join(parts, " ")
}

// intervalLine names the interval between the two lowest held notes as an
// exact ratio, so the harmony is legible as math.
func (m Model) intervalLine() string {
	freqs := m.player.Held()
	if len(freqs) < 2 {
		return labelStyle.Render("  interval  ") + aboutStyle.Render("hold two notes to measure")
	}

	lo, hi := freqs[0], freqs[1]
	if lo > hi {
		lo, hi = hi, lo
	}
	iv := tuning.Analyze(lo, hi)
	return labelStyle.Render("  interval  ") + valueStyle.Render(fmt.Sprintf("%s   %.1f Hz : %.1f Hz   %.1f cents",
		iv, lo, hi, iv.Cents))
}

func (m Model) keyboardRows() string {
	rows := make([]string, 0, 3)
	for _, r := range m.board.Rows {
		cells := make([]string, 0, len(r))
		for _, k := range r {
			label := fmt.Sprintf("%s\n%s", strings.ToUpper(k.Char), k.Name)
			st := keyStyle
			if m.player.Down(k.Char) {
				st = activeKeyStyle
			}
			cells = append(cells, st.Render(label))
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (m Model) footer() string {
	lines := []string{
		"TAB inst • 1-0 preset • SHIFT+1-0 save • ←→ octave • ↑↓ param • ; next param",
		"[ ] viz • \\ tuning • = editor • - + volume • / help • SPACE panic • ESC quit",
	}
	return helpStyle.Render(strings.Join(lines, "\n"))
}

func (m Model) editorPanel() string {
	status := okStyle.Render("valid")
	if m.editErr != "" {
		status = errStyle.Render(m.editErr)
	}

	body := lipgloss.JoinVertical(lipgloss.Left,
		panelTitleStyle.Render("Equation")+aboutStyle.Render("   variables: p t f env vel sr a b c d noise"),
		m.editor.View(),
		status,
		helpStyle.Render("ENTER play • CTRL+S save to config • ESC cancel"),
	)
	return editorStyle.Render(body)
}

func (m Model) helpScreen() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("piango function reference") + "\n\n")

	b.WriteString(panelTitleStyle.Render("Variables") + "\n")
	for _, v := range [][2]string{
		{"p", "phase, 0..2pi through one cycle"},
		{"x", "phase normalized to 0..1"},
		{"t", "time in seconds since the note started"},
		{"f", "note frequency in Hz"},
		{"env", "envelope level, 0..1"},
		{"vel", "key velocity, 0..1"},
		{"sr", "sample rate"},
		{"a b c d", "the four tweakable parameters"},
		{"noise", "uniform random in -1..1"},
	} {
		b.WriteString(fmt.Sprintf("  %-10s %s\n", v[0], aboutStyle.Render(v[1])))
	}

	b.WriteString("\n" + panelTitleStyle.Render("Aggregates") + "\n")
	b.WriteString(aboutStyle.Render("  sum(k, from, to, body)    prod(k, from, to, body)") + "\n")
	b.WriteString(aboutStyle.Render("  example: (4/pi) sum(k,0,24, sin((2k+1)p)/(2k+1))") + "\n")

	b.WriteString("\n" + panelTitleStyle.Render("Functions") + "\n")
	b.WriteString(wrapWords(functionNames(), 74, "  "))

	b.WriteString("\n" + helpStyle.Render("press / or ESC to go back"))
	return b.String()
}

func row(label, value string) string {
	return labelStyle.Render(fmt.Sprintf("  %-10s", label)) + valueStyle.Render(value)
}

func blank(w, h int) string {
	return strings.Join(NewCanvas(w, h).Rows(), "\n")
}

// centered renders a short hint in the middle of an otherwise empty plot area.
func centered(w, h int, text string) string {
	lines := make([]string, h)
	mid := h / 2
	pad := max((w-len(text))/2, 0)
	for i := range lines {
		if i == mid {
			lines[i] = strings.Repeat(" ", pad) + aboutStyle.Render(text)
		}
	}
	return strings.Join(lines, "\n")
}

// functionNames lists the built-in function names for the help screen.
func functionNames() []string {
	fns := mathexpr.Functions()
	out := make([]string, 0, len(fns))
	for _, f := range fns {
		out = append(out, f.Name)
	}
	sort.Strings(out)
	return out
}

func wrapWords(words []string, width int, indent string) string {
	var b strings.Builder
	line := indent
	for _, w := range words {
		if len(line)+len(w)+1 > width {
			b.WriteString(line + "\n")
			line = indent
		}
		line += w + " "
	}
	if strings.TrimSpace(line) != "" {
		b.WriteString(line + "\n")
	}
	return b.String()
}
