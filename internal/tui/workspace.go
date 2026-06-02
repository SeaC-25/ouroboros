package tui

import (
	"path"
	"sort"
	"strings"
)

type WorkspaceEntryKind string

const (
	WorkspaceEntryFile      WorkspaceEntryKind = "file"
	WorkspaceEntryDirectory WorkspaceEntryKind = "directory"
)

type WorkspaceEntry struct {
	Name     string
	Path     string
	Kind     WorkspaceEntryKind
	Children []WorkspaceEntry
}

type WorkspacePanel struct {
	Entries      []WorkspaceEntry
	SelectedPath string
}

func DefaultWorkspacePanel() WorkspacePanel {
	return WorkspacePanel{
		Entries: []WorkspaceEntry{
			{Name: "mission.yaml", Kind: WorkspaceEntryFile},
			{Name: "scope.yaml", Kind: WorkspaceEntryFile},
			{Name: "artifacts", Kind: WorkspaceEntryDirectory},
			{Name: "evidence", Kind: WorkspaceEntryDirectory},
			{Name: "attempts", Kind: WorkspaceEntryDirectory},
			{Name: "findings", Kind: WorkspaceEntryDirectory},
			{Name: "reports", Kind: WorkspaceEntryDirectory},
			{Name: "replay", Kind: WorkspaceEntryDirectory},
			{Name: "toolchain", Kind: WorkspaceEntryDirectory},
		},
	}
}

func (p WorkspacePanel) Select(selectedPath string) WorkspacePanel {
	p.SelectedPath = normalizeWorkspacePath(selectedPath)
	return p
}

func (p WorkspacePanel) SelectedEntry() (WorkspaceEntry, bool) {
	selected := normalizeWorkspacePath(p.SelectedPath)
	if selected == "" {
		return WorkspaceEntry{}, false
	}
	for _, entry := range p.sortedEntries(p.Entries, "") {
		if entry.path == selected {
			return entry.entry, true
		}
	}
	return WorkspaceEntry{}, false
}

func (p WorkspacePanel) Render(width int, height int) string {
	return renderPanel("Workspace", p.Rows(), width, height)
}

func (p WorkspacePanel) Rows() []string {
	entries := p.sortedEntries(p.Entries, "")
	rows := make([]string, 0, len(entries))
	selected := normalizeWorkspacePath(p.SelectedPath)
	for _, item := range entries {
		marker := "  "
		if selected != "" && item.path == selected {
			marker = "> "
		}
		rows = append(rows, marker+strings.Repeat("  ", item.depth)+displayWorkspaceEntry(item.entry))
	}
	return rows
}

type workspaceTreeItem struct {
	entry WorkspaceEntry
	path  string
	depth int
}

func (p WorkspacePanel) sortedEntries(entries []WorkspaceEntry, parentPath string) []workspaceTreeItem {
	entries = append([]WorkspaceEntry(nil), entries...)
	sort.SliceStable(entries, func(i, j int) bool {
		leftPriority := workspacePriority(entries[i])
		rightPriority := workspacePriority(entries[j])
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		return displayWorkspaceEntry(entries[i]) < displayWorkspaceEntry(entries[j])
	})

	items := []workspaceTreeItem{}
	for _, entry := range entries {
		entryPath := workspaceEntryPath(entry, parentPath)
		items = append(items, workspaceTreeItem{
			entry: entry,
			path:  entryPath,
		})
		children := p.sortedEntries(entry.Children, entryPath)
		for _, child := range children {
			child.depth++
			items = append(items, child)
		}
	}
	return items
}

func workspacePriority(entry WorkspaceEntry) int {
	if priority, ok := canonicalWorkspaceOrder[normalizeWorkspacePath(workspaceEntryPath(entry, ""))]; ok {
		return priority
	}
	return len(canonicalWorkspaceOrder) + 1
}

var canonicalWorkspaceOrder = map[string]int{
	"mission.yaml": 0,
	"scope.yaml":   1,
	"artifacts":    2,
	"evidence":     3,
	"attempts":     4,
	"findings":     5,
	"reports":      6,
	"replay":       7,
	"toolchain":    8,
}

func workspaceEntryPath(entry WorkspaceEntry, parentPath string) string {
	entryPath := normalizeWorkspacePath(entry.Path)
	if entryPath == "" {
		entryPath = normalizeWorkspacePath(entry.Name)
	}
	if parentPath == "" || strings.Contains(entryPath, "/") {
		return entryPath
	}
	return normalizeWorkspacePath(path.Join(parentPath, entryPath))
}

func displayWorkspaceEntry(entry WorkspaceEntry) string {
	name := strings.TrimSpace(entry.Name)
	if name == "" {
		name = normalizeWorkspacePath(entry.Path)
	}
	if entry.Kind == WorkspaceEntryDirectory && !strings.HasSuffix(name, "/") {
		name += "/"
	}
	return name
}

func normalizeWorkspacePath(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" {
		return ""
	}
	value = strings.TrimPrefix(value, "./")
	value = strings.Trim(value, "/")
	if value == "." {
		return ""
	}
	return path.Clean(value)
}
