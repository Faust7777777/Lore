package main

import (
	"bufio"
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
	"obsidian-harness/internal/console"
	"obsidian-harness/internal/mcp"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/operatoragent"
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
		runtime, err := app.OpenRuntime(workDir)
		if err != nil {
			fmt.Fprintf(stderr, "open runtime: %v\n", err)
			return 1
		}
		managed, err := runtime.ManagedStatus()
		if err != nil {
			fmt.Fprintf(stderr, "status: %v\n", err)
			return 1
		}
		fmt.Fprint(stdout, tui.RenderManagedStatus(version, managed))
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
	case "console":
		return runConsoleCommand(args[1:], os.Stdin, stdout, stderr)
	case "draft":
		return runDraftCommand(args[1:], stdout, stderr)
	case "process-sink":
		return runProcessSinkCommand(args[1:], stdout, stderr)
	case "models":
		return runModelsCommand(args[1:], stdout, stderr)
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
  console              Operator console: NL -> one explicit reviewed action
  draft                Review and act on pending drafts
  process-sink         Inspect checkpoint and daily report status
  models               List models from the configured LLM endpoint
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

func runConsoleCommand(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	workDir, utterance, err := parseConsoleFlags(args, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	runtime, err := app.OpenRuntime(workDir)
	if err != nil {
		fmt.Fprintf(stderr, "open runtime: %v\n", err)
		return 1
	}
	session := console.NewSession(version)

	if strings.TrimSpace(utterance) != "" {
		output, err := session.Handle(utterance, runtime)
		if err != nil {
			fmt.Fprintf(stderr, "console: %v\n", err)
			return 1
		}
		fmt.Fprint(stdout, output)
		return 0
	}

	fmt.Fprintln(stdout, "Obsidian Harness Console")
	fmt.Fprintln(stdout, "The operator agent picks one explicit action per prompt. Type `help` for examples. Type `exit` to quit. A configured model-backed operator agent is required.")
	scanner := bufio.NewScanner(stdin)
	for {
		fmt.Fprint(stdout, "> ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				fmt.Fprintf(stderr, "console: %v\n", err)
				return 1
			}
			return 0
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if isConsoleExit(line) {
			fmt.Fprintln(stdout, "Console stopped")
			return 0
		}

		output, err := session.Handle(line, runtime)
		if err != nil {
			fmt.Fprintf(stderr, "console: %v\n", err)
			continue
		}
		fmt.Fprint(stdout, output)
		if !strings.HasSuffix(output, "\n") {
			fmt.Fprintln(stdout)
		}
	}
}

func runDraftCommand(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "draft: missing subcommand")
		fmt.Fprintln(stderr, "usage: obsidian-harness draft <list|review|approve|reject|request-revision|apply> [flags] [id]")
		return 1
	}

	switch args[0] {
	case "list":
		workDir, _, err := parseDraftFlags("draft list", args[1:], stderr, false)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		runtime, err := app.OpenRuntime(workDir)
		if err != nil {
			fmt.Fprintf(stderr, "open runtime: %v\n", err)
			return 1
		}
		drafts, err := runtime.ListDrafts()
		if err != nil {
			fmt.Fprintf(stderr, "draft list: %v\n", err)
			return 1
		}
		fmt.Fprint(stdout, tui.RenderDraftList(drafts))
		return 0
	case "review":
		workDir, draftID, err := parseDraftFlags("draft review", args[1:], stderr, true)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		runtime, err := app.OpenRuntime(workDir)
		if err != nil {
			fmt.Fprintf(stderr, "open runtime: %v\n", err)
			return 1
		}
		review, err := runtime.ReviewDraft(draftID)
		if err != nil {
			fmt.Fprintf(stderr, "draft review: %v\n", err)
			return 1
		}
		fmt.Fprint(stdout, tui.RenderDraftReview(review))
		return 0
	case "approve", "reject", "request-revision", "apply":
		workDir, draftID, err := parseDraftFlags("draft "+args[0], args[1:], stderr, true)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		runtime, err := app.OpenRuntime(workDir)
		if err != nil {
			fmt.Fprintf(stderr, "open runtime: %v\n", err)
			return 1
		}

		var updated model.Draft
		switch args[0] {
		case "approve":
			updated, err = runtime.ApproveDraft(draftID)
		case "reject":
			updated, err = runtime.RejectDraft(draftID)
		case "request-revision":
			updated, err = runtime.RequestDraftRevision(draftID)
		case "apply":
			updated, err = runtime.ApplyDraft(draftID)
		}
		if err != nil {
			fmt.Fprintf(stderr, "draft %s: %v\n", args[0], err)
			return 1
		}
		fmt.Fprint(stdout, tui.RenderDraftActionResult(args[0], updated))
		return 0
	default:
		fmt.Fprintf(stderr, "draft: unknown subcommand %q\n", args[0])
		return 1
	}
}

func runProcessSinkCommand(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "process-sink: missing subcommand")
		fmt.Fprintln(stderr, "usage: obsidian-harness process-sink day [--workdir <dir>] [--agent <id>] [--day YYYY-MM-DD]")
		return 1
	}

	switch args[0] {
	case "day":
		workDir, agentID, day, err := parseProcessSinkDayFlags(args[1:], stderr)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		runtime, err := app.OpenRuntime(workDir)
		if err != nil {
			fmt.Fprintf(stderr, "open runtime: %v\n", err)
			return 1
		}
		view, err := runtime.ProcessSinkDay(agentID, day)
		if err != nil {
			fmt.Fprintf(stderr, "process-sink day: %v\n", err)
			return 1
		}
		fmt.Fprint(stdout, tui.RenderProcessSinkDay(view))
		return 0
	default:
		fmt.Fprintf(stderr, "process-sink: unknown subcommand %q\n", args[0])
		return 1
	}
}

func runModelsCommand(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "models: missing subcommand")
		fmt.Fprintln(stderr, "usage: obsidian-harness models list")
		return 1
	}

	switch args[0] {
	case "list":
		cfg, enabled, err := operatoragent.LoadEnvConfig()
		if err != nil {
			fmt.Fprintf(stderr, "models list: %v\n", err)
			return 1
		}
		if !enabled {
			fmt.Fprintln(stderr, "models list: configure OBSIDIAN_HARNESS_LLM_BASE_URL and OBSIDIAN_HARNESS_LLM_API_KEY first")
			return 1
		}

		catalog, err := operatoragent.DiscoverModels(context.Background(), cfg)
		if err != nil {
			fmt.Fprintf(stderr, "models list: %v\n", err)
			return 1
		}

		fmt.Fprintf(stdout, "Available models (%d)\n", len(catalog.Models))
		for _, modelName := range catalog.Models {
			marker := " "
			if strings.EqualFold(modelName, catalog.Recommended) {
				marker = "*"
			}
			fmt.Fprintf(stdout, "%s %s\n", marker, modelName)
		}
		if strings.TrimSpace(cfg.Model) != "" {
			fmt.Fprintf(stdout, "\nConfigured model: %s\n", cfg.Model)
		}
		if catalog.Recommended != "" {
			fmt.Fprintf(stdout, "Recommended model: %s\n", catalog.Recommended)
		}
		return 0
	default:
		fmt.Fprintf(stderr, "models: unknown subcommand %q\n", args[0])
		return 1
	}
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

func parseConsoleFlags(args []string, stderr io.Writer) (string, string, error) {
	flags := flag.NewFlagSet("console", flag.ContinueOnError)
	flags.SetOutput(stderr)

	workDir := flags.String("workdir", "", "workdir that contains vault/ and state/")
	once := flags.String("once", "", "single utterance to execute")
	if err := flags.Parse(args); err != nil {
		return "", "", err
	}
	if strings.TrimSpace(*workDir) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", "", err
		}
		*workDir = cwd
	}
	return filepath.Clean(*workDir), strings.TrimSpace(*once), nil
}

func parseDraftFlags(name string, args []string, stderr io.Writer, requireID bool) (string, string, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)

	workDir := flags.String("workdir", "", "workdir that contains vault/ and state/")
	if err := flags.Parse(args); err != nil {
		return "", "", err
	}
	if strings.TrimSpace(*workDir) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", "", err
		}
		*workDir = cwd
	}

	var draftID string
	if requireID {
		remaining := flags.Args()
		if len(remaining) == 0 || strings.TrimSpace(remaining[0]) == "" {
			return "", "", fmt.Errorf("%s: draft id is required", name)
		}
		draftID = strings.TrimSpace(remaining[0])
	}

	return filepath.Clean(*workDir), draftID, nil
}

func isConsoleExit(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "exit", "quit", "q":
		return true
	default:
		return false
	}
}

func parseProcessSinkDayFlags(args []string, stderr io.Writer) (string, string, time.Time, error) {
	flags := flag.NewFlagSet("process-sink day", flag.ContinueOnError)
	flags.SetOutput(stderr)

	workDir := flags.String("workdir", "", "workdir that contains vault/ and state/")
	agentID := flags.String("agent", "codex", "agent id")
	dayValue := flags.String("day", time.Now().Format("2006-01-02"), "report day in YYYY-MM-DD")
	if err := flags.Parse(args); err != nil {
		return "", "", time.Time{}, err
	}
	if strings.TrimSpace(*workDir) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", "", time.Time{}, err
		}
		*workDir = cwd
	}

	day, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(*dayValue), time.Local)
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("process-sink day: invalid --day value: %w", err)
	}
	return filepath.Clean(*workDir), strings.TrimSpace(*agentID), day, nil
}
