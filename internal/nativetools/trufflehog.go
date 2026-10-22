package nativetools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// trufflehogFinding mirrors the trufflehog JSONL record shape
// (DetectorName / Raw / SourceMetadata).
type trufflehogFinding struct {
	DetectorName   string `json:"DetectorName"`
	Raw            string `json:"Raw"`
	SourceMetadata struct {
		Data struct {
			Filesystem struct {
				File string `json:"file"`
				Line int    `json:"line"`
			} `json:"Filesystem"`
		} `json:"Data"`
	} `json:"SourceMetadata"`
}

// ScanSecretsTrufflehog reuses the built-in detectors but emits trufflehog's
// JSONL schema when the plan node's parser is trufflehog_json. Online
// verification (--verified) is out of scope: findings are reported unverified,
// which is exactly what trufflehog emits without network access.
func ScanSecretsTrufflehog(ctx context.Context, target string, opts Options) (string, error) {
	paths, err := collectScanPaths(strings.TrimSpace(target))
	if err != nil {
		return "", err
	}

	builder := strings.Builder{}
	count := 0
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		leaks, err := scanFile(path)
		if err != nil {
			continue
		}
		for _, leak := range leaks {
			if count >= 512 {
				break
			}
			finding := trufflehogFinding{DetectorName: leak.RuleID, Raw: leak.Secret}
			finding.SourceMetadata.Data.Filesystem.File = path
			finding.SourceMetadata.Data.Filesystem.Line = leak.StartLine
			encoded, err := json.Marshal(finding)
			if err != nil {
				return "", fmt.Errorf("encode trufflehog finding: %w", err)
			}
			builder.Write(encoded)
			builder.WriteByte('\n')
			count++
		}
	}
	return strings.TrimRight(builder.String(), "\n"), nil
}
