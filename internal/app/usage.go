package app

import (
	"fmt"
	"time"

	"obsidian-harness/internal/model"
)

// RecordUsage appends model-call usage records to the runtime store.
//
// It is the boundary that lets callers (console session, persona
// extractor, process-sink summarizer) persist usage without depending on
// the store package directly. It is also the single chokepoint that
// enforces usage.track_usage: when an operator disables tracking this is
// a no-op. Empty input is a no-op and returns nil. Records are appended
// in order; the first append error short-circuits and is returned to the
// caller so that genuine store failures are not silently dropped.
func (r *Runtime) RecordUsage(records []model.UsageRecord) error {
	if r == nil || r.Store == nil {
		return fmt.Errorf("app: runtime is not initialized")
	}
	// usage.track_usage gate. The flag was previously parsed and validated
	// but never enforced, so an operator who opted out still got usage
	// rows. Honour it here so all three billers (chat / persona_extract /
	// process_sink) respect it through this one path.
	if !r.Config.Usage.TrackUsage {
		return nil
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

// SummarizeUsage returns the per-day rollup of recorded usage for the
// given local day. It is a thin pass-through onto the underlying store
// so that CLI commands and other app-layer consumers do not reach into
// store internals directly.
func (r *Runtime) SummarizeUsage(day time.Time) (model.UsageSummary, error) {
	if r == nil || r.Store == nil {
		return model.UsageSummary{}, fmt.Errorf("app: runtime is not initialized")
	}
	return r.Store.Usage().SummarizeUsage(day)
}
