package app

import "obsidian-harness/internal/model"

func (r *Runtime) ManagedStatus() (model.ManagedStatusView, error) {
	r.Harness.UpdateDependencies(r.ProcessSinkSummarizer != nil && r.processSinkSummarizerErr == nil, false)
	return r.Harness.ManagedStatus()
}
