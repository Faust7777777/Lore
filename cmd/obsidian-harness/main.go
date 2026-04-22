package main

import (
	"fmt"
	"io"
	"os"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/tui"
)

const version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	application := app.New(app.Config{Version: version})

	if len(args) == 0 {
		fmt.Fprint(stdout, tui.RenderStatus(application.Status()))
		return 0
	}

	switch args[0] {
	case "status":
		fmt.Fprint(stdout, tui.RenderStatus(application.Status()))
		return 0
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage())
		return 0
	case "version", "-v", "--version":
		fmt.Fprintln(stdout, version)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n\n%s", args[0], usage())
		return 1
	}
}

func usage() string {
	return `obsidian-harness

Usage:
  obsidian-harness [command]

Commands:
  status   Render the current text status view
  version  Print the CLI version
  help     Show this help text
`
}
