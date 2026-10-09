package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

const (
	flagProvider = "服务商（也可写在命令后面）"
	flagAccount  = "账号：标签、邮箱或订阅名（也可写在命令后面）"
)

// flagSet is a thin wrapper over flag.FlagSet that knows whether --json was
// passed, which flags take values, and leftover PROVIDER / ACCOUNT positionals.
type flagSet struct {
	fs              *flag.FlagSet
	valued          map[string]bool
	provider        *string
	account         *string
	extraUnexpected string
}

// newFlagSet creates the wrapper with the underlying FlagSet ready to use;
// jsonOut, when non-nil, receives the --json flag. Callers must invoke
// Parse(args) themselves.
func newFlagSet(name string, jsonOut *bool) *flagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	f := &flagSet{fs: fs, valued: map[string]bool{}}
	if jsonOut != nil {
		f.BoolVar(jsonOut, "json", false, "输出 JSON 数据")
	}
	return f
}

func (f *flagSet) providerAccount(provider, account *string) {
	f.provider = provider
	f.account = account
	f.StringVar(provider, "provider", "", flagProvider)
	if account != nil {
		f.StringVar(account, "account", "", flagAccount)
	}
}

// Help replaces -h output with a short how-to, then the flags.
func (f *flagSet) Help(text string) {
	f.fs.Usage = func() {
		fmt.Fprint(os.Stderr, strings.TrimRight(text, "\n")+"\n\n")
		f.fs.PrintDefaults()
	}
}

func (f *flagSet) Usage() { f.fs.Usage() }

// Parse parses flags (they may appear after positionals), then fills
// PROVIDER / ACCOUNT from leftovers. Extra leftovers are an error.
func (f *flagSet) Parse(args []string) bool {
	if err := f.fs.Parse(arrangeArgs(args, f.valued)); err != nil {
		return false
	}
	rest := f.fs.Args()
	n := 0
	if f.provider != nil && *f.provider == "" && n < len(rest) {
		*f.provider = rest[n]
		n++
	}
	if f.account != nil && *f.account == "" && n < len(rest) {
		*f.account = rest[n]
		n++
	}
	if n < len(rest) {
		fmt.Fprintf(os.Stderr, "多出来的参数 %q\n", rest[n])
		f.extraUnexpected = rest[n]
		return false
	}
	return true
}

func (f *flagSet) StringVar(p *string, name, value, usage string) {
	f.valued[name] = true
	f.fs.StringVar(p, name, value, usage)
}

func (f *flagSet) IntVar(p *int, name string, value int, usage string) {
	f.valued[name] = true
	f.fs.IntVar(p, name, value, usage)
}

func (f *flagSet) BoolVar(p *bool, name string, value bool, usage string) {
	f.valued[name] = false
	f.fs.BoolVar(p, name, value, usage)
}

// arrangeArgs moves flags to the front so `leeway usage cursor --json` works
// with the stdlib flag parser, which otherwise stops at the first positional.
func arrangeArgs(args []string, valued map[string]bool) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			pos = append(pos, a)
			pos = append(pos, args[i+1:]...)
			break
		}
		if strings.HasPrefix(a, "--") {
			name := strings.TrimPrefix(a, "--")
			if name == "" {
				pos = append(pos, a)
				continue
			}
			if strings.Contains(name, "=") {
				flags = append(flags, a)
				continue
			}
			flags = append(flags, a)
			if valued[name] && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			flags = append(flags, a)
			continue
		}
		pos = append(pos, a)
	}
	return append(flags, pos...)
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
