package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"time"

	"obsidian-harness/internal/app"
)

// runInboxCommand renders the unified operator queue -- the one place an
// operator can see every pending decision (drafts to review, findings to
// triage, persona candidates to review) plus a usage glance, instead of
// running `lore draft`, `lore findings`, and `lore persona` separately.
func runInboxCommand(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("inbox", flag.ContinueOnError)
	flags.SetOutput(stderr)
	asJSON := flags.Bool("json", false, "emit the operator queue as a single-line JSON object")
	if err := flags.Parse(args); err != nil {
		return 1
	}
	workDir, err := resolveWorkDir(flags.Args())
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	runtime, err := app.OpenRuntime(workDir)
	if err != nil {
		fmt.Fprintf(stderr, "open runtime: %v\n", err)
		return 1
	}
	defer closeRuntime(stderr, runtime, "inbox")

	queue, err := runtime.OperatorQueue(time.Now())
	if err != nil {
		fmt.Fprintf(stderr, "inbox: %v\n", err)
		return 1
	}
	if *asJSON {
		if err := emitOperatorQueueJSON(stdout, queue); err != nil {
			fmt.Fprintf(stderr, "inbox: emit json: %v\n", err)
			return 1
		}
		return 0
	}
	renderOperatorQueue(stdout, queue)
	return 0
}

// operatorQueueNudge is a one-line, non-blocking pointer to `lore inbox`
// for surfaces (like `lore status`) that want to flag pending work without
// the full list. Empty when nothing awaits a decision.
func operatorQueueNudge(actionItems int) string {
	if actionItems <= 0 {
		return ""
	}
	return fmt.Sprintf("\nOperator queue: %d item(s) awaiting a decision -- run `lore inbox`.\n", actionItems)
}

type operatorQueueJSON struct {
	Day                   string                       `json:"day"`
	ActionItems           int                          `json:"action_items"`
	PendingDrafts         []operatorQueueDraftJSON     `json:"pending_drafts"`
	OpenFindings          []operatorQueueFindingJSON   `json:"open_findings"`
	OpenPersonaCandidates []operatorQueueCandidateJSON `json:"open_persona_candidates"`
	TodayUsage            operatorQueueUsageJSON       `json:"today_usage"`
}

type operatorQueueDraftJSON struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
}

type operatorQueueFindingJSON struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
}

type operatorQueueCandidateJSON struct {
	ID            string `json:"id"`
	Field         string `json:"field"`
	ProposedValue string `json:"proposed_value"`
}

type operatorQueueUsageJSON struct {
	Calls       int `json:"calls"`
	TotalTokens int `json:"total_tokens"`
}

// emitOperatorQueueJSON writes the queue as a single-line JSON object so a
// scripting / TUI consumer can drive a dashboard without re-querying each
// source. Arrays are always present (possibly empty), never null.
func emitOperatorQueueJSON(stdout io.Writer, queue app.OperatorQueue) error {
	out := operatorQueueJSON{
		Day:                   queue.Day.Format("2006-01-02"),
		ActionItems:           queue.ActionItemCount(),
		PendingDrafts:         make([]operatorQueueDraftJSON, 0, len(queue.PendingDrafts)),
		OpenFindings:          make([]operatorQueueFindingJSON, 0, len(queue.OpenFindings)),
		OpenPersonaCandidates: make([]operatorQueueCandidateJSON, 0, len(queue.OpenPersonaCandidates)),
		TodayUsage: operatorQueueUsageJSON{
			Calls:       queue.TodayUsage.Calls,
			TotalTokens: queue.TodayUsage.TotalTokens,
		},
	}
	for _, d := range queue.PendingDrafts {
		out.PendingDrafts = append(out.PendingDrafts, operatorQueueDraftJSON{ID: d.ID, Kind: string(d.Kind), Title: d.Title})
	}
	for _, f := range queue.OpenFindings {
		out.OpenFindings = append(out.OpenFindings, operatorQueueFindingJSON{ID: f.ID, Severity: string(f.Severity), Title: f.Title})
	}
	for _, c := range queue.OpenPersonaCandidates {
		out.OpenPersonaCandidates = append(out.OpenPersonaCandidates, operatorQueueCandidateJSON{ID: c.ID, Field: c.Candidate.Field, ProposedValue: c.Candidate.ProposedValue})
	}

	encoded, err := json.Marshal(out)
	if err != nil {
		return err
	}
	if _, err := stdout.Write(encoded); err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout)
	return err
}

func renderOperatorQueue(stdout io.Writer, queue app.OperatorQueue) {
	fmt.Fprintln(stdout, "Operator Queue")
	fmt.Fprintln(stdout, "==============")
	fmt.Fprintf(stdout, "Day:          %s\n", queue.Day.Format("2006-01-02"))
	fmt.Fprintf(stdout, "Action items: %d\n", queue.ActionItemCount())

	fmt.Fprintf(stdout, "\nDrafts pending review (%d):\n", len(queue.PendingDrafts))
	if len(queue.PendingDrafts) == 0 {
		fmt.Fprintln(stdout, "  (none)")
	}
	for _, d := range queue.PendingDrafts {
		fmt.Fprintf(stdout, "  %-16s %-22s %s\n", d.ID, d.Kind, d.Title)
	}

	fmt.Fprintf(stdout, "\nOpen findings (%d):\n", len(queue.OpenFindings))
	if len(queue.OpenFindings) == 0 {
		fmt.Fprintln(stdout, "  (none)")
	}
	for _, f := range queue.OpenFindings {
		fmt.Fprintf(stdout, "  %-16s %-8s %s\n", f.ID, f.Severity, f.Title)
	}

	fmt.Fprintf(stdout, "\nOpen persona candidates (%d):\n", len(queue.OpenPersonaCandidates))
	if len(queue.OpenPersonaCandidates) == 0 {
		fmt.Fprintln(stdout, "  (none)")
	}
	for _, c := range queue.OpenPersonaCandidates {
		fmt.Fprintf(stdout, "  %-16s %s = %s\n", c.ID, c.Candidate.Field, c.Candidate.ProposedValue)
	}

	fmt.Fprintf(stdout, "\nToday's usage: %d calls / %d tokens\n", queue.TodayUsage.Calls, queue.TodayUsage.TotalTokens)
}
