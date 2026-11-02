package api

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ouroboros/ouroboros/internal/nativetools"
)

// maxDistilledObservation caps the distilled observation summary handed to
// the model (L3 red line: routine decisions must not drag kilobytes of raw
// observation into the context). The full raw observation stays in
// runtime/observations.jsonl for deep-dive via ACTION:READ.
const maxDistilledObservation = 600

// maxDistilledBodyPreview caps the body excerpt inside the summary.
const maxDistilledBodyPreview = 120

// distillObservationForAI condenses a raw ObserveResult JSON into a compact
// structured summary (≤ maxDistilledObservation bytes):
//
//	status=200 t=85ms len=4.2KB server=nginx | diff: status 200→403,+Set-Cookie/-X-Powered | forms:1(/login:user,pass) links:12 keys:token,user | <body first 120 bytes>
//
// Diff takes priority when a baseline diff is present; otherwise the
// first-sight fingerprint (server / x_powered_by) is shown. It never fails:
// parse errors fall back to the first 400 bytes of the raw string
// (fail-open, never blocks the loop).
func distillObservationForAI(rawJSON string) string {
	rawJSON = strings.TrimSpace(rawJSON)
	if rawJSON == "" {
		return ""
	}
	var obs nativetools.ObserveResult
	if err := json.Unmarshal([]byte(rawJSON), &obs); err != nil {
		// Not an ObserveResult (legacy text, error output) — fail open.
		return truncateForAI(rawJSON, 400)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "status=%d t=%.0fms len=%s", obs.Status, obs.ElapsedMS, humanBytes(obs.BodySize))
	if server := firstToken(obs.Server); server != "" {
		fmt.Fprintf(&b, " server=%s", server)
	}
	if xpb := firstToken(obs.XPoweredBy); xpb != "" {
		fmt.Fprintf(&b, " xpb=%s", xpb)
	}

	// Baseline diff has priority over the first-sight fingerprint.
	if obs.Diff != nil {
		b.WriteString(" | diff:")
		var parts []string
		if obs.Diff.StatusChanged {
			parts = append(parts, fmt.Sprintf("status %d→%d", obs.Diff.ControlStatus, obs.Status))
		}
		if obs.Diff.SizeDelta != 0 {
			parts = append(parts, fmt.Sprintf("len %+d", obs.Diff.SizeDelta))
		}
		if obs.Diff.BodyChanged {
			parts = append(parts, "body~")
		}
		for _, header := range obs.Diff.HeaderAdded {
			parts = append(parts, "+"+header)
		}
		for _, header := range obs.Diff.HeaderRemoved {
			parts = append(parts, "-"+header)
		}
		if len(parts) == 0 {
			parts = append(parts, "no-change")
		}
		b.WriteString(strings.Join(parts, ","))
	}

	// Structured observations.
	if len(obs.Forms) > 0 {
		b.WriteString(" | forms:")
		for i, form := range obs.Forms {
			if i > 0 {
				b.WriteString(";")
			}
			fmt.Fprintf(&b, "%s(%s)", form.Action, strings.Join(form.Fields, ","))
			// Hard stop once the summary is full; later forms are in
			// observations.jsonl.
			if b.Len() >= maxDistilledObservation {
				break
			}
		}
	}
	if len(obs.Links) > 0 {
		fmt.Fprintf(&b, " links:%d", len(obs.Links))
	}
	if len(obs.JSONKeys) > 0 {
		fmt.Fprintf(&b, " keys:%s", strings.Join(obs.JSONKeys, ","))
	}

	// Body excerpt: first 120 bytes, single line.
	if preview := strings.TrimSpace(obs.BodyPreview); preview != "" {
		b.WriteString(" | " + firstNBytes(preview, maxDistilledBodyPreview))
	}

	out := b.String()
	if len(out) > maxDistilledObservation {
		out = firstNBytes(out, maxDistilledObservation)
	}
	return out
}

// firstToken returns the first whitespace-free token of value (Server /
// X-Powered-By echoes often carry version noise after a space).
func firstToken(value string) string {
	if idx := strings.IndexAny(value, " \t"); idx >= 0 {
		return value[:idx]
	}
	return value
}

// firstNBytes cuts value to at most n bytes. A byte cut may clip a trailing
// multibyte rune — acceptable for a lossy AI-facing summary.
func firstNBytes(value string, n int) string {
	if len(value) <= n {
		return value
	}
	return value[:n]
}

// humanBytes formats n as a compact size string ("853B", "4.2KB", "3.1MB").
func humanBytes(n int) string {
	switch {
	case n <= 0:
		return "0B"
	case n < 1024:
		return fmt.Sprintf("%dB", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1fKB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(n)/(1024*1024))
	}
}
