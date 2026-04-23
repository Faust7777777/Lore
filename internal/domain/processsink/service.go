package processsink

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"obsidian-harness/internal/model"
	"obsidian-harness/internal/store"
)

type Writer interface {
	Write(path string, content []byte) error
}

type Service struct {
	store   store.ProcessSinkStore
	writer  Writer
	rootDir string
}

func NewService(store store.ProcessSinkStore, writer Writer, rootDir string) *Service {
	return &Service{
		store:   store,
		writer:  writer,
		rootDir: rootDir,
	}
}

func (s *Service) WriteCheckpoint(window model.SessionWindow, title string, content string, rawTranscript string, at time.Time) (model.CheckpointDoc, error) {
	windowKey := window.Key()
	doc := model.CheckpointDoc{
		WindowKey:     windowKey,
		Path:          s.checkpointPath(window),
		Window:        window,
		State:         model.CheckpointMaterialized,
		Title:         withFallback(title, s.defaultCheckpointTitle(window)),
		Content:       strings.TrimSpace(content),
		RawTranscript: rawTranscript,
		CreatedAt:     at,
		UpdatedAt:     at,
	}

	if doc.Content == "" {
		doc.State = model.CheckpointPlaceholder
		doc.Content = s.placeholderContent(window)
	}

	if existing, err := s.store.GetCheckpointByWindowKey(windowKey); err == nil {
		doc.CreatedAt = existing.CreatedAt
	}

	if err := s.store.SaveCheckpoint(doc); err != nil {
		return model.CheckpointDoc{}, err
	}
	if s.writer != nil {
		if err := s.writer.Write(doc.Path, []byte(renderCheckpointMarkdown(doc))); err != nil {
			return model.CheckpointDoc{}, err
		}
	}
	return doc, nil
}

func (s *Service) RollupDay(agentID string, day time.Time, at time.Time) (model.DailyReport, error) {
	checkpoints, err := s.store.ListCheckpointsByDay(agentID, day)
	if err != nil {
		return model.DailyReport{}, err
	}
	return s.rollupDayWithCheckpoints(agentID, day, checkpoints, "", "", at)
}

func (s *Service) RollupDayWithSummary(agentID string, day time.Time, title string, content string, at time.Time) (model.DailyReport, error) {
	checkpoints, err := s.store.ListCheckpointsByDay(agentID, day)
	if err != nil {
		return model.DailyReport{}, err
	}
	return s.rollupDayWithCheckpoints(agentID, day, checkpoints, title, content, at)
}

func (s *Service) rollupDayWithCheckpoints(agentID string, day time.Time, checkpoints []model.CheckpointDoc, title string, content string, at time.Time) (model.DailyReport, error) {
	windowKeys := make([]string, 0, len(checkpoints))
	lines := make([]string, 0, len(checkpoints))
	for _, checkpoint := range checkpoints {
		windowKeys = append(windowKeys, checkpoint.WindowKey)
		lines = append(lines, fmt.Sprintf("- %s -> %s", checkpoint.Window.WindowStart.Format("15:04"), checkpoint.Title))
	}
	if len(lines) == 0 {
		lines = append(lines, "- no checkpoints recorded")
	}

	report := model.DailyReport{
		AgentID:    agentID,
		ReportDay:  model.NormalizeDay(day),
		Path:       s.dailyReportPath(agentID, day),
		WindowKeys: windowKeys,
		Title:      withFallback(title, fmt.Sprintf("%s daily report", agentID)),
		Content:    withFallback(content, strings.Join(lines, "\n")),
		CreatedAt:  at,
		UpdatedAt:  at,
	}

	if existing, err := s.store.GetDailyReport(agentID, day); err == nil {
		report.CreatedAt = existing.CreatedAt
	}

	if err := s.store.SaveDailyReport(report); err != nil {
		return model.DailyReport{}, err
	}
	if s.writer != nil {
		if err := s.writer.Write(report.Path, []byte(renderDailyReportMarkdown(report))); err != nil {
			return model.DailyReport{}, err
		}
	}
	return report, nil
}

func (s *Service) checkpointPath(window model.SessionWindow) string {
	day := model.NormalizeDay(window.WindowStart).Format("2006-01-02")
	name := fmt.Sprintf("%s-%s.md", window.WindowStart.Format("1504"), window.WindowEnd.Format("1504"))
	return filepath.Join(s.rootDir, window.AgentID, "checkpoints", day, name)
}

func (s *Service) dailyReportPath(agentID string, day time.Time) string {
	return filepath.Join(s.rootDir, agentID, "daily", model.NormalizeDay(day).Format("2006-01-02")+".md")
}

func (s *Service) defaultCheckpointTitle(window model.SessionWindow) string {
	return fmt.Sprintf("%s checkpoint %s-%s", window.AgentID, window.WindowStart.Format("15:04"), window.WindowEnd.Format("15:04"))
}

func (s *Service) placeholderContent(window model.SessionWindow) string {
	return fmt.Sprintf(
		"no incremental transcript was ingested for %s-%s",
		window.WindowStart.Format("15:04"),
		window.WindowEnd.Format("15:04"),
	)
}

func renderCheckpointMarkdown(doc model.CheckpointDoc) string {
	return fmt.Sprintf(
		"# %s\n\n- window_key: `%s`\n- agent: `%s`\n- session: `%s`\n- state: `%s`\n\n%s\n",
		doc.Title,
		doc.WindowKey,
		doc.Window.AgentID,
		doc.Window.SessionID,
		doc.State,
		doc.Content,
	)
}

func renderDailyReportMarkdown(report model.DailyReport) string {
	return fmt.Sprintf(
		"# %s\n\n- day: `%s`\n- agent: `%s`\n- checkpoints: %d\n\n%s\n",
		report.Title,
		report.ReportDay.Format("2006-01-02"),
		report.AgentID,
		len(report.WindowKeys),
		report.Content,
	)
}

func withFallback(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
