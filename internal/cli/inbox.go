package cli

import (
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
	workDir, err := resolveWorkDir(args)
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
	renderOperatorQueue(stdout, queue)
	return 0
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
