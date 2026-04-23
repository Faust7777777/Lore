package cli

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

	"obsidian-harness/internal/adapter/codexappserver"
	"obsidian-harness/internal/app"
)

type codexAppServerReaderCloser interface {
	ReadThread(params codexappserver.ReadThreadParams) (codexappserver.Thread, error)
	Close() error
}

var startCodexAppServerProcess = func(ctx context.Context, workDir string, command string, args []string, version string) (codexAppServerReaderCloser, error) {
	return codexappserver.StartProcess(ctx, workDir, command, args, codexappserver.ClientInfo{
		Name:    "lore",
		Title:   "Lore",
		Version: version,
	}, codexappserver.Capabilities{
		ExperimentalAPI: true,
	})
}

func runImportCodexAppServer(args []string, stdout io.Writer, stderr io.Writer, version string) int {
	params, workDir, serverWorkDir, command, commandArgs, err := parseCodexAppServerFlags("import-codex-appserver", args, stderr)
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

	client, err := startCodexAppServerProcess(ctx, serverWorkDir, command, commandArgs, version)
	if err != nil {
		fmt.Fprintf(stderr, "import-codex-appserver: %v\n", err)
		return 1
	}
	defer client.Close()

	result, err := runtime.ImportCodexAppServerSource(client, params, time.Now())
	if err != nil {
		fmt.Fprintf(stderr, "import-codex-appserver: %v\n", err)
		return 1
	}

	fmt.Fprintf(
		stdout,
		"Codex app-server thread imported\n- agent: %s\n- session: %s\n- checkpoints: %d\n- daily reports: %d\n",
		result.AgentID,
		result.SessionID,
		len(result.Checkpoints),
		len(result.Reports),
	)
	return 0
}

func parseCodexAppServerFlags(name string, args []string, stderr io.Writer) (app.ImportCodexAppServerSourceParams, string, string, string, []string, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)

	workDir := flags.String("workdir", "", "workdir that contains vault/ and state/")
	serverWorkDir := flags.String("server-workdir", "", "working directory used to start the Codex app-server process")
	threadID := flags.String("thread", "", "Codex app-server thread id to import")
	sourcePath := flags.String("source", "", "optional source label recorded in Lore")
	agentID := flags.String("agent", "", "override agent id")
	sessionID := flags.String("session", "", "override session id")
	windowSize := flags.Duration("window", 0, "override checkpoint window size, e.g. 30m")
	skipRollup := flags.Bool("skip-rollup", false, "skip daily rollup after import")

	if err := flags.Parse(args); err != nil {
		return app.ImportCodexAppServerSourceParams{}, "", "", "", nil, err
	}
	if strings.TrimSpace(*threadID) == "" {
		return app.ImportCodexAppServerSourceParams{}, "", "", "", nil, fmt.Errorf("%s: --thread is required", name)
	}
	if strings.TrimSpace(*workDir) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return app.ImportCodexAppServerSourceParams{}, "", "", "", nil, err
		}
		*workDir = cwd
	}
	if strings.TrimSpace(*serverWorkDir) == "" {
		*serverWorkDir = *workDir
	}

	remaining := flags.Args()
	if len(remaining) == 0 || strings.TrimSpace(remaining[0]) == "" {
		return app.ImportCodexAppServerSourceParams{}, "", "", "", nil, fmt.Errorf("%s: app-server command is required after --", name)
	}

	return app.ImportCodexAppServerSourceParams{
			ThreadID:   strings.TrimSpace(*threadID),
			SourcePath: strings.TrimSpace(*sourcePath),
			AgentID:    strings.TrimSpace(*agentID),
			SessionID:  strings.TrimSpace(*sessionID),
			Window:     *windowSize,
			SkipRollup: *skipRollup,
		},
		filepath.Clean(*workDir),
		filepath.Clean(*serverWorkDir),
		strings.TrimSpace(remaining[0]),
		remaining[1:],
		nil
}
