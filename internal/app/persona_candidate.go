package app

import (
	"fmt"

	"obsidian-harness/internal/persona"
)

// RecordPersonaCandidate persists one persona candidate via the
// runtime's store. Returns the stored record, an isNew flag (false
// when the DedupKey already mapped to an existing row, see
// store.PersonaCandidateStore.UpsertCandidate), and any store-side
// error. The console fire-and-forget extraction goroutine (P4) calls
// this -- callers expecting to bill the work should NOT depend on
// the bool, only the error.
func (r *Runtime) RecordPersonaCandidate(record persona.PersonaCandidateRecord) (persona.PersonaCandidateRecord, bool, error) {
	if r == nil || r.Store == nil {
		return persona.PersonaCandidateRecord{}, false, fmt.Errorf("app: runtime is not initialized")
	}
	return r.Store.PersonaCandidates().UpsertCandidate(record)
}
