package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

// flagSet is a thin wrapper over flag.FlagSet that knows whether --json was
// passed (checked by commands that support both output modes).
type flagSet struct {
	fs *flag.FlagSet
}

// newFlagSet creates the wrapper with the underlying FlagSet ready to use;
// jsonOut, when non-nil, receives the --json flag. Callers must invoke
// Parse(args) themselves.
func newFlagSet(name string, jsonOut *bool) *flagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if jsonOut != nil {
		fs.BoolVar(jsonOut, "json", false, "machine-readable JSON output")
	}
	return &flagSet{fs: fs}
}

// Parse parses the arguments; returns false on -h/parse error.
func (f *flagSet) Parse(args []string) bool {
	if err := f.fs.Parse(args); err != nil {
		return false
	}
	return true
}

// StringVar, IntVar, BoolVar delegate to the underlying FlagSet.
func (f *flagSet) StringVar(p *string, name, value, usage string) {
	f.fs.StringVar(p, name, value, usage)
}

func (f *flagSet) IntVar(p *int, name string, value int, usage string) {
	f.fs.IntVar(p, name, value, usage)
}

func (f *flagSet) BoolVar(p *bool, name string, value bool, usage string) {
	f.fs.BoolVar(p, name, value, usage)
}

// jsonEncoder is a convenience for printing indented JSON to stdout.
type jsonEncoder struct{}

func (e *jsonEncoder) Encode(v any) int {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintln(os.Stderr, "json:", err)
		return 2
	}
	return 0
}
