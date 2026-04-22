package runtime

import (
	"strings"
	"sync"
	"time"

	"obsidian-harness/internal/model"
)

type DependencyProbe struct {
	ModelAvailable   bool
	AdapterConnected bool
}

type HealthService struct {
	mu       sync.RWMutex
	snapshot model.HealthSnapshot
}

func NewHealthService() *HealthService {
	return &HealthService{
		snapshot: model.HealthSnapshot{
			Outcome:          model.NewOutcome(model.StatusBlocked, model.ReasonModelUnavailable, model.ReasonAdapterDown),
			ModelAvailable:   false,
			AdapterConnected: false,
			Message:          "initializing",
			CheckedAt:        time.Now(),
		},
	}
}

func (s *HealthService) Update(probe DependencyProbe) model.HealthSnapshot {
	reasons := make([]model.ReasonCode, 0, 2)
	statuses := make([]model.Status, 0, 2)
	parts := make([]string, 0, 2)

	if probe.ModelAvailable {
		statuses = append(statuses, model.StatusOK)
	} else {
		statuses = append(statuses, model.StatusBlocked)
		reasons = append(reasons, model.ReasonModelUnavailable)
		parts = append(parts, "model unavailable")
	}

	if probe.AdapterConnected {
		statuses = append(statuses, model.StatusOK)
	} else {
		statuses = append(statuses, model.StatusAdapterDisconnected)
		reasons = append(reasons, model.ReasonAdapterDown)
		parts = append(parts, "adapter disconnected")
	}

	status := model.MergeStatus(statuses...)
	message := "healthy"
	if len(parts) > 0 {
		message = strings.Join(parts, "; ")
	}

	snapshot := model.HealthSnapshot{
		Outcome:          model.NewOutcome(status, reasons...),
		ModelAvailable:   probe.ModelAvailable,
		AdapterConnected: probe.AdapterConnected,
		Message:          message,
		CheckedAt:        time.Now(),
	}

	s.mu.Lock()
	s.snapshot = snapshot
	s.mu.Unlock()

	return snapshot
}

func (s *HealthService) Snapshot() model.HealthSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshot
}
