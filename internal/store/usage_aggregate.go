package store

import (
	"strings"

	"obsidian-harness/internal/model"
)

// usageModelUnknown is the bucket key for a usage record that carries
// no provider/model identity (legacy rows, or a biller that did not
// stamp the fields).
const usageModelUnknown = "unknown"

// UsagePurposeBucket maps a raw UsageRecord.Purpose onto its breakdown
// bucket key. An empty/absent Purpose folds into "chat": before the
// Purpose field existed the operator agent was the only biller, so an
// unlabeled call is a chat call. This keeps a single canonical bucket
// for the common case rather than a separate empty-string bucket.
func UsagePurposeBucket(purpose string) string {
	if strings.TrimSpace(purpose) == "" {
		return model.UsagePurposeChat
	}
	return purpose
}

// UsageModelBucket builds the "provider/model" key for a record's
// ByModel sub-bucket. Both parts empty -> "unknown"; a single empty
// part is substituted with "unknown" so the slash form stays parseable
// (e.g. "deepseek/unknown"). Whitespace is trimmed so stray padding in
// stored payloads does not fragment buckets.
func UsageModelBucket(provider, modelName string) string {
	provider = strings.TrimSpace(provider)
	modelName = strings.TrimSpace(modelName)
	if provider == "" && modelName == "" {
		return usageModelUnknown
	}
	if provider == "" {
		provider = usageModelUnknown
	}
	if modelName == "" {
		modelName = usageModelUnknown
	}
	return provider + "/" + modelName
}

// AccumulateUsageBreakdown folds one usage record's tokens into the
// breakdown map, updating both the purpose-level stats and the
// per-model sub-bucket underneath it. All three store backends call
// this so the purpose/model bucketing + fallback rules live in exactly
// one place. The caller owns the top-line UsageSummary totals; this
// helper only maintains the PurposeBreakdown map.
func AccumulateUsageBreakdown(breakdown map[string]model.UsagePurposeStats, purpose, provider, modelName string, prompt, completion int) {
	purposeKey := UsagePurposeBucket(purpose)
	stats := breakdown[purposeKey]
	stats.Calls++
	stats.PromptTokens += prompt
	stats.CompletionTokens += completion

	if stats.ByModel == nil {
		stats.ByModel = map[string]model.UsagePurposeStats{}
	}
	modelKey := UsageModelBucket(provider, modelName)
	leaf := stats.ByModel[modelKey]
	leaf.Calls++
	leaf.PromptTokens += prompt
	leaf.CompletionTokens += completion
	stats.ByModel[modelKey] = leaf

	breakdown[purposeKey] = stats
}
