package main

import (
	"os"

	// Command packages self-register into internal/cli's registry.
	_ "papertrail/internal/batch"
	"papertrail/internal/cli"
	_ "papertrail/internal/link"
	_ "papertrail/internal/section"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
