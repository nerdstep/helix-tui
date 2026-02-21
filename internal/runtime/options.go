package runtime

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"helix-tui/internal/configfile"
)

type runOptions struct {
	configPath     string
	configExplicit bool
	headless       bool
	version        bool
}

func parseRunOptions(args []string, stderr io.Writer) (runOptions, error) {
	configPath := configfile.DefaultPath

	headless := false
	version := false
	fs := newFlagSet(stderr)
	fs.StringVar(&configPath, "config", configPath, "path to TOML config file")
	fs.BoolVar(&headless, "headless", false, "run without TUI; useful for autonomous mode")
	fs.BoolVar(&version, "version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return runOptions{}, err
	}
	if fs.NArg() > 0 {
		return runOptions{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	explicit := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "config" {
			explicit = true
		}
	})

	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return runOptions{}, fmt.Errorf("config path cannot be empty")
	}

	return runOptions{
		configPath:     configPath,
		configExplicit: explicit,
		headless:       headless,
		version:        version,
	}, nil
}

func newFlagSet(stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("helix", flag.ContinueOnError)
	if stderr == nil {
		stderr = io.Discard
	}
	fs.SetOutput(stderr)
	return fs
}
