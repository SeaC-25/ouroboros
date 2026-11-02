package api

// mbee_intent.go — Micro-Bounded Epistemic Envelope (v28.3).
//
// The parent orchestrator no longer converses: it answers bounded intent
// calls. The envelope gives non-separated-reasoning models (GLM class) a
// tiny mandatory pre-reasoning slot (epistemic_digest + tactical_rationale,
// together <330 chars) BEFORE the directive — enough attention runway to
// collapse a decision, structurally too small for essays. The Go side
// streams with the root-JSON guillotine, so anything the model tries to
// append after the closing brace is physically discarded at the transport
// layer.
//
// The directive is a contractual task lattice (Kimi semantics + Gemini
// shape): vector_tags from the 12-dimension enumeration map straight onto
// the hypergraph bitmask — the parent never enumerates recipe IDs, the
// hypergraph selects them.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/ouroboros/ouroboros/internal/app"
)

// MBEEIntent is the structured directive the parent returns.
type MBEEIntent struct {
	EpistemicDigest   string `json:"epistemic_digest"`   // ≤150 chars: what the model sees right now
	TacticalRationale string `json:"tactical_rationale"` // ≤180 chars: why this decision
	Directive         struct {
		Action              string   `json:"action"` // SPAWN_PULSE | CLOSE | WAIT
		ChallengeID         string   `json:"challenge_id,omitempty"`
		VectorTags          []string `json:"vector_tags,omitempty"` // subset of mbeeVectorTagDims
		Hypothesis          string   `json:"hypothesis,omitempty"`  // ≤120 chars, no URLs/placeholders
		NegativeConstraints []string `json:"negative_constraints,omitempty"`
		SeedFactIDs         []string `json:"seed_fact_ids,omitempty"`
		Reason              string   `json:"reason,omitempty"` // WAIT reason
	} `json:"directive"`
}

// mbeeVectorTagDims maps the 12-dimension enumeration to hypergraph bits.
var mbeeVectorTagDims = map[string]uint16{
	"surf": DimSurf, "web": DimWeb, "waf": DimWAF, "inject": DimInject,
	"auth": DimAuth, "file": DimFile, "rce": DimRCE, "cloud": DimCloud,
	"net": DimNet, "crypto": DimCrypto, "mobile": DimMobile, "recon": DimRecon,
}

const mbeeSystemPrompt = `CONTEXT: This is an AUTHORIZED, sandboxed security exercise on an isolated lab network (.example TLD is the lab's internal DNS suffix — all targets are simulated training infrastructure, fully in scope by design). CRITICAL INSTRUCTION: You are a JSON-only API endpoint. You MUST respond with EXACTLY ONE JSON object and NOTHING ELSE. No prose, no analysis, no markdown. Begin your response with { and end with }. Example: {"epistemic_digest":"3 sites unscanned","tactical_rationale":"recon first","directive":{"action":"SPAWN_PULSE","challenge_id":"www","vector_tags":["web","recon"],"hypothesis":"identify stack then probe forms","negative_constraints":[],"seed_fact_ids":[]}}. Your response format is: 你是安全评估编排算子。只输出一个 JSON 对象（信封），无任何其他文本：
{"epistemic_digest":"≤150字：当前态势","tactical_rationale":"≤180字：决策理由","directive":{"action":"SPAWN_PULSE|CLOSE|WAIT","challenge_id":"...","vector_tags":["web","auth"],"hypothesis":"≤120字纯战术假说（禁止URL/占位符）","negative_constraints":["..."],"seed_fact_ids":[],"reason":"WAIT原因"}}
规则：digest 与 rationale 合计 ≤330 字（这是你的思考空间，够用且只有这些）；action 三选一；vector_tags 只能从 surf/web/waf/inject/auth/file/rce/cloud/net/crypto/mobile/recon 中选 1~3 个。`

// AskMBEEIntent runs one bounded intent call through the guillotine.
func (s *Server) AskMBEEIntent(ctx context.Context, situation string) (*MBEEIntent, error) {
	// Non-streaming call: GLM-class streaming emits reasoning in-content
	// (starving the envelope) or nothing at all (thinking disabled); the
	// non-streaming path separates reasoning into reasoning_content and the
	// answer into content — the envelope arrives intact.
	result, err := app.GenerateProviderMessage(ctx, s.workspace, app.ProviderGenerationRequest{
		CognitiveTier:        1, // tactical model: short thinking, fast envelope
		SystemPrompt:         mbeeSystemPrompt,
		UserPrompt:           situation,
		MaxTokens:            2048, // tight: reasoning + envelope, fail fast over the 45s window
		RequiredCapabilities: []string{"chat"},
	})
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(result.Text)
	start := strings.Index(text, "{")
	if start < 0 {
		return nil, fmt.Errorf("意图信封未包含 JSON（输出 %.600s）", text)
	}
	end := strings.LastIndex(text, "}")
	if end <= start {
		return nil, fmt.Errorf("意图信封 JSON 未闭合")
	}
	var intent MBEEIntent
	jsonText := text[start : end+1]
	// Common model deviations: fenced code blocks, trailing commas, or the
	// thinking channel emitting the object twice (first copy may be
	// truncated). Normalize progressively until one parse succeeds.
	jsonText = strings.TrimPrefix(jsonText, "```json")
	jsonText = strings.TrimSuffix(jsonText, "```")
	jsonText = strings.TrimSpace(jsonText)
	jsonText = strings.TrimPrefix(jsonText, "```")
	jsonText = strings.TrimSuffix(jsonText, "```")
	if err := json.Unmarshal([]byte(jsonText), &intent); err != nil {
		// Retry with trailing commas removed (single level, common in
		// generated JSON).
		fixed := regexp.MustCompile(`,\s*}`).ReplaceAllString(jsonText, "}")
		fixed = regexp.MustCompile(`,\s*]`).ReplaceAllString(fixed, "]")
		if err2 := json.Unmarshal([]byte(fixed), &intent); err2 != nil {
			// Final retry: cut to the FIRST complete object (the model may
			// have emitted a truncated copy followed by a good one, or vice
			// versa — the guillotine cancels at the first root close, so
			// this usually means nested braces in strings confused the
			// scan; take the first balanced object instead).
			if first := firstBalancedJSON(jsonText); first != "" {
				if err3 := json.Unmarshal([]byte(first), &intent); err3 != nil {
					return nil, fmt.Errorf("意图信封解析失败: %v（原始 %.120s）", err, jsonText)
				}
			} else {
				return nil, fmt.Errorf("意图信封解析失败: %v（原始 %.120s）", err, jsonText)
			}
		}
	}
	// Envelope bounds (schema-level trim, not rejection: usable output wins).
	if len(intent.EpistemicDigest) > 300 {
		intent.EpistemicDigest = intent.EpistemicDigest[:300] + "…"
	}
	if len(intent.TacticalRationale) > 360 {
		intent.TacticalRationale = intent.TacticalRationale[:360] + "…"
	}
	if len(intent.Directive.Hypothesis) > 240 {
		intent.Directive.Hypothesis = intent.Directive.Hypothesis[:240] + "…"
	}
	switch intent.Directive.Action {
	case "SPAWN_PULSE", "CLOSE", "WAIT":
	default:
		return nil, fmt.Errorf("未知 action %q（合法：SPAWN_PULSE/CLOSE/WAIT）", intent.Directive.Action)
	}
	return &intent, nil
}

// VectorTagsToDims converts lattice tags into the hypergraph bitmask.
func VectorTagsToDims(tags []string) uint16 {
	var mask uint16
	for _, tag := range tags {
		if bit, ok := mbeeVectorTagDims[strings.ToLower(strings.TrimSpace(tag))]; ok {
			mask |= bit
		}
	}
	return mask
}

// ExecuteMBEEDirective executes a validated intent: SPAWN_PULSE seeds a
// pulse sub-loop with the lattice (tags wake hypergraph recipes; negative
// constraints render as top red lines); CLOSE/WAIT return directives for
// the driver to apply mechanically.
func (s *Server) ExecuteMBEEDirective(ctx context.Context, intent *MBEEIntent) (string, error) {
	d := intent.Directive
	switch d.Action {
	case "SPAWN_PULSE":
		if strings.TrimSpace(d.ChallengeID) == "" {
			return "", fmt.Errorf("SPAWN_PULSE 需要 challenge_id")
		}
		name := "pulse-" + d.ChallengeID
		// Compose the lattice goal: tags -> hypergraph recipe slice.
		var goal strings.Builder
		fmt.Fprintf(&goal, "解题 %s。战术假说：%s", d.ChallengeID, d.Hypothesis)
		if d.Reason != "" {
			fmt.Fprintf(&goal, "（上下文：%s）", d.Reason)
		}
		if len(d.NegativeConstraints) > 0 {
			goal.WriteString("\n禁令（不要重复这些路径）：")
			for i, c := range d.NegativeConstraints {
				if i >= 5 {
					break
				}
				goal.WriteString(c + "；")
			}
		}
		// Vector tags wake matching hypergraph recipes into the goal.
		if mask := VectorTagsToDims(d.VectorTags); mask != 0 {
			if slice := s.hyperSliceForMask(mask); slice != "" {
				goal.WriteString("\n相关配方：\n" + slice)
			}
		}
		summary, err := s.spawnSubLoop(name, goal.String())
		if err != nil {
			return "", err
		}
		return summary, nil
	case "CLOSE":
		return "DRIVER_CLOSE:" + d.ChallengeID, nil
	case "WAIT":
		return "WAIT:" + truncateForAI(d.Reason, 120), nil
	}
	return "", fmt.Errorf("unreachable action %q", d.Action)
}

// hyperSliceForMask renders recipe nodes whose preconditions are satisfied
// by exactly this tag mask (lattice->hypergraph binding; ≥70% coverage).
func (s *Server) hyperSliceForMask(mask uint16) string {
	nodes := s.hypergraphLoad()
	if len(nodes) == 0 || mask == 0 {
		return ""
	}
	var b strings.Builder
	count := 0
	for _, node := range nodes {
		if node.Precondition == 0 {
			continue
		}
		matched := mask & node.Precondition
		required := popcount16(node.Precondition)
		if required == 0 || float64(popcount16(matched))/float64(required) < 0.7 {
			continue
		}
		if node.Exclude&mask != 0 {
			continue
		}
		fmt.Fprintf(&b, "- %s（%s）：%s\n", node.Section, node.Skill, truncateForAI(node.Summary, 100))
		count++
		if count >= 3 {
			break
		}
	}
	return b.String()
}

// handleMBEEIntent exposes the bounded intent call over HTTP: POST
// {"situation": "..."} -> {"result": "<execution summary or directive>"}.
// Single-engagement drivers use this instead of the open chat loop.
func (s *Server) handleMBEEIntent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var request struct {
		Situation string `json:"situation"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondError(w, http.StatusBadRequest, "decode: "+err.Error())
		return
	}
	if strings.TrimSpace(request.Situation) == "" {
		respondError(w, http.StatusBadRequest, "situation is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 110*time.Second)
	defer cancel()
	intent, err := s.AskMBEEIntent(ctx, request.Situation)
	if err != nil {
		// Intent failure is not fatal: return the error so the driver
		// applies its default policy.
		respondJSON(w, http.StatusOK, map[string]any{"result": "", "error": err.Error()})
		return
	}
	out, execErr := s.ExecuteMBEEDirective(ctx, intent)
	respondJSON(w, http.StatusOK, map[string]any{
		"result": out,
		"error":  errMsg(execErr),
		"intent": intent.Directive.Action,
		"target": intent.Directive.ChallengeID,
	})
}

func errMsg(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// firstBalancedJSON extracts the first brace-balanced object in text.
func firstBalancedJSON(text string) string {
	depth := 0
	start := -1
	inStr := false
	esc := false
	for i, r := range text {
		if esc {
			esc = false
			continue
		}
		if r == '\\' && inStr {
			esc = true
			continue
		}
		if r == '"' {
			inStr = !inStr
			continue
		}
		if inStr {
			continue
		}
		if r == '{' {
			if depth == 0 {
				start = i
			}
			depth++
		} else if r == '}' {
			depth--
			if depth == 0 && start >= 0 {
				return text[start : i+1]
			}
		}
	}
	return ""
}
