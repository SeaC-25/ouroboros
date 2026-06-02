package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

func TestReplayWorkspaceRebuildsRunSnapshot(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}

	replay, err := ReplayWorkspace(ctx, root, run.RunID)
	if err != nil {
		t.Fatalf("replay workspace: %v", err)
	}
	if replay.RunID != run.RunID {
		t.Fatalf("unexpected replay run id: %#v", replay)
	}
	if replay.EventCount != run.EventCount {
		t.Fatalf("expected %d replayed events, got %d", run.EventCount, replay.EventCount)
	}
	if replay.Status.Model != "offline-model" || replay.Status.ToolHealth != "ready" {
		t.Fatalf("unexpected replay status: %#v", replay.Status)
	}
	if replay.LaneCounts[eventlog.ReplayLaneMission] == 0 || replay.LaneCounts[eventlog.ReplayLaneToolchain] == 0 {
		t.Fatalf("expected mission and toolchain lanes, got %#v", replay.LaneCounts)
	}
	if replay.ReportPath != run.ReportPath {
		t.Fatalf("expected report path %q, got %q", run.ReportPath, replay.ReportPath)
	}
	if len(replay.Evidence.Nodes()) == 0 {
		t.Fatalf("expected replay result to expose evidence graph nodes")
	}
	cursor, total := replay.TUIModel.ReplayCursor()
	if cursor != replay.EventCount || total != replay.EventCount {
		t.Fatalf("expected replay TUI model cursor at final event %d/%d, got %d/%d", replay.EventCount, replay.EventCount, cursor, total)
	}
	for _, want := range []string{"Workspace", "Mission Conversation", "Attack Graph", "Runtime Tabs", "model:offline-model"} {
		if !strings.Contains(replay.TUIView, want) {
			t.Fatalf("expected TUI view to contain %q, got:\n%s", want, replay.TUIView)
		}
	}
}

func TestCreateReplayWorkbenchSessionWritesMetadataOnlyArtifact(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	primary, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run primary workspace: %v", err)
	}
	comparison, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run comparison workspace: %v", err)
	}
	before, err := ReplayWorkspace(ctx, root, primary.RunID)
	if err != nil {
		t.Fatalf("replay before session save: %v", err)
	}
	createdAt := time.Date(2026, 6, 1, 2, 0, 0, 0, time.UTC)

	session, err := CreateReplayWorkbenchSession(ctx, root, ReplayWorkbenchSessionOptions{
		PrimaryRunID:  primary.RunID,
		CompareRunIDs: []string{comparison.RunID},
		SelectedRunID: comparison.RunID,
		Filters: map[string]string{
			"lane": "toolchain",
			"q":    "needle",
			"node": "fact-target",
		},
		CreatedBy: "operator",
		CreatedAt: createdAt,
		Reason:    "save focused graph review",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	if session.Status != "saved" || session.Mode != "metadata-only" || session.PrimaryRunID != primary.RunID || session.SelectedRunID != comparison.RunID {
		t.Fatalf("unexpected session metadata: %#v", session)
	}
	if session.Path == "" || filepath.Dir(session.Path) != filepath.Join(root, "replay", "workbench-sessions") {
		t.Fatalf("expected workspace-confined session path, got %q", session.Path)
	}
	if len(session.RunIDs) != 2 || session.RunIDs[0] != primary.RunID || session.RunIDs[1] != comparison.RunID {
		t.Fatalf("unexpected run ids: %#v", session.RunIDs)
	}
	if session.Filters["lane"] != "toolchain" || session.Filters["q"] != "needle" || session.Filters["node"] != "fact-target" {
		t.Fatalf("unexpected filters: %#v", session.Filters)
	}
	if session.Endpoints["html"] != "/?lane=toolchain&node=fact-target&q=needle&run="+comparison.RunID || session.Endpoints["graph"] != "/api/graph?lane=toolchain&node=fact-target&q=needle" {
		t.Fatalf("unexpected endpoints: %#v", session.Endpoints)
	}
	if session.CreatedBy != "operator" || session.CreatedAt != createdAt.Format(time.RFC3339) || session.Reason != "save focused graph review" {
		t.Fatalf("unexpected audit metadata: %#v", session)
	}

	listed, err := ListReplayWorkbenchSessions(root)
	if err != nil {
		t.Fatalf("list replay workbench sessions: %v", err)
	}
	if len(listed) != 1 || listed[0].Path != session.Path {
		t.Fatalf("unexpected session listing: %#v", listed)
	}
	read, err := ReadReplayWorkbenchSession(root, filepath.Base(session.Path))
	if err != nil {
		t.Fatalf("read replay workbench session by basename: %v", err)
	}
	if read.Path != session.Path || read.SelectedRunID != comparison.RunID {
		t.Fatalf("unexpected read session: %#v", read)
	}
	if _, err := ReadReplayWorkbenchSession(root, filepath.Join(t.TempDir(), "outside.json")); err == nil {
		t.Fatalf("expected outside replay workbench session path to be rejected")
	}

	after, err := ReplayWorkspace(ctx, root, primary.RunID)
	if err != nil {
		t.Fatalf("replay after session save: %v", err)
	}
	if after.EventCount != before.EventCount {
		t.Fatalf("saved replay workbench session must not mutate replay events, before=%d after=%d", before.EventCount, after.EventCount)
	}
}

func TestPreviewReplayWorkbenchSessionDoesNotWriteArtifact(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}

	session, err := PreviewReplayWorkbenchSession(ctx, root, ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Filters: map[string]string{
			"lane": "all",
			"edge": "edge-1",
		},
	})
	if err != nil {
		t.Fatalf("preview replay workbench session: %v", err)
	}
	if session.Status != "dry-run" || session.Mode != "metadata-only" || session.Path != "" {
		t.Fatalf("unexpected dry-run session: %#v", session)
	}
	if _, ok := session.Filters["lane"]; ok {
		t.Fatalf("lane=all should be normalized out of filters: %#v", session.Filters)
	}
	if session.Filters["edge"] != "edge-1" || session.Endpoints["graph_ndjson"] != "/api/graph.ndjson?edge=edge-1" {
		t.Fatalf("unexpected edge filter/session endpoints: filters=%#v endpoints=%#v", session.Filters, session.Endpoints)
	}
	listed, err := ListReplayWorkbenchSessions(root)
	if err != nil {
		t.Fatalf("list replay workbench sessions after preview: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("dry-run preview must not write artifacts, got %#v", listed)
	}
	if _, err := PreviewReplayWorkbenchSession(ctx, root, ReplayWorkbenchSessionOptions{PrimaryRunID: run.RunID, SelectedRunID: "missing-run"}); err == nil {
		t.Fatalf("expected unknown selected run to be rejected")
	}
}

func TestCreateReplayWorkbenchCollaborationWritesMetadataOnlyArtifact(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := CreateReplayWorkbenchSession(ctx, root, ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Filters: map[string]string{
			"lane": "timeline",
			"q":    "collab",
		},
		CreatedBy: "operator",
		Reason:    "shared review baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	before, err := ReplayWorkspace(ctx, root, run.RunID)
	if err != nil {
		t.Fatalf("replay before collaboration save: %v", err)
	}
	createdAt := time.Date(2026, 6, 1, 3, 45, 0, 0, time.UTC)

	preview, err := PreviewReplayWorkbenchCollaboration(ctx, root, ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob", "alice", " "},
		CreatedBy:    "lead",
		CreatedAt:    createdAt,
		Reason:       "pair review",
	})
	if err != nil {
		t.Fatalf("preview replay workbench collaboration: %v", err)
	}
	if preview.Status != "dry-run" || preview.Mode != "metadata-only" || preview.Path != "" {
		t.Fatalf("unexpected dry-run collaboration: %#v", preview)
	}
	if preview.SessionPath != session.Path || preview.PrimaryRunID != run.RunID || preview.SelectedRunID != run.RunID {
		t.Fatalf("unexpected collaboration session metadata: %#v", preview)
	}
	if len(preview.Participants) != 2 || preview.Participants[0] != "alice" || preview.Participants[1] != "bob" {
		t.Fatalf("unexpected normalized participants: %#v", preview.Participants)
	}
	if collaborations, err := ListReplayWorkbenchCollaborations(root); err != nil {
		t.Fatalf("list collaborations after preview: %v", err)
	} else if len(collaborations) != 0 {
		t.Fatalf("dry-run preview must not write collaboration artifacts, got %#v", collaborations)
	}

	collaboration, err := CreateReplayWorkbenchCollaboration(ctx, root, ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		CreatedAt:    createdAt,
		Reason:       "pair review",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	if collaboration.Status != "saved" || collaboration.Mode != "metadata-only" || collaboration.SessionPath != session.Path {
		t.Fatalf("unexpected saved collaboration metadata: %#v", collaboration)
	}
	if collaboration.Path == "" || filepath.Dir(collaboration.Path) != filepath.Join(root, "replay", "workbench-collaborations") {
		t.Fatalf("expected workspace-confined collaboration path, got %q", collaboration.Path)
	}
	if collaboration.CreatedBy != "lead" || collaboration.CreatedAt != createdAt.Format(time.RFC3339) || collaboration.Reason != "pair review" {
		t.Fatalf("unexpected collaboration audit metadata: %#v", collaboration)
	}
	if collaboration.Endpoints["collaborations"] != "/api/collaborations" || collaboration.Endpoints["session"] != "/api/session?lane=timeline&q=collab" {
		t.Fatalf("unexpected collaboration endpoints: %#v", collaboration.Endpoints)
	}

	listed, err := ListReplayWorkbenchCollaborations(root)
	if err != nil {
		t.Fatalf("list replay workbench collaborations: %v", err)
	}
	if len(listed) != 1 || listed[0].Path != collaboration.Path {
		t.Fatalf("unexpected collaboration listing: %#v", listed)
	}
	read, err := ReadReplayWorkbenchCollaboration(root, filepath.Base(collaboration.Path))
	if err != nil {
		t.Fatalf("read replay workbench collaboration by basename: %v", err)
	}
	if read.Path != collaboration.Path || len(read.Participants) != 2 {
		t.Fatalf("unexpected read collaboration: %#v", read)
	}
	if _, err := ReadReplayWorkbenchCollaboration(root, filepath.Join(t.TempDir(), "outside.json")); err == nil {
		t.Fatalf("expected outside replay workbench collaboration path to be rejected")
	}

	after, err := ReplayWorkspace(ctx, root, run.RunID)
	if err != nil {
		t.Fatalf("replay after collaboration save: %v", err)
	}
	if after.EventCount != before.EventCount {
		t.Fatalf("saved replay workbench collaboration must not mutate replay events, before=%d after=%d", before.EventCount, after.EventCount)
	}
}

func TestUpdateReplayWorkbenchCollaborationPresenceWritesMetadataOnlyArtifact(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := CreateReplayWorkbenchSession(ctx, root, ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Reason:       "presence baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := CreateReplayWorkbenchCollaboration(ctx, root, ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		Reason:       "presence review",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	before, err := ReplayWorkspace(ctx, root, run.RunID)
	if err != nil {
		t.Fatalf("replay before collaboration presence: %v", err)
	}
	seenAt := time.Date(2026, 6, 1, 5, 40, 0, 0, time.UTC)

	preview, err := PreviewReplayWorkbenchCollaborationPresence(ctx, root, ReplayWorkbenchCollaborationPresenceOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Participant:       "alice",
		Status:            "online",
		SeenBy:            "lead",
		SeenAt:            seenAt,
		Reason:            "joined review",
	})
	if err != nil {
		t.Fatalf("preview replay workbench collaboration presence: %v", err)
	}
	if preview.Path != "" || preview.Status != "online" || preview.Mode != "metadata-only" || preview.CollaborationPath != collaboration.Path || preview.SessionPath != session.Path || preview.Participant != "alice" || preview.SeenBy != "lead" {
		t.Fatalf("unexpected dry-run collaboration presence: %#v", preview)
	}
	if listed, err := ListReplayWorkbenchCollaborationPresence(root); err != nil {
		t.Fatalf("list presence after dry run: %v", err)
	} else if len(listed) != 0 {
		t.Fatalf("dry-run presence must not write artifacts, got %#v", listed)
	}

	presence, err := UpdateReplayWorkbenchCollaborationPresence(ctx, root, ReplayWorkbenchCollaborationPresenceOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Participant:       "alice",
		Status:            "away",
		SeenBy:            "alice",
		SeenAt:            seenAt.Add(time.Minute),
		Reason:            "review still active",
	})
	if err != nil {
		t.Fatalf("update replay workbench collaboration presence: %v", err)
	}
	if presence.Path == "" || presence.Status != "away" || presence.Participant != "alice" || presence.SeenAt != seenAt.Add(time.Minute).Format(time.RFC3339) || presence.Reason != "review still active" {
		t.Fatalf("unexpected persisted collaboration presence: %#v", presence)
	}
	listed, err := ListReplayWorkbenchCollaborationPresence(root)
	if err != nil {
		t.Fatalf("list replay workbench collaboration presence: %v", err)
	}
	if len(listed) != 1 || listed[0].Path != presence.Path || listed[0].Status != "away" {
		t.Fatalf("unexpected collaboration presence listing: %#v", listed)
	}
	read, err := ReadReplayWorkbenchCollaborationPresence(root, filepath.Base(presence.Path))
	if err != nil {
		t.Fatalf("read replay workbench collaboration presence by basename: %v", err)
	}
	if read.Path != presence.Path || read.CollaborationPath != collaboration.Path || read.Participant != "alice" {
		t.Fatalf("unexpected read collaboration presence: %#v", read)
	}
	if _, err := UpdateReplayWorkbenchCollaborationPresence(ctx, root, ReplayWorkbenchCollaborationPresenceOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Participant:       "alice",
		Status:            "unknown",
	}); err == nil {
		t.Fatalf("expected unsupported collaboration presence status to fail")
	}
	if _, err := ReadReplayWorkbenchCollaborationPresence(root, filepath.Join(t.TempDir(), "outside.json")); err == nil {
		t.Fatalf("expected outside replay workbench collaboration presence path to be rejected")
	}
	after, err := ReplayWorkspace(ctx, root, run.RunID)
	if err != nil {
		t.Fatalf("replay after collaboration presence: %v", err)
	}
	if after.EventCount != before.EventCount {
		t.Fatalf("collaboration presence must not mutate replay events, before=%d after=%d", before.EventCount, after.EventCount)
	}
}

func TestCreateReplayWorkbenchCollaborationNoteWritesMetadataOnlyArtifact(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := CreateReplayWorkbenchSession(ctx, root, ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Filters: map[string]string{
			"node": "fact-target",
		},
		Reason: "note baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := CreateReplayWorkbenchCollaboration(ctx, root, ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		Reason:       "note review",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	before, err := ReplayWorkspace(ctx, root, run.RunID)
	if err != nil {
		t.Fatalf("replay before collaboration note: %v", err)
	}
	createdAt := time.Date(2026, 6, 1, 8, 20, 0, 0, time.UTC)

	preview, err := PreviewReplayWorkbenchCollaborationNote(ctx, root, ReplayWorkbenchCollaborationNoteOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Author:            "alice",
		Body:              "Need to verify this graph node with Bob before accepting.",
		TargetKind:        "graph-node",
		TargetID:          "fact-target",
		CreatedAt:         createdAt,
		Reason:            "collaborative evidence review",
	})
	if err != nil {
		t.Fatalf("preview replay workbench collaboration note: %v", err)
	}
	if preview.Path != "" || preview.Status != "dry-run" || preview.Mode != "metadata-only" {
		t.Fatalf("unexpected dry-run collaboration note: %#v", preview)
	}
	if preview.CollaborationPath != collaboration.Path || preview.SessionPath != session.Path || preview.PrimaryRunID != run.RunID || preview.SelectedRunID != run.RunID {
		t.Fatalf("unexpected note collaboration/session metadata: %#v", preview)
	}
	if preview.Author != "alice" || preview.Body != "Need to verify this graph node with Bob before accepting." || preview.TargetKind != "graph-node" || preview.TargetID != "fact-target" || preview.CreatedAt != createdAt.Format(time.RFC3339) {
		t.Fatalf("unexpected note review metadata: %#v", preview)
	}
	if listed, err := ListReplayWorkbenchCollaborationNotes(root); err != nil {
		t.Fatalf("list notes after preview: %v", err)
	} else if len(listed) != 0 {
		t.Fatalf("dry-run note must not write artifacts, got %#v", listed)
	}

	note, err := CreateReplayWorkbenchCollaborationNote(ctx, root, ReplayWorkbenchCollaborationNoteOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Author:            "alice",
		Body:              "Need to verify this graph node with Bob before accepting.",
		TargetKind:        "graph-node",
		TargetID:          "fact-target",
		CreatedAt:         createdAt,
		Reason:            "collaborative evidence review",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration note: %v", err)
	}
	if note.Path == "" || filepath.Dir(note.Path) != filepath.Join(root, "replay", "workbench-collaboration-notes") {
		t.Fatalf("expected workspace-confined collaboration note path, got %q", note.Path)
	}
	if note.Status != "saved" || note.Mode != "metadata-only" || note.CollaborationPath != collaboration.Path || note.SessionPath != session.Path {
		t.Fatalf("unexpected saved note metadata: %#v", note)
	}
	if note.Author != "alice" || note.Body != preview.Body || note.TargetKind != "graph-node" || note.TargetID != "fact-target" || note.Reason != "collaborative evidence review" {
		t.Fatalf("unexpected saved note fields: %#v", note)
	}
	listed, err := ListReplayWorkbenchCollaborationNotes(root)
	if err != nil {
		t.Fatalf("list replay workbench collaboration notes: %v", err)
	}
	if len(listed) != 1 || listed[0].Path != note.Path {
		t.Fatalf("unexpected collaboration note listing: %#v", listed)
	}
	read, err := ReadReplayWorkbenchCollaborationNote(root, filepath.Base(note.Path))
	if err != nil {
		t.Fatalf("read replay workbench collaboration note by basename: %v", err)
	}
	if read.Path != note.Path || read.Body != note.Body || read.TargetID != "fact-target" {
		t.Fatalf("unexpected read collaboration note: %#v", read)
	}
	if _, err := CreateReplayWorkbenchCollaborationNote(ctx, root, ReplayWorkbenchCollaborationNoteOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Author:            "alice",
	}); err == nil {
		t.Fatalf("expected empty collaboration note body to fail")
	}
	if _, err := ReadReplayWorkbenchCollaborationNote(root, filepath.Join(t.TempDir(), "outside.json")); err == nil {
		t.Fatalf("expected outside replay workbench collaboration note path to be rejected")
	}
	after, err := ReplayWorkspace(ctx, root, run.RunID)
	if err != nil {
		t.Fatalf("replay after collaboration note: %v", err)
	}
	if after.EventCount != before.EventCount {
		t.Fatalf("collaboration note must not mutate replay events, before=%d after=%d", before.EventCount, after.EventCount)
	}
}

func TestSelectReplayWorkbenchCollaborationWritesCurrentMetadataOnlySelection(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := CreateReplayWorkbenchSession(ctx, root, ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Filters:      map[string]string{"lane": "timeline"},
		Reason:       "saved collaboration baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	base, err := CreateReplayWorkbenchCollaboration(ctx, root, ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice"},
		CreatedAt:    time.Date(2026, 6, 1, 3, 50, 0, 0, time.UTC),
		Reason:       "baseline pair review",
	})
	if err != nil {
		t.Fatalf("create baseline collaboration: %v", err)
	}
	candidate, err := CreateReplayWorkbenchCollaboration(ctx, root, ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		CreatedAt:    time.Date(2026, 6, 1, 3, 51, 0, 0, time.UTC),
		Reason:       "current pair review",
	})
	if err != nil {
		t.Fatalf("create candidate collaboration: %v", err)
	}
	before, err := ReplayWorkspace(ctx, root, run.RunID)
	if err != nil {
		t.Fatalf("replay before collaboration selection: %v", err)
	}
	selectedAt := time.Date(2026, 6, 1, 3, 52, 0, 0, time.UTC)

	selection, err := SelectReplayWorkbenchCollaboration(root, filepath.Base(candidate.Path), ReplayWorkbenchCollaborationSelectionOptions{
		SelectedBy: "alice",
		SelectedAt: selectedAt,
		Reason:     "current collaboration focus",
	})
	if err != nil {
		t.Fatalf("select replay workbench collaboration: %v", err)
	}
	if selection.Path != filepath.Join(root, "replay", "workbench-collaboration-selection.json") {
		t.Fatalf("unexpected collaboration selection path: %#v", selection)
	}
	if selection.Status != "current" || selection.Mode != "metadata-only" || selection.CollaborationPath != candidate.Path || selection.SessionPath != session.Path {
		t.Fatalf("unexpected current collaboration selection metadata: %#v", selection)
	}
	if selection.PrimaryRunID != run.RunID || selection.SelectedRunID != run.RunID || selection.RunIDs != 1 || selection.Filters != 1 || selection.Endpoints != len(candidate.Endpoints) || selection.Participants != 2 {
		t.Fatalf("unexpected collaboration selection counts: %#v", selection)
	}
	if len(selection.SupersededPaths) != 1 || selection.SupersededPaths[0] != base.Path {
		t.Fatalf("unexpected superseded collaborations: %#v", selection.SupersededPaths)
	}
	if selection.SelectedBy != "alice" || selection.SelectedAt != selectedAt.Format(time.RFC3339) || selection.Reason != "current collaboration focus" {
		t.Fatalf("unexpected collaboration selection actor metadata: %#v", selection)
	}
	read, found, err := ReadReplayWorkbenchCollaborationSelection(root)
	if err != nil {
		t.Fatalf("read replay workbench collaboration selection: %v", err)
	}
	if !found || read.CollaborationPath != candidate.Path || read.SelectedBy != "alice" {
		t.Fatalf("expected persisted collaboration selection, found=%v selection=%#v", found, read)
	}
	if _, err := SelectReplayWorkbenchCollaboration(root, filepath.Join(t.TempDir(), "outside.json"), ReplayWorkbenchCollaborationSelectionOptions{SelectedBy: "operator"}); err == nil {
		t.Fatalf("expected outside replay workbench collaboration selection path to be rejected")
	}
	after, err := ReplayWorkspace(ctx, root, run.RunID)
	if err != nil {
		t.Fatalf("replay after collaboration selection: %v", err)
	}
	if after.EventCount != before.EventCount {
		t.Fatalf("replay workbench collaboration selection must not mutate replay events, before=%d after=%d", before.EventCount, after.EventCount)
	}
}

func TestClearReplayWorkbenchCollaborationSelectionWritesClearedMetadataOnlyMarker(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := CreateReplayWorkbenchSession(ctx, root, ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Reason:       "saved collaboration baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := CreateReplayWorkbenchCollaboration(ctx, root, ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		Reason:       "pair review",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	if _, err := SelectReplayWorkbenchCollaboration(root, filepath.Base(collaboration.Path), ReplayWorkbenchCollaborationSelectionOptions{
		SelectedBy: "alice",
		Reason:     "current collaboration focus",
	}); err != nil {
		t.Fatalf("select replay workbench collaboration: %v", err)
	}
	clearedAt := time.Date(2026, 6, 1, 3, 55, 0, 0, time.UTC)

	selection, err := ClearReplayWorkbenchCollaborationSelection(root, ReplayWorkbenchCollaborationSelectionClearOptions{
		ClearedBy: "bob",
		ClearedAt: clearedAt,
		Reason:    "collaboration archived",
	})
	if err != nil {
		t.Fatalf("clear replay workbench collaboration selection: %v", err)
	}
	if selection.Status != "cleared" || selection.Mode != "metadata-only" || selection.CollaborationPath != collaboration.Path || selection.SessionPath != session.Path {
		t.Fatalf("unexpected cleared collaboration selection metadata: %#v", selection)
	}
	if selection.ClearedBy != "bob" || selection.ClearedAt != clearedAt.Format(time.RFC3339) || selection.Reason != "collaboration archived" {
		t.Fatalf("unexpected cleared collaboration actor metadata: %#v", selection)
	}
	read, found, err := ReadReplayWorkbenchCollaborationSelection(root)
	if err != nil {
		t.Fatalf("read cleared replay workbench collaboration selection: %v", err)
	}
	if !found || read.Status != "cleared" || read.CollaborationPath != collaboration.Path {
		t.Fatalf("expected persisted cleared collaboration selection, found=%v selection=%#v", found, read)
	}
}

func TestAnnotateReplayWorkbenchSessionWritesMetadataOnlyDecision(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := CreateReplayWorkbenchSession(ctx, root, ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Reason:       "saved review focus",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	before, err := ReplayWorkspace(ctx, root, run.RunID)
	if err != nil {
		t.Fatalf("replay before annotation: %v", err)
	}
	reviewedAt := time.Date(2026, 6, 1, 2, 35, 0, 0, time.UTC)

	annotated, err := AnnotateReplayWorkbenchSession(root, filepath.Base(session.Path), ReplayWorkbenchSessionAnnotationOptions{
		Decision:   "accepted",
		Reason:     "authorized comparison baseline",
		Reviewer:   "alice",
		ReviewedAt: reviewedAt,
	})
	if err != nil {
		t.Fatalf("annotate replay workbench session: %v", err)
	}
	if annotated.Path != session.Path || annotated.Annotation == nil {
		t.Fatalf("expected annotation on original artifact, got %#v", annotated)
	}
	if annotated.Annotation.Decision != "accepted" || annotated.Annotation.Reason != "authorized comparison baseline" || annotated.Annotation.Reviewer != "alice" || annotated.Annotation.ReviewedAt != reviewedAt.Format(time.RFC3339) {
		t.Fatalf("unexpected annotation metadata: %#v", annotated.Annotation)
	}
	read, err := ReadReplayWorkbenchSession(root, filepath.Base(session.Path))
	if err != nil {
		t.Fatalf("read annotated replay workbench session: %v", err)
	}
	if read.Annotation == nil || read.Annotation.Decision != "accepted" {
		t.Fatalf("expected persisted annotation, got %#v", read)
	}
	listed, err := ListReplayWorkbenchSessions(root)
	if err != nil {
		t.Fatalf("list annotated replay workbench sessions: %v", err)
	}
	if len(listed) != 1 || listed[0].Annotation == nil || listed[0].Annotation.Reviewer != "alice" {
		t.Fatalf("expected listed annotation, got %#v", listed)
	}
	if _, err := AnnotateReplayWorkbenchSession(root, filepath.Base(session.Path), ReplayWorkbenchSessionAnnotationOptions{Decision: "unknown"}); err == nil {
		t.Fatalf("expected unsupported replay workbench session annotation decision to fail")
	}
	if _, err := AnnotateReplayWorkbenchSession(root, filepath.Join(t.TempDir(), "outside.json"), ReplayWorkbenchSessionAnnotationOptions{Decision: "deferred"}); err == nil {
		t.Fatalf("expected outside replay workbench session annotation path to be rejected")
	}
	after, err := ReplayWorkspace(ctx, root, run.RunID)
	if err != nil {
		t.Fatalf("replay after annotation: %v", err)
	}
	if after.EventCount != before.EventCount {
		t.Fatalf("replay workbench session annotation must not mutate replay events, before=%d after=%d", before.EventCount, after.EventCount)
	}
}

func TestAnnotateReplayWorkbenchCollaborationWritesMetadataOnlyDecision(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := CreateReplayWorkbenchSession(ctx, root, ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Reason:       "saved collaboration baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := CreateReplayWorkbenchCollaboration(ctx, root, ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		Reason:       "pair review",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	before, err := ReplayWorkspace(ctx, root, run.RunID)
	if err != nil {
		t.Fatalf("replay before collaboration annotation: %v", err)
	}
	reviewedAt := time.Date(2026, 6, 1, 5, 10, 0, 0, time.UTC)

	annotated, err := AnnotateReplayWorkbenchCollaboration(root, filepath.Base(collaboration.Path), ReplayWorkbenchCollaborationAnnotationOptions{
		Decision:   "deferred",
		Reason:     "needs second operator",
		Reviewer:   "alice",
		ReviewedAt: reviewedAt,
	})
	if err != nil {
		t.Fatalf("annotate replay workbench collaboration: %v", err)
	}
	if annotated.Path != collaboration.Path || annotated.Annotation == nil {
		t.Fatalf("expected annotation on original collaboration artifact, got %#v", annotated)
	}
	if annotated.Annotation.Decision != "deferred" ||
		annotated.Annotation.Reason != "needs second operator" ||
		annotated.Annotation.Reviewer != "alice" ||
		annotated.Annotation.ReviewedAt != reviewedAt.Format(time.RFC3339) {
		t.Fatalf("unexpected collaboration annotation metadata: %#v", annotated.Annotation)
	}
	read, err := ReadReplayWorkbenchCollaboration(root, filepath.Base(collaboration.Path))
	if err != nil {
		t.Fatalf("read annotated replay workbench collaboration: %v", err)
	}
	if read.Annotation == nil || read.Annotation.Decision != "deferred" {
		t.Fatalf("expected persisted collaboration annotation, got %#v", read)
	}
	listed, err := ListReplayWorkbenchCollaborations(root)
	if err != nil {
		t.Fatalf("list annotated replay workbench collaborations: %v", err)
	}
	if len(listed) != 1 || listed[0].Annotation == nil || listed[0].Annotation.Reviewer != "alice" {
		t.Fatalf("expected listed collaboration annotation, got %#v", listed)
	}
	if _, err := AnnotateReplayWorkbenchCollaboration(root, filepath.Base(collaboration.Path), ReplayWorkbenchCollaborationAnnotationOptions{Decision: "unknown"}); err == nil {
		t.Fatalf("expected unsupported replay workbench collaboration annotation decision to fail")
	}
	if _, err := AnnotateReplayWorkbenchCollaboration(root, filepath.Join(t.TempDir(), "outside.json"), ReplayWorkbenchCollaborationAnnotationOptions{Decision: "accepted"}); err == nil {
		t.Fatalf("expected outside replay workbench collaboration annotation path to be rejected")
	}
	after, err := ReplayWorkspace(ctx, root, run.RunID)
	if err != nil {
		t.Fatalf("replay after collaboration annotation: %v", err)
	}
	if after.EventCount != before.EventCount {
		t.Fatalf("replay workbench collaboration annotation must not mutate replay events, before=%d after=%d", before.EventCount, after.EventCount)
	}
}

func TestAnnotateReplayWorkbenchCollaborationNoteWritesMetadataOnlyDecision(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := CreateReplayWorkbenchSession(ctx, root, ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Reason:       "saved note baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := CreateReplayWorkbenchCollaboration(ctx, root, ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		Reason:       "pair review",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	note, err := CreateReplayWorkbenchCollaborationNote(ctx, root, ReplayWorkbenchCollaborationNoteOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Author:            "alice",
		Body:              "Confirm graph node before accepting.",
		TargetKind:        "graph-node",
		TargetID:          "fact-target",
		Reason:            "collaborative evidence review",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration note: %v", err)
	}
	before, err := ReplayWorkspace(ctx, root, run.RunID)
	if err != nil {
		t.Fatalf("replay before collaboration note annotation: %v", err)
	}
	reviewedAt := time.Date(2026, 6, 1, 9, 30, 0, 0, time.UTC)

	annotated, err := AnnotateReplayWorkbenchCollaborationNote(root, filepath.Base(note.Path), ReplayWorkbenchCollaborationNoteAnnotationOptions{
		Decision:   "accepted",
		Reason:     "validated evidence note",
		Reviewer:   "alice",
		ReviewedAt: reviewedAt,
	})
	if err != nil {
		t.Fatalf("annotate replay workbench collaboration note: %v", err)
	}
	if annotated.Path != note.Path || annotated.Annotation == nil {
		t.Fatalf("expected annotation on original note artifact, got %#v", annotated)
	}
	if annotated.Annotation.Decision != "accepted" ||
		annotated.Annotation.Reason != "validated evidence note" ||
		annotated.Annotation.Reviewer != "alice" ||
		annotated.Annotation.ReviewedAt != reviewedAt.Format(time.RFC3339) {
		t.Fatalf("unexpected collaboration note annotation metadata: %#v", annotated.Annotation)
	}
	read, err := ReadReplayWorkbenchCollaborationNote(root, filepath.Base(note.Path))
	if err != nil {
		t.Fatalf("read annotated replay workbench collaboration note: %v", err)
	}
	if read.Annotation == nil || read.Annotation.Decision != "accepted" {
		t.Fatalf("expected persisted collaboration note annotation, got %#v", read)
	}
	listed, err := ListReplayWorkbenchCollaborationNotes(root)
	if err != nil {
		t.Fatalf("list annotated replay workbench collaboration notes: %v", err)
	}
	if len(listed) != 1 || listed[0].Annotation == nil || listed[0].Annotation.Reviewer != "alice" {
		t.Fatalf("expected listed collaboration note annotation, got %#v", listed)
	}
	if _, err := AnnotateReplayWorkbenchCollaborationNote(root, filepath.Base(note.Path), ReplayWorkbenchCollaborationNoteAnnotationOptions{Decision: "unknown"}); err == nil {
		t.Fatalf("expected unsupported replay workbench collaboration note annotation decision to fail")
	}
	if _, err := AnnotateReplayWorkbenchCollaborationNote(root, filepath.Join(t.TempDir(), "outside.json"), ReplayWorkbenchCollaborationNoteAnnotationOptions{Decision: "accepted"}); err == nil {
		t.Fatalf("expected outside replay workbench collaboration note annotation path to be rejected")
	}
	after, err := ReplayWorkspace(ctx, root, run.RunID)
	if err != nil {
		t.Fatalf("replay after collaboration note annotation: %v", err)
	}
	if after.EventCount != before.EventCount {
		t.Fatalf("replay workbench collaboration note annotation must not mutate replay events, before=%d after=%d", before.EventCount, after.EventCount)
	}
}

func TestSelectReplayWorkbenchCollaborationNoteWritesCurrentMetadataOnlySelection(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := CreateReplayWorkbenchSession(ctx, root, ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Reason:       "saved note baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := CreateReplayWorkbenchCollaboration(ctx, root, ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		Reason:       "pair review",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	base, err := CreateReplayWorkbenchCollaborationNote(ctx, root, ReplayWorkbenchCollaborationNoteOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Author:            "alice",
		Body:              "Baseline note",
		TargetKind:        "graph-node",
		TargetID:          "fact-base",
		CreatedAt:         time.Date(2026, 6, 1, 9, 40, 0, 0, time.UTC),
		Reason:            "baseline evidence note",
	})
	if err != nil {
		t.Fatalf("create baseline replay workbench collaboration note: %v", err)
	}
	candidate, err := CreateReplayWorkbenchCollaborationNote(ctx, root, ReplayWorkbenchCollaborationNoteOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Author:            "bob",
		Body:              "Current note",
		TargetKind:        "graph-edge",
		TargetID:          "edge-1",
		CreatedAt:         time.Date(2026, 6, 1, 9, 41, 0, 0, time.UTC),
		Reason:            "current evidence note",
	})
	if err != nil {
		t.Fatalf("create candidate replay workbench collaboration note: %v", err)
	}
	before, err := ReplayWorkspace(ctx, root, run.RunID)
	if err != nil {
		t.Fatalf("replay before collaboration note selection: %v", err)
	}

	selected, err := SelectReplayWorkbenchCollaborationNote(root, filepath.Base(candidate.Path), ReplayWorkbenchCollaborationNoteSelectionOptions{
		SelectedBy: "alice",
		SelectedAt: time.Date(2026, 6, 1, 9, 42, 0, 0, time.UTC),
		Reason:     "current note focus",
	})
	if err != nil {
		t.Fatalf("select replay workbench collaboration note: %v", err)
	}
	if selected.Status != "current" ||
		selected.Mode != "metadata-only" ||
		selected.NotePath != candidate.Path ||
		selected.CollaborationPath != collaboration.Path ||
		selected.SessionPath != session.Path ||
		selected.PrimaryRunID != run.RunID ||
		selected.SelectedRunID != run.RunID ||
		selected.Author != "bob" ||
		selected.TargetKind != "graph-edge" ||
		selected.TargetID != "edge-1" ||
		selected.SelectedBy != "alice" ||
		selected.SelectedAt != "2026-06-01T09:42:00Z" ||
		selected.Reason != "current note focus" ||
		len(selected.SupersededPaths) != 1 ||
		selected.SupersededPaths[0] != base.Path {
		t.Fatalf("unexpected replay workbench collaboration note selection: %#v", selected)
	}
	read, found, err := ReadReplayWorkbenchCollaborationNoteSelection(root)
	if err != nil {
		t.Fatalf("read replay workbench collaboration note selection: %v", err)
	}
	if !found || read.NotePath != candidate.Path || read.SelectedBy != "alice" {
		t.Fatalf("expected persisted replay workbench collaboration note selection, found=%v selection=%#v", found, read)
	}
	cleared, err := ClearReplayWorkbenchCollaborationNoteSelection(root, ReplayWorkbenchCollaborationNoteSelectionClearOptions{
		ClearedBy: "bob",
		ClearedAt: time.Date(2026, 6, 1, 9, 45, 0, 0, time.UTC),
		Reason:    "note focus closed",
	})
	if err != nil {
		t.Fatalf("clear replay workbench collaboration note selection: %v", err)
	}
	if cleared.Status != "cleared" ||
		cleared.NotePath != candidate.Path ||
		cleared.ClearedBy != "bob" ||
		cleared.ClearedAt != "2026-06-01T09:45:00Z" ||
		cleared.Reason != "note focus closed" {
		t.Fatalf("unexpected cleared replay workbench collaboration note selection: %#v", cleared)
	}
	if _, err := SelectReplayWorkbenchCollaborationNote(root, filepath.Join(t.TempDir(), "outside.json"), ReplayWorkbenchCollaborationNoteSelectionOptions{}); err == nil {
		t.Fatalf("expected outside replay workbench collaboration note selection path to be rejected")
	}
	after, err := ReplayWorkspace(ctx, root, run.RunID)
	if err != nil {
		t.Fatalf("replay after collaboration note selection: %v", err)
	}
	if after.EventCount != before.EventCount {
		t.Fatalf("replay workbench collaboration note selection must not mutate replay events, before=%d after=%d", before.EventCount, after.EventCount)
	}
}

func TestSelectReplayWorkbenchSessionWritesCurrentMetadataOnlySelection(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	base, err := CreateReplayWorkbenchSession(ctx, root, ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		CreatedAt:    time.Date(2026, 6, 1, 2, 40, 0, 0, time.UTC),
		Reason:       "baseline saved view",
	})
	if err != nil {
		t.Fatalf("create baseline session: %v", err)
	}
	candidate, err := CreateReplayWorkbenchSession(ctx, root, ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Filters: map[string]string{
			"lane": "toolchain",
		},
		CreatedAt: time.Date(2026, 6, 1, 2, 41, 0, 0, time.UTC),
		Reason:    "current saved view",
	})
	if err != nil {
		t.Fatalf("create candidate session: %v", err)
	}
	before, err := ReplayWorkspace(ctx, root, run.RunID)
	if err != nil {
		t.Fatalf("replay before selection: %v", err)
	}
	selectedAt := time.Date(2026, 6, 1, 2, 42, 0, 0, time.UTC)

	selection, err := SelectReplayWorkbenchSession(root, filepath.Base(candidate.Path), ReplayWorkbenchSessionSelectionOptions{
		SelectedBy: "alice",
		SelectedAt: selectedAt,
		Reason:     "current investigation focus",
	})
	if err != nil {
		t.Fatalf("select replay workbench session: %v", err)
	}
	if selection.Path != filepath.Join(root, "replay", "workbench-session-selection.json") {
		t.Fatalf("unexpected selection path: %#v", selection)
	}
	if selection.Status != "current" || selection.Mode != "metadata-only" || selection.SessionPath != candidate.Path || selection.PrimaryRunID != run.RunID || selection.SelectedRunID != run.RunID {
		t.Fatalf("unexpected current selection metadata: %#v", selection)
	}
	if selection.RunIDs != 1 || selection.Filters != 1 || selection.Endpoints != len(candidate.Endpoints) {
		t.Fatalf("unexpected selection counts: %#v", selection)
	}
	if len(selection.SupersededPaths) != 1 || selection.SupersededPaths[0] != base.Path {
		t.Fatalf("unexpected superseded sessions: %#v", selection.SupersededPaths)
	}
	if selection.SelectedBy != "alice" || selection.SelectedAt != selectedAt.Format(time.RFC3339) || selection.Reason != "current investigation focus" {
		t.Fatalf("unexpected actor metadata: %#v", selection)
	}
	read, found, err := ReadReplayWorkbenchSessionSelection(root)
	if err != nil {
		t.Fatalf("read replay workbench session selection: %v", err)
	}
	if !found || read.SessionPath != candidate.Path || read.SelectedBy != "alice" {
		t.Fatalf("expected persisted current selection, found=%v selection=%#v", found, read)
	}
	if _, err := SelectReplayWorkbenchSession(root, filepath.Join(t.TempDir(), "outside.json"), ReplayWorkbenchSessionSelectionOptions{SelectedBy: "operator"}); err == nil {
		t.Fatalf("expected outside replay workbench session selection path to be rejected")
	}
	after, err := ReplayWorkspace(ctx, root, run.RunID)
	if err != nil {
		t.Fatalf("replay after selection: %v", err)
	}
	if after.EventCount != before.EventCount {
		t.Fatalf("replay workbench session selection must not mutate replay events, before=%d after=%d", before.EventCount, after.EventCount)
	}
}

func TestClearReplayWorkbenchSessionSelectionWritesClearedMetadataOnlyMarker(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := CreateReplayWorkbenchSession(ctx, root, ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Reason:       "saved view",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := SelectReplayWorkbenchSession(root, filepath.Base(session.Path), ReplayWorkbenchSessionSelectionOptions{
		SelectedBy: "alice",
		Reason:     "current investigation focus",
	}); err != nil {
		t.Fatalf("select session: %v", err)
	}
	clearedAt := time.Date(2026, 6, 1, 2, 45, 0, 0, time.UTC)

	selection, err := ClearReplayWorkbenchSessionSelection(root, ReplayWorkbenchSessionSelectionClearOptions{
		ClearedBy: "bob",
		ClearedAt: clearedAt,
		Reason:    "investigation archived",
	})
	if err != nil {
		t.Fatalf("clear replay workbench session selection: %v", err)
	}
	if selection.Status != "cleared" || selection.Mode != "metadata-only" || selection.SessionPath != session.Path {
		t.Fatalf("unexpected cleared selection metadata: %#v", selection)
	}
	if selection.ClearedBy != "bob" || selection.ClearedAt != clearedAt.Format(time.RFC3339) || selection.Reason != "investigation archived" {
		t.Fatalf("unexpected cleared actor metadata: %#v", selection)
	}
	read, found, err := ReadReplayWorkbenchSessionSelection(root)
	if err != nil {
		t.Fatalf("read cleared replay workbench session selection: %v", err)
	}
	if !found || read.Status != "cleared" || read.SessionPath != session.Path {
		t.Fatalf("expected persisted cleared selection, found=%v selection=%#v", found, read)
	}
}

func TestCreateReplayWorkbenchSessionRejectsSymlinkedSessionDirectoryBeforeWriting(t *testing.T) {
	ctx := context.Background()
	root, runID := replayWorkbenchRunForSafetyTest(t, ctx)
	outsideDir := filepath.Join(t.TempDir(), "outside-sessions")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("create outside session dir: %v", err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(root, "replay", "workbench-sessions")); err != nil {
		t.Fatalf("symlink session dir: %v", err)
	}

	_, err := CreateReplayWorkbenchSession(ctx, root, ReplayWorkbenchSessionOptions{PrimaryRunID: runID, Reason: "symlink check"})
	if err == nil || !strings.Contains(err.Error(), "replay workbench session directory must not be a symlink") {
		t.Fatalf("expected symlinked session directory rejection, got %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(outsideDir, "*.json"))
	if err != nil {
		t.Fatalf("glob outside sessions: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("symlinked session directory must not receive artifacts, got %#v", matches)
	}
}

func TestAnnotateReplayWorkbenchSessionRejectsSymlinkedSessionFileBeforeWriting(t *testing.T) {
	ctx := context.Background()
	root, _, session := replayWorkbenchSessionForSafetyTest(t, ctx)
	outside := replayWorkbenchReplaceWithSymlinkedCopyForSafetyTest(t, session.Path, "outside-session.json")
	before := replayWorkbenchReadFileForSafetyTest(t, outside)

	_, err := AnnotateReplayWorkbenchSession(root, filepath.Base(session.Path), ReplayWorkbenchSessionAnnotationOptions{Decision: "accepted", Reviewer: "alice"})
	if err == nil || !strings.Contains(err.Error(), "replay workbench session file must not be a symlink") {
		t.Fatalf("expected symlinked session file rejection, got %v", err)
	}
	after := replayWorkbenchReadFileForSafetyTest(t, outside)
	if string(after) != string(before) {
		t.Fatalf("symlinked session annotation must not rewrite outside file")
	}
}

func TestReplayWorkbenchSessionSelectionRejectsSymlinkedMarkerBeforeWriting(t *testing.T) {
	ctx := context.Background()
	root, _, session := replayWorkbenchSessionForSafetyTest(t, ctx)
	if _, err := SelectReplayWorkbenchSession(root, filepath.Base(session.Path), ReplayWorkbenchSessionSelectionOptions{SelectedBy: "alice"}); err != nil {
		t.Fatalf("select session before symlink marker: %v", err)
	}
	marker := filepath.Join(root, "replay", "workbench-session-selection.json")
	outside := replayWorkbenchReplaceWithSymlinkedCopyForSafetyTest(t, marker, "outside-session-selection.json")
	before := replayWorkbenchReadFileForSafetyTest(t, outside)

	_, err := ClearReplayWorkbenchSessionSelection(root, ReplayWorkbenchSessionSelectionClearOptions{ClearedBy: "bob"})
	if err == nil || !strings.Contains(err.Error(), "replay workbench session selection file must not be a symlink") {
		t.Fatalf("expected symlinked session selection rejection, got %v", err)
	}
	after := replayWorkbenchReadFileForSafetyTest(t, outside)
	if string(after) != string(before) {
		t.Fatalf("symlinked session selection clear must not rewrite outside file")
	}
}

func TestCreateReplayWorkbenchCollaborationRejectsSymlinkedCollaborationDirectoryBeforeWriting(t *testing.T) {
	ctx := context.Background()
	root, _, session := replayWorkbenchSessionForSafetyTest(t, ctx)
	outsideDir := filepath.Join(t.TempDir(), "outside-collaborations")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("create outside collaboration dir: %v", err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(root, "replay", "workbench-collaborations")); err != nil {
		t.Fatalf("symlink collaboration dir: %v", err)
	}

	_, err := CreateReplayWorkbenchCollaboration(ctx, root, ReplayWorkbenchCollaborationOptions{SessionPath: filepath.Base(session.Path), Participants: []string{"alice"}})
	if err == nil || !strings.Contains(err.Error(), "replay workbench collaboration directory must not be a symlink") {
		t.Fatalf("expected symlinked collaboration directory rejection, got %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(outsideDir, "*.json"))
	if err != nil {
		t.Fatalf("glob outside collaborations: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("symlinked collaboration directory must not receive artifacts, got %#v", matches)
	}
}

func TestAnnotateReplayWorkbenchCollaborationRejectsSymlinkedCollaborationFileBeforeWriting(t *testing.T) {
	ctx := context.Background()
	root, _, collaboration := replayWorkbenchCollaborationForSafetyTest(t, ctx)
	outside := replayWorkbenchReplaceWithSymlinkedCopyForSafetyTest(t, collaboration.Path, "outside-collaboration.json")
	before := replayWorkbenchReadFileForSafetyTest(t, outside)

	_, err := AnnotateReplayWorkbenchCollaboration(root, filepath.Base(collaboration.Path), ReplayWorkbenchCollaborationAnnotationOptions{Decision: "accepted", Reviewer: "alice"})
	if err == nil || !strings.Contains(err.Error(), "replay workbench collaboration file must not be a symlink") {
		t.Fatalf("expected symlinked collaboration file rejection, got %v", err)
	}
	after := replayWorkbenchReadFileForSafetyTest(t, outside)
	if string(after) != string(before) {
		t.Fatalf("symlinked collaboration annotation must not rewrite outside file")
	}
}

func TestUpdateReplayWorkbenchCollaborationPresenceRejectsSymlinkedPresenceDirectoryBeforeWriting(t *testing.T) {
	ctx := context.Background()
	root, _, collaboration := replayWorkbenchCollaborationForSafetyTest(t, ctx)
	outsideDir := filepath.Join(t.TempDir(), "outside-presence")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("create outside presence dir: %v", err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(root, "replay", "workbench-collaboration-presence")); err != nil {
		t.Fatalf("symlink presence dir: %v", err)
	}

	_, err := UpdateReplayWorkbenchCollaborationPresence(ctx, root, ReplayWorkbenchCollaborationPresenceOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Participant:       "alice",
		Status:            "online",
	})
	if err == nil || !strings.Contains(err.Error(), "replay workbench collaboration presence directory must not be a symlink") {
		t.Fatalf("expected symlinked presence directory rejection, got %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(outsideDir, "*.json"))
	if err != nil {
		t.Fatalf("glob outside presence: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("symlinked presence directory must not receive artifacts, got %#v", matches)
	}
}

func TestUpdateReplayWorkbenchCollaborationPresenceRejectsSymlinkedPresenceFileBeforeWriting(t *testing.T) {
	ctx := context.Background()
	root, _, collaboration := replayWorkbenchCollaborationForSafetyTest(t, ctx)
	presence, err := UpdateReplayWorkbenchCollaborationPresence(ctx, root, ReplayWorkbenchCollaborationPresenceOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Participant:       "alice",
		Status:            "online",
	})
	if err != nil {
		t.Fatalf("create presence before symlink: %v", err)
	}
	outside := replayWorkbenchReplaceWithSymlinkedCopyForSafetyTest(t, presence.Path, "outside-presence.json")
	before := replayWorkbenchReadFileForSafetyTest(t, outside)

	_, err = UpdateReplayWorkbenchCollaborationPresence(ctx, root, ReplayWorkbenchCollaborationPresenceOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Participant:       "alice",
		Status:            "away",
	})
	if err == nil || !strings.Contains(err.Error(), "replay workbench collaboration presence file must not be a symlink") {
		t.Fatalf("expected symlinked presence file rejection, got %v", err)
	}
	after := replayWorkbenchReadFileForSafetyTest(t, outside)
	if string(after) != string(before) {
		t.Fatalf("symlinked presence update must not rewrite outside file")
	}
}

func TestCreateReplayWorkbenchCollaborationNoteRejectsSymlinkedNoteDirectoryBeforeWriting(t *testing.T) {
	ctx := context.Background()
	root, _, collaboration := replayWorkbenchCollaborationForSafetyTest(t, ctx)
	outsideDir := filepath.Join(t.TempDir(), "outside-notes")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("create outside note dir: %v", err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(root, "replay", "workbench-collaboration-notes")); err != nil {
		t.Fatalf("symlink note dir: %v", err)
	}

	_, err := CreateReplayWorkbenchCollaborationNote(ctx, root, ReplayWorkbenchCollaborationNoteOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Author:            "alice",
		Body:              "review note",
	})
	if err == nil || !strings.Contains(err.Error(), "replay workbench collaboration note directory must not be a symlink") {
		t.Fatalf("expected symlinked note directory rejection, got %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(outsideDir, "*.json"))
	if err != nil {
		t.Fatalf("glob outside notes: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("symlinked note directory must not receive artifacts, got %#v", matches)
	}
}

func TestAnnotateReplayWorkbenchCollaborationNoteRejectsSymlinkedNoteFileBeforeWriting(t *testing.T) {
	ctx := context.Background()
	root, _, collaboration := replayWorkbenchCollaborationForSafetyTest(t, ctx)
	note, err := CreateReplayWorkbenchCollaborationNote(ctx, root, ReplayWorkbenchCollaborationNoteOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Author:            "alice",
		Body:              "review note",
	})
	if err != nil {
		t.Fatalf("create note before symlink: %v", err)
	}
	outside := replayWorkbenchReplaceWithSymlinkedCopyForSafetyTest(t, note.Path, "outside-note.json")
	before := replayWorkbenchReadFileForSafetyTest(t, outside)

	_, err = AnnotateReplayWorkbenchCollaborationNote(root, filepath.Base(note.Path), ReplayWorkbenchCollaborationNoteAnnotationOptions{Decision: "accepted", Reviewer: "alice"})
	if err == nil || !strings.Contains(err.Error(), "replay workbench collaboration note file must not be a symlink") {
		t.Fatalf("expected symlinked note file rejection, got %v", err)
	}
	after := replayWorkbenchReadFileForSafetyTest(t, outside)
	if string(after) != string(before) {
		t.Fatalf("symlinked note annotation must not rewrite outside file")
	}
}

func TestReplayWorkbenchCollaborationSelectionRejectsSymlinkedMarkerBeforeWriting(t *testing.T) {
	ctx := context.Background()
	root, _, collaboration := replayWorkbenchCollaborationForSafetyTest(t, ctx)
	if _, err := SelectReplayWorkbenchCollaboration(root, filepath.Base(collaboration.Path), ReplayWorkbenchCollaborationSelectionOptions{SelectedBy: "alice"}); err != nil {
		t.Fatalf("select collaboration before symlink marker: %v", err)
	}
	marker := filepath.Join(root, "replay", "workbench-collaboration-selection.json")
	outside := replayWorkbenchReplaceWithSymlinkedCopyForSafetyTest(t, marker, "outside-collaboration-selection.json")
	before := replayWorkbenchReadFileForSafetyTest(t, outside)

	_, err := ClearReplayWorkbenchCollaborationSelection(root, ReplayWorkbenchCollaborationSelectionClearOptions{ClearedBy: "bob"})
	if err == nil || !strings.Contains(err.Error(), "replay workbench collaboration selection file must not be a symlink") {
		t.Fatalf("expected symlinked collaboration selection rejection, got %v", err)
	}
	after := replayWorkbenchReadFileForSafetyTest(t, outside)
	if string(after) != string(before) {
		t.Fatalf("symlinked collaboration selection clear must not rewrite outside file")
	}
}

func TestReplayWorkbenchCollaborationNoteSelectionRejectsSymlinkedMarkerBeforeWriting(t *testing.T) {
	ctx := context.Background()
	root, _, collaboration := replayWorkbenchCollaborationForSafetyTest(t, ctx)
	note, err := CreateReplayWorkbenchCollaborationNote(ctx, root, ReplayWorkbenchCollaborationNoteOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Author:            "alice",
		Body:              "review note",
	})
	if err != nil {
		t.Fatalf("create note before selection symlink: %v", err)
	}
	if _, err := SelectReplayWorkbenchCollaborationNote(root, filepath.Base(note.Path), ReplayWorkbenchCollaborationNoteSelectionOptions{SelectedBy: "alice"}); err != nil {
		t.Fatalf("select note before symlink marker: %v", err)
	}
	marker := filepath.Join(root, "replay", "workbench-collaboration-note-selection.json")
	outside := replayWorkbenchReplaceWithSymlinkedCopyForSafetyTest(t, marker, "outside-note-selection.json")
	before := replayWorkbenchReadFileForSafetyTest(t, outside)

	_, err = ClearReplayWorkbenchCollaborationNoteSelection(root, ReplayWorkbenchCollaborationNoteSelectionClearOptions{ClearedBy: "bob"})
	if err == nil || !strings.Contains(err.Error(), "replay workbench collaboration note selection file must not be a symlink") {
		t.Fatalf("expected symlinked note selection rejection, got %v", err)
	}
	after := replayWorkbenchReadFileForSafetyTest(t, outside)
	if string(after) != string(before) {
		t.Fatalf("symlinked note selection clear must not rewrite outside file")
	}
}

func replayWorkbenchRunForSafetyTest(t *testing.T, ctx context.Context) (string, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	return root, run.RunID
}

func replayWorkbenchSessionForSafetyTest(t *testing.T, ctx context.Context) (string, string, ReplayWorkbenchSessionArtifact) {
	t.Helper()
	root, runID := replayWorkbenchRunForSafetyTest(t, ctx)
	session, err := CreateReplayWorkbenchSession(ctx, root, ReplayWorkbenchSessionOptions{
		PrimaryRunID: runID,
		CreatedAt:    time.Date(2026, 6, 2, 8, 30, 0, 0, time.UTC),
		Reason:       "safety fixture",
	})
	if err != nil {
		t.Fatalf("create replay workbench session fixture: %v", err)
	}
	return root, runID, session
}

func replayWorkbenchCollaborationForSafetyTest(t *testing.T, ctx context.Context) (string, string, ReplayWorkbenchCollaborationArtifact) {
	t.Helper()
	root, runID, session := replayWorkbenchSessionForSafetyTest(t, ctx)
	collaboration, err := CreateReplayWorkbenchCollaboration(ctx, root, ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		CreatedAt:    time.Date(2026, 6, 2, 8, 31, 0, 0, time.UTC),
		Reason:       "safety collaboration fixture",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration fixture: %v", err)
	}
	return root, runID, collaboration
}

func replayWorkbenchReplaceWithSymlinkedCopyForSafetyTest(t *testing.T, originalPath, outsideName string) string {
	t.Helper()
	data := replayWorkbenchReadFileForSafetyTest(t, originalPath)
	outside := filepath.Join(t.TempDir(), outsideName)
	if err := os.WriteFile(outside, data, 0o644); err != nil {
		t.Fatalf("write outside symlink target: %v", err)
	}
	if err := os.Remove(originalPath); err != nil {
		t.Fatalf("remove original before symlink: %v", err)
	}
	if err := os.Symlink(outside, originalPath); err != nil {
		t.Fatalf("symlink original to outside target: %v", err)
	}
	return outside
}

func replayWorkbenchReadFileForSafetyTest(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func TestReplayWorkspaceRejectsMissingRun(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := RunWorkspace(ctx, root); err != nil {
		t.Fatalf("run workspace: %v", err)
	}

	_, err := ReplayWorkspace(ctx, root, "missing-run")
	if err == nil {
		t.Fatalf("expected missing run to fail")
	}
	if !strings.Contains(err.Error(), "missing-run") {
		t.Fatalf("expected missing run id in error, got %v", err)
	}
}

func TestReplayWorkspaceDoesNotInventReportPathForFailedRun(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  targets:
    - kind: url
      value: https://app.example
  exclusions: []
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - sleep
  - "1"
run_timeout_ms: 1
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	observedEvents := []eventlog.Event{}
	_, err := RunWorkspaceWithOptions(ctx, root, RunOptions{
		OnEvent: func(event eventlog.Event) error {
			observedEvents = append(observedEvents, event)
			return nil
		},
	})
	if err == nil {
		t.Fatalf("expected failed run")
	}
	if len(observedEvents) == 0 {
		t.Fatalf("expected failed run to persist events")
	}

	replay, err := ReplayWorkspace(ctx, root, observedEvents[0].RunID)
	if err != nil {
		t.Fatalf("replay failed run: %v", err)
	}
	if replay.Status.ToolHealth != "failed" {
		t.Fatalf("expected failed replay tool health, got %#v", replay.Status)
	}
	if replay.ReportPath != "" {
		t.Fatalf("failed run must not invent report path, got %q", replay.ReportPath)
	}
}
