package instrument

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ConfigDir returns the directory holding the user's instrument file,
// respecting XDG_CONFIG_HOME when it is set.
func ConfigDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "piango"), nil
}

// ConfigPath returns the full path to instruments.json.
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "instruments.json"), nil
}

// file is the on-disk schema. Keeping the instrument list under a key leaves
// room to add settings later without breaking existing files.
type file struct {
	Instruments []Def `json:"instruments"`
}

// maxConfigSize caps how much we will read, so a huge or malformed file cannot
// exhaust memory.
const maxConfigSize = 4 << 20 // 4 MiB

// LoadUser reads and compiles the user's custom instruments. A missing file is
// not an error: it simply returns nothing. Instruments that fail to compile are
// skipped and reported in the returned problems, so one typo cannot stop the
// app from starting.
func LoadUser() (insts []*Instrument, problems []string, err error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, nil, err
	}
	return LoadFile(path)
}

// LoadFile reads and compiles instruments from a specific path.
func LoadFile(path string) (insts []*Instrument, problems []string, err error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	if info.Size() > maxConfigSize {
		return nil, nil, fmt.Errorf("%s is larger than the %d byte limit", path, maxConfigSize)
	}

	var cfg file
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}

	for _, d := range cfg.Instruments {
		inst, err := Compile(d)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		insts = append(insts, inst)
	}
	return insts, problems, nil
}

// Save writes definitions to the user's config, creating the directory when
// needed. It writes to a temporary file and renames, so an interrupted write
// cannot leave a truncated config behind.
func Save(defs []Def) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(file{Instruments: defs}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), ".instruments-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// Append adds one definition to the user's config, replacing any existing
// instrument with the same name.
func Append(d Def) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}

	var cfg file
	if data, err := os.ReadFile(path); err == nil {
		// Ignore a corrupt existing file rather than refusing to save.
		_ = json.Unmarshal(data, &cfg)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	replaced := false
	for i := range cfg.Instruments {
		if strings.EqualFold(cfg.Instruments[i].Name, d.Name) {
			cfg.Instruments[i] = d
			replaced = true
			break
		}
	}
	if !replaced {
		cfg.Instruments = append(cfg.Instruments, d)
	}
	return Save(cfg.Instruments)
}

// ExampleConfig is a starter file written when the user has no config yet.
const ExampleConfig = `{
  "instruments": [
    {
      "name": "My Organ",
      "expr": "sum(k, 1, 4, sin(k*p)/k)",
      "formula": "y = Σ(k=1..4) sin(kp)/k",
      "about": "A four-harmonic additive organ.",
      "envelope": { "attack": 0.01, "decay": 0.1, "sustain": 0.9, "release": 0.2 }
    },
    {
      "name": "My FM",
      "expr": "sin(p + a*sin(b*p))",
      "formula": "y = sin(p + I sin(r p))",
      "about": "Two-operator FM with live index and ratio.",
      "params": [
        { "name": "index", "value": 2.5, "min": 0, "max": 10 },
        { "name": "ratio", "value": 2.0, "min": 0.25, "max": 8 }
      ],
      "envelope": { "attack": 0.005, "decay": 0.5, "sustain": 0.4, "release": 0.3 }
    }
  ]
}
`

// WriteExample creates a starter config if none exists, returning the path and
// whether it wrote anything.
func WriteExample() (path string, created bool, err error) {
	path, err = ConfigPath()
	if err != nil {
		return "", false, err
	}
	if _, err := os.Stat(path); err == nil {
		return path, false, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return path, false, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return path, false, err
	}
	if err := os.WriteFile(path, []byte(ExampleConfig), 0o644); err != nil {
		return path, false, err
	}
	return path, true, nil
}
