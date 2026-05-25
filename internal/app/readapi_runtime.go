package app

import (
	"time"

	"obsidian-harness/internal/model"
	"obsidian-harness/internal/tools"
)

func (r *Runtime) SystemDocGet(name string) (model.VaultDocument, error) {
	return r.Harness.SystemDocGet(name)
}

func (r *Runtime) VaultRead(relPath string) (model.VaultDocument, error) {
	return r.Harness.VaultRead(relPath)
}

func (r *Runtime) VaultList(relDir string) ([]model.VaultEntry, error) {
	return r.Harness.VaultList(relDir)
}

func (r *Runtime) VaultSearchText(query string, relDir string, limit int) ([]model.SearchHit, error) {
	return r.Harness.VaultSearchText(query, relDir, limit)
}

func (r *Runtime) VaultResolve(query string, relDir string, limit int) (model.VaultResolveResult, error) {
	return r.Harness.VaultResolve(query, relDir, limit)
}
func (r *Runtime) VaultBacklinks(relPath string, limit int) ([]model.SearchHit, error) {
	return r.Harness.VaultBacklinks(relPath, limit)
}

func (r *Runtime) DocClassify(relPath string) model.DocClassificationView {
	return r.Harness.DocClassify(relPath)
}

func (r *Runtime) ContextPack(targetPath string, task string, limit int) (model.ContextPack, error) {
	return r.Harness.ContextPack(targetPath, task, limit)
}

func (r *Runtime) WorkDirPath() string {
	return r.Config.Paths.WorkDir
}

func (r *Runtime) VaultRootPath() string {
	return r.Config.Paths.VaultRoot
}

func (r *Runtime) StateDirPath() string {
	return r.Config.Paths.StateDir
}

func (r *Runtime) WriteLowRiskNote(relPath string, content string, overwrite bool) (model.VaultDocument, error) {
	return r.Harness.WriteLowRiskNote(relPath, []byte(content), overwrite, time.Now())
}

// ProposalTools returns persona_update_propose + markdown_note_propose
// bound to this runtime's harness. Reuses the same tools.RegisterProposal
// call MCP uses so console and MCP surfaces share field validation and
// schema; SurfaceConsole filter lets a future tool opt out of console
// exposure without changes here.
func (r *Runtime) ProposalTools() []tools.Tool {
	registry := tools.NewRegistry()
	if err := tools.RegisterProposal(registry, r.Harness); err != nil {
		panic(err)
	}
	return registry.ListBySurface(tools.SurfaceConsole)
}
