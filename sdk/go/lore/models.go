package lore

import "time"

type Status string

type ReasonCode string

type DocClass string

type Outcome struct {
	Status      Status       `json:"status"`
	ReasonCodes []ReasonCode `json:"reason_codes,omitempty"`
}

type HealthSnapshot struct {
	Outcome          Outcome   `json:"outcome"`
	ModelAvailable   bool      `json:"model_available"`
	AdapterConnected bool      `json:"adapter_connected"`
	Message          string    `json:"message,omitempty"`
	CheckedAt        time.Time `json:"checked_at"`
}

type ManagedCoreStatus struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
}

type ManagedStatus struct {
	Ready     bool                `json:"ready"`
	WorkDir   string              `json:"work_dir"`
	VaultRoot string              `json:"vault_root"`
	CoreDocs  []ManagedCoreStatus `json:"core_docs"`
	Health    HealthSnapshot      `json:"health"`
}

type AttachmentRef struct {
	Raw   string `json:"raw,omitempty"`
	Path  string `json:"path"`
	Kind  string `json:"kind"`
	Embed bool   `json:"embed,omitempty"`
}

type VaultDocument struct {
	Path        string          `json:"path"`
	DocClass    DocClass        `json:"doc_class"`
	BaseVersion string          `json:"base_version,omitempty"`
	Content     string          `json:"content"`
	Attachments []AttachmentRef `json:"attachments,omitempty"`
}

type VaultEntry struct {
	Path     string   `json:"path"`
	Name     string   `json:"name"`
	Kind     string   `json:"kind"`
	DocClass DocClass `json:"doc_class,omitempty"`
}

type SearchHit struct {
	Path     string   `json:"path"`
	Line     int      `json:"line"`
	Preview  string   `json:"preview"`
	DocClass DocClass `json:"doc_class,omitempty"`
}

type VaultResolveResult struct {
	Query        string              `json:"query"`
	Status       string              `json:"status"`
	SelectedPath string              `json:"selected_path,omitempty"`
	Matches      []VaultResolveMatch `json:"matches,omitempty"`
	Reason       string              `json:"reason,omitempty"`
}

type VaultResolveMatch struct {
	Path     string   `json:"path"`
	Score    float64  `json:"score"`
	Reason   string   `json:"reason"`
	DocClass DocClass `json:"doc_class,omitempty"`
}

type DocClassification struct {
	Path     string   `json:"path"`
	DocClass DocClass `json:"doc_class"`
	Matches  []string `json:"matches,omitempty"`
}

type ContextPack struct {
	Task        string          `json:"task,omitempty"`
	TargetPath  string          `json:"target_path,omitempty"`
	Managed     ManagedStatus   `json:"managed_status"`
	SystemDoc   *VaultDocument  `json:"system_doc,omitempty"`
	ProgressDoc *VaultDocument  `json:"progress_doc,omitempty"`
	CurrentWeek *VaultDocument  `json:"current_week,omitempty"`
	TargetDoc   *VaultDocument  `json:"target_doc,omitempty"`
	RelatedHits []SearchHit     `json:"related_hits,omitempty"`
	Backlinks   []SearchHit     `json:"backlinks,omitempty"`
	Attachments []AttachmentRef `json:"attachments,omitempty"`
	Notes       []string        `json:"notes,omitempty"`
}

type SystemDocGetRequest struct {
	Name string
}

type VaultReadRequest struct {
	Path string
}

type VaultListRequest struct {
	Dir string
}

type VaultSearchTextRequest struct {
	Query string
	Dir   string
	Limit int
}

type VaultResolveRequest struct {
	Query string
	Dir   string
	Limit int
}

type VaultBacklinksRequest struct {
	Path  string
	Limit int
}

type ContextPackRequest struct {
	TargetPath string
	Task       string
	Limit      int
}

type DocClassifyRequest struct {
	Path string
}
