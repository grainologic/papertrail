package cli

import (
	"fmt"
)

const whereUsage = "pt where"

// whereCmd is the where command; Run is wired in init so the initializer stays cycle-free.
var whereCmd = Command{
	Name:    "where",
	Usage:   whereUsage,
	Summary: "Print the item root directory",
	Help:    "Prints the resolved absolute item root, discovered by walking up from the current directory.",
}

func init() {
	whereCmd.Run = runWhere
	Register(whereCmd)
}

func runWhere(env Env, args []string) int {
	fs := NewFlagSet(env, whereCmd)
	jf := &JSONFlag{}
	fs.Var(jf, "json", "print the result as JSON")

	if code, done := ParseFlags(env, fs, whereCmd, args); done {
		return code
	}
	if fs.NArg() != 0 {
		return UsageExit(env, jf.enabled, whereUsage)
	}

	root, code, ok := RootFor(env, jf.enabled)
	if !ok {
		return code
	}
	if jf.enabled {
		return PrintJSON(env, map[string]string{"root": root})
	}
	fmt.Fprintln(env.Stdout, root)
	return 0
}
