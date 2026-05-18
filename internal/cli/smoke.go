package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"obsidian-harness/internal/app"
)

func runSmokeCommand(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "smoke: missing subcommand")
		fmt.Fprintln(stderr, "usage: lore smoke p0 [--workdir <dir>] [--full]")
		return 1
	}

	switch args[0] {
	case "p0":
		workDir, full, err := parseSmokeFlags(args[1:], stderr)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}

		runtime, err := app.OpenRuntime(workDir)
		if err != nil {
			fmt.Fprintf(stderr, "open runtime: %v\n", err)
			return 1
		}
		defer closeRuntime(stderr, runtime, "smoke p0")

		result, err := runtime.SmokeP0(time.Now())
		if err != nil {
			fmt.Fprintf(stderr, "smoke p0: %v\n", err)
			return 1
		}

		status := "passed"
		exitCode := 0
		if !result.OK() {
			status = "failed"
			exitCode = 1
		}

		fmt.Fprintf(stdout, "P0 smoke %s\n", status)
		fmt.Fprintf(stdout, "- workdir: %s\n", result.Managed.WorkDir)
		fmt.Fprintf(stdout, "- ready: %t\n", result.Managed.Ready)
		fmt.Fprintf(stdout, "- draft: %s (%s)\n", result.Draft.ID, result.Draft.State)
		fmt.Fprintf(stdout, "- checkpoint: %s\n", result.Checkpoint.Path)
		fmt.Fprintf(stdout, "- report: %s\n", result.Report.Path)
		fmt.Fprintln(stdout, "- checks:")
		for _, check := range result.Checks {
			marker := "ok"
			if !check.OK {
				marker = "fail"
			}
			fmt.Fprintf(stdout, "  [%s] %s", marker, check.Name)
			if detail := strings.TrimSpace(check.Detail); detail != "" {
				fmt.Fprintf(stdout, " - %s", detail)
			}
			fmt.Fprintln(stdout)
		}

		if exitCode != 0 {
			return exitCode
		}

		if full {
			if err := runGovernedNoteSmoke(runtime, stdout, stderr); err != nil {
				return 1
			}
		}
		return exitCode
	default:
		fmt.Fprintf(stderr, "smoke: unknown subcommand %q\n", args[0])
		return 1
	}
}

func runGovernedNoteSmoke(runtime *app.Runtime, stdout io.Writer, stderr io.Writer) error {
	fmt.Fprintln(stdout, "\nRunning governed note intake smoke...")
	noteResult, err := runtime.SmokeGovernedMarkdownNoteIntake(time.Now())
	if err != nil {
		fmt.Fprintf(stderr, "governed note smoke: %v\n", err)
		return err
	}

	status := "passed"
	if !noteResult.OK() {
		status = "failed"
	}
	fmt.Fprintf(stdout, "Governed note intake %s\n", status)
	fmt.Fprintf(stdout, "- proposal: draft=%s status=%s\n", noteResult.Proposal.DraftID, noteResult.Proposal.Status)
	fmt.Fprintf(stdout, "- applied: %s (%s)\n", noteResult.Applied.ID, noteResult.Applied.State)
	fmt.Fprintln(stdout, "- checks:")
	for _, check := range noteResult.Checks {
		marker := "ok"
		if !check.OK {
			marker = "fail"
		}
		fmt.Fprintf(stdout, "  [%s] %s", marker, check.Name)
		if detail := strings.TrimSpace(check.Detail); detail != "" {
			fmt.Fprintf(stdout, " - %s", detail)
		}
		fmt.Fprintln(stdout)
	}
	if !noteResult.OK() {
		return fmt.Errorf("governed note intake smoke failed")
	}
	return nil
}

func parseSmokeFlags(args []string, stderr io.Writer) (string, bool, error) {
	flags := flag.NewFlagSet("smoke p0", flag.ContinueOnError)
	flags.SetOutput(stderr)

	workDir := flags.String("workdir", "", "workdir that contains vault/ and state/")
	full := flags.Bool("full", false, "also run governed note intake smoke")
	if err := flags.Parse(args); err != nil {
		return "", false, err
	}
	if strings.TrimSpace(*workDir) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", false, err
		}
		*workDir = cwd
	}
	return filepath.Clean(*workDir), *full, nil
}
