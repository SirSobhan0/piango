// Command piango is a terminal synthesizer where every instrument is an
// equation. It plays live from the computer keyboard, shows what the math is
// doing, and can render equations to WAV offline.
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/speaker"

	"github.com/SirSobhan0/piango/internal/instrument"
	"github.com/SirSobhan0/piango/internal/mathexpr"
	"github.com/SirSobhan0/piango/internal/synth"
	"github.com/SirSobhan0/piango/internal/tui"
)

const (
	sampleRate = 44100
	bufferSize = 512 // about 12 ms of latency
)

type options struct {
	eval     string
	render   string
	duration float64
	freq     float64
	list     bool
	funcs    bool
	example  bool
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "piango:", err)
		os.Exit(1)
	}
}

func run() error {
	var o options
	flag.StringVar(&o.eval, "eval", "", "play or render this equation instead of a built-in instrument")
	flag.StringVar(&o.render, "render", "", "render to this WAV file instead of playing live")
	flag.Float64Var(&o.duration, "duration", 2, "note length in seconds when rendering")
	flag.Float64Var(&o.freq, "freq", 440, "note frequency in Hz when rendering")
	flag.BoolVar(&o.list, "list", false, "list the available instruments and exit")
	flag.BoolVar(&o.funcs, "functions", false, "list the equation language's functions and exit")
	flag.BoolVar(&o.example, "write-example", false, "write a commented example config file and exit")
	flag.Usage = usage
	flag.Parse()

	switch {
	case o.funcs:
		printFunctions()
		return nil
	case o.example:
		return writeExample()
	}

	insts, err := loadInstruments(o.eval)
	if err != nil {
		return err
	}

	if o.list {
		printInstruments(insts)
		return nil
	}
	if o.render != "" {
		return renderFile(o, insts[0])
	}
	return play(insts)
}

func usage() {
	out := flag.CommandLine.Output()
	fmt.Fprintln(out, "piango - a terminal synthesizer built on equations")
	fmt.Fprintln(out, "\nUsage:")
	fmt.Fprintln(out, "  piango                              play live")
	fmt.Fprintln(out, `  piango -eval "sin(p)+0.3sin(3p)"    play a custom equation`)
	fmt.Fprintln(out, `  piango -eval "saw(x)" -render o.wav  render one note to a file`)
	fmt.Fprintln(out, "\nFlags:")
	flag.PrintDefaults()
}

// loadInstruments returns the built-ins plus anything in the user's config
// file. If eval is set, that equation is placed first so it is selected on
// start.
func loadInstruments(eval string) ([]*instrument.Instrument, error) {
	insts := instrument.Builtins()

	user, problems, err := instrument.LoadUser()
	if err != nil {
		return nil, err
	}
	for _, p := range problems {
		fmt.Fprintln(os.Stderr, "piango: skipping instrument:", p)
	}
	insts = append(insts, user...)

	if eval != "" {
		in, err := instrument.Compile(instrument.Def{Name: "Command line", Expr: eval})
		if err != nil {
			return nil, err
		}
		insts = append([]*instrument.Instrument{in}, insts...)
	}
	return insts, nil
}

func printInstruments(insts []*instrument.Instrument) {
	for _, in := range insts {
		fmt.Printf("%-22s %s\n", in.Name, in.Display())
		if in.About != "" {
			fmt.Printf("%-22s %s\n", "", in.About)
		}
	}
}

func printFunctions() {
	fns := mathexpr.Functions()
	sort.Slice(fns, func(i, j int) bool { return fns[i].Name < fns[j].Name })

	fmt.Println("Variables:")
	for _, v := range [][2]string{
		{"p, phase", "phase in radians, 0..2pi over one cycle"},
		{"x", "phase normalized to 0..1"},
		{"t, time", "seconds since the note started"},
		{"f, freq", "note frequency in Hz"},
		{"env", "envelope level, 0..1"},
		{"vel", "key velocity, 0..1"},
		{"sr", "sample rate"},
		{"a, b, c, d", "tweakable parameters"},
		{"noise", "uniform random in -1..1"},
	} {
		fmt.Printf("  %-12s %s\n", v[0], v[1])
	}

	fmt.Println("\nAggregates:")
	fmt.Println("  sum(k, from, to, body)    prod(k, from, to, body)")

	fmt.Println("\nFunctions:")
	for _, f := range fns {
		fmt.Printf("  %-12s %s\n", f.Name, f.Doc)
	}
}

func writeExample() error {
	path, created, err := instrument.WriteExample()
	if err != nil {
		return err
	}
	if !created {
		fmt.Printf("%s already exists, leaving it alone\n", path)
		return nil
	}
	fmt.Printf("wrote %s\n", path)
	return nil
}

// renderFile writes a single note to a WAV file, which is the quickest way to
// hear what an equation does without opening the UI.
func renderFile(o options, in *instrument.Instrument) error {
	if o.duration <= 0 {
		return fmt.Errorf("duration must be positive")
	}
	if o.freq <= 0 || o.freq >= sampleRate/2 {
		return fmt.Errorf("freq must be between 0 and %d Hz", sampleRate/2)
	}

	r := synth.Render{
		Instrument: in,
		SampleRate: sampleRate,
		Notes: []synth.RenderNote{
			{Start: 0, Duration: o.duration, Freq: o.freq, Velocity: 1},
		},
		Tail: in.Envelope.Release + 0.1,
	}
	if err := r.WriteWAV(o.render); err != nil {
		return err
	}

	fmt.Printf("wrote %s: %s at %.1f Hz for %.2fs\n", o.render, in.Name, o.freq, o.duration)
	return nil
}

func play(insts []*instrument.Instrument) error {
	engine := synth.NewEngine(insts[0], sampleRate)

	format := beep.Format{SampleRate: sampleRate, NumChannels: 2, Precision: 2}
	if err := speaker.Init(format.SampleRate, bufferSize); err != nil {
		return fmt.Errorf("could not open the audio device: %w", err)
	}
	defer speaker.Close()

	// The engine is itself a streamer, so there is no shared mixer to race on.
	speaker.Play(engine)

	p := tea.NewProgram(tui.New(engine, insts), tea.WithAltScreen())
	_, err := p.Run()

	engine.Panic()
	// Let the last buffer drain so quitting does not click.
	time.Sleep(50 * time.Millisecond)
	return err
}
