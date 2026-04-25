package runtime

import (
	"context"

	"obsidian-harness/internal/model"
	"obsidian-harness/internal/store"
)

type Auditor struct {
	store  store.AuditStore
	broker Broker
}

func NewAuditor(store store.AuditStore, broker Broker) *Auditor {
	return &Auditor{store: store, broker: broker}
}

func (a *Auditor) Record(ctx context.Context, record model.AuditRecord) error {
	record = model.NormalizeAuditRecord(record)
	if err := a.store.AppendAudit(record); err != nil {
		return err
	}
	if a.broker == nil {
		return nil
	}
	return a.broker.Publish(ctx, Event{
		ID:         record.ID,
		Type:       EventAuditRecorded,
		Source:     "auditor",
		OccurredAt: record.OccurredAt,
		Payload:    record,
	})
}
