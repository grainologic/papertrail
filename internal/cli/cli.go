// Package cli holds the pt command registry and the core commands. Each command writes results to stdout and errors to stderr, returning the process exit code. Every command accepts --json for a structured result or a structured {"error": ...} object.
package cli

import (
	"fmt"
	"io"
	"os"

	"papertrail/internal/config"
)

// Run executes one pt command and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	return RunIn(".", args, stdout, stderr)
}

// RunIn executes one pt command from the given starting directory, the seam tests and callers use instead of changing the process working directory.
func RunIn(dir string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: pt <command> [args]")
		fmt.Fprintln(stderr, "Run 'pt --help' for commands.")
		return 2
	}
	if args[0] == "--version" || args[0] == "-v" {
		args = append([]string{"version"}, args[1:]...)
	}
	if args[0] == "--help" || args[0] == "-h" {
		args = append([]string{"help"}, args[1:]...)
	}
	cmd, ok := Lookup(args[0])
	if !ok && len(args) > 1 {
		// Two-word command names ("section get") are matched before the single-word miss is reported.
		if c, two := Lookup(args[0] + " " + args[1]); two {
			cmd, ok = c, true
			args = args[1:]
		}
	}
	if !ok {
		fmt.Fprintf(stderr, "pt: unknown command %q\n", args[0])
		return 2
	}
	return cmd.Run(Env{Dir: dir, Stdin: os.Stdin, Stdout: stdout, Stderr: stderr}, args[1:])
}

const initUsage = "pt init"

// initCmd is the init command. Run is wired in init so the package-level initializer stays free of the func reference and the registry and runInit share one definition.
var initCmd = Command{
	Name:    "init",
	Usage:   initUsage,
	Summary: "Create a new papertrail project",
	Help:    "Initializes a .papertrail directory (config.toml, INDEX.md, FRONTIER.md, HEALTH.md) at the current location. Refuses to overwrite an existing project.",
}

func init() {
	initCmd.Run = runInit
	Register(initCmd)
}

func runInit(env Env, args []string) int {
	fs := NewFlagSet(env, initCmd)
	jf := &JSONFlag{}
	fs.Var(jf, "json", "print the result as JSON")

	if code, done := ParseFlags(env, fs, initCmd, args); done {
		return code
	}
	if fs.NArg() != 0 {
		return UsageExit(env, jf.enabled, initUsage)
	}
	if err := config.Init(env.Dir); err != nil {
		return PrintError(env, jf.enabled, 1, err)
	}
	out := map[string]string{"initialized": config.ConfigDir}
	if jf.enabled {
		return PrintJSON(env, out)
	}
	fmt.Fprintln(env.Stdout, "initialized .papertrail")
	return 0
}
