package app

import (
	"fmt"

	"obsidian-harness/internal/model"
)

// RecordUsage appends model-call usage records to the runtime store.
//
// It is the boundary that lets callers (console session, future
// process-sink summarizer) persist usage without depending on the
// store package directly. Empty input is a no-op and returns nil.
// Records are appended in order; the first append error short-circuits
// and is returned to the caller so that genuine store failures are not
// silently dropped.
func (r *Runtime) RecordUsage(records []model.UsageRecord) error {
	if r == nil || r.Store == nil {
		return fmt.Errorf("app: runtime is not initialized")
	}
	if len(records) == 0 {
		return nil
	}
	usageStore := r.Store.Usage()
	for _, record := range records {
		if err := usageStore.AppendUsage(record); err != nil {
			return err
		}
	}
	return nil
}
