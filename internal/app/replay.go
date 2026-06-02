package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ouroboros/ouroboros/internal/eventlog"
	"github.com/ouroboros/ouroboros/internal/evidence"
	"github.com/ouroboros/ouroboros/internal/tui"
)

type ReplayResult struct {
	RunID      string
	EventCount int
	LaneCounts map[eventlog.ReplayLane]int
	Timeline   []eventlog.ReplayTimelineEvent
	Evidence   evidence.Graph
	Status     eventlog.ReplayStatus
	StatusLine string
	ReportPath string
	TUIView    string
	TUIModel   tui.Model
}

func ReplayWorkspace(ctx context.Context, root string, runID string) (ReplayResult, error) {
	if strings.TrimSpace(runID) == "" {
		return ReplayResult{}, fmt.Errorf("run id is required")
	}
	doctor, err := DoctorWorkspace(root)
	if err != nil {
		return ReplayResult{}, err
	}
	if !doctor.Ready {
		return ReplayResult{}, fmt.Errorf("workspace is not ready: %s", strings.Join(doctor.Errors, "; "))
	}

	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		return ReplayResult{}, err
	}
	defer store.Close()

	events, err := store.Replay(ctx, runID)
	if err != nil {
		return ReplayResult{}, err
	}
	if len(events) == 0 {
		return ReplayResult{}, fmt.Errorf("run %s has no replay events", runID)
	}
	snapshot, err := eventlog.BuildReplaySnapshot(events)
	if err != nil {
		return ReplayResult{}, err
	}

	laneCounts := map[eventlog.ReplayLane]int{}
	for _, event := range snapshot.Timeline {
		laneCounts[event.Lane]++
	}
	tuiModel := tuiModelFromEventsForWorkspace(root, events)

	return ReplayResult{
		RunID:      snapshot.RunID,
		EventCount: len(events),
		LaneCounts: laneCounts,
		Timeline:   append([]eventlog.ReplayTimelineEvent(nil), snapshot.Timeline...),
		Evidence:   snapshot.Evidence,
		Status:     snapshot.Status,
		StatusLine: tui.RenderStatusBar(100, tui.StatusFromReplay(snapshot)),
		ReportPath: replayReportPath(events),
		TUIView:    tuiModel.View(),
		TUIModel:   tuiModel,
	}, nil
}

func replayReportPath(events []eventlog.Event) string {
	for _, event := range events {
		if event.Type != eventlog.EventReplay {
			continue
		}
		var payload struct {
			Status     string `json:"status"`
			ReportPath string `json:"report_path"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil && strings.TrimSpace(payload.Status) == "report_ready" && strings.TrimSpace(payload.ReportPath) != "" {
			return payload.ReportPath
		}
	}
	return ""
}
