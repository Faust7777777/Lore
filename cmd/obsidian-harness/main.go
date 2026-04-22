package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
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
	case "sync-codex-jsonl":
		return runSyncCodexJSONL(args[1:], stdout, stderr)
	case "attach-codex-jsonl":
		return runAttachCodexJSONL(args[1:], stdout, stderr)
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
  sync-codex-jsonl     Sync a Codex session JSONL only when the file changed
  attach-codex-jsonl   Poll a Codex session JSONL and keep syncing it
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
	params, workDir, _, _, err := parseCodexJSONLFlags("import-codex-jsonl", args, stderr, false, false)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	runtime, err := app.OpenRuntime(workDir)
	if err != nil {
		fmt.Fprintf(stderr, "open runtime: %v\n", err)
		return 1
	}

	result, err := runtime.ImportCodexJSONL(params, time.Now())
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

func runSyncCodexJSONL(args []string, stdout io.Writer, stderr io.Writer) int {
	params, workDir, _, _, err := parseCodexJSONLFlags("sync-codex-jsonl", args, stderr, false, false)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	runtime, err := app.OpenRuntime(workDir)
	if err != nil {
		fmt.Fprintf(stderr, "open runtime: %v\n", err)
		return 1
	}

	result, err := runtime.SyncCodexJSONL(params, time.Now())
	if err != nil {
		fmt.Fprintf(stderr, "sync-codex-jsonl: %v\n", err)
		return 1
	}
	if !result.Changed {
		fmt.Fprintf(stdout, "Codex JSONL unchanged\n- cursor: %s\n", result.Fingerprint)
		return 0
	}
	fmt.Fprintf(
		stdout,
		"Codex JSONL synced\n- agent: %s\n- session: %s\n- cursor: %s\n- checkpoints: %d\n- daily reports: %d\n",
		result.Import.AgentID,
		result.Import.SessionID,
		result.Fingerprint,
		len(result.Import.Checkpoints),
		len(result.Import.Reports),
	)
	return 0
}

func runAttachCodexJSONL(args []string, stdout io.Writer, stderr io.Writer) int {
	params, workDir, pollEvery, once, err := parseAttachCodexJSONLFlags(args, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	runtime, err := app.OpenRuntime(workDir)
	if err != nil {
		fmt.Fprintf(stderr, "open runtime: %v\n", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	fmt.Fprintf(stdout, "Attaching Codex JSONL\n- input: %s\n- poll: %s\n", params.InputPath, pollEvery)
	for {
		result, err := runtime.SyncCodexJSONL(params, time.Now())
		if err != nil {
			fmt.Fprintf(stderr, "attach-codex-jsonl: %v\n", err)
			return 1
		}
		if result.Changed {
			fmt.Fprintf(
				stdout,
				"Synced\n- agent: %s\n- session: %s\n- checkpoints: %d\n- daily reports: %d\n",
				result.Import.AgentID,
				result.Import.SessionID,
				len(result.Import.Checkpoints),
				len(result.Import.Reports),
			)
		} else {
			fmt.Fprintln(stdout, "No changes")
		}
		if once {
			return 0
		}

		timer := time.NewTimer(pollEvery)
		select {
		case <-ctx.Done():
			timer.Stop()
			fmt.Fprintln(stdout, "Attach stopped")
			return 0
		case <-timer.C:
		}
	}
}

func parseCodexJSONLFlags(name string, args []string, stderr io.Writer, includePoll bool, includeOnce bool) (app.ImportCodexJSONLParams, string, time.Duration, bool, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)

	workDir := flags.String("workdir", "", "workdir that contains vault/ and state/")
	inputPath := flags.String("input", "", "path to Codex session JSONL")
	agentID := flags.String("agent", "", "override agent id")
	sessionID := flags.String("session", "", "override session id")
	windowSize := flags.Duration("window", 0, "override checkpoint window size, e.g. 30m")
	skipRollup := flags.Bool("skip-rollup", false, "skip daily rollup after import")
	pollEvery := 5 * time.Second
	once := false
	if includePoll {
		flags.DurationVar(&pollEvery, "poll", 5*time.Second, "poll interval for attach mode")
	}
	if includeOnce {
		flags.BoolVar(&once, "once", false, "run one sync cycle and exit")
	}

	if err := flags.Parse(args); err != nil {
		return app.ImportCodexJSONLParams{}, "", 0, false, err
	}
	if strings.TrimSpace(*inputPath) == "" {
		return app.ImportCodexJSONLParams{}, "", 0, false, fmt.Errorf("%s: --input is required", name)
	}
	if strings.TrimSpace(*workDir) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return app.ImportCodexJSONLParams{}, "", 0, false, err
		}
		*workDir = cwd
	}

	return app.ImportCodexJSONLParams{
		InputPath:  *inputPath,
		AgentID:    *agentID,
		SessionID:  *sessionID,
		Window:     *windowSize,
		SkipRollup: *skipRollup,
	}, filepath.Clean(*workDir), pollEvery, once, nil
}

func parseAttachCodexJSONLFlags(args []string, stderr io.Writer) (app.ImportCodexJSONLParams, string, time.Duration, bool, error) {
	return parseCodexJSONLFlags("attach-codex-jsonl", args, stderr, true, true)
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
