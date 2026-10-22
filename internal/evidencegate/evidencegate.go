// Package evidencegate implements the GateMesh Evidence gate
// (doc/gatemesh-architecture-v1.md §L2): findings stay E2_Tentative until an
// independent replay reproduces the signal (E3_Verified). Downstream attack
// chains may only consume E3.
//
// Ported from KaliMCP kali_mcp/core/verifier.py with one GateMesh addition:
// differential verification — a positive and a negative sample request must
// produce a statistically distinguishable response before promotion (kills
// false-positive cascades from regex/status heuristics).
package evidencegate

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// EvidenceLevel is the promotion ladder.
type EvidenceLevel string

const (
	// E0Raw is un-parsed tool output on disk.
	E0Raw EvidenceLevel = "E0_RAW"
	// E1Parsed is a parser-extracted signal (not yet trusted).
	E1Parsed EvidenceLevel = "E1_PARSED"
	// E2Tentative is a candidate finding awaiting independent replay.
	E2Tentative EvidenceLevel = "E2_TENTATIVE"
	// E3Verified passed differential replay; downstream may consume.
	E3Verified EvidenceLevel = "E3_VERIFIED"
	// EFalsePositive failed replay or differential distinction.
	EFalsePositive EvidenceLevel = "FALSE_POSITIVE"
)

// Reproducer abstracts "run a command / request and return output".
// The executor integration (native tools, guarded subprocess) satisfies this.
type Reproducer interface {
	// Reproduce runs the minimal replay for a finding and returns the
	// combined stdout+stderr text.
	Reproduce(ctx context.Context, reproduceCmd string) (string, error)
}

// Finding is the ledger entry for one candidate.
type Finding struct {
	ID             string
	Target         string
	ReproduceCmd   string
	ExpectedSignal string // literal, "re:<regex>" or "status:<code>"
	Level          EvidenceLevel
}

// Signal reports whether the replay output matches the expected signal.
// Port of verifier._match_signal.
func Signal(output, expectedSignal string) bool {
	signal := strings.TrimSpace(expectedSignal)
	if signal == "" {
		return false
	}
	body := output
	if strings.HasPrefix(signal, "re:") {
		pattern := signal[3:]
		re, err := regexp.Compile("(?im)" + pattern)
		if err != nil {
			return strings.Contains(body, pattern)
		}
		return re.MatchString(body)
	}
	if strings.HasPrefix(signal, "status:") {
		return strings.Contains(body, strings.TrimSpace(strings.SplitN(signal, ":", 2)[1]))
	}
	return strings.Contains(strings.ToLower(body), strings.ToLower(signal))
}

// PromoteDifferential runs the two-phase promotion:
//  1. positive sample — the finding's reproduce command must match its signal
//  2. negative sample — a baseline command (caller-supplied; e.g. the same
//     request with the sentinel input) must NOT match the signal
//
// Only when positive matches and negative does not is the finding promoted
// to E3Verified. Positive failure demotes to EFalsePositive. A missing
// negative command degrades to positive-only verification (legacy behavior,
// recorded in the result).
type PromotionResult struct {
	FindingID   string
	From        EvidenceLevel
	To          EvidenceLevel
	PositiveHit bool
	NegativeHit bool
	Note        string
}

func PromoteDifferential(ctx context.Context, f Finding, repro Reproducer, negativeCmd string) PromotionResult {
	res := PromotionResult{FindingID: f.ID, From: f.Level, To: f.Level}

	if strings.TrimSpace(f.ReproduceCmd) == "" {
		res.To = E2Tentative // blocked; stays tentative
		res.Note = "missing reproduce command"
		return res
	}
	if repro == nil {
		res.Note = "no reproducer"
		return res
	}

	out, err := repro.Reproduce(ctx, f.ReproduceCmd)
	if err != nil {
		out = out + "\n" + err.Error()
	}
	res.PositiveHit = Signal(out, f.ExpectedSignal)

	if !res.PositiveHit {
		res.To = EFalsePositive
		res.Note = "positive sample failed to reproduce signal"
		return res
	}

	if strings.TrimSpace(negativeCmd) == "" {
		res.To = E3Verified
		res.Note = "promoted (positive-only; no negative sample supplied)"
		return res
	}

	negOut, err := repro.Reproduce(ctx, negativeCmd)
	if err != nil {
		negOut = negOut + "\n" + err.Error()
	}
	res.NegativeHit = Signal(negOut, f.ExpectedSignal)
	if res.NegativeHit {
		// The signal fires on the baseline too — not a real finding.
		res.To = EFalsePositive
		res.Note = "signal also matches negative sample (baseline indistinguishable)"
		return res
	}
	res.To = E3Verified
	res.Note = "promoted (differential: positive hit, negative clean)"
	return res
}

// String makes levels log-friendly.
func (l EvidenceLevel) String() string { return string(l) }

// VerifyError is returned by strict promotion helpers.
type VerifyError struct {
	FindingID string
	Reason    string
}

func (e *VerifyError) Error() string {
	return fmt.Sprintf("evidence gate: finding %s: %s", e.FindingID, e.Reason)
}
