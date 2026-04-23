package main

import (
	"io"
	"os"

	"obsidian-harness/internal/cli"
)

const version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	return cli.Run(args, os.Stdin, stdout, stderr, version)
}

func runConsoleCommand(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	return cli.RunConsoleCommand(args, stdin, stdout, stderr, version)
}

