package model

type CoreContext struct {
	PersonaSummary     string   `json:"persona_summary,omitempty"`
	WeaknessSummary    string   `json:"weakness_summary,omitempty"`
	SystemRulesSummary string   `json:"system_rules_summary,omitempty"`
	ProgressSummary    string   `json:"progress_summary,omitempty"`
	PendingDrafts      []string `json:"pending_drafts,omitempty"`
	Notes              []string `json:"notes,omitempty"`
}
