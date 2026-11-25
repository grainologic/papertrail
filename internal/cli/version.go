package cli

import "fmt"

// Version is the pt release version.
// Maintained manually in lockstep with integrations/pi/package.json "version".
const Version = "1.0.1"

const versionUsage = "pt version"

// versionCmd is the version command; Run is wired in init so the initializer stays cycle-free.
var versionCmd = Command{
	Name:    "version",
	Usage:   versionUsage,
	Summary: "Print the pt release version",
}

func init() {
	versionCmd.Run = runVersion
	Register(versionCmd)
}

func runVersion(env Env, args []string) int {
	fs := NewFlagSet(env, versionCmd)
	jf := &JSONFlag{}
	fs.Var(jf, "json", "print the result as JSON")

	if code, done := ParseFlags(env, fs, versionCmd, args); done {
		return code
	}
	if fs.NArg() != 0 {
		return UsageExit(env, jf.enabled, versionUsage)
	}
	if jf.enabled {
		return PrintJSON(env, map[string]string{"version": Version})
	}
	fmt.Fprintf(env.Stdout, "pt v%s\n", Version)
	return 0
}
