package main

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/speaker"
)

// --- 1. INSTRUMENT DEFINITIONS ---

type Oscillator func(phase float64) float64

type Instrument struct {
	Name string
	Osc  Oscillator
}

var instruments = []Instrument{
	{Name: "Electric Piano", Osc: oscPiano},
	{Name: "8-Bit Square", Osc: oscSquare},
	{Name: "Synth Saw", Osc: oscSaw},
	{Name: "Soft Flute", Osc: oscTriangle},
	{Name: "Church Organ", Osc: oscOrgan},
	{Name: "Pure Sine", Osc: oscSine},
	{Name: "Gameboy Pulse", Osc: oscPulse},
	{Name: "Sci-Fi Noise", Osc: oscNoise},
}

func oscPiano(p float64) float64 {
	v1 := math.Sin(p)
	v2 := math.Sin(p*2.0) * 0.5
	v3 := math.Sin(p*3.0) * 0.2
	return (v1 + v2 + v3) * 0.15
}

func oscSquare(p float64) float64 {
	if math.Sin(p) >= 0 {
		return 0.1
	}
	return -0.1
}

func oscSaw(p float64) float64 {
	norm := p / (2 * math.Pi)
	return (2.0*norm - 1.0) * 0.1
}

func oscTriangle(p float64) float64 {
	norm := p / (2 * math.Pi)
	return (2.0*math.Abs(2.0*norm-1.0) - 1.0) * 0.2
}

func oscOrgan(p float64) float64 {
	v1 := math.Sin(p) * 1.0
	v2 := math.Sin(p*2.0) * 0.5
	v3 := math.Sin(p*4.0) * 0.25
	v4 := math.Sin(p*8.0) * 0.125
	return (v1 + v2 + v3 + v4) * 0.1
}

func oscSine(p float64) float64 {
	return math.Sin(p) * 0.3
}

func oscPulse(p float64) float64 {
	if math.Mod(p, 2*math.Pi) < (math.Pi / 2) {
		return 0.1
	}
	return -0.1
}

func oscNoise(p float64) float64 {
	tone := math.Sin(p)
	noise := rand.Float64()*2.0 - 1.0
	return (tone*0.5 + noise*0.5) * 0.15
}

// --- 2. AUDIO ENGINE ---

var (
	mixer         = &beep.Mixer{}
	sampleRate    = beep.SampleRate(44100)
	voiceLock     sync.Mutex
	voices        = make(map[string]*ActiveVoice)
	currentInstID = 0
)

type ActiveVoice struct {
	streamer *SynthStreamer
	lastSeen time.Time
	staccato bool
}

type SynthStreamer struct {
	freq       float64
	phase      float64
	vol        float64
	osc        Oscillator
	decaySpeed float64
	releasing  bool
	finished   bool
}

func (s *SynthStreamer) Stream(samples [][2]float64) (n int, ok bool) {
	const twoPi = 2 * math.Pi
	step := s.freq * twoPi / float64(sampleRate)

	attackSpeed := 0.1

	for i := range samples {
		raw := s.osc(s.phase)

		if s.releasing {
			s.vol -= s.decaySpeed
			if s.vol <= 0 {
				s.vol = 0
				s.finished = true
				return i, false
			}
		} else {
			if s.vol < 1.0 {
				s.vol += attackSpeed
			}
		}

		final := raw * s.vol
		samples[i][0] = final
		samples[i][1] = final

		s.phase += step
		if s.phase >= twoPi {
			s.phase -= twoPi
		}
	}
	return len(samples), true
}

func (s *SynthStreamer) Err() error { return nil }
func (s *SynthStreamer) Stop()      { s.releasing = true }
func (s *SynthStreamer) Sustain()   { s.releasing = false; s.finished = false }

func updateVoice(key string, freq float64, staccato bool) {
	voiceLock.Lock()
	defer voiceLock.Unlock()

	now := time.Now()

	if v, ok := voices[key]; ok {
		delta := now.Sub(v.lastSeen)
		if delta < 75*time.Millisecond {
			v.lastSeen = now
			v.staccato = staccato // Update staccato state dynamically
			v.streamer.Sustain()
			return
		}
		v.streamer.Stop()
	}

	decay := 0.001
	if staccato {
		decay = 0.05 // Much faster fade out for staccato
	}

	inst := instruments[currentInstID]
	s := &SynthStreamer{freq: freq, vol: 0, osc: inst.Osc, decaySpeed: decay}
	voices[key] = &ActiveVoice{streamer: s, lastSeen: now, staccato: staccato}
	mixer.Add(s)
}

func checkWatchdog() {
	voiceLock.Lock()
	defer voiceLock.Unlock()

	now := time.Now()

	for k, v := range voices {
		// Normal sustain waits 600ms. Staccato cuts off super fast (100ms)
		threshold := 600 * time.Millisecond
		if v.staccato {
			threshold = 100 * time.Millisecond
		}

		if now.Sub(v.lastSeen) > threshold {
			v.streamer.Stop()
			if v.streamer.finished {
				delete(voices, k)
			}
		}
	}
}

// --- 3. DATA & MODEL ---

type Note struct {
	Key, Name string
	Freq      float64
}

var noteMap = map[string]Note{}
var sortedRows [3][]Note

func initNotes() {
	getFreq := func(n int) float64 {
		return 440.0 * math.Pow(2.0, float64(n)/12.0)
	}

	rows := [][]struct {
		k, n string
		s    int
	}{
		{{"q", "Do", 3}, {"w", "Re", 5}, {"e", "Mi", 7}, {"r", "Fa", 8}, {"t", "Sol", 10}, {"y", "La", 12}, {"u", "Si", 14}},
		{{"a", "Do", -9}, {"s", "Re", -7}, {"d", "Mi", -5}, {"f", "Fa", -4}, {"g", "Sol", -2}, {"h", "La", 0}, {"j", "Si", 2}},
		{{"z", "Do", -21}, {"x", "Re", -19}, {"c", "Mi", -17}, {"v", "Fa", -16}, {"b", "Sol", -14}, {"n", "La", -12}, {"m", "Si", -10}},
	}

	for i, rowData := range rows {
		var r []Note
		for _, d := range rowData {
			n := Note{d.k, d.n, getFreq(d.s)}
			noteMap[d.k] = n
			r = append(r, n)
		}
		sortedRows[i] = r
	}
}

// --- 4. TUI VISUALS & LOGIC ---

type TickMsg time.Time

type model struct {
	activeKeys map[string]bool
	instName   string
	width      int
	height     int
	spectrum   []float64
}

const numBars = 42

func initialModel() model {
	return model{
		activeKeys: make(map[string]bool),
		instName:   instruments[0].Name,
		spectrum:   make([]float64, numBars),
	}
}

func tick() tea.Cmd {
	return tea.Tick(time.Millisecond*30, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

func (m model) Init() tea.Cmd { return tick() }

// freqToBucket maps a frequency logarithmically to our visualizer bars
func freqToBucket(freq float64) int {
	minF, maxF := 100.0, 4000.0
	if freq < minF {
		freq = minF
	}
	if freq > maxF {
		freq = maxF
	}
	ratio := math.Log(freq/minF) / math.Log(maxF/minF)
	bucket := int(ratio * float64(numBars))
	if bucket >= numBars {
		bucket = numBars - 1
	}
	return bucket
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case TickMsg:
		checkWatchdog()

		voiceLock.Lock()
		newActive := make(map[string]bool)

		// 1. Decay visualizer bars
		for i := range m.spectrum {
			m.spectrum[i] *= 0.82 // 82% decay rate
		}

		// 2. Map playing notes to visualizer
		for k, v := range voices {
			if !v.streamer.finished {
				newActive[k] = true
				if note, ok := noteMap[k]; ok {
					// Excite fundamental frequency
					b1 := freqToBucket(note.Freq)
					m.spectrum[b1] = 1.0

					// Excite harmonics (fakes an FFT spectrum)
					if b2 := freqToBucket(note.Freq * 2.0); b2 < numBars {
						m.spectrum[b2] += 0.5
					}
					if b3 := freqToBucket(note.Freq * 3.0); b3 < numBars {
						m.spectrum[b3] += 0.25
					}
					if b4 := freqToBucket(note.Freq * 4.0); b4 < numBars {
						m.spectrum[b4] += 0.1
					}
				}
			}
		}

		// 3. Cap spectrum max
		for i := range m.spectrum {
			if m.spectrum[i] > 1.0 {
				m.spectrum[i] = 1.0
			}
		}

		voiceLock.Unlock()
		m.activeKeys = newActive
		return m, tick()

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEscape:
			return m, tea.Quit

		case tea.KeySpace:
			speaker.Clear()
			mixer = &beep.Mixer{}
			speaker.Play(mixer)
			voiceLock.Lock()
			voices = make(map[string]*ActiveVoice)
			voiceLock.Unlock()
			return m, nil

		case tea.KeyTab:
			voiceLock.Lock()
			currentInstID++
			if currentInstID >= len(instruments) {
				currentInstID = 0
			}
			m.instName = instruments[currentInstID].Name
			voiceLock.Unlock()
			return m, nil
		}

		input := msg.String()
		lowerInput := strings.ToLower(input)
		isStaccato := input != lowerInput // True if Shift is held down

		if note, ok := noteMap[lowerInput]; ok {
			updateVoice(lowerInput, note.Freq, isStaccato)
		}
	}
	return m, nil
}

// --- STYLES ---
var (
	panelStyle = lipgloss.NewStyle().
			Padding(1, 3).
			Border(lipgloss.ThickBorder()).
			BorderForeground(lipgloss.Color("#444444")) // Sleek grey

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			MarginBottom(1).
			Padding(0, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#00E6C3")) // Cyan accent

	instStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00E6C3")).
			Background(lipgloss.Color("#111111")).
			Padding(0, 1).
			MarginBottom(1)

	visStyle = lipgloss.NewStyle().
			MarginBottom(2)

	waveColor = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00E6C3")) // Unified cyan theme

	keyStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#333333")).
			Foreground(lipgloss.Color("#AAAAAA")).
			Width(7).
			Height(3).
			Align(lipgloss.Center)

	activeKeyStyle = keyStyle.Copy().
			BorderForeground(lipgloss.Color("#00E6C3")).
			Foreground(lipgloss.Color("#000000")).
			Background(lipgloss.Color("#00E6C3")).
			Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")).
			MarginTop(2)
)

func (m model) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	// 1. Header
	header := lipgloss.JoinHorizontal(lipgloss.Center,
		titleStyle.Render("🎹 PIANGO"),
		"   ",
		instStyle.Render("Preset: "+m.instName),
	)

	// 2. Wave Visualizer (Symmetrical)
	var visLines []string
	for r := 3; r >= -3; r-- {
		line := ""
		for _, val := range m.spectrum {
			h := val * 3.0
			absR := float64(math.Abs(float64(r)))

			if r == 0 {
				if h > 0.1 {
					line += "█"
				} else {
					line += "━"
				}
			} else if r > 0 { // Top half
				if h >= absR {
					line += "█"
				} else if h >= absR-0.5 {
					line += "▄"
				} else {
					line += " "
				}
			} else { // Bottom half
				if h >= absR {
					line += "█"
				} else if h >= absR-0.5 {
					line += "▀"
				} else {
					line += " "
				}
			}
			line += " " // spacing between bars
		}
		visLines = append(visLines, waveColor.Render(line))
	}
	visualizer := visStyle.Render(strings.Join(visLines, "\n"))

	// 3. Keys
	var rowsStr []string
	rowLabels := []string{"High", "Mid ", "Low "}

	for i, rowNotes := range sortedRows {
		var renderedKeys []string

		// Row Label
		label := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6272A4")).
			Width(6).
			Align(lipgloss.Right).
			MarginRight(2).
			MarginTop(1).
			Render(fmt.Sprintf("\n%s", rowLabels[i]))

		renderedKeys = append(renderedKeys, label)

		// Keys
		for _, n := range rowNotes {
			keyContent := fmt.Sprintf("%s\n%s", n.Name, strings.ToUpper(n.Key))
			if m.activeKeys[n.Key] {
				renderedKeys = append(renderedKeys, activeKeyStyle.Render(keyContent))
			} else {
				renderedKeys = append(renderedKeys, keyStyle.Render(keyContent))
			}
		}
		rowsStr = append(rowsStr, lipgloss.JoinHorizontal(lipgloss.Top, renderedKeys...))
	}
	keyboard := lipgloss.JoinVertical(lipgloss.Left, rowsStr...)

	// 4. Footer
	help := helpStyle.Render("TAB: Instrument  •  SHIFT+KEY: Fast End  •  SPACE: Silence  •  ESC: Quit")

	// 5. Assemble and Center
	ui := lipgloss.JoinVertical(lipgloss.Center, header, visualizer, keyboard, help)
	panel := panelStyle.Render(ui)

	// Place in the dead center of the terminal screen
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel)
}

func main() {
	speaker.Init(sampleRate, sampleRate.N(50*time.Millisecond))
	speaker.Play(mixer)
	initNotes()

	// Run Bubble Tea with the Alt Screen (Full-Screen) flag
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
	}
}
