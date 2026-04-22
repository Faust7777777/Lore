package operatoragent

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"obsidian-harness/internal/model"
)

var (
	draftIDPattern = regexp.MustCompile(`draft-[a-zA-Z0-9_-]+`)
	datePattern    = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)
)

type FallbackAgent struct{}

func NewFallback() Agent {
	return FallbackAgent{}
}

func (FallbackAgent) Decide(input string, ctx Context) (Decision, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return Decision{}, fmt.Errorf("empty input")
	}

	lower := strings.ToLower(raw)
	if asksForBackgroundRuntime(lower) {
		return Decision{}, fmt.Errorf("timed and background runtime tasks stay outside the operator console")
	}

	decision := Decision{
		AgentID: withFallback(parseAgentID(lower), withFallback(strings.TrimSpace(ctx.DefaultAgentID), "codex")),
		Day:     resolveDay(lower, ctx.Now),
		DraftID: parseDraftID(raw),
	}

	switch {
	case hasAny(lower,
		"help",
		"what can you do",
		"\u5e2e\u52a9",
		"\u600e\u4e48\u7528",
		"\u547d\u4ee4",
		"ç”¯î†¼å§ª"):
		decision.Action = ActionHelp
		return decision, nil
	case hasAny(lower,
		"status",
		"current status",
		"managed status",
		"\u72b6\u6001",
		"\u5f53\u524d\u72b6\u6001",
		"\u7cfb\u7edf\u72b6\u6001",
		"è¤°æ’³å¢ é˜èˆµâ‚¬?",
		"ç»¯è¤ç²ºé˜èˆµâ‚¬?",
		"éªå¬¬æ¹…é˜èˆµâ‚¬?",
		"éªå¬¬æ¹…è¤°æ’³å¢ é˜èˆµâ‚¬?"):
		decision.Action = ActionShowStatus
		return decision, nil
	case hasAny(lower,
		"review draft",
		"open draft",
		"\u5ba1\u9605\u8349\u6848",
		"\u67e5\u770b\u8349\u6848\u8be6\u60c5",
		"éŒãƒ§æ¹…é‘½å¤‹î”",
		"éŽµæ’³ç´‘é‘½å¤‹î”",
		"ç€¹ï¿ æ§„é‘½å¤‹î”"):
		decision.Action = ActionReviewDraft
		decision.UseFocusedDraft = hasAny(lower, "current draft", "\u5f53\u524d\u8349\u6848")
		return decision, nil
	case hasAny(lower, "é‘½å¤‹î”"):
		decision.Action = ActionReviewDraft
		decision.UseFocusedDraft = hasAny(lower, "current draft", "\u5f53\u524d\u8349\u6848")
		return decision, nil
	case hasAny(lower,
		"list drafts",
		"show drafts",
		"pending drafts",
		"\u5217\u51fa\u8349\u6848",
		"\u67e5\u770b\u8349\u6848",
		"\u5f85\u5ba1",
		"\u5f85\u5ba1\u8349\u6848",
		"å¯°å‘­î…¸é‘½å¤‹î”"):
		decision.Action = ActionListDrafts
		decision.PendingOnly = hasAny(lower, "pending", "\u5f85\u5ba1", "\u5f85\u5ba1\u9605")
		return decision, nil
	case hasAny(lower,
		"approve",
		"\u6279\u51c6",
		"\u901a\u8fc7",
		"éŽµç‘°å™¯",
		"é–«æ°³ç¹ƒé‘½å¤‹î”"):
		decision.Action = ActionApproveDraft
		decision.UseFocusedDraft = hasAny(lower, "current draft", "\u5f53\u524d\u8349\u6848")
		return decision, nil
	case hasAny(lower,
		"reject",
		"\u62d2\u7edd",
		"éŽ·æŽ”ç²·"):
		decision.Action = ActionRejectDraft
		decision.UseFocusedDraft = hasAny(lower, "current draft", "\u5f53\u524d\u8349\u6848")
		return decision, nil
	case hasAny(lower,
		"request revision",
		"revision",
		"\u4fee\u8ba2",
		"\u91cd\u65b0\u751f\u6210"):
		decision.Action = ActionRequestDraftRevision
		decision.UseFocusedDraft = hasAny(lower, "current draft", "\u5f53\u524d\u8349\u6848")
		return decision, nil
	case hasAny(lower,
		"apply",
		"\u5e94\u7528",
		"\u5199\u56de",
		"\u843d\u76d8",
		"æ´æ—‚æ•¤"):
		decision.Action = ActionApplyDraft
		decision.UseFocusedDraft = hasAny(lower, "current draft", "\u5f53\u524d\u8349\u6848")
		return decision, nil
	case hasAny(lower,
		"process sink",
		"checkpoint",
		"daily report",
		"\u65e5\u62a5",
		"\u65f6\u62a5"):
		decision.Action = ActionShowProcessSinkDay
		return decision, nil
	default:
		return Decision{}, fmt.Errorf("unrecognized input; ask for status, drafts, review/apply, or process sink day")
	}
}

func parseDraftID(value string) string {
	return strings.TrimSpace(draftIDPattern.FindString(strings.TrimSpace(value)))
}

func parseAgentID(value string) string {
	for _, agentID := range []string{"codex", "claude", "openclaw", "hermes"} {
		if strings.Contains(value, agentID) {
			return agentID
		}
	}
	return ""
}

func resolveDay(value string, now time.Time) time.Time {
	if now.IsZero() {
		now = time.Now()
	}
	if match := datePattern.FindString(value); match != "" {
		if day, err := time.ParseInLocation("2006-01-02", match, time.Local); err == nil {
			return model.NormalizeDay(day)
		}
	}
	if hasAny(value, "yesterday", "\u6628\u5929", "\u6628\u65e5") {
		return model.NormalizeDay(now.AddDate(0, 0, -1))
	}
	return model.NormalizeDay(now)
}

func asksForBackgroundRuntime(value string) bool {
	return hasAny(value,
		"attach-codex-jsonl",
		"sync-codex-jsonl",
		"import-codex-jsonl",
		"scheduler",
		"schedule",
		"poll",
		"\u5b9a\u65f6",
		"\u5de1\u68c0",
		"\u6bcf30\u5206\u949f",
		"\u534a\u5c0f\u65f6")
}

func hasAny(value string, patterns ...string) bool {
	for _, pattern := range patterns {
		if strings.Contains(value, pattern) {
			return true
		}
	}
	return false
}

func withFallback(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
