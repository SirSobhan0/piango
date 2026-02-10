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

var (
	mixer      = &beep.Mixer{}
	sampleRate = beep.SampleRate(44100)
	voiceLock  sync.Mutex
	voices     = make(map[string]*ActiveVoice)
)

type ActiveVoice struct {
	streamer *PianoStreamer
	lastSeen time.Time
}

type PianoStreamer struct {
	freq      float64
	phase     float64
	vol       float64
	releasing bool
	finished  bool
}

func (s *PianoStreamer) Stream(samples [][2]float64) (n int, ok bool) {
	const twoPi = 2 * math.Pi
	step := s.freq * twoPi / float64(sampleRate)

	attackSpeed := 0.1
	decaySpeed := 0.001

	for i := range samples {
		v1 := math.Sin(s.phase)
		v2 := math.Sin(s.phase*2.0) * 0.5
		v3 := math.Sin(s.phase*3.0) * 0.2
		raw := (v1 + v2 + v3) * 0.15

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
		if s.phase > twoPi {
			s.phase -= twoPi
		}
	}
	return len(samples), true
}

func (s *PianoStreamer) Err() error { return nil }
func (s *PianoStreamer) Stop()      { s.releasing = true }
func (s *PianoStreamer) Sustain()   { s.releasing = false; s.finished = false }

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

	s := &PianoStreamer{freq: freq, vol: 0}
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
}

func initialModel() model {
	return model{activeKeys: make(map[string]bool)}
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
		}

		if note, ok := noteMap[msg.String()]; ok {
			updateVoice(msg.String(), note.Freq)
		}
	}
	return m, nil
}

func (m model) View() string {
	s := "\n  🎹 PIANGO\n"
	s += "  ========\n\n"

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

	s += "  [ SPACE: Silence ]    [ ESC: Quit ]\n"
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
