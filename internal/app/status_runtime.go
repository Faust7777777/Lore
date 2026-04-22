package app

import "obsidian-harness/internal/model"

func (r *Runtime) ManagedStatus() (model.ManagedStatusView, error) {
	return r.Harness.ManagedStatus()
}
