package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// NewFlagSet builds a flag set that reports errors through the returned exit-code convention: parse failures print usage and mean exit 2. fs.Usage is suppressed so the flag package's own usage lines never print; help is written by ParseFlags instead.
func NewFlagSet(env Env, cmd Command) *flag.FlagSet {
	fs := flag.NewFlagSet(cmd.Name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	return fs
}

// ParseArgs separates interspersed positional arguments from flags, so "pt create feature Fix it --priority 1" parses like "--priority 1" first. Unknown flag names are left for the flag package to reject. "--" ends flag parsing; the rest is positional.
func ParseArgs(fs *flag.FlagSet, args []string) []string {
	var flagArgs, positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(arg) < 2 || arg[0] != '-' {
			positional = append(positional, arg)
			continue
		}
		name := strings.TrimLeft(arg, "-")
		if strings.Contains(name, "=") {
			flagArgs = append(flagArgs, arg)
			continue
		}
		f := fs.Lookup(name)
		// Boolean flags are the ones whose default is a boolean literal;
		// every flag this CLI registers follows that rule.
		if f == nil || f.DefValue == "false" || f.DefValue == "true" {
			flagArgs = append(flagArgs, arg)
			continue
		}
		if i+1 >= len(args) {
			// Missing value: let the flag package report it.
			flagArgs = append(flagArgs, arg)
			continue
		}
		i++
		flagArgs = append(flagArgs, arg, args[i])
	}
	return append(flagArgs, positional...)
}

// ParseFlags parses args for cmd against fs and maps the outcome to the pt exit-code and help conventions. It returns the exit code (0 when parsing succeeded or help was handled) and whether the command should return immediately.
func ParseFlags(env Env, fs *flag.FlagSet, cmd Command, args []string) (int, bool) {
	if err := fs.Parse(ParseArgs(fs, args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			// --json may appear after --help in the raw args and never reach the flag set, so help honors it from either source.
			if JSONEnabled(fs) || hasArg(args, "--json") {
				return writeHelpJSON(env, fs, cmd), true
			}
			return writeHelpText(env, fs, cmd), true
		}
		return UsageExit(env, JSONEnabled(fs), cmd.Usage), true
	}
	return 0, false
}

func hasArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

// writeHelpText prints the human-readable help for cmd and its registered flags to stdout.
func writeHelpText(env Env, fs *flag.FlagSet, cmd Command) int {
	fmt.Fprintf(env.Stdout, "usage: %s\n", cmd.Usage)
	if cmd.Summary != "" {
		fmt.Fprintf(env.Stdout, "\n%s\n", cmd.Summary)
	}
	fs.VisitAll(func(f *flag.Flag) {
		fmt.Fprintf(env.Stdout, "  -%s\n      %s\n", f.Name, f.Usage)
	})
	if cmd.Help != "" {
		fmt.Fprintf(env.Stdout, "\n%s\n", cmd.Help)
	}
	return 0
}

// writeHelpJSON prints the structured help for cmd and its registered flags to stdout.
func writeHelpJSON(env Env, fs *flag.FlagSet, cmd Command) int {
	type flagDoc struct {
		Name  string `json:"name"`
		Usage string `json:"usage"`
	}
	var flags []flagDoc
	fs.VisitAll(func(f *flag.Flag) {
		flags = append(flags, flagDoc{Name: f.Name, Usage: f.Usage})
	})
	return PrintJSON(env, map[string]any{
		"name":    cmd.Name,
		"usage":   cmd.Usage,
		"summary": cmd.Summary,
		"help":    cmd.Help,
		"flags":   flags,
	})
}

// JSONFlag is registered on every command so --json is accepted uniformly.
type JSONFlag struct{ enabled bool }

func (j *JSONFlag) String() string {
	if j.enabled {
		return "true"
	}
	return "false"
}
func (j *JSONFlag) Set(s string) error {
	j.enabled = s == "true" || s == "1" || s == "yes"
	return nil
}
func (j *JSONFlag) IsBoolFlag() bool { return true }

// JSONEnabled reports whether --json was requested.
func JSONEnabled(fs *flag.FlagSet) bool {
	if j, ok := fs.Lookup("json").Value.(*JSONFlag); ok {
		return j.enabled
	}
	return false
}

// PrintJSON encodes a structured result to stdout with a trailing newline.
func PrintJSON(env Env, v any) int {
	enc := json.NewEncoder(env.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(env.Stderr, "pt: encode json: %v\n", err)
		return 1
	}
	return 0
}

// PrintError reports a failure and returns the process exit code. With --json the error is a structured object on stdout; otherwise it is a message on stderr. A usage error (exit 2) means the arguments themselves were wrong.
func PrintError(env Env, asJSON bool, code int, err error) int {
	if asJSON {
		if c := PrintJSON(env, map[string]string{"error": err.Error()}); c != 0 {
			return c
		}
		return code
	}
	fmt.Fprintf(env.Stderr, "pt: %v\n", err)
	return code
}

// UsageExit prints a command's usage and returns the usage exit code.
func UsageExit(env Env, asJSON bool, usage string) int {
	if asJSON {
		if c := PrintJSON(env, map[string]string{"error": "invalid arguments"}); c != 0 {
			return c
		}
		return 2
	}
	fmt.Fprintln(env.Stderr, "usage:", usage)
	return 2
}
