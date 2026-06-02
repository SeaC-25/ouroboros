package tui

import (
	"strings"
	"testing"
)

func TestDefaultWorkspacePanelRendersRequiredEntriesInCanonicalOrder(t *testing.T) {
	panel := DefaultWorkspacePanel()

	view := panel.Render(28, 12)
	expectedOrder := []string{
		"mission.yaml",
		"scope.yaml",
		"artifacts/",
		"evidence/",
		"attempts/",
		"findings/",
		"reports/",
		"replay/",
		"toolchain/",
	}

	previous := -1
	for _, name := range expectedOrder {
		index := strings.Index(view, name)
		if index < 0 {
			t.Fatalf("workspace view missing %q:\n%s", name, view)
		}
		if index <= previous {
			t.Fatalf("workspace entry %q rendered out of order:\n%s", name, view)
		}
		previous = index
	}
}

func TestWorkspacePanelSortsUnknownEntriesAfterCanonicalEntries(t *testing.T) {
	panel := WorkspacePanel{
		Entries: []WorkspaceEntry{
			{Name: "zz-extra", Kind: WorkspaceEntryDirectory},
			{Name: "scope.yaml", Kind: WorkspaceEntryFile},
			{Name: "artifacts", Kind: WorkspaceEntryDirectory},
			{Name: "mission.yaml", Kind: WorkspaceEntryFile},
			{Name: "aa-extra.md", Kind: WorkspaceEntryFile},
		},
	}

	rows := panel.Rows()
	if rows[0] != "  mission.yaml" || rows[1] != "  scope.yaml" || rows[2] != "  artifacts/" {
		t.Fatalf("canonical entries not sorted first: %#v", rows)
	}
	if rows[len(rows)-2] != "  aa-extra.md" || rows[len(rows)-1] != "  zz-extra/" {
		t.Fatalf("unknown entries not sorted by display name: %#v", rows)
	}
}

func TestWorkspacePanelTracksSelectedPath(t *testing.T) {
	panel := DefaultWorkspacePanel().Select("evidence")

	selected, ok := panel.SelectedEntry()
	if !ok {
		t.Fatalf("expected selected entry")
	}
	if selected.Name != "evidence" || selected.Kind != WorkspaceEntryDirectory {
		t.Fatalf("unexpected selected entry: %#v", selected)
	}

	view := panel.Render(28, 12)
	if !strings.Contains(view, "> evidence/") {
		t.Fatalf("expected selected evidence row:\n%s", view)
	}
	if strings.Contains(view, "> reports/") {
		t.Fatalf("unexpected selected reports row:\n%s", view)
	}
}
