package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/config"
	"obsidian-harness/internal/mcp"
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
		workDir, err := resolveWorkDir(args[1:])
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		cfg := config.Default(workDir)
		application = app.New(app.Config{
			Version:   version,
			VaultPath: cfg.Paths.VaultRoot,
			Profile:   "local",
			State:     inferManagedState(cfg),
		})
		fmt.Fprint(stdout, tui.RenderStatus(application.Status()))
		return 0
	case "bootstrap":
		workDir, err := resolveWorkDir(args[1:])
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		runtime, err := app.OpenRuntime(workDir)
		if err != nil {
			fmt.Fprintf(stderr, "open runtime: %v\n", err)
			return 1
		}
		created, err := runtime.Bootstrap(time.Now())
		if err != nil {
			fmt.Fprintf(stderr, "bootstrap: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "bootstrapped %d managed docs under %s\n", len(created), runtime.Config.Paths.VaultRoot)
		for _, ref := range created {
			fmt.Fprintf(stdout, "- %s (%s)\n", ref.Path, ref.Class)
		}
		return 0
	case "demo-p0a":
		workDir, err := resolveWorkDir(args[1:])
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		runtime, err := app.OpenRuntime(workDir)
		if err != nil {
			fmt.Fprintf(stderr, "open runtime: %v\n", err)
			return 1
		}
		draft, err := runtime.DemoP0A(time.Now())
		if err != nil {
			fmt.Fprintf(stderr, "demo-p0a: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "P0-A completed\n- draft: %s\n- state: %s\n- target: %s\n", draft.ID, draft.State, filepath.Join(runtime.Config.Paths.VaultRoot, draft.Target.Path))
		return 0
	case "demo-p0b":
		workDir, err := resolveWorkDir(args[1:])
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		runtime, err := app.OpenRuntime(workDir)
		if err != nil {
			fmt.Fprintf(stderr, "open runtime: %v\n", err)
			return 1
		}
		result, err := runtime.DemoP0B(time.Now())
		if err != nil {
			fmt.Fprintf(stderr, "demo-p0b: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "P0-B completed\n- checkpoint: %s\n- report: %s\n", result.Checkpoint.Path, result.Report.Path)
		return 0
	case "mcp":
		workDir, err := resolveWorkDir(args[1:])
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		runtime, err := app.OpenRuntime(workDir)
		if err != nil {
			fmt.Fprintf(stderr, "open runtime: %v\n", err)
			return 1
		}
		server := mcp.NewServer(runtime.Harness, version)
		if err := server.Serve(context.Background(), os.Stdin, stdout); err != nil {
			fmt.Fprintf(stderr, "mcp: %v\n", err)
			return 1
		}
		return 0
	case "import-codex-jsonl":
		return runImportCodexJSONL(args[1:], stdout, stderr)
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
  status [workdir]     Render the current text status view
  bootstrap [workdir]  Scaffold the managed vault skeleton
  demo-p0a [workdir]   Run the managed doc -> draft -> apply demo chain
  demo-p0b [workdir]   Run the checkpoint -> daily report demo chain
  mcp [workdir]        Run the read-only MCP server over stdio
  import-codex-jsonl   Import a Codex session JSONL into checkpoints and daily reports
  version              Print the CLI version
  help                 Show this help text
`
}

func resolveWorkDir(args []string) (string, error) {
	if len(args) > 0 && args[0] != "" {
		return filepath.Clean(args[0]), nil
	}
	return os.Getwd()
}

func runImportCodexJSONL(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("import-codex-jsonl", flag.ContinueOnError)
	flags.SetOutput(stderr)

	workDir := flags.String("workdir", "", "workdir that contains vault/ and state/")
	inputPath := flags.String("input", "", "path to Codex session JSONL")
	agentID := flags.String("agent", "", "override agent id")
	sessionID := flags.String("session", "", "override session id")
	windowSize := flags.Duration("window", 0, "override checkpoint window size, e.g. 30m")
	skipRollup := flags.Bool("skip-rollup", false, "skip daily rollup after import")

	if err := flags.Parse(args); err != nil {
		return 1
	}
	if strings.TrimSpace(*inputPath) == "" {
		fmt.Fprintln(stderr, "import-codex-jsonl: --input is required")
		return 1
	}
	if strings.TrimSpace(*workDir) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "import-codex-jsonl: %v\n", err)
			return 1
		}
		*workDir = cwd
	}

	runtime, err := app.OpenRuntime(filepath.Clean(*workDir))
	if err != nil {
		fmt.Fprintf(stderr, "open runtime: %v\n", err)
		return 1
	}

	result, err := runtime.ImportCodexJSONL(app.ImportCodexJSONLParams{
		InputPath:  *inputPath,
		AgentID:    *agentID,
		SessionID:  *sessionID,
		Window:     *windowSize,
		SkipRollup: *skipRollup,
	}, time.Now())
	if err != nil {
		fmt.Fprintf(stderr, "import-codex-jsonl: %v\n", err)
		return 1
	}

	fmt.Fprintf(
		stdout,
		"Codex JSONL imported\n- agent: %s\n- session: %s\n- checkpoints: %d\n- daily reports: %d\n",
		result.AgentID,
		result.SessionID,
		len(result.Checkpoints),
		len(result.Reports),
	)
	return 0
}

func inferManagedState(cfg config.Config) string {
	paths := []string{
		filepath.Join(cfg.Paths.VaultRoot, cfg.Vault.ManagedCore.SystemDoc),
		filepath.Join(cfg.Paths.VaultRoot, cfg.Vault.ManagedCore.ProgressIndex),
		filepath.Join(cfg.Paths.VaultRoot, cfg.Vault.ManagedCore.Persona),
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			return "bootstrap"
		}
	}
	return "ready"
}
