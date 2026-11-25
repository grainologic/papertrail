package cli

import (
	"io"
	"sort"
)

// Env carries the process context one command run needs.
type Env struct {
	// Dir is the starting directory for configuration lookup.
	Dir string
	// Stdin, Stdout, and Stderr carry process input and output.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// Command is one pt subcommand. Run parses args (all arguments after the command name), writes results to env.Stdout and errors to env.Stderr, and returns the process exit code. Summary and Help feed the top-level help and the command's --help output; either may be empty.
type Command struct {
	Name    string
	Usage   string
	Summary string
	Help    string
	Run     func(env Env, args []string) int
}

var registry = map[string]Command{}

// Register adds commands to the registry, rejecting duplicate names. Command packages call Register from their own init; the binary imports those packages for their side effects.
func Register(commands ...Command) {
	for _, c := range commands {
		if _, dup := registry[c.Name]; dup {
			panic("cli: duplicate command " + c.Name)
		}
		registry[c.Name] = c
	}
}

// Lookup returns the registered command with the given name.
func Lookup(name string) (Command, bool) {
	c, ok := registry[name]
	return c, ok
}

// Commands returns every registered command sorted by name.
func Commands() []Command {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Command, 0, len(names))
	for _, name := range names {
		out = append(out, registry[name])
	}
	return out
}
