package app

import "obsidian-harness/internal/model"

func (r *Runtime) BuildCoreContext(limit int) (model.CoreContext, error) {
	return r.Harness.BuildCoreContext(limit)
}
