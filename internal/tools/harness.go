package tools

import "obsidian-harness/internal/model"

// Harness is the read-only subset of orchestrator.Harness that built-in
// read-only tools depend on. Defining it here as an interface follows the
// "accept interfaces, return structs" pattern: callers pass a real
// *orchestrator.Harness which satisfies the interface implicitly, while
// tests can substitute a fake without dragging in the full orchestrator
// dependency graph.
//
// The signatures match orchestrator.Harness exactly. Adding a new method
// here requires also adding it to orchestrator.Harness; removing one
// requires no change on the orchestrator side.
type Harness interface {
	ManagedStatus() (model.ManagedStatusView, error)
	SystemDocGet(name string) (model.VaultDocument, error)
	VaultRead(relPath string) (model.VaultDocument, error)
	VaultList(relDir string) ([]model.VaultEntry, error)
	VaultSearchText(query string, relDir string, limit int) ([]model.SearchHit, error)
	VaultResolve(query string, relDir string, limit int) (model.VaultResolveResult, error)
	VaultBacklinks(relPath string, limit int) ([]model.SearchHit, error)
	DocClassify(relPath string) model.DocClassificationView
	ContextPack(targetPath string, task string, limit int) (model.ContextPack, error)
}
