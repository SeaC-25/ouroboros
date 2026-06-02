package report

import (
	"strings"
)

func RenderMarkdown(report Report) string {
	var builder strings.Builder
	builder.WriteString("# Ouroboros Report\n\n")
	writeFindings(&builder, report.VerifiedFindings)
	writePaths(&builder, report.Paths)
	writeEvidenceRefs(&builder, report.EvidenceRefs)
	writeStrings(&builder, "## Exhausted Areas", report.ExhaustedAreas)
	writeStrings(&builder, "## Next-Run Suggestions", report.NextRunSuggestions)
	return strings.TrimRight(builder.String(), "\n") + "\n"
}

func writeFindings(builder *strings.Builder, findings []Finding) {
	builder.WriteString("## Verified Findings\n\n")
	if len(findings) == 0 {
		builder.WriteString("- None\n\n")
		return
	}
	for _, finding := range findings {
		builder.WriteString("- ")
		builder.WriteString(finding.Title)
		if finding.ID != "" {
			builder.WriteString(" (`")
			builder.WriteString(finding.ID)
			builder.WriteString("`)")
		}
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
}

func writePaths(builder *strings.Builder, paths []Path) {
	builder.WriteString("## Attack Paths\n\n")
	if len(paths) == 0 {
		builder.WriteString("- None\n\n")
		return
	}
	for _, path := range paths {
		builder.WriteString("- ")
		builder.WriteString(strings.Join(path.Steps, " -> "))
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
}

func writeEvidenceRefs(builder *strings.Builder, refs []EvidenceRef) {
	builder.WriteString("## Evidence References\n\n")
	if len(refs) == 0 {
		builder.WriteString("- None\n\n")
		return
	}
	for _, ref := range refs {
		builder.WriteString("- ")
		builder.WriteString(ref.Ref)
		if ref.ID != "" {
			builder.WriteString(" (`")
			builder.WriteString(ref.ID)
			builder.WriteString("`)")
		}
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
}

func writeStrings(builder *strings.Builder, title string, values []string) {
	builder.WriteString(title)
	builder.WriteString("\n\n")
	if len(values) == 0 {
		builder.WriteString("- None\n\n")
		return
	}
	for _, value := range values {
		builder.WriteString("- ")
		builder.WriteString(value)
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
}
