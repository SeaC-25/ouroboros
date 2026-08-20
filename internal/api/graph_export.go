package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

type graphExportFile struct {
	ExportedAt int64            `json:"exportedAt"`
	Graph      EvidenceGraphDTO `json:"graph"`
	Nodes      []GraphNodeDTO   `json:"nodes"`
	Edges      []GraphEdgeDTO   `json:"edges"`
}

func ExportEvidenceGraph(ctx context.Context, workspace string, store *eventlog.SQLiteStore) (GraphExportDTO, error) {
	graph, err := ProjectEvidenceGraph(ctx, workspace, store)
	if err != nil {
		return GraphExportDTO{}, err
	}
	exportedAt := time.Now().Unix()
	payload := graphExportFile{
		ExportedAt: exportedAt,
		Graph:      graph,
		Nodes:      graph.Nodes,
		Edges:      graph.Edges,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return GraphExportDTO{}, err
	}

	relativePath := filepath.Join("exports", "attack-graph.json")
	absolutePath := filepath.Join(workspace, relativePath)
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
		return GraphExportDTO{}, err
	}
	if err := os.WriteFile(absolutePath, append(data, '\n'), 0o644); err != nil {
		return GraphExportDTO{}, err
	}
	info, err := os.Stat(absolutePath)
	if err != nil {
		return GraphExportDTO{}, err
	}
	return GraphExportDTO{
		FilePath:   filepath.ToSlash(relativePath),
		NodeCount:  len(graph.Nodes),
		EdgeCount:  len(graph.Edges),
		ModifiedAt: info.ModTime().Unix(),
	}, nil
}
