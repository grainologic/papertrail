package cli

import (
	"fmt"
	"strings"
)

const helpUsage = "pt help [command]"

// helpCmd is the help command, routed to from "--help", "-h", and "pt help".
var helpCmd = Command{
	Name:    "help",
	Usage:   helpUsage,
	Summary: "Show help for pt or one command",
}

func init() {
	helpCmd.Run = runHelp
	Register(helpCmd)
}

func runHelp(env Env, args []string) int {
	asJSON := hasArg(args, "--json")
	var rest []string
	for _, a := range args {
		if a != "--json" {
			rest = append(rest, a)
		}
	}
	if len(rest) == 0 || rest[0] == "--help" || rest[0] == "-h" {
		return printOverview(env, asJSON)
	}
	cmd, ok, name := resolveHelpCommand(rest)
	if !ok {
		return printHelpUnknown(env, asJSON, name)
	}
	// --json must precede --help so the flag set records it before parsing stops at ErrHelp.
	helpArgs := []string{"--help"}
	if asJSON {
		helpArgs = []string{"--json", "--help"}
	}
	return cmd.Run(env, helpArgs)
}

// resolveHelpCommand returns the command named by args, allowing the two-word forms ("section get"), along with the name matched.
func resolveHelpCommand(args []string) (Command, bool, string) {
	if c, ok := Lookup(args[0]); ok {
		return c, true, args[0]
	}
	if len(args) > 1 {
		if c, ok := Lookup(args[0] + " " + args[1]); ok {
			return c, true, args[0] + " " + args[1]
		}
	}
	return Command{}, false, args[0]
}

func printHelpUnknown(env Env, asJSON bool, name string) int {
	if asJSON {
		if code := PrintJSON(env, map[string]string{"error": "unknown command " + name}); code != 0 {
			return code
		}
		return 2
	}
	fmt.Fprintf(env.Stderr, "pt: unknown command %q\n", name)
	return 2
}

// helpCommandDoc is the JSON-safe shape of one command in the overview; Command itself carries a func and cannot marshal.
type helpCommandDoc struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
}

// itemValues mirrors the fixed item vocabulary for the overview help.
var itemValues = struct {
	types    []string
	statuses []string
	priority string
	modes    []string
}{
	types:    []string{"bug", "chore", "debate", "feature", "feedback", "milestone", "question", "research", "spike", "task"},
	statuses: []string{"open", "in_progress", "review", "blocked", "closed"},
	priority: "0-4, default 2 (0 highest)",
	modes:    []string{"auto", "hitl"},
}

func printOverview(env Env, asJSON bool) int {
	cmds := Commands()
	if asJSON {
		docs := make([]helpCommandDoc, 0, len(cmds))
		for _, c := range cmds {
			docs = append(docs, helpCommandDoc{Name: c.Name, Summary: c.Summary})
		}
		doc := map[string]any{
			"commands":  docs,
			"types":     itemValues.types,
			"statuses":  itemValues.statuses,
			"priority":  itemValues.priority,
			"modes":     itemValues.modes,
			"exitCodes": []int{0, 1, 2},
		}
		return PrintJSON(env, doc)
	}
	fmt.Fprintln(env.Stdout, "pt tracks work items in markdown files under a .papertrail directory.")
	fmt.Fprintln(env.Stdout)
	fmt.Fprintln(env.Stdout, "Commands:")
	for _, c := range cmds {
		if c.Summary == "" {
			fmt.Fprintf(env.Stdout, "  %s\n", c.Name)
		} else {
			fmt.Fprintf(env.Stdout, "  %-13s %s\n", c.Name, c.Summary)
		}
	}
	fmt.Fprintln(env.Stdout)
	fmt.Fprintln(env.Stdout, "Item values:")
	fmt.Fprintf(env.Stdout, "  type: %s\n", strings.Join(itemValues.types, ", "))
	fmt.Fprintf(env.Stdout, "  status: %s\n", strings.Join(itemValues.statuses, ", "))
	fmt.Fprintf(env.Stdout, "  priority: %s\n", itemValues.priority)
	fmt.Fprintf(env.Stdout, "  mode: %s\n", strings.Join(itemValues.modes, ", "))
	fmt.Fprintln(env.Stdout)
	fmt.Fprintln(env.Stdout, "Conventions:")
	fmt.Fprintln(env.Stdout, "  --json selects structured output on stdout, including errors")
	fmt.Fprintln(env.Stdout, "  IDs are matched by unique prefix")
	fmt.Fprintln(env.Stdout, "  configuration walks up from .papertrail/config.toml")
	fmt.Fprintln(env.Stdout, "  exit codes: 0 ok, 1 error, 2 usage")
	fmt.Fprintln(env.Stdout)
	fmt.Fprintln(env.Stdout, "More: pt help <command> or pt <command> --help")
	return 0
}
