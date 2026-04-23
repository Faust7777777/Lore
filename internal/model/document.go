package model

type GovernanceMode string

const (
	GovernanceDefault     GovernanceMode = "default"
	GovernanceLowRisk     GovernanceMode = "low_risk"
	GovernanceProcessSink GovernanceMode = "process_sink"
)

type DocClass string

const (
	DocClassUnknown       DocClass = "unknown"
	DocClassSystemDoc     DocClass = "system_doc"
	DocClassProgressIndex DocClass = "progress_index"
	DocClassPersona       DocClass = "persona"
	DocClassAgentDoc      DocClass = "agent_doc"
	DocClassIdentityDoc   DocClass = "identity_doc"
	DocClassPlanMaster    DocClass = "plan_master"
	DocClassPlanWeek      DocClass = "plan_week"
	DocClassCheckpoint    DocClass = "checkpoint"
	DocClassDailyReport   DocClass = "daily_report"
	DocClassArchive       DocClass = "archive"
	DocClassNote          DocClass = "note"
)

type DocumentRef struct {
	Path        string   `json:"path"`
	Class       DocClass `json:"class"`
	BaseVersion string   `json:"base_version,omitempty"`
}

type ManagedCorePaths struct {
	SystemDoc     string `json:"system_doc"`
	ProgressIndex string `json:"progress_index"`
	Persona       string `json:"persona"`
	AgentDoc      string `json:"agent_doc"`
	IdentityDoc   string `json:"identity_doc"`
}

type WritePolicy struct {
	Class         DocClass        `json:"class"`
	Governance    GovernanceMode  `json:"governance"`
	DraftRequired bool            `json:"draft_required"`
	Allowed       []Profile       `json:"allowed_profiles,omitempty"`
	ToolAttrs     *ToolAttributes `json:"tool_attributes,omitempty"`
}
