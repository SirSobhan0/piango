package main

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
}

// -- Waveform Math --

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
}

type SynthStreamer struct {
	freq      float64
	phase     float64
	vol       float64
	osc       Oscillator
	releasing bool
	finished  bool
}

func (s *SynthStreamer) Stream(samples [][2]float64) (n int, ok bool) {
	const twoPi = 2 * math.Pi
	step := s.freq * twoPi / float64(sampleRate)

	attackSpeed := 0.1
	decaySpeed := 0.001

	for i := range samples {
		raw := s.osc(s.phase)

		if s.releasing {
			s.vol -= decaySpeed
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

func updateVoice(key string, freq float64) {
	voiceLock.Lock()
	defer voiceLock.Unlock()

	now := time.Now()

	if v, ok := voices[key]; ok {
		delta := now.Sub(v.lastSeen)

		if delta < 75*time.Millisecond {
			v.lastSeen = now
			v.streamer.Sustain()
			return
		}
		v.streamer.Stop()
	}

	inst := instruments[currentInstID]

	s := &SynthStreamer{
		freq: freq,
		vol:  0,
		osc:  inst.Osc,
	}

	voices[key] = &ActiveVoice{streamer: s, lastSeen: now}
	mixer.Add(s)
}

func checkWatchdog() {
	voiceLock.Lock()
	defer voiceLock.Unlock()

	now := time.Now()
	threshold := 600 * time.Millisecond

	for k, v := range voices {
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

type TickMsg time.Time

type model struct {
	activeKeys map[string]bool
	instName   string
}

func initialModel() model {
	return model{
		activeKeys: make(map[string]bool),
		instName:   instruments[0].Name,
	}
}

func tick() tea.Cmd {
	return tea.Tick(time.Millisecond*30, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

func (m model) Init() tea.Cmd { return tick() }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case TickMsg:
		checkWatchdog()

		voiceLock.Lock()
		newActive := make(map[string]bool)
		for k, v := range voices {
			if !v.streamer.finished {
				newActive[k] = true
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

		// --- INSTRUMENT CHANGE ---
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

		if note, ok := noteMap[msg.String()]; ok {
			updateVoice(msg.String(), note.Freq)
		}
	}
	return m, nil
}

func (m model) View() string {
	s := "\n  🎹 PIANGO - " + m.instName + "\n"
	s += "  " + strings.Repeat("=", len(s)-3) + "\n\n"

	drawRow := func(notes []Note) string {
		res := "  "
		for _, n := range notes {
			if m.activeKeys[n.Key] {
				res += fmt.Sprintf("[\x1b[32;1m %s \x1b[0m] ", strings.ToUpper(n.Name))
			} else {
				res += fmt.Sprintf("[ %s ] ", n.Name)
			}
		}
		return res
	}

	s += "  High: " + drawRow(sortedRows[0]) + "\n\n"
	s += "  Mid : " + drawRow(sortedRows[1]) + "\n\n"
	s += "  Low : " + drawRow(sortedRows[2]) + "\n\n\n"

	s += "  [ TAB: Change Inst ]  [ SPACE: Silence ]  [ ESC: Quit ]\n"
	return s
}

func main() {
	speaker.Init(sampleRate, sampleRate.N(50*time.Millisecond))
	speaker.Play(mixer)
	initNotes()
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
	}
}
