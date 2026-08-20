package api

import "strings"

// unifiedLineDiff 生成适合 IDE 展示的轻量行级 diff。
// 不使用外部依赖，输出带 +/- 标记的完整行变更，便于 AI 动作结果直接回显。
func unifiedLineDiff(path, oldText, newText string) string {
	oldLines := splitDiffLines(oldText)
	newLines := splitDiffLines(newText)
	if len(oldLines) == 0 && len(newLines) == 0 {
		return ""
	}
	if strings.Join(oldLines, "\n") == strings.Join(newLines, "\n") {
		return ""
	}
	maxLines := 600
	if len(oldLines) > maxLines || len(newLines) > maxLines {
		return "diff 内容过大，已省略；请直接查看文件当前内容。"
	}

	// LCS 回溯生成逐行操作。
	n, m := len(oldLines), len(newLines)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	var ops []string
	i, j := 0, 0
	for i < n && j < m {
		if oldLines[i] == newLines[j] {
			ops = append(ops, "  "+oldLines[i])
			i++
			j++
		} else if dp[i+1][j] >= dp[i][j+1] {
			ops = append(ops, "- "+oldLines[i])
			i++
		} else {
			ops = append(ops, "+ "+newLines[j])
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, "- "+oldLines[i])
	}
	for ; j < m; j++ {
		ops = append(ops, "+ "+newLines[j])
	}

	var builder strings.Builder
	builder.WriteString("--- a/")
	builder.WriteString(path)
	builder.WriteString("\n+++ b/")
	builder.WriteString(path)
	builder.WriteString("\n")
	for _, op := range ops {
		builder.WriteString(op)
		builder.WriteString("\n")
	}
	return builder.String()
}

func splitDiffLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
