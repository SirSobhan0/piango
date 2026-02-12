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

type Oscillator func(phase float64) float64

type Instrument struct {
	Name string
	Osc  Oscillator
}

var instruments = []Instrument{
	{Name: "Electric Piano", Osc: oscPiano},
	{Name: "Retro Square", Osc: oscSquare},
	{Name: "FM Metallic", Osc: oscFM},
	{Name: "Distorted Lead", Osc: oscDistortion},
	{Name: "Glass Bell", Osc: oscBell},
	{Name: "Cyberpunk Crunch", Osc: oscBitcrush},
	{Name: "Alien Ring Mod", Osc: oscAlien},
	{Name: "Hollow Choir", Osc: oscGhost},
	{Name: "Acid Wavefolder", Osc: oscWavefolder},
	{Name: "808 Sub Bass", Osc: oscSubBass},
	{Name: "PWM Pad", Osc: oscPWM},
	{Name: "Accordion", Osc: oscAccordion},
	{Name: "Noise", Osc: oscNoise},
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

func oscFM(p float64) float64 {
	modulator := math.Sin(p*3.14) * 2.0
	return math.Sin(p+modulator) * 0.2
}

func oscDistortion(p float64) float64 {
	val := math.Sin(p) * 5.0
	if val > 1.0 {
		val = 1.0
	} else if val < -1.0 {
		val = -1.0
	}
	return val * 0.08
}

func oscBell(p float64) float64 {
	v1 := math.Sin(p)
	v2 := math.Sin(p*2.76) * 0.6
	v3 := math.Sin(p*5.4) * 0.4
	v4 := math.Sin(p*8.9) * 0.2
	return (v1 + v2 + v3 + v4) * 0.1
}

func oscBitcrush(p float64) float64 {
	norm := p / (2 * math.Pi)
	saw := 2.0*norm - 1.0
	steps := 4.0
	crushed := math.Floor(saw*steps) / steps
	return crushed * 0.15
}

func oscAlien(p float64) float64 {
	norm := p / (2 * math.Pi)
	tri := 2.0*math.Abs(2.0*norm-1.0) - 1.0
	ring := math.Sin(p * 5.67)
	return tri * ring * 0.25
}

func oscGhost(p float64) float64 {
	v1 := math.Sin(p)
	v3 := math.Sin(p*3.0) * 0.3
	v5 := math.Sin(p*5.0) * 0.1
	return (v1 + v3 + v5) * math.Cos(p*0.5) * 0.2
}

func oscWavefolder(p float64) float64 {
	val := math.Sin(p) * 3.0
	return math.Sin(val) * 0.2
}

func oscSubBass(p float64) float64 {
	val := math.Sin(p) * 1.5
	return math.Tanh(val) * 0.3
}

func oscPWM(p float64) float64 {
	norm := p / (2 * math.Pi)
	saw1 := 2.0*norm - 1.0

	p2 := math.Mod(p+1.5, 2*math.Pi)
	norm2 := p2 / (2 * math.Pi)
	saw2 := 2.0*norm2 - 1.0

	return (saw1 - saw2) * 0.1
}

func oscAccordion(p float64) float64 {
	v1 := math.Sin(p)
	v2 := math.Sin(p*2.0) * 0.5
	v3 := math.Sin(p*3.0) * 0.8
	v4 := math.Sin(p*4.0) * 0.2
	v5 := math.Sin(p*5.0) * 0.4
	return (v1 + v2 + v3 + v4 + v5) * 0.15
}

func oscNoise(p float64) float64 {
	rand.NewSource(int64(p))
	return (rand.Float64()*2.0 - 1.0) * 0.1
}

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
			v.staccato = staccato
			v.streamer.Sustain()
			return
		}
		v.streamer.Stop()
	}

	decay := 0.001
	if staccato {
		decay = 0.05
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
	activeKeys  map[string]bool
	instName    string
	width       int
	height      int
	spectrum    []float64
	octaveShift int
}

const numBars = 42

func initialModel() model {
	return model{
		activeKeys:  make(map[string]bool),
		instName:    instruments[0].Name,
		spectrum:    make([]float64, numBars),
		octaveShift: 0,
	}
}

func tick() tea.Cmd {
	return tea.Tick(time.Millisecond*30, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

func (m model) Init() tea.Cmd { return tick() }

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

		for i := range m.spectrum {
			m.spectrum[i] *= 0.82
		}

		for k, v := range voices {
			if !v.streamer.finished {
				newActive[k] = true
				if note, ok := noteMap[k]; ok {
					shiftedFreq := note.Freq * math.Pow(2.0, float64(m.octaveShift))

					b1 := freqToBucket(shiftedFreq)
					m.spectrum[b1] = 1.0

					if b2 := freqToBucket(shiftedFreq * 2.0); b2 < numBars {
						m.spectrum[b2] += 0.5
					}
					if b3 := freqToBucket(shiftedFreq * 3.0); b3 < numBars {
						m.spectrum[b3] += 0.25
					}
					if b4 := freqToBucket(shiftedFreq * 4.0); b4 < numBars {
						m.spectrum[b4] += 0.1
					}
				}
			}
		}

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

		case tea.KeyLeft:
			if m.octaveShift > -2 {
				m.octaveShift--
			}
			return m, nil

		case tea.KeyRight:
			if m.octaveShift < 2 {
				m.octaveShift++
			}
			return m, nil
		}

		input := msg.String()
		lowerInput := strings.ToLower(input)
		isStaccato := input != lowerInput

		if note, ok := noteMap[lowerInput]; ok {
			shiftedFreq := note.Freq * math.Pow(2.0, float64(m.octaveShift))
			updateVoice(lowerInput, shiftedFreq, isStaccato)
		}
	}
	return m, nil
}

var (
	panelStyle = lipgloss.NewStyle().
			Padding(1, 3).
			Border(lipgloss.ThickBorder()).
			BorderForeground(lipgloss.Color("#444444"))

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			MarginBottom(1).
			Padding(0, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#00E6C3"))

	instStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00E6C3")).
			Background(lipgloss.Color("#111111")).
			Padding(0, 1).
			MarginBottom(1)

	visStyle = lipgloss.NewStyle().
			MarginBottom(2)

	waveColor = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00E6C3"))

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

	octStr := fmt.Sprintf("%+d", m.octaveShift)
	if m.octaveShift == 0 {
		octStr = " 0"
	}

	header := lipgloss.JoinHorizontal(lipgloss.Center,
		titleStyle.Render("🎹 PIANGO"),
		"   ",
		instStyle.Render("Preset: "+m.instName),
		"   ",
		instStyle.Render("Octave: "+octStr),
	)

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
			} else if r > 0 {
				if h >= absR {
					line += "█"
				} else if h >= absR-0.5 {
					line += "▄"
				} else {
					line += " "
				}
			} else {
				if h >= absR {
					line += "█"
				} else if h >= absR-0.5 {
					line += "▀"
				} else {
					line += " "
				}
			}
			line += " "
		}
		visLines = append(visLines, waveColor.Render(line))
	}
	visualizer := visStyle.Render(strings.Join(visLines, "\n"))

	var rowsStr []string
	rowLabels := []string{"High", "Mid ", "Low "}

	for i, rowNotes := range sortedRows {
		var renderedKeys []string

		label := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6272A4")).
			Width(6).
			Align(lipgloss.Right).
			MarginRight(2).
			MarginTop(1).
			Render(fmt.Sprintf("\n%s", rowLabels[i]))

		renderedKeys = append(renderedKeys, label)

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

	help := helpStyle.Render("TAB: Instrument  •  LEFT/RIGHT: Octave  •  SHIFT+KEY: Fast End  •  SPACE: Silence  •  ESC: Quit")

	ui := lipgloss.JoinVertical(lipgloss.Center, header, visualizer, keyboard, help)
	panel := panelStyle.Render(ui)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel)
}

func main() {
	speaker.Init(sampleRate, sampleRate.N(50*time.Millisecond))
	speaker.Play(mixer)
	initNotes()

	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
	}
}
