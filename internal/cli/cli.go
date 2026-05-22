package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/console"
	"obsidian-harness/internal/mcp"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/operatoragent"
	"obsidian-harness/internal/persona"
	"obsidian-harness/internal/sessionlog"
	"obsidian-harness/internal/tui"
)

func Run(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, version string) int {
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
		defer closeRuntime(stderr, runtime, "status")
		managed, err := runtime.ManagedStatus()
		if err != nil {
			fmt.Fprintf(stderr, "status: %v\n", err)
			return 1
		}
		fmt.Fprint(stdout, tui.RenderManagedStatus(version, managed))
		fmt.Fprint(stdout, tui.RenderConfigLayers(runtime.ConfigDiagnostics))
		if summary, err := runtime.SummarizeUsage(time.Now()); err == nil && summary.Calls > 0 {
			fmt.Fprint(stdout, renderTodayUsageTail(summary))
		}
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
		defer closeRuntime(stderr, runtime, "bootstrap")
		created, err := runtime.Bootstrap(time.Now())
		if err != nil {
			fmt.Fprintf(stderr, "bootstrap: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "bootstrapped %d managed docs under %s\n", len(created), runtime.Config.Paths.VaultRoot)
		for _, ref := range created {
			fmt.Fprintf(stdout, "- %s (%s)\n", ref.Path, ref.Class)
		}
		fmt.Fprintf(stdout, "\nNext steps:\n")
		fmt.Fprintf(stdout, "  1. Open %s and fill in your vault goals.\n", filepath.Join(runtime.Config.Paths.VaultRoot, "00-\u7cfb\u7edf", "\u7cfb\u7edf\u8bf4\u660e.md"))
		fmt.Fprintf(stdout, "  2. Open %s and add your profile.\n", filepath.Join(runtime.Config.Paths.VaultRoot, "03-\u753b\u50cf", "\u4eba\u7269\u753b\u50cf.md"))
		fmt.Fprintf(stdout, "  3. Run: lore tui --workdir %q\n", workDir)
		if len(created) == 0 {
			fmt.Fprintf(stdout, "(All managed docs already existed. No files were created.)\n")
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
		defer closeRuntime(stderr, runtime, "demo-p0a")
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
		defer closeRuntime(stderr, runtime, "demo-p0b")
		result, err := runtime.DemoP0B(time.Now())
		if err != nil {
			fmt.Fprintf(stderr, "demo-p0b: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "P0-B completed\n- checkpoint: %s\n- report: %s\n", result.Checkpoint.Path, result.Report.Path)
		return 0
	case "tui":
		return RunTUICommand(args[1:], stdin, stdout, stderr, version)
	case "console":
		return RunConsoleCommand(args[1:], stdin, stdout, stderr, version)
	case "sessions":
		return runSessionsCommand(args[1:], stdout, stderr)
	case "daemon":
		return runDaemonCommand(args[1:], stdout, stderr)
	case "draft":
		return runDraftCommand(args[1:], stdout, stderr)
	case "findings":
		return runFindingsCommand(args[1:], stdout, stderr)
	case "persona":
		return runPersonaCommand(args[1:], stdout, stderr)
	case "usage":
		return runUsageCommand(args[1:], stdout, stderr)
	case "process-sink":
		return runProcessSinkCommand(args[1:], stdout, stderr)
	case "smoke":
		return runSmokeCommand(args[1:], stdout, stderr)
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
		defer closeRuntime(stderr, runtime, "mcp")
		server := mcp.NewServer(runtime.Harness, version)
		if err := server.Serve(context.Background(), os.Stdin, stdout); err != nil {
			fmt.Fprintf(stderr, "mcp: %v\n", err)
			return 1
		}
		return 0
	case "import-codex-jsonl":
		return runImportCodexJSONL(args[1:], stdout, stderr)
	case "import-external-jsonl":
		return runImportExternalJSONL(args[1:], stdout, stderr)
	case "import-codex-appserver":
		return runImportCodexAppServer(args[1:], stdout, stderr, version)
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
	return `Lore

Command:
  lore [command]

Commands:
  status [workdir]     Render the current text status view
  bootstrap [workdir]  Scaffold the managed vault skeleton
  demo-p0a [workdir]   Run the managed doc -> draft -> apply demo chain
  demo-p0b [workdir]   Run the checkpoint -> daily report demo chain
  tui                  Lore dashboard + natural language operator loop
  console              Lore natural-language agent loop
  sessions             List or search explicit local session transcripts
  daemon               Run the vault watcher daemon / one-shot scan
  draft                Review and act on pending drafts
  findings             Inspect and close post-scan governance findings
  persona              Review LLM-mined persona update candidates
  usage [workdir]      Summarize model-call usage and token cost
  process-sink         Inspect checkpoint and daily report status
  smoke                Run verification smoke checks (for example: smoke p0)
  models               List models from the configured LLM endpoint
  mcp [workdir]        Run the MCP intake server over stdio (read + proposal, no direct write)
  import-codex-jsonl   Import a Codex session JSONL into checkpoints and daily reports
  import-external-jsonl Import Lore external transcript JSONL into checkpoints and daily reports
  import-codex-appserver Import a Codex app-server thread into checkpoints and daily reports
  sync-codex-jsonl     Sync a Codex session JSONL only when the file changed
  attach-codex-jsonl   Poll a Codex session JSONL and keep syncing it
  version              Print the CLI version
  help                 Show this help text

Notes:
  - add --local-exec to console/tui to expose local workspace tools
  - shell_exec additionally requires LORE_AGENT_ENABLE_SHELL=1
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
	defer closeRuntime(stderr, runtime, "import-codex-jsonl")

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

func runImportExternalJSONL(args []string, stdout io.Writer, stderr io.Writer) int {
	params, workDir, err := parseExternalJSONLFlags("import-external-jsonl", args, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	runtime, err := app.OpenRuntime(workDir)
	if err != nil {
		fmt.Fprintf(stderr, "open runtime: %v\n", err)
		return 1
	}
	defer closeRuntime(stderr, runtime, "import-external-jsonl")

	result, err := runtime.ImportExternalTranscriptJSONL(params, time.Now())
	if err != nil {
		fmt.Fprintf(stderr, "import-external-jsonl: %v\n", err)
		return 1
	}

	fmt.Fprintf(
		stdout,
		"External transcript JSONL imported\n- agent: %s\n- session: %s\n- checkpoints: %d\n- daily reports: %d\n",
		result.AgentID,
		result.SessionID,
		len(result.Checkpoints),
		len(result.Reports),
	)
	return 0
}

func runSessionsCommand(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "sessions: subcommand is required: list or search")
		return 1
	}
	switch args[0] {
	case "list":
		workDir, limit, err := parseSessionsListFlags(args[1:], stderr)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		runtime, err := app.OpenRuntime(workDir)
		if err != nil {
			fmt.Fprintf(stderr, "open runtime: %v\n", err)
			return 1
		}
		defer closeRuntime(stderr, runtime, "sessions list")
		sessions, err := sessionlog.ListRecent(sessionLogRoot(runtime), limit)
		if err != nil {
			fmt.Fprintf(stderr, "sessions list: %v\n", err)
			return 1
		}
		renderSessionSummaries(stdout, sessions)
		return 0
	case "show":
		workDir, sessionID, err := parseSessionsShowFlags(args[1:], stderr)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		runtime, err := app.OpenRuntime(workDir)
		if err != nil {
			fmt.Fprintf(stderr, "open runtime: %v\n", err)
			return 1
		}
		defer closeRuntime(stderr, runtime, "sessions show")
		snapshot, err := sessionlog.Load(sessionLogRoot(runtime), sessionID)
		if err != nil {
			fmt.Fprintf(stderr, "sessions show: %v\n", err)
			return 1
		}
		renderSessionSnapshot(stdout, snapshot)
		return 0
	case "search":
		workDir, query, limit, err := parseSessionsSearchFlags(args[1:], stderr)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		runtime, err := app.OpenRuntime(workDir)
		if err != nil {
			fmt.Fprintf(stderr, "open runtime: %v\n", err)
			return 1
		}
		defer closeRuntime(stderr, runtime, "sessions search")
		sessions, err := sessionlog.Search(sessionLogRoot(runtime), query, limit)
		if err != nil {
			fmt.Fprintf(stderr, "sessions search: %v\n", err)
			return 1
		}
		renderSessionSummaries(stdout, sessions)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown sessions subcommand: %s\n", args[0])
		return 1
	}
}

func renderSessionSnapshot(stdout io.Writer, snapshot sessionlog.Snapshot) {
	summary := snapshot.Summary
	fmt.Fprintf(stdout, "Session: %s\n", summary.ID)
	fmt.Fprintf(stdout, "Updated: %s\n", summary.UpdatedAt.Format("2006-01-02 15:04"))
	fmt.Fprintf(stdout, "Turns: %d\n", summary.TurnCount)
	if summary.Title != "" {
		fmt.Fprintf(stdout, "Title: %s\n", summary.Title)
	}
	if len(snapshot.WorkingSet) > 0 {
		fmt.Fprintln(stdout, "Working Set:")
		for _, item := range snapshot.WorkingSet {
			fmt.Fprintf(stdout, "- %s %s\n", item.Kind, item.Path)
		}
	}
	if len(snapshot.History) == 0 {
		fmt.Fprintln(stdout, "No conversation history.")
		return
	}
	fmt.Fprintln(stdout, "Conversation:")
	for _, turn := range snapshot.History {
		role := strings.TrimSpace(turn.Role)
		if role == "" {
			role = "unknown"
		}
		fmt.Fprintf(stdout, "%s: %s\n", role, clipOneLine(turn.Content, 240))
	}
}
func clipOneLine(value string, limit int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
func renderSessionSummaries(stdout io.Writer, sessions []sessionlog.Summary) {
	if len(sessions) == 0 {
		fmt.Fprintln(stdout, "No sessions found.")
		return
	}
	for _, summary := range sessions {
		fmt.Fprintf(stdout, "%s\t%s\t%d\t%s\n", summary.ID, summary.UpdatedAt.Format("2006-01-02 15:04"), summary.TurnCount, summary.Title)
	}
}
// consolePersonaDrainTimeout caps how long the console / TUI shell
// waits at exit for in-flight persona extraction goroutines to land
// their candidates in the store. Short enough that a stuck LLM does
// not visibly delay shell exit; long enough that a healthy mid-call
// extraction reaches the store before sqlite closes.
const consolePersonaDrainTimeout = 2 * time.Second

func RunConsoleCommand(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, version string) int {
	workDir, utterance, localExec, resume, resumeID, err := parseConsoleFlags(args, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	runtime, err := app.OpenRuntime(workDir)
	if err != nil {
		fmt.Fprintf(stderr, "open runtime: %v\n", err)
		return 1
	}
	defer closeRuntime(stderr, runtime, "console")
	session := console.NewSession(version)
	session.EnableLocalWorkTools = localExec
	// Wire the persona candidate extractor from the runtime so the
	// fire-and-forget mining path (P4) is actually live in production.
	// When LLM env is unset OpenRuntime returns a nil extractor, in
	// which case Session.PersonaExtractor stays nil and the goroutine
	// is never spawned. Drain before closeRuntime so in-flight
	// extractions land in sqlite before the store closes -- defer
	// ordering is LIFO, so this drain runs BEFORE closeRuntime even
	// though it is registered later.
	session.PersonaExtractor = runtime.PersonaExtractor
	session.PersonaExtractLogger = runtime.PersonaExtractLogger
	session.PersonaExtractTimeout = runtime.PersonaExtractTimeout
	defer session.DrainPersonaExtractions(consolePersonaDrainTimeout)
	if err := configureSessionRecorder(session, runtime, resume, resumeID, stdin, stdout, stderr, "console"); err != nil {
		fmt.Fprintf(stderr, "console: %v\n", err)
		return 1
	}
	defer closeSessionRecorder(session, "console")

	if strings.TrimSpace(utterance) != "" {
		output, err := session.Handle(utterance, runtime)
		if err != nil {
			fmt.Fprintf(stderr, "console: %v\n", err)
			return 1
		}
		fmt.Fprint(stdout, output)
		return 0
	}

	fmt.Fprintln(stdout, "Lore Console")
	fmt.Fprintln(stdout, "Lore runs a bounded natural-language agent loop. Type `help` for examples. Type `exit` to quit. A configured model-backed operator agent is required.")
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
			fmt.Fprintln(stdout, "Lore stopped")
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

func RunTUICommand(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, version string) int {
	workDir, utterance, localExec, agentID, day, resume, resumeID, err := parseTUIFlags(args, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	runtime, err := app.OpenRuntime(workDir)
	if err != nil {
		fmt.Fprintf(stderr, "open runtime: %v\n", err)
		return 1
	}
	defer closeRuntime(stderr, runtime, "tui")

	session := console.NewSession(version)
	session.DefaultAgentID = agentID
	session.EnableLocalWorkTools = localExec
	// Same persona extractor wiring + drain defer as the console
	// path; see RunConsoleCommand for the LIFO ordering rationale.
	session.PersonaExtractor = runtime.PersonaExtractor
	session.PersonaExtractLogger = runtime.PersonaExtractLogger
	session.PersonaExtractTimeout = runtime.PersonaExtractTimeout
	defer session.DrainPersonaExtractions(consolePersonaDrainTimeout)
	if err := configureSessionRecorder(session, runtime, resume, resumeID, stdin, stdout, stderr, "tui"); err != nil {
		fmt.Fprintf(stderr, "tui: %v\n", err)
		return 1
	}
	defer closeSessionRecorder(session, "tui")
	interactive := strings.TrimSpace(utterance) == ""
	shellEnabled := shellProfileEnabled(localExec)

	render := func(lastOutput string) error {
		if interactive {
			clearInteractiveTUI(stdout)
		}
		viewModel, err := loadWorkbenchViewModel(version, runtime, session, agentID, day, localExec, shellEnabled, lastOutput)
		if err != nil {
			return err
		}
		fmt.Fprint(stdout, tui.RenderWorkbenchViewModel(viewModel))
		return nil
	}

	if strings.TrimSpace(utterance) != "" {
		output, err := session.Handle(utterance, runtime)
		if err != nil {
			fmt.Fprintf(stderr, "tui: %v\n", err)
			return 1
		}
		if err := render(output); err != nil {
			fmt.Fprintf(stderr, "tui: %v\n", err)
			return 1
		}
		return 0
	}

	if interactive && supportsInteractiveWorkbench(stdin, stdout) {
		driver := interactiveWorkbenchDriver{
			version:      version,
			runtime:      runtime,
			session:      session,
			agentID:      agentID,
			day:          day,
			localExec:    localExec,
			shellEnabled: shellEnabled,
		}
		if err := tui.RunInteractiveWorkbench(stdin, stdout, driver); err != nil {
			fmt.Fprintf(stderr, "tui: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, "Lore stopped")
		return 0
	}

	if err := render(""); err != nil {
		fmt.Fprintf(stderr, "tui: %v\n", err)
		return 1
	}

	scanner := bufio.NewScanner(stdin)
	lastOutput := ""
	for {
		fmt.Fprint(stdout, "\nlore> ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				fmt.Fprintf(stderr, "tui: %v\n", err)
				return 1
			}
			fmt.Fprintln(stdout, "\nLore stopped")
			return 0
		}

		line := strings.TrimSpace(scanner.Text())
		switch strings.ToLower(line) {
		case "":
			continue
		case "/quit", "/exit":
			fmt.Fprintln(stdout, "Lore stopped")
			return 0
		case "/refresh":
			if err := render(lastOutput); err != nil {
				fmt.Fprintf(stderr, "tui: %v\n", err)
				return 1
			}
			continue
		case "/status":
			managed, err := runtime.ManagedStatus()
			if err != nil {
				fmt.Fprintf(stderr, "tui: %v\n", err)
				return 1
			}
			lastOutput = tui.RenderManagedStatus(version, managed) + tui.RenderConfigLayers(runtime.ConfigDiagnostics)
			if err := render(lastOutput); err != nil {
				fmt.Fprintf(stderr, "tui: %v\n", err)
				return 1
			}
			continue
		case "/drafts":
			drafts, err := runtime.ListDrafts()
			if err != nil {
				fmt.Fprintf(stderr, "tui: %v\n", err)
				return 1
			}
			lastOutput = tui.RenderDraftList(drafts)
			if err := render(lastOutput); err != nil {
				fmt.Fprintf(stderr, "tui: %v\n", err)
				return 1
			}
			continue
		}

		output, err := session.Handle(line, runtime)
		if err != nil {
			lastOutput = "Error: " + err.Error()
			if err := render(lastOutput); err != nil {
				fmt.Fprintf(stderr, "tui: %v\n", err)
				return 1
			}
			continue
		}
		lastOutput = output
		if err := render(lastOutput); err != nil {
			fmt.Fprintf(stderr, "tui: %v\n", err)
			return 1
		}
	}
}

func sessionLogRoot(runtime *app.Runtime) string {
	return filepath.Join(runtime.Config.Paths.StateDir, "sessions")
}

func configureSessionRecorder(session *console.Session, runtime *app.Runtime, resume bool, resumeID string, stdin io.Reader, stdout io.Writer, stderr io.Writer, command string) error {
	root := sessionLogRoot(runtime)
	if resumeID == "" && resume {
		if !isTerminalReader(stdin) {
			if err := printRecentSessions(root, stdout); err != nil {
				return err
			}
			return fmt.Errorf("--resume requires an interactive terminal; use --resume-id <id> in scripts")
		}
		selected, err := chooseRecentSession(root, stdin, stdout)
		if err != nil {
			return err
		}
		resumeID = selected
	}
	if resumeID != "" {
		recorder, snapshot, err := sessionlog.Resume(root, resumeID)
		if err != nil {
			return fmt.Errorf("resume %s: %w", resumeID, err)
		}
		session.History = append([]operatoragent.ConversationTurn(nil), snapshot.History...)
		session.WorkingSet = append([]operatoragent.WorkingSetItem(nil), snapshot.WorkingSet...)
		session.Recorder = recorder
		fmt.Fprintf(stderr, "%s resumed session %s\n", command, recorder.SessionID())
		return nil
	}

	now := time.Now()
	recorder, err := sessionlog.Start(root, sessionlog.Meta{
		AgentID:   "lore",
		Model:     strings.TrimSpace(os.Getenv("LORE_MODEL")),
		WorkDir:   runtime.Config.Paths.WorkDir,
		VaultRoot: runtime.Config.Paths.VaultRoot,
		StartedAt: now,
	})
	if err != nil {
		return err
	}
	session.Recorder = recorder
	return nil
}

func closeSessionRecorder(session *console.Session, reason string) {
	if recorder, ok := session.Recorder.(interface{ Close(string) error }); ok {
		_ = recorder.Close(reason)
	}
}

func chooseRecentSession(root string, stdin io.Reader, stdout io.Writer) (string, error) {
	if err := printRecentSessions(root, stdout); err != nil {
		return "", err
	}
	fmt.Fprint(stdout, "Select session number: ")
	scanner := bufio.NewScanner(stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return "", fmt.Errorf("no session selected")
	}
	choice := strings.TrimSpace(scanner.Text())
	recent, err := sessionlog.ListRecent(root, 20)
	if err != nil {
		return "", err
	}
	for i, summary := range recent {
		if choice == fmt.Sprintf("%d", i+1) || choice == summary.ID {
			return summary.ID, nil
		}
	}
	return "", fmt.Errorf("invalid session selection: %s", choice)
}

func printRecentSessions(root string, stdout io.Writer) error {
	recent, err := sessionlog.ListRecent(root, 20)
	if err != nil {
		return err
	}
	if len(recent) == 0 {
		return fmt.Errorf("no previous sessions found")
	}
	fmt.Fprintln(stdout, "Resume session")
	for i, summary := range recent {
		fmt.Fprintf(stdout, "%d. %s  %s  %s\n", i+1, summary.ID, summary.UpdatedAt.Format("2006-01-02 15:04"), summary.Title)
	}
	return nil
}

func clearInteractiveTUI(stdout io.Writer) {
	file, ok := stdout.(*os.File)
	if !ok {
		return
	}
	info, err := file.Stat()
	if err != nil {
		return
	}
	if info.Mode()&os.ModeCharDevice == 0 {
		return
	}
	fmt.Fprint(stdout, "\x1b[H\x1b[2J")
}

func isTerminalReader(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
func supportsInteractiveWorkbench(stdin io.Reader, stdout io.Writer) bool {
	inputFile, ok := stdin.(*os.File)
	if !ok {
		return false
	}
	outputFile, ok := stdout.(*os.File)
	if !ok {
		return false
	}
	inputInfo, err := inputFile.Stat()
	if err != nil || inputInfo.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	outputInfo, err := outputFile.Stat()
	if err != nil || outputInfo.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	return true
}

func runDraftCommand(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "draft: missing subcommand")
		fmt.Fprintln(stderr, "usage: lore draft <list|review|approve|reject|request-revision|apply> [flags] [id]")
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
		defer closeRuntime(stderr, runtime, "draft list")
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
		defer closeRuntime(stderr, runtime, "draft review")
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
		defer closeRuntime(stderr, runtime, "draft "+args[0])

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

func runFindingsCommand(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: lore findings <list|resolve|ignore> [flags] [id]")
		return 1
	}

	switch args[0] {
	case "list":
		workDir, limit, err := parseFindingsListFlags(args[1:], stderr)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		runtime, err := app.OpenRuntime(workDir)
		if err != nil {
			fmt.Fprintf(stderr, "open runtime: %v\n", err)
			return 1
		}
		defer closeRuntime(stderr, runtime, "findings list")
		findings, err := runtime.ListFindings(limit)
		if err != nil {
			fmt.Fprintf(stderr, "findings list: %v\n", err)
			return 1
		}
		renderFindingList(stdout, findings)
		return 0
	case "resolve", "ignore":
		workDir, findingID, err := parseFindingActionFlags("findings "+args[0], args[1:], stderr)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		runtime, err := app.OpenRuntime(workDir)
		if err != nil {
			fmt.Fprintf(stderr, "open runtime: %v\n", err)
			return 1
		}
		defer closeRuntime(stderr, runtime, "findings "+args[0])

		var finding model.Finding
		switch args[0] {
		case "resolve":
			finding, err = runtime.ResolveFinding(findingID)
		case "ignore":
			finding, err = runtime.IgnoreFinding(findingID)
		}
		if err != nil {
			fmt.Fprintf(stderr, "findings %s: %v\n", args[0], err)
			return 1
		}
		renderFindingActionResult(stdout, args[0], finding)
		return 0
	default:
		fmt.Fprintf(stderr, "findings: unknown subcommand %q\n", args[0])
		return 1
	}
}

func runPersonaCommand(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: lore persona <candidates|errors> [args]")
		fmt.Fprintln(stderr, "  candidates  list/show/dismiss/draft/recover persona memory candidates")
		fmt.Fprintln(stderr, "  errors      tail the persona extraction failure log")
		return 1
	}
	switch args[0] {
	case "candidates":
		// handled below
	case "errors":
		return runPersonaErrorsCommand(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "persona: unknown subcommand %q\n", args[0])
		return 1
	}
	sub := args[1:]
	if len(sub) == 0 {
		fmt.Fprintln(stderr, "usage: lore persona candidates <list|show|dismiss|draft|recover> [flags] [id]")
		return 1
	}
	switch sub[0] {
	case "list":
		workDir, stateFilter, limit, err := parsePersonaListFlags(sub[1:], stderr)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		runtime, err := app.OpenRuntime(workDir)
		if err != nil {
			fmt.Fprintf(stderr, "open runtime: %v\n", err)
			return 1
		}
		defer closeRuntime(stderr, runtime, "persona candidates list")
		records, err := runtime.ListPersonaCandidates(stateFilter, limit)
		if err != nil {
			fmt.Fprintf(stderr, "persona candidates list: %v\n", err)
			return 1
		}
		renderPersonaCandidateList(stdout, stateFilter, records)
		return 0
	case "show", "dismiss":
		workDir, candidateID, err := parsePersonaActionFlags("persona candidates "+sub[0], sub[1:], stderr)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		runtime, err := app.OpenRuntime(workDir)
		if err != nil {
			fmt.Fprintf(stderr, "open runtime: %v\n", err)
			return 1
		}
		defer closeRuntime(stderr, runtime, "persona candidates "+sub[0])

		switch sub[0] {
		case "show":
			record, err := runtime.GetPersonaCandidate(candidateID)
			if err != nil {
				fmt.Fprintf(stderr, "persona candidates show: %v\n", err)
				return 1
			}
			renderPersonaCandidateDetail(stdout, record)
		case "dismiss":
			record, err := runtime.DismissPersonaCandidate(candidateID, time.Now())
			if err != nil {
				fmt.Fprintf(stderr, "persona candidates dismiss: %v\n", err)
				// Operator hint when the candidate is in any
				// Drafted shape: dismiss deliberately refuses
				// to handle drafted candidates because the
				// linked draft (if any) needs to flow through
				// the harness review path first. Surface the
				// two follow-up commands so the operator does
				// not have to dig through docs.
				if errors.Is(err, app.ErrPersonaCandidateAlreadyDrafted) {
					fmt.Fprintln(stderr, "  hint: drafted candidates are not dismissed directly.")
					fmt.Fprintf(stderr, "        if the candidate is in the partial-orphan shape (DraftID empty), abandon it via:\n")
					fmt.Fprintf(stderr, "          lore persona candidates recover --force-dismiss %s\n", candidateID)
					fmt.Fprintln(stderr, "        otherwise reject the linked draft first via `lore draft reject <draft-id>`.")
				}
				return 1
			}
			renderPersonaCandidateActionResult(stdout, "dismiss", record, "")
		}
		return 0
	case "draft":
		workDir, candidateID, retryRejected, err := parsePersonaDraftFlags(sub[1:], stderr)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		runtime, err := app.OpenRuntime(workDir)
		if err != nil {
			fmt.Fprintf(stderr, "open runtime: %v\n", err)
			return 1
		}
		defer closeRuntime(stderr, runtime, "persona candidates draft")
		var record persona.PersonaCandidateRecord
		var result model.PersonaUpdateProposalResult
		if retryRejected {
			record, result, err = runtime.RetryRejectedPersonaDraft(candidateID, time.Now())
		} else {
			record, result, err = runtime.CreatePersonaDraftFromCandidate(candidateID, time.Now())
		}
		if err != nil {
			fmt.Fprintf(stderr, "persona candidates draft: %v\n", err)
			renderPersonaDraftErrorHint(stderr, candidateID, retryRejected, err)
			return 1
		}
		action := "draft"
		if retryRejected {
			action = "draft (retry-rejected)"
		}
		renderPersonaCandidateActionResult(stdout, action, record, result.DraftID)
		return 0
	case "recover":
		workDir, candidateID, draftID, forceDismiss, err := parsePersonaRecoverFlags(sub[1:], stderr)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		runtime, err := app.OpenRuntime(workDir)
		if err != nil {
			fmt.Fprintf(stderr, "open runtime: %v\n", err)
			return 1
		}
		defer closeRuntime(stderr, runtime, "persona candidates recover")
		var record persona.PersonaCandidateRecord
		if forceDismiss {
			record, err = runtime.ForceDismissPartialPersonaCandidate(candidateID, time.Now())
			if err != nil {
				fmt.Fprintf(stderr, "persona candidates recover: %v\n", err)
				return 1
			}
			renderPersonaCandidateActionResult(stdout, "recover --force-dismiss", record, "")
			return 0
		}
		record, err = runtime.RecoverPersonaCandidateLink(candidateID, draftID, time.Now())
		if err != nil {
			fmt.Fprintf(stderr, "persona candidates recover: %v\n", err)
			return 1
		}
		renderPersonaCandidateActionResult(stdout, "recover --link", record, record.DraftID)
		return 0
	default:
		fmt.Fprintf(stderr, "persona candidates: unknown subcommand %q\n", sub[0])
		return 1
	}
}

func parsePersonaListFlags(args []string, stderr io.Writer) (string, persona.PersonaCandidateState, int, error) {
	flags := flag.NewFlagSet("persona candidates list", flag.ContinueOnError)
	flags.SetOutput(stderr)
	workDir := flags.String("workdir", "", "workdir that contains vault/ and state/")
	limit := flags.Int("limit", 20, "maximum candidates to show (0 = no limit)")
	state := flags.String("state", "open", "candidate state to list: open, drafted, dismissed")
	if err := flags.Parse(args); err != nil {
		return "", "", 0, err
	}
	// Reject negative limit explicitly: the store contract treats
	// limit <= 0 as "no cap", so a user typing --limit -1 would
	// silently get unlimited output. Matching the strict-validation
	// shape of parsePersonaErrorsFlags --tail and parseUsageFlags
	// --days so the CLI surface behaves consistently across
	// numeric flags.
	if *limit < 0 {
		return "", "", 0, fmt.Errorf("persona candidates list: --limit must be >= 0")
	}
	resolved, err := defaultWorkDir(*workDir)
	if err != nil {
		return "", "", 0, err
	}
	wantState := persona.NormalizeCandidateState(persona.PersonaCandidateState(strings.TrimSpace(*state)))
	switch wantState {
	case persona.PersonaCandidateOpen, persona.PersonaCandidateDrafted, persona.PersonaCandidateDismissed:
	default:
		return "", "", 0, fmt.Errorf("persona candidates list: --state must be one of open, drafted, dismissed")
	}
	return resolved, wantState, *limit, nil
}

func parsePersonaActionFlags(name string, args []string, stderr io.Writer) (string, string, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	workDir := flags.String("workdir", "", "workdir that contains vault/ and state/")
	if err := flags.Parse(args); err != nil {
		return "", "", err
	}
	remaining := flags.Args()
	if len(remaining) == 0 || strings.TrimSpace(remaining[0]) == "" {
		return "", "", fmt.Errorf("%s: persona candidate id is required", name)
	}
	resolved, err := defaultWorkDir(*workDir)
	if err != nil {
		return "", "", err
	}
	return resolved, strings.TrimSpace(remaining[0]), nil
}

// parsePersonaDraftFlags accepts --workdir and the new --retry-rejected
// flag, then a positional candidate ID. --retry-rejected routes the
// CLI to RetryRejectedPersonaDraft instead of the default
// CreatePersonaDraftFromCandidate, so the same `lore persona candidates
// draft` surface covers both first-attempt promotion and recovery from
// a rejected/expired/superseded draft.
func parsePersonaDraftFlags(args []string, stderr io.Writer) (string, string, bool, error) {
	flags := flag.NewFlagSet("persona candidates draft", flag.ContinueOnError)
	flags.SetOutput(stderr)
	workDir := flags.String("workdir", "", "workdir that contains vault/ and state/")
	retryRejected := flags.Bool("retry-rejected", false, "retry a candidate whose previous draft is rejected, expired, or superseded")
	if err := flags.Parse(args); err != nil {
		return "", "", false, err
	}
	remaining := flags.Args()
	if len(remaining) == 0 || strings.TrimSpace(remaining[0]) == "" {
		return "", "", false, fmt.Errorf("persona candidates draft: persona candidate id is required")
	}
	resolved, err := defaultWorkDir(*workDir)
	if err != nil {
		return "", "", false, err
	}
	return resolved, strings.TrimSpace(remaining[0]), *retryRejected, nil
}

// parsePersonaRecoverFlags accepts --workdir and the recover mode flags
// --link <draft-id> and --force-dismiss. Exactly one of the two modes
// must be supplied; both-set and neither-set are user errors so the
// CLI can never silently default to one path.
func parsePersonaRecoverFlags(args []string, stderr io.Writer) (string, string, string, bool, error) {
	flags := flag.NewFlagSet("persona candidates recover", flag.ContinueOnError)
	flags.SetOutput(stderr)
	workDir := flags.String("workdir", "", "workdir that contains vault/ and state/")
	linkDraftID := flags.String("link", "", "link the candidate to an existing orphan persona_update draft")
	forceDismiss := flags.Bool("force-dismiss", false, "abandon a partial drafted candidate by transitioning it to dismissed")
	if err := flags.Parse(args); err != nil {
		return "", "", "", false, err
	}
	remaining := flags.Args()
	if len(remaining) == 0 || strings.TrimSpace(remaining[0]) == "" {
		return "", "", "", false, fmt.Errorf("persona candidates recover: persona candidate id is required")
	}
	linkSet := strings.TrimSpace(*linkDraftID) != ""
	if linkSet && *forceDismiss {
		return "", "", "", false, fmt.Errorf("persona candidates recover: --link and --force-dismiss are mutually exclusive")
	}
	if !linkSet && !*forceDismiss {
		return "", "", "", false, fmt.Errorf("persona candidates recover: exactly one of --link <draft-id> or --force-dismiss is required")
	}
	resolved, err := defaultWorkDir(*workDir)
	if err != nil {
		return "", "", "", false, err
	}
	return resolved, strings.TrimSpace(remaining[0]), strings.TrimSpace(*linkDraftID), *forceDismiss, nil
}

// renderPersonaDraftErrorHint maps the typed errors returned by
// CreatePersonaDraftFromCandidate and RetryRejectedPersonaDraft
// into operator-facing follow-up commands. The base error message
// is already printed; this helper appends a "hint:" block telling
// the operator which CLI command actually addresses the situation
// (recover --link, recover --force-dismiss, retry-rejected, the
// `lore draft review` path, etc.). Matches the dismiss-hint shape
// at the `case "dismiss":` branch so the operator sees the same
// idiom across persona-candidate failures.
func renderPersonaDraftErrorHint(stderr io.Writer, candidateID string, retryRejected bool, err error) {
	switch {
	case errors.Is(err, app.ErrPersonaCandidateDismissed):
		fmt.Fprintln(stderr, "  hint: this candidate was dismissed; its DedupKey is a tombstone.")
		fmt.Fprintln(stderr, "        the underlying fact will not be re-extracted from a console turn unless")
		fmt.Fprintln(stderr, "        the user repeats the utterance with materially different evidence.")
	case errors.Is(err, app.ErrPersonaCandidateAlreadyDrafted):
		fmt.Fprintln(stderr, "  hint: the candidate is in the partial-orphan shape (Drafted with empty DraftID).")
		fmt.Fprintln(stderr, "        either link it to the orphan persona_update draft (find via `lore draft list`):")
		fmt.Fprintf(stderr, "          lore persona candidates recover --link <draft-id> %s\n", candidateID)
		fmt.Fprintln(stderr, "        or abandon it (DedupKey kept as tombstone):")
		fmt.Fprintf(stderr, "          lore persona candidates recover --force-dismiss %s\n", candidateID)
	case errors.Is(err, app.ErrPersonaCandidateLinkedStateRequired):
		if retryRejected {
			fmt.Fprintln(stderr, "  hint: --retry-rejected only applies to candidates already promoted once.")
			fmt.Fprintf(stderr, "        for a first-time promote, drop the flag:\n          lore persona candidates draft %s\n", candidateID)
			fmt.Fprintln(stderr, "        for a candidate in the partial-orphan shape (Drafted, no DraftID),")
			fmt.Fprintln(stderr, "        use `recover --link` or `recover --force-dismiss` instead.")
		}
	case errors.Is(err, app.ErrPersonaDraftNotTerminalForRetry):
		fmt.Fprintln(stderr, "  hint: the linked draft is still in flight (pending_review / approved / applied).")
		fmt.Fprintln(stderr, "        review it via:")
		fmt.Fprintln(stderr, "          lore draft list                      # locate the linked draft")
		fmt.Fprintln(stderr, "          lore draft review <draft-id>         # inspect it")
		fmt.Fprintln(stderr, "          lore draft reject <draft-id>         # if you want to retry afterwards")
		fmt.Fprintln(stderr, "        retry-rejected only fires after the linked draft reaches rejected /")
		fmt.Fprintln(stderr, "        expired / superseded.")
	}
}

// runPersonaErrorsCommand is the read-side counterpart to the
// persona-extract.log file the console fire-and-forget goroutine
// writes from B-P9. Operators can now `lore persona errors --tail
// 20 --stage extract` instead of computing the workdir-relative
// log path and tailing the file manually. This closes the
// observability loop: B-P9 wires the writer, B-P11c wires the
// reader. The runtime still owns the file path layout (via
// Runtime.PersonaExtractLogPath), so a future move of the log to
// a different subpath needs no CLI change.
func runPersonaErrorsCommand(args []string, stdout io.Writer, stderr io.Writer) int {
	workDir, tail, stageFilter, since, err := parsePersonaErrorsFlags(args, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	runtime, err := app.OpenRuntime(workDir)
	if err != nil {
		fmt.Fprintf(stderr, "open runtime: %v\n", err)
		return 1
	}
	defer closeRuntime(stderr, runtime, "persona errors")
	return renderPersonaExtractErrors(stdout, stderr, runtime.PersonaExtractLogPath(), tail, stageFilter, since, time.Now().UTC())
}

func parsePersonaErrorsFlags(args []string, stderr io.Writer) (workDir string, tail int, stage string, since time.Duration, err error) {
	flags := flag.NewFlagSet("persona errors", flag.ContinueOnError)
	flags.SetOutput(stderr)
	workDirFlag := flags.String("workdir", "", "workdir that contains vault/ and state/")
	tailFlag := flags.Int("tail", 20, "show only the last N matching lines (0 = no limit)")
	stageFlag := flags.String("stage", "", "filter by stage: extract, store, parse_warning (empty = all)")
	sinceFlag := flags.Duration("since", 0, "only show lines newer than this duration (e.g. 1h, 30m; 0 = no time filter)")
	if parseErr := flags.Parse(args); parseErr != nil {
		err = parseErr
		return
	}
	if *tailFlag < 0 {
		err = fmt.Errorf("persona errors: --tail must be >= 0")
		return
	}
	if s := strings.TrimSpace(*stageFlag); s != "" {
		switch s {
		case "extract", "store", "parse_warning":
		default:
			err = fmt.Errorf("persona errors: --stage must be one of extract, store, parse_warning")
			return
		}
	}
	if *sinceFlag < 0 {
		err = fmt.Errorf("persona errors: --since must be >= 0")
		return
	}
	resolved, resolveErr := defaultWorkDir(*workDirFlag)
	if resolveErr != nil {
		err = resolveErr
		return
	}
	workDir = resolved
	tail = *tailFlag
	stage = strings.TrimSpace(*stageFlag)
	since = *sinceFlag
	return
}

// personaLogEntry is the parsed shape of a single persona-extract.log
// line. Lines that fail to parse (malformed, truncated) are still
// retained with parsed=false so the operator sees the raw line; the
// stage/since filters skip them rather than crashing.
type personaLogEntry struct {
	raw    string
	ts     time.Time
	stage  string
	parsed bool
}

func parsePersonaLogLine(raw string) personaLogEntry {
	fields := strings.Split(raw, "\t")
	if len(fields) < 4 {
		return personaLogEntry{raw: raw}
	}
	ts, err := time.Parse(time.RFC3339Nano, fields[0])
	if err != nil {
		return personaLogEntry{raw: raw}
	}
	if !strings.HasPrefix(fields[1], "stage=") {
		return personaLogEntry{raw: raw, ts: ts}
	}
	return personaLogEntry{
		raw:    raw,
		ts:     ts,
		stage:  strings.TrimPrefix(fields[1], "stage="),
		parsed: true,
	}
}

// renderPersonaExtractErrors reads the persona-extract.log file at
// logPath, applies the requested filters, and prints a tail-style
// report to stdout. now is parameter-injected so tests can pin a
// stable "now" against the file's RFC3339Nano timestamps when
// exercising the --since filter.
func renderPersonaExtractErrors(stdout, stderr io.Writer, logPath string, tail int, stageFilter string, since time.Duration, now time.Time) int {
	header := func() {
		fmt.Fprintln(stdout, "Persona Extraction Errors")
		fmt.Fprintln(stdout, "=========================")
		fmt.Fprintf(stdout, "Source: %s\n", logPath)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			header()
			fmt.Fprintln(stdout)
			fmt.Fprintln(stdout, "No log file yet. Either extraction has not produced any failures in this workdir, or the runtime has not been opened here.")
			return 0
		}
		fmt.Fprintf(stderr, "persona errors: read log: %v\n", err)
		return 1
	}

	var cutoff time.Time
	if since > 0 {
		cutoff = now.Add(-since)
	}

	var all []personaLogEntry
	for _, raw := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		all = append(all, parsePersonaLogLine(raw))
	}

	filtered := make([]personaLogEntry, 0, len(all))
	for _, e := range all {
		if stageFilter != "" && (!e.parsed || e.stage != stageFilter) {
			continue
		}
		if since > 0 {
			if e.ts.IsZero() || e.ts.Before(cutoff) {
				continue
			}
		}
		filtered = append(filtered, e)
	}

	shown := filtered
	if tail > 0 && len(filtered) > tail {
		shown = filtered[len(filtered)-tail:]
	}

	header()
	var filterParts []string
	if tail > 0 {
		filterParts = append(filterParts, fmt.Sprintf("tail=%d", tail))
	}
	if stageFilter != "" {
		filterParts = append(filterParts, fmt.Sprintf("stage=%s", stageFilter))
	}
	if since > 0 {
		filterParts = append(filterParts, fmt.Sprintf("since=%s", since))
	}
	if len(filterParts) > 0 {
		fmt.Fprintf(stdout, "Filter: %s\n", strings.Join(filterParts, ", "))
	}
	fmt.Fprintln(stdout)

	if len(shown) == 0 {
		fmt.Fprintln(stdout, "No matching entries.")
		return 0
	}

	for _, e := range shown {
		fmt.Fprintln(stdout, e.raw)
	}
	fmt.Fprintln(stdout)
	if len(filtered) != len(all) {
		fmt.Fprintf(stdout, "%d of %d entries shown (filtered from %d total).\n", len(shown), len(filtered), len(all))
	} else {
		fmt.Fprintf(stdout, "%d of %d entries shown.\n", len(shown), len(all))
	}
	return 0
}

func renderPersonaCandidateList(stdout io.Writer, state persona.PersonaCandidateState, records []persona.PersonaCandidateRecord) {
	fmt.Fprintln(stdout, "Persona Candidates")
	fmt.Fprintln(stdout, "==================")
	fmt.Fprintf(stdout, "State: %s\n", state)
	fmt.Fprintln(stdout)
	if len(records) == 0 {
		fmt.Fprintln(stdout, "No candidates.")
		return
	}
	fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", "ID", "FIELD", "PROPOSED", "CONFIDENCE", "CONFLICT", "SOURCE", "DRAFT")
	for _, record := range records {
		conflict := "no"
		if record.Candidate.Conflict {
			conflict = "yes"
		}
		// DRAFT column carries the linked draft ID when the candidate
		// has been promoted (State=Drafted with DraftID populated by
		// LinkCandidateDraft). Empty / partial / open / dismissed
		// rows render "-" rather than blank so awk/cut pipelines keep
		// a stable 7-column schema regardless of state filter.
		draftID := strings.TrimSpace(record.DraftID)
		if draftID == "" {
			draftID = "-"
		}
		fmt.Fprintf(
			stdout,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			record.ID,
			record.Candidate.Field,
			clipOneLine(record.Candidate.ProposedValue, 40),
			record.Candidate.Confidence,
			conflict,
			record.Candidate.SourceKind,
			draftID,
		)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Next:")
	fmt.Fprintln(stdout, "  lore persona candidates show <id>")
	fmt.Fprintln(stdout, "  lore persona candidates draft <id>")
	fmt.Fprintln(stdout, "  lore persona candidates dismiss <id>")
}

func renderPersonaCandidateDetail(stdout io.Writer, record persona.PersonaCandidateRecord) {
	fmt.Fprintln(stdout, "Persona Candidate")
	fmt.Fprintln(stdout, "=================")
	fmt.Fprintf(stdout, "ID:             %s\n", record.ID)
	fmt.Fprintf(stdout, "State:          %s\n", record.State)
	if strings.TrimSpace(record.DraftID) != "" {
		fmt.Fprintf(stdout, "Draft:          %s\n", record.DraftID)
	}
	fmt.Fprintf(stdout, "Field:          %s\n", record.Candidate.Field)
	fmt.Fprintf(stdout, "Proposed value: %s\n", record.Candidate.ProposedValue)
	if strings.TrimSpace(record.Candidate.CurrentValue) != "" {
		fmt.Fprintf(stdout, "Current value:  %s\n", record.Candidate.CurrentValue)
	}
	fmt.Fprintf(stdout, "Evidence:       %s\n", record.Candidate.EvidenceQuote)
	if strings.TrimSpace(record.Candidate.Reason) != "" {
		fmt.Fprintf(stdout, "Reason:         %s\n", record.Candidate.Reason)
	}
	fmt.Fprintf(stdout, "Confidence:     %s\n", record.Candidate.Confidence)
	fmt.Fprintf(stdout, "Conflict:       %v\n", record.Candidate.Conflict)
	fmt.Fprintf(stdout, "Source:         %s\n", record.Candidate.SourceKind)
	if strings.TrimSpace(record.Candidate.SourceSessionID) != "" {
		fmt.Fprintf(stdout, "Session:        %s\n", record.Candidate.SourceSessionID)
	}
	fmt.Fprintf(stdout, "Observed at:    %s\n", record.Candidate.ObservedAt.Format(time.RFC3339))
	fmt.Fprintf(stdout, "Created at:     %s\n", record.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(stdout, "Updated at:     %s\n", record.UpdatedAt.Format(time.RFC3339))
	fmt.Fprintf(stdout, "Dedup key:      %s\n", record.DedupKey)
}

func renderPersonaCandidateActionResult(stdout io.Writer, action string, record persona.PersonaCandidateRecord, draftID string) {
	fmt.Fprintln(stdout, "Persona Candidate Updated")
	fmt.Fprintln(stdout, "=========================")
	fmt.Fprintf(stdout, "Action: %s\n", action)
	fmt.Fprintf(stdout, "ID:     %s\n", record.ID)
	fmt.Fprintf(stdout, "State:  %s\n", record.State)
	fmt.Fprintf(stdout, "Field:  %s\n", record.Candidate.Field)
	if draftID != "" {
		fmt.Fprintf(stdout, "Draft:  %s (pending_review; use `lore draft review %s`)\n", draftID, draftID)
	}
}

func runUsageCommand(args []string, stdout io.Writer, stderr io.Writer) int {
	workDir, days, err := parseUsageFlags(args, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	runtime, err := app.OpenRuntime(workDir)
	if err != nil {
		fmt.Fprintf(stderr, "open runtime: %v\n", err)
		return 1
	}
	defer closeRuntime(stderr, runtime, "usage")

	now := time.Now()
	// Iterate from oldest to newest day so the output reads left to
	// right in time order. SummarizeUsage normalizes each day, so
	// passing different times-of-day on the same day collapses
	// correctly.
	daily := make([]model.UsageSummary, 0, days)
	for offset := days - 1; offset >= 0; offset-- {
		day := now.AddDate(0, 0, -offset)
		summary, err := runtime.SummarizeUsage(day)
		if err != nil {
			fmt.Fprintf(stderr, "usage: %v\n", err)
			return 1
		}
		daily = append(daily, summary)
	}
	renderUsageReport(stdout, days, daily)
	return 0
}

func parseUsageFlags(args []string, stderr io.Writer) (string, int, error) {
	flags := flag.NewFlagSet("usage", flag.ContinueOnError)
	flags.SetOutput(stderr)
	days := flags.Int("days", 1, "number of trailing days to summarize (>=1)")
	if err := flags.Parse(args); err != nil {
		return "", 0, err
	}
	if *days < 1 {
		return "", 0, fmt.Errorf("usage: --days must be >= 1")
	}
	// Allow positional [workdir] after flags so both `lore usage ./dir`
	// and `lore usage --days 7 ./dir` work, matching the style of
	// `lore status [workdir]`.
	resolved, err := resolveWorkDir(flags.Args())
	if err != nil {
		return "", 0, err
	}
	return resolved, *days, nil
}

func renderUsageReport(stdout io.Writer, days int, daily []model.UsageSummary) {
	fmt.Fprintln(stdout, "Usage")
	fmt.Fprintln(stdout, "=====")
	if days == 1 {
		fmt.Fprintln(stdout, "Window: today")
	} else {
		fmt.Fprintf(stdout, "Window: trailing %d days\n", days)
	}

	var totalCalls, totalPrompt, totalCompletion int
	for _, summary := range daily {
		totalCalls += summary.Calls
		totalPrompt += summary.PromptTokens
		totalCompletion += summary.CompletionTokens
	}

	if totalCalls == 0 {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "No usage recorded.")
		return
	}

	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "%-12s %8s %10s %12s %8s\n", "DAY", "CALLS", "PROMPT", "COMPLETION", "TOTAL")
	for _, summary := range daily {
		fmt.Fprintf(
			stdout,
			"%-12s %8d %10d %12d %8d\n",
			summary.Day.Format("2006-01-02"),
			summary.Calls,
			summary.PromptTokens,
			summary.CompletionTokens,
			summary.TotalTokens,
		)
	}
	// B-P11a: surface a per-purpose breakdown so operators can see
	// what fraction of cost went to chat vs persona_extract vs
	// process_sink. Aggregate across the queried window because
	// printing breakdown rows under every DAY row would crowd the
	// table; the daily DAY/CALLS/PROMPT/COMPLETION/TOTAL line still
	// answers "what did today cost overall".
	purposeTotals := map[string]model.UsagePurposeStats{}
	for _, summary := range daily {
		for purpose, stats := range summary.PurposeBreakdown {
			agg := purposeTotals[purpose]
			agg.Calls += stats.Calls
			agg.PromptTokens += stats.PromptTokens
			agg.CompletionTokens += stats.CompletionTokens
			purposeTotals[purpose] = agg
		}
	}
	if len(purposeTotals) > 0 {
		purposes := make([]string, 0, len(purposeTotals))
		for purpose := range purposeTotals {
			purposes = append(purposes, purpose)
		}
		sort.Strings(purposes)
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "By purpose:")
		for _, purpose := range purposes {
			label := purpose
			if label == "" {
				label = "unspecified"
			}
			stats := purposeTotals[purpose]
			fmt.Fprintf(
				stdout,
				"  %-16s %d calls / %d prompt + %d completion = %d tokens\n",
				label,
				stats.Calls,
				stats.PromptTokens,
				stats.CompletionTokens,
				stats.TotalTokens(),
			)
		}
	}

	fmt.Fprintln(stdout)
	fmt.Fprintf(
		stdout,
		"Total: %d calls / %d prompt + %d completion = %d tokens\n",
		totalCalls, totalPrompt, totalCompletion, totalPrompt+totalCompletion,
	)
}

func renderTodayUsageTail(summary model.UsageSummary) string {
	return fmt.Sprintf(
		"\nToday Usage\n-----------\n%d calls / %d prompt + %d completion = %d tokens\n",
		summary.Calls, summary.PromptTokens, summary.CompletionTokens, summary.TotalTokens,
	)
}

func renderFindingList(stdout io.Writer, findings []model.Finding) {
	fmt.Fprintln(stdout, "Findings")
	fmt.Fprintln(stdout, "========")
	if len(findings) == 0 {
		fmt.Fprintln(stdout, "No findings.")
		return
	}
	fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\t%s\n", "ID", "STATE", "SEVERITY", "KIND", "TARGET", "TITLE")
	for _, finding := range findings {
		fmt.Fprintf(
			stdout,
			"%s\t%s\t%s\t%s\t%s\t%s\n",
			finding.ID,
			string(finding.State),
			string(finding.Severity),
			string(finding.Kind),
			finding.Target.Path,
			clipOneLine(finding.Title, 72),
		)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Next:")
	fmt.Fprintln(stdout, "  lore findings resolve <id>")
	fmt.Fprintln(stdout, "  lore findings ignore <id>")
}

func renderFindingActionResult(stdout io.Writer, action string, finding model.Finding) {
	fmt.Fprintln(stdout, "Finding Updated")
	fmt.Fprintln(stdout, "===============")
	fmt.Fprintf(stdout, "Action: %s\n", action)
	fmt.Fprintf(stdout, "ID: %s\n", finding.ID)
	fmt.Fprintf(stdout, "State: %s\n", finding.State)
	fmt.Fprintf(stdout, "Kind: %s\n", finding.Kind)
	fmt.Fprintf(stdout, "Target: %s\n", finding.Target.Path)
	fmt.Fprintf(stdout, "Title: %s\n", finding.Title)
}

func runDaemonCommand(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "daemon: missing subcommand")
		fmt.Fprintln(stderr, "usage: lore daemon run [--workdir <dir>] [--poll 2s] [--debounce 500ms] [--once] [--codex-jsonl <session.jsonl>]")
		return 1
	}

	switch args[0] {
	case "run":
		workDir, pollEvery, debounce, once, codexParams, err := parseDaemonFlags(args[1:], stderr)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}

		runtime, err := app.OpenRuntime(workDir)
		if err != nil {
			fmt.Fprintf(stderr, "open runtime: %v\n", err)
			return 1
		}
		defer closeRuntime(stderr, runtime, "daemon run")
		runtime.Config.Vault.DebounceWindow = debounce

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		if err := runtime.RunVaultDaemon(ctx, app.VaultDaemonRunOptions{
			PollEvery:  pollEvery,
			Once:       once,
			Stdout:     stdout,
			CodexJSONL: codexParams,
		}); err != nil {
			fmt.Fprintf(stderr, "daemon run: %v\n", err)
			return 1
		}
		return 0
	default:
		fmt.Fprintf(stderr, "daemon: unknown subcommand %q\n", args[0])
		return 1
	}
}

func runProcessSinkCommand(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "process-sink: missing subcommand")
		fmt.Fprintln(stderr, "usage: lore process-sink day [--workdir <dir>] [--agent <id>] [--day YYYY-MM-DD]")
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
		defer closeRuntime(stderr, runtime, "process-sink day")
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
		fmt.Fprintln(stderr, "usage: lore models list")
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
			fmt.Fprintln(stderr, "models list: configure LORE_LLM_BASE_URL and LORE_LLM_API_KEY first (legacy OBSIDIAN_HARNESS_LLM_* is still supported)")
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
	defer closeRuntime(stderr, runtime, "sync-codex-jsonl")

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
	defer closeRuntime(stderr, runtime, "attach-codex-jsonl")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if once {
		result, err := runtime.SyncCodexJSONL(params, time.Now())
		if err != nil {
			fmt.Fprintf(stderr, "attach-codex-jsonl: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Attaching Codex JSONL\n- input: %s\n- poll: %s\n", params.InputPath, pollEvery)
		writeCodexAttachSummary(stdout, result)
		return 0
	}

	if err := RunCodexAttachLoop(ctx, runtime, params, pollEvery, stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "attach-codex-jsonl: %v\n", err)
		return 1
	}
	return 0
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

func parseExternalJSONLFlags(name string, args []string, stderr io.Writer) (app.ImportExternalTranscriptJSONLParams, string, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)

	workDir := flags.String("workdir", "", "workdir that contains vault/ and state/")
	inputPath := flags.String("input", "", "path to external agent transcript JSONL")
	agentID := flags.String("agent", "", "override external agent id")
	sessionID := flags.String("session", "", "override external session id")
	windowSize := flags.Duration("window", 0, "override checkpoint window size, e.g. 30m")
	skipRollup := flags.Bool("skip-rollup", false, "skip daily rollup after import")

	if err := flags.Parse(args); err != nil {
		return app.ImportExternalTranscriptJSONLParams{}, "", err
	}
	if strings.TrimSpace(*inputPath) == "" {
		return app.ImportExternalTranscriptJSONLParams{}, "", fmt.Errorf("%s: --input is required", name)
	}
	if strings.TrimSpace(*workDir) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return app.ImportExternalTranscriptJSONLParams{}, "", err
		}
		*workDir = cwd
	}

	return app.ImportExternalTranscriptJSONLParams{
		InputPath:  *inputPath,
		AgentID:    *agentID,
		SessionID:  *sessionID,
		Window:     *windowSize,
		SkipRollup: *skipRollup,
	}, filepath.Clean(*workDir), nil
}

func parseSessionsListFlags(args []string, stderr io.Writer) (string, int, error) {
	flags := flag.NewFlagSet("sessions list", flag.ContinueOnError)
	flags.SetOutput(stderr)

	workDir := flags.String("workdir", "", "workdir that contains vault/ and state/")
	limit := flags.Int("limit", 20, "maximum sessions to show")
	if err := flags.Parse(args); err != nil {
		return "", 0, err
	}
	resolved, err := defaultWorkDir(*workDir)
	if err != nil {
		return "", 0, err
	}
	return resolved, *limit, nil
}

func parseSessionsShowFlags(args []string, stderr io.Writer) (string, string, error) {
	flags := flag.NewFlagSet("sessions show", flag.ContinueOnError)
	flags.SetOutput(stderr)

	workDir := flags.String("workdir", "", "workdir that contains vault/ and state/")
	if err := flags.Parse(args); err != nil {
		return "", "", err
	}
	remaining := flags.Args()
	if len(remaining) == 0 || strings.TrimSpace(remaining[0]) == "" {
		return "", "", fmt.Errorf("sessions show: session id is required")
	}
	resolved, err := defaultWorkDir(*workDir)
	if err != nil {
		return "", "", err
	}
	return resolved, strings.TrimSpace(remaining[0]), nil
}
func parseSessionsSearchFlags(args []string, stderr io.Writer) (string, string, int, error) {
	flags := flag.NewFlagSet("sessions search", flag.ContinueOnError)
	flags.SetOutput(stderr)

	workDir := flags.String("workdir", "", "workdir that contains vault/ and state/")
	limit := flags.Int("limit", 20, "maximum sessions to show")
	if err := flags.Parse(args); err != nil {
		return "", "", 0, err
	}
	query := strings.TrimSpace(strings.Join(flags.Args(), " "))
	if query == "" {
		return "", "", 0, fmt.Errorf("sessions search: query is required")
	}
	resolved, err := defaultWorkDir(*workDir)
	if err != nil {
		return "", "", 0, err
	}
	return resolved, query, *limit, nil
}

func defaultWorkDir(value string) (string, error) {
	if strings.TrimSpace(value) != "" {
		return filepath.Clean(value), nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Clean(cwd), nil
}
func parseConsoleFlags(args []string, stderr io.Writer) (string, string, bool, bool, string, error) {
	flags := flag.NewFlagSet("console", flag.ContinueOnError)
	flags.SetOutput(stderr)

	workDir := flags.String("workdir", "", "workdir that contains vault/ and state/")
	once := flags.String("once", "", "single utterance to execute")
	localExec := flags.Bool("local-exec", false, "expose local workspace tools in the main agent loop")
	resume := flags.Bool("resume", false, "choose one of the recent 20 workspace sessions to resume")
	resumeID := flags.String("resume-id", "", "resume a specific workspace session id")
	if err := flags.Parse(args); err != nil {
		return "", "", false, false, "", err
	}
	if strings.TrimSpace(*workDir) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", "", false, false, "", err
		}
		*workDir = cwd
	}
	return filepath.Clean(*workDir), strings.TrimSpace(*once), *localExec, *resume, strings.TrimSpace(*resumeID), nil
}

func parseTUIFlags(args []string, stderr io.Writer) (string, string, bool, string, time.Time, bool, string, error) {
	flags := flag.NewFlagSet("tui", flag.ContinueOnError)
	flags.SetOutput(stderr)

	workDir := flags.String("workdir", "", "workdir that contains vault/ and state/")
	once := flags.String("once", "", "single utterance to execute")
	localExec := flags.Bool("local-exec", false, "expose local workspace tools in the main agent loop")
	agentID := flags.String("agent", "codex", "default agent for process-sink panel")
	dayRaw := flags.String("day", "", "day for process-sink panel (YYYY-MM-DD)")
	resume := flags.Bool("resume", false, "choose one of the recent 20 workspace sessions to resume")
	resumeID := flags.String("resume-id", "", "resume a specific workspace session id")
	if err := flags.Parse(args); err != nil {
		return "", "", false, "", time.Time{}, false, "", err
	}
	if strings.TrimSpace(*workDir) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", "", false, "", time.Time{}, false, "", err
		}
		*workDir = cwd
	}

	day, err := resolveWorkbenchDay(*dayRaw, time.Now())
	if err != nil {
		return "", "", false, "", time.Time{}, false, "", fmt.Errorf("tui: invalid --day: %w", err)
	}

	return filepath.Clean(*workDir), strings.TrimSpace(*once), *localExec, strings.TrimSpace(*agentID), day, *resume, strings.TrimSpace(*resumeID), nil
}
func parseDaemonFlags(args []string, stderr io.Writer) (string, time.Duration, time.Duration, bool, *app.ImportCodexJSONLParams, error) {
	flags := flag.NewFlagSet("daemon run", flag.ContinueOnError)
	flags.SetOutput(stderr)

	workDir := flags.String("workdir", "", "workdir that contains vault/ and state/")
	pollEvery := flags.Duration("poll", 2*time.Second, "poll interval for vault scans")
	debounce := flags.Duration("debounce", 500*time.Millisecond, "minimum stable age before processing a file change")
	once := flags.Bool("once", false, "run one scan and exit")
	codexJSONL := flags.String("codex-jsonl", "", "path to a Codex session JSONL to sync each cycle")
	agentID := flags.String("agent", "", "override Codex agent id for daemon transcript sync")
	sessionID := flags.String("session", "", "override Codex session id for daemon transcript sync")
	windowSize := flags.Duration("window", 0, "override checkpoint window size for daemon transcript sync")
	skipRollup := flags.Bool("skip-rollup", false, "skip daily rollup for daemon transcript sync")

	if err := flags.Parse(args); err != nil {
		return "", 0, 0, false, nil, err
	}
	if strings.TrimSpace(*workDir) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", 0, 0, false, nil, err
		}
		*workDir = cwd
	}

	var codexParams *app.ImportCodexJSONLParams
	if strings.TrimSpace(*codexJSONL) != "" {
		cleanInput := filepath.Clean(strings.TrimSpace(*codexJSONL))
		codexParams = &app.ImportCodexJSONLParams{
			InputPath:  cleanInput,
			AgentID:    strings.TrimSpace(*agentID),
			SessionID:  strings.TrimSpace(*sessionID),
			Window:     *windowSize,
			SkipRollup: *skipRollup,
		}
	}

	return filepath.Clean(*workDir), *pollEvery, *debounce, *once, codexParams, nil
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

func parseFindingsListFlags(args []string, stderr io.Writer) (string, int, error) {
	flags := flag.NewFlagSet("findings list", flag.ContinueOnError)
	flags.SetOutput(stderr)

	workDir := flags.String("workdir", "", "workdir that contains vault/ and state/")
	limit := flags.Int("limit", 20, "maximum findings to show")
	if err := flags.Parse(args); err != nil {
		return "", 0, err
	}
	resolved, err := defaultWorkDir(*workDir)
	if err != nil {
		return "", 0, err
	}
	return resolved, *limit, nil
}

func parseFindingActionFlags(name string, args []string, stderr io.Writer) (string, string, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)

	workDir := flags.String("workdir", "", "workdir that contains vault/ and state/")
	if err := flags.Parse(args); err != nil {
		return "", "", err
	}
	remaining := flags.Args()
	if len(remaining) == 0 || strings.TrimSpace(remaining[0]) == "" {
		return "", "", fmt.Errorf("%s: finding id is required", name)
	}
	resolved, err := defaultWorkDir(*workDir)
	if err != nil {
		return "", "", err
	}
	return resolved, strings.TrimSpace(remaining[0]), nil
}

func isConsoleExit(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "exit", "quit", "q":
		return true
	default:
		return false
	}
}

func resolveWorkbenchDay(raw string, now time.Time) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return model.NormalizeDay(now), nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", raw, time.Local)
	if err != nil {
		return time.Time{}, err
	}
	return model.NormalizeDay(parsed), nil
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

func shellProfileEnabled(localExec bool) bool {
	if !localExec {
		return false
	}
	value := strings.TrimSpace(os.Getenv("LORE_AGENT_ENABLE_SHELL"))
	return value == "1" || strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
}

func closeRuntime(stderr io.Writer, runtime *app.Runtime, scope string) {
	if runtime == nil {
		return
	}
	if err := runtime.Close(); err != nil {
		fmt.Fprintf(stderr, "%s: close runtime: %v\n", scope, err)
	}
}
