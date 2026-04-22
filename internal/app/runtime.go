package app

import (
	"path/filepath"
	"time"

	"obsidian-harness/internal/config"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/orchestrator"
	"obsidian-harness/internal/store"
	"obsidian-harness/internal/store/jsonstore"
	"obsidian-harness/internal/vault"
)

type Runtime struct {
	Config  config.Config
	Harness *orchestrator.Harness
	Store   store.StateStore
}

type DemoP0BResult struct {
	Checkpoint model.CheckpointDoc
	Report     model.DailyReport
}

func OpenRuntime(workDir string) (*Runtime, error) {
	cfg := config.Default(workDir)
	statePath := filepath.Join(cfg.Paths.StateDir, "store.json")
	st, err := jsonstore.New(statePath, cfg.Vault.TempSuffix)
	if err != nil {
		return nil, err
	}
	h, err := orchestrator.New(cfg, st)
	if err != nil {
		return nil, err
	}
	return &Runtime{
		Config:  cfg,
		Harness: h,
		Store:   st,
	}, nil
}

func (r *Runtime) Bootstrap(now time.Time) ([]model.DocumentRef, error) {
	return r.Harness.BootstrapManagedVault(now)
}

func (r *Runtime) DemoP0A(now time.Time) (model.Draft, error) {
	r.Harness.UpdateDependencies(true, true)
	if _, err := r.Bootstrap(now); err != nil {
		return model.Draft{}, err
	}

	relPath := filepath.Join("0-\u6392\u671f", "04-\u6267\u884c", "demo-week.md")
	absPath := filepath.Join(r.Config.Paths.VaultRoot, relPath)
	if _, err := vault.WriteFileAtomic(absPath, []byte("# Demo Week\n\n- [x] learn SQL triggers"), r.Config.Vault.TempSuffix); err != nil {
		return model.Draft{}, err
	}

	draft, err := r.Harness.ObserveDocumentChange(relPath, []byte("learn SQL triggers"), now)
	if err != nil {
		return model.Draft{}, err
	}
	if _, err := r.Harness.ApproveDraft(draft.ID, now.Add(time.Minute)); err != nil {
		return model.Draft{}, err
	}
	return r.Harness.ApplyDraft(draft.ID, now.Add(2*time.Minute))
}

func (r *Runtime) DemoP0B(now time.Time) (DemoP0BResult, error) {
	r.Harness.UpdateDependencies(true, true)

	window := model.SessionWindow{
		AgentID:     "codex",
		SessionID:   "demo-thread",
		WindowStart: now,
		WindowEnd:   now.Add(30 * time.Minute),
	}
	checkpoint, err := r.Harness.IngestSessionWindow(
		window,
		"demo checkpoint",
		"Summarized external agent work for this window.",
		"raw transcript placeholder",
		window.WindowEnd,
	)
	if err != nil {
		return DemoP0BResult{}, err
	}
	report, err := r.Harness.RollupDaily(window.AgentID, now, now.Add(12*time.Hour))
	if err != nil {
		return DemoP0BResult{}, err
	}
	return DemoP0BResult{
		Checkpoint: checkpoint,
		Report:     report,
	}, nil
}
