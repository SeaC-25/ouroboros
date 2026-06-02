package tui

import "fmt"

const (
	MinWorkspaceWidth  = 20
	MinWorkbenchWidth  = 34
	MinGraphStackWidth = 26
	StatusBarHeight    = 1
	MinContentHeight   = 8
	MinTotalWidth      = MinWorkspaceWidth + MinWorkbenchWidth + MinGraphStackWidth
	MinTotalHeight     = MinContentHeight + StatusBarHeight
)

type Rect struct {
	X      int
	Y      int
	Width  int
	Height int
}

type Layout struct {
	Width           int
	Height          int
	ContentHeight   int
	StatusBarHeight int
	Workspace       Rect
	Workbench       Rect
	GraphStack      Rect
	Ready           bool
	Message         string
}

func NewLayout(width int, height int) Layout {
	layout := Layout{
		Width:           width,
		Height:          height,
		StatusBarHeight: StatusBarHeight,
	}
	if width < MinTotalWidth {
		layout.Message = fmt.Sprintf("terminal too small: need at least %dx%d", MinTotalWidth, MinTotalHeight)
		return layout
	}
	if height < MinTotalHeight {
		layout.Message = fmt.Sprintf("terminal too small: need at least %dx%d", MinTotalWidth, MinTotalHeight)
		return layout
	}

	workspaceWidth := maxInt(MinWorkspaceWidth, width*22/100)
	graphWidth := maxInt(MinGraphStackWidth, width*30/100)
	workbenchWidth := width - workspaceWidth - graphWidth

	if workbenchWidth < MinWorkbenchWidth {
		deficit := MinWorkbenchWidth - workbenchWidth
		takeFromGraph := minInt(deficit, graphWidth-MinGraphStackWidth)
		graphWidth -= takeFromGraph
		deficit -= takeFromGraph
		takeFromWorkspace := minInt(deficit, workspaceWidth-MinWorkspaceWidth)
		workspaceWidth -= takeFromWorkspace
		deficit -= takeFromWorkspace
		workbenchWidth = width - workspaceWidth - graphWidth
	}

	layout.ContentHeight = height - StatusBarHeight
	layout.Workspace = Rect{X: 0, Y: 0, Width: workspaceWidth, Height: layout.ContentHeight}
	layout.Workbench = Rect{X: workspaceWidth, Y: 0, Width: workbenchWidth, Height: layout.ContentHeight}
	layout.GraphStack = Rect{X: workspaceWidth + workbenchWidth, Y: 0, Width: graphWidth, Height: layout.ContentHeight}
	layout.Ready = true
	return layout
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
