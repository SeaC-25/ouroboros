package api

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

func ExportReport(ctx context.Context, workspace string, store *eventlog.SQLiteStore) (ReportExportDTO, error) {
	graph, err := ProjectEvidenceGraph(ctx, workspace, store)
	if err != nil {
		return ReportExportDTO{}, err
	}
	findings, err := ProjectReportFindings(workspace, graph)
	if err != nil {
		return ReportExportDTO{}, err
	}
	reports, err := ProjectReports(workspace)
	if err != nil {
		return ReportExportDTO{}, err
	}
	workspaceDTO := ProjectWorkspace(workspace)
	exportedAt := time.Now().Unix()
	content := renderReportMarkdown(workspaceDTO, reports, findings, exportedAt)

	relativePath := filepath.Join("reports", "ouroboros-report.md")
	absolutePath := filepath.Join(workspace, relativePath)
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
		return ReportExportDTO{}, err
	}
	if err := os.WriteFile(absolutePath, []byte(content), 0o644); err != nil {
		return ReportExportDTO{}, err
	}
	info, err := os.Stat(absolutePath)
	if err != nil {
		return ReportExportDTO{}, err
	}
	return ReportExportDTO{
		FilePath:     filepath.ToSlash(relativePath),
		FindingCount: len(findings),
		ReportCount:  len(reports),
		ModifiedAt:   info.ModTime().Unix(),
	}, nil
}

func renderReportMarkdown(workspace WorkspaceDTO, reports []ReportDTO, findings []ReportFindingDTO, exportedAt int64) string {
	var builder strings.Builder
	builder.WriteString("# Ouroboros 渗透测试报告\n\n")
	writeReportField(&builder, "工作区", workspace.Name)
	writeReportField(&builder, "工作区路径", workspace.Path)
	writeReportField(&builder, "操作者", workspace.Operator)
	writeReportField(&builder, "导出时间", time.Unix(exportedAt, 0).Format(time.RFC3339))
	builder.WriteString("\n## 已有报告文件\n\n")
	if len(reports) == 0 {
		builder.WriteString("- 暂无已有报告文件\n")
	} else {
		for _, report := range reports {
			builder.WriteString(fmt.Sprintf("- %s `%s` (%s)\n", sanitizeMarkdownLine(report.Title), report.Path, report.Status))
		}
	}
	builder.WriteString("\n## 发现\n\n")
	if len(findings) == 0 {
		builder.WriteString("暂无后端投影出的发现。\n")
		return builder.String()
	}
	for index, finding := range findings {
		builder.WriteString(fmt.Sprintf("### %d. %s\n\n", index+1, sanitizeMarkdownLine(finding.Title)))
		writeReportField(&builder, "严重级别", finding.Severity)
		writeReportField(&builder, "目标", finding.Endpoint)
		writeReportField(&builder, "来源", finding.Source)
		writeReportField(&builder, "证据文件", finding.FilePath)
		writeReportField(&builder, "图谱节点", finding.NodeID)
		writeReportField(&builder, "执行 ID", finding.ExecutionID)
		writeReportField(&builder, "回放事件 ID", finding.ReplayEventID)
		builder.WriteString("\n")
	}
	return builder.String()
}

func writeReportField(builder *strings.Builder, label string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	builder.WriteString(fmt.Sprintf("- **%s:** %s\n", label, sanitizeMarkdownLine(value)))
}

func sanitizeMarkdownLine(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}
