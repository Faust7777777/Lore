package app

import "obsidian-harness/internal/model"

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
