package app

import (
	"time"

	"obsidian-harness/internal/model"
	"obsidian-harness/internal/store"
)

type ProcessSinkDayView struct {
	AgentID     string
	Day         time.Time
	Checkpoints []model.CheckpointDoc
	Report      *model.DailyReport
}

func (r *Runtime) ProcessSinkDay(agentID string, day time.Time) (ProcessSinkDayView, error) {
	checkpoints, err := r.Store.ProcessSink().ListCheckpointsByDay(agentID, day)
	if err != nil {
		return ProcessSinkDayView{}, err
	}

	view := ProcessSinkDayView{
		AgentID:     agentID,
		Day:         model.NormalizeDay(day),
		Checkpoints: checkpoints,
	}

	report, err := r.Store.ProcessSink().GetDailyReport(agentID, day)
	if err == nil {
		view.Report = &report
	} else if err != store.ErrNotFound {
		return ProcessSinkDayView{}, err
	}

	return view, nil
}
