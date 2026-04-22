package app

import (
	"time"

	"obsidian-harness/internal/model"
)

type DraftReview struct {
	Draft              model.Draft
	TargetDocument     *model.VaultDocument
	CurrentBaseVersion string
	BaseVersionMatches bool
}

func (r *Runtime) ListDrafts() ([]model.Draft, error) {
	return r.Harness.ListDrafts()
}

func (r *Runtime) ReviewDraft(id string) (DraftReview, error) {
	draft, err := r.Harness.GetDraft(id)
	if err != nil {
		return DraftReview{}, err
	}

	review := DraftReview{
		Draft: draft,
	}
	target, err := r.Harness.VaultRead(draft.Target.Path)
	if err == nil {
		review.TargetDocument = &target
		review.CurrentBaseVersion = target.BaseVersion
		review.BaseVersionMatches = draft.Target.BaseVersion != "" && draft.Target.BaseVersion == target.BaseVersion
	}
	return review, nil
}

func (r *Runtime) ApproveDraft(id string) (model.Draft, error) {
	return r.Harness.ApproveDraft(id, time.Now())
}

func (r *Runtime) RejectDraft(id string) (model.Draft, error) {
	return r.Harness.RejectDraft(id, time.Now())
}

func (r *Runtime) RequestDraftRevision(id string) (model.Draft, error) {
	return r.Harness.RequestDraftRevision(id, time.Now())
}

func (r *Runtime) ApplyDraft(id string) (model.Draft, error) {
	return r.Harness.ApplyDraft(id, time.Now())
}
