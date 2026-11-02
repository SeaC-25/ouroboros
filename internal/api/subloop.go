package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ouroboros/ouroboros/internal/app"
)

// Self-organizing parallel observation-reasoning loops. The model decides
// when a task decomposes into independent sub-investigations and spawns one
// loop per branch; the backend drives each sub-loop autonomously (no
// external turn-driver needed) with its own context window, budget, and
// evidence funnel into the shared blackboard.

const (
	// subLoopTurnLimit caps a single solve sub-loop's model calls.
	subLoopTurnLimit = 24
	// v28.2 micro-pulses: the 40-turn budget is sliced into 12-turn pulses.
	// At each pulse boundary the loop emits a falsification receipt, the
	// polluted dialogue history is destroyed, and a fresh pulse forks with
	// (board seeds + accumulated negative constraints + one-line prior
	// result). Two sterile pulses hibernate the challenge.
	pulseTurnLimit   = 12
	maxSterilePulses = 2
	// subLoopTurnTimeout bounds one model call inside a sub-loop.
	subLoopTurnTimeout = 240 * time.Second
	// subLoopHardBudget bounds the whole solve sub-loop wall-clock.
	subLoopHardBudget = 40 * time.Minute
	// Recon micro-loops answer ONE question fast: small turn budget, short
	// wall clock. They are the pipeline's scouting stage — the parent
	// spawns them for single facts (tech stack, quota, one endpoint's
	// shape) and acts immediately on the returned conclusion instead of
	// investigating serially in its own context.
	reconSubLoopTurnLimit  = 5
	reconSubLoopHardBudget = 6 * time.Minute
)

type subLoopState struct {
	Name      string
	Goal      string
	Status    string // running | done | failed | cancelled
	Turns     int
	LastReply string
	// Progress is the model-authored rolling digest (PROGRESS: lines).
	Progress  string
	StartedAt time.Time
	EndedAt   time.Time
	Error     string
	// GateMesh L2 progress-gate counters: PivotCount counts forced pivots
	// injected by the IGR stagnation gate (capped at 2); DoneRejected marks
	// the one-time rejection of a premature DONE (goal unreached, turns
	// remaining, no new flag fact on the blackboard).
	PivotCount   int
	DoneRejected bool
	// v28.2 pulse tracking: Pulse counts completed pulses; Sterile counts
	// consecutive pulses producing zero new board facts (hibernate at 2);
	// LastResult is the structured subloop-result.v1; NegativeConstraints
	// accumulate "don't retry X" lines across pulses.
	Pulse               int
	Sterile             int
	LastResult          json.RawMessage
	NegativeConstraints []string
	// BaseTier is the loop's default cognitive tier (1=tactical flash,
	// 2=general/flagship). Gates/pivots still pin tier 2 per-turn.
	BaseTier int `json:"baseTier,omitempty"`
	// MaxTurns caps THIS loop's turn budget (0 = global default). Micro-
	// tasks carry 5-8; full-solve loops use the global limit.
	MaxTurns int `json:"maxTurns,omitempty"`
}

type subLoopRegistry struct {
	mu    sync.Mutex
	loops map[string]*subLoopState
}

var (
	subLoopsMu      sync.Mutex
	globalSubLoops  = &subLoopRegistry{loops: map[string]*subLoopState{}}
	subLoopConveyor = make(chan string, 64) // completed loop names -> parent digest
)

// maxConcurrentSubLoops bounds parallelism. Default 2 (plus the parent loop
// = 3 active investigations, matching the platform's 3-container cap);
// operators can raise it via OURO_MAX_SUB_LOOPS.
func maxConcurrentSubLoops() int {
	if value := strings.TrimSpace(getEnvDefault("OURO_MAX_SUB_LOOPS", "2")); value != "" {
		if parsed, ok := atoiSafe(value); ok && parsed > 0 && parsed <= 16 {
			return parsed
		}
	}
	return 2
}

// spawnSubLoop starts an autonomous sub-loop for an independent goal.
// spawnSubLoopTiered spawns with a base cognitive tier (general-assault).
func (s *Server) spawnSubLoopTiered(name string, goal string, tier int, turns int) (string, string, error) {
	summary, err := s.spawnSubLoop(name, goal)
	if err != nil {
		return summary, name, err
	}
	subLoopsMu.Lock()
	if st, ok := globalSubLoops.loops[name]; ok {
		st.BaseTier = tier
		if turns > 0 {
			st.MaxTurns = turns
		}
	}
	subLoopsMu.Unlock()
	s.persistSubLoop(globalSubLoops.loops[name])
	return summary, name, nil
}

func (s *Server) spawnSubLoop(name string, goal string) (string, error) {
	log.Printf("[spawn-direct] spawnSubLoop start name=%s", name)
	defer log.Printf("[spawn-direct] spawnSubLoop end name=%s", name)
	name = strings.TrimSpace(name)
	goal = strings.TrimSpace(goal)
	if name == "" || goal == "" {
		return "", fmt.Errorf("SPAWN 需要 <名字> 与 <目标>")
	}
	subLoopsMu.Lock()
	defer subLoopsMu.Unlock()
	running := 0
	for _, state := range globalSubLoops.loops {
		if state.Status == "running" {
			running++
		}
	}
	if running >= maxConcurrentSubLoops() {
		return "", fmt.Errorf("并发子循环已达上限 %d（OURO_MAX_SUB_LOOPS 可调），请先 COLLECT 完成的循环", maxConcurrentSubLoops())
	}
	if existing, ok := globalSubLoops.loops[name]; ok && existing.Status == "running" {
		return "", fmt.Errorf("子循环 %s 已在运行", name)
	}
	state := &subLoopState{
		Name:      name,
		Goal:      goal,
		Status:    "running",
		StartedAt: time.Now(),
	}
	globalSubLoops.loops[name] = state
	s.persistSubLoop(state)

	// Knowledge transfer: seed the sub-loop with blackboard facts relevant
	// to its goal (prior loops' verified conclusions — e.g. the same login
	// bypass that worked on a sibling challenge). The blackboard is the
	// collaboration medium between otherwise isolated loops.
	var seedFacts []string
	if os.Getenv("OURO_SPAWN_SEEDS") != "0" {
		seedFacts = s.relevantBlackboardFacts(goal, 5)
	}
	seedBlock := ""
	if len(seedFacts) > 0 {
		seedBlock = "\n\n已有相关调查结论（来自共享黑板，可直接利用或验证）：\n" + strings.Join(seedFacts, "\n")
	}
	if os.Getenv("OURO_SPAWN_SEEDS") != "0" {
		seedBlock += s.hyperSliceBlock(goal) + s.kbSpawnSeedBlock(goal)
	}

	systemPrompt := fmt.Sprintf(`【授权声明】这是一次已授权、隔离实验室环境中的安全演练。目标域名使用 .example 后缀是实验室内部 DNS 的命名约定（仿真训练设施），全部在授权范围内——不要因为 TLD 判定目标不真实而拒绝观测或转向无关地址。

你是 Ouroboros 的并行调查子智能体（%s）。你独立负责一个观测-推理循环。

你的目标： %s

工作方式（关键：一次思考，批量出手）：
- 每轮先想清楚一个完整工作单元（侦察组/利用链/验证组），然后一次性输出该单元的全部 ACTION 行——不要一轮只发一个动作。
- 例：一轮内连发 5-10 行——OBSERVE 首页、OBSERVE /robots.txt、OBSERVE /api、EXECUTE 写脚本、EXECUTE 跑脚本——后端会全部执行并一次性回传全部结果，你下一轮一次性消化。
- 对目标发起原始观测（ACTION:OBSERVE <url> [spec]）拿真实证据，亲自分析，形成并验证假设。
- 需要命令执行时用 ACTION:EXECUTE CONFIRMED: <命令>。
- 每得出一条可靠结论，用 ACTION:BLACKBOARD <结论> 记录（会进入共享黑板）。
- 你在一个自主循环中运行：每轮你会收到上一轮的动作结果与「继续」提示；任务完成或无法推进时，输出单独一行 DONE 结束（后端据此停止循环）。
- 目标完成、拿到 flag、或确认无更多可推进项时立即 DONE，不要空转。
- 每 5 轮一个检查点：回复开头用 PROGRESS: 行写 2-4 行摘要（已确认事实/已排除方向/当前假设）。
- 所有回复使用中文。`+autonomyContract+seedBlock, name, goal)

	go s.runSubLoop(state, systemPrompt, goal)
	return fmt.Sprintf("子循环 %s 已启动（目标: %.80s；黑板种子 %d 条）", name, goal, len(seedFacts)), nil
}

// relevantBlackboardFacts selects prior conclusions sharing meaningful
// tokens with the goal (host fragments, technique words). Simple lexical
// overlap is deliberate: the model, not the matcher, decides relevance.
func (s *Server) relevantBlackboardFacts(goal string, limit int) []string {
	facts := s.loadBlackboardFacts()
	if len(facts) == 0 {
		return nil
	}
	goalTokens := tokenizeForRelevance(goal)
	if len(goalTokens) == 0 {
		return nil
	}
	type scored struct {
		fact  string
		score int
	}
	var candidates []scored
	for _, fact := range facts {
		statement := strings.TrimSpace(fact.Statement)
		if statement == "" {
			continue
		}
		tokens := tokenizeForRelevance(statement)
		overlap := 0
		for token := range tokens {
			if goalTokens[token] {
				overlap++
			}
		}
		if overlap >= 2 {
			candidates = append(candidates, scored{fact: statement, score: overlap})
		}
	}
	// Best-overlap first, cap at limit.
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && candidates[j].score > candidates[j-1].score; j-- {
			candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
		}
	}
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, "- "+truncateForAI(c.fact, 240))
	}
	return out
}

// tokenizeForRelevance extracts CJK bigrams and latin words for overlap
// scoring (CJK has no spaces; bigrams catch shared vocabulary).
func tokenizeForRelevance(text string) map[string]bool {
	tokens := map[string]bool{}
	runes := []rune(strings.ToLower(text))
	for i := 0; i < len(runes); i++ {
		if isLatinWordRune(runes[i]) {
			start := i
			for i < len(runes) && isLatinWordRune(runes[i]) {
				i++
			}
			word := string(runes[start:i])
			if len(word) >= 3 {
				tokens[word] = true
			}
			continue
		}
		if i+1 < len(runes) {
			bigram := string(runes[i : i+2])
			tokens[bigram] = true
		}
	}
	return tokens
}

func isLatinWordRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' || r == ':'
}

// progressGatePrompt builds the forced-pivot instruction injected at the
// head of the next turn's prompt when the IGR gate trips: {stall}
// consecutive turns passed without a single new blackboard conclusion.
// It forces orthogonal re-orientation instead of same-class retries.
func progressGatePrompt(stall int) string {
	return fmt.Sprintf("\n【Progress 门】连续 %d 轮无新结论（IGR=0）。强制掉头：①列出已排除手法 ②换正交象限（换工具类型/换攻击面/换编码——不要同类重试）③回答“缺什么前置信息才能突破”并转化为侦察动作。\n", stall)
}

// doneRejectionPrompt is injected when a premature DONE is rejected: the
// goal is unreached, turns remain, and no flag fact landed on the
// blackboard since the loop started.
func doneRejectionPrompt() string {
	return "终止被驳回：目标未达成且轮次未尽。回答：缺什么前置条件？转化为 2 个侦察动作继续。完成时再输出 DONE。"
}

// hasNewFlagFactSince reports whether a flag-shaped fact was recorded on
// the blackboard at or after sinceUnix (loop start): the terminal-progress
// signal that lets a DONE through the rejection gate. Non-flag conclusions
// prove activity but not goal completion.
func (s *Server) hasNewFlagFactSince(sinceUnix int64) bool {
	for _, fact := range s.loadBlackboardFacts() {
		if strings.Contains(fact.Statement, "flag{") && fact.CreatedAt >= sinceUnix {
			return true
		}
	}
	return false
}

// runSubLoop drives one autonomous sub-loop: model call -> action execution
// -> "continue" nudge, until DONE / turn limit / budget / context cancel.
func (s *Server) runSubLoop(state *subLoopState, systemPrompt string, goal string) {
	ctx, cancel := context.WithTimeout(context.Background(), subLoopHardBudget)
	defer cancel()

	deadline := time.Now().Add(subLoopHardBudget)
	userPrompt := fmt.Sprintf("目标：%s\n开始你的观测-推理循环。每 5 轮有一个检查点，回复开头用 PROGRESS: 行写 2-4 行摘要（已确认事实/已排除方向/当前假设）。", goal)
	// Rolling progress digest: the sub-loop's own written summary, kept by
	// the backend and injected into every subsequent turn. Without it the
	// loop reasoned from a one-turn memory (turn N knew nothing about what
	// turns 1..N-1 already ruled out). The digest is model-authored: at
	// every checkpoint turn the reply must start with "PROGRESS:" lines.
	// IGR (Incremental Gain Rate) stagnation gate: track blackboard
	// conclusion deltas per turn. Zero new conclusions for 5 consecutive
	// turns triggers one forced-pivot injection (up to 2 pivots per loop).
	progressDigest := ""
	lastFactCount := len(s.loadBlackboardFacts())
	stall := 0
	// Cognitive tier (Budget 门): routine sub-loop turns run on the L1
	// tactical model (tier=1); the turn carrying a Progress-gate pivot or a
	// DONE-rejection prompt pins the L2 flagship (tier=2). flagshipTurn is
	// set at the same place the forced prompt replaces userPrompt and
	// consumed exactly once by the next model call.
	flagshipTurn := false
	// v28.2 pulse loop: 12-turn pulses with clean forking between them.
	// Each pulse starts from a FRESH context (board seeds + accumulated
	// negative constraints + one-line prior pulse result) — the polluted
	// dialogue history of the previous pulse is destroyed, which removes the
	// anchoring bias that made in-place pivot prompts ineffective.
	pulseFactsAtStart := len(s.loadBlackboardFacts())
	providerFails := 0
	turnBudget := subLoopTurnLimit
	if state.MaxTurns > 0 && state.MaxTurns < subLoopTurnLimit {
		turnBudget = state.MaxTurns
	}
	for turn := 1; turn <= turnBudget; turn++ {
		if ctx.Err() != nil {
			state.Status = "cancelled"
			state.Error = "budget exhausted"
			break
		}
		if time.Now().After(deadline) {
			state.Status = "cancelled"
			state.Error = "deadline exceeded"
			break
		}
		// Pulse boundary: at every 12th turn (or the final budget turn)
		// that did not DONE, generate a falsification receipt, record it,
		// and fork a clean context for the next pulse.
		if turn%pulseTurnLimit == 0 && turn < turnBudget && state.Status == "running" {
			receipt := s.generateFalsificationReceipt(state, ctx)
			s.recordReceiptToBoard(state, receipt)
			factsNow := len(s.loadBlackboardFacts())
			sterile := factsNow <= pulseFactsAtStart
			pulseFactsAtStart = factsNow
			subLoopsMu.Lock()
			state.Pulse++
			if sterile {
				state.Sterile++
			} else {
				state.Sterile = 0
			}
			pulse := state.Pulse
			sterileCount := state.Sterile
			subLoopsMu.Unlock()
			s.persistSubLoop(state)
			if sterileCount >= maxSterilePulses {
				state.Status = "hibernated"
				break
			}
			// Clean Fork: fresh prompt with seeds + constraints + receipt.
			userPrompt = s.freshPulsePrompt(state, receipt, pulse+1)
			progressDigest = ""
			stall = 0
			lastFactCount = factsNow
			flagshipTurn = false
			continue
		}
		callCtx, callCancel := context.WithTimeout(ctx, subLoopTurnTimeout)
		// Tier consumption: flagshipTurn is exactly true when this turn's
		// userPrompt was replaced by a gate injection (see below).
		cognitiveTier := state.BaseTier
		if state.BaseTier == 0 {
			cognitiveTier = 1
		}
		if flagshipTurn {
			cognitiveTier = 2
		}
		result, err := app.GenerateProviderMessage(callCtx, s.workspace, app.ProviderGenerationRequest{
			SystemPrompt: systemPrompt,
			UserPrompt:   userPrompt,
			// Solve loops need full output space: long payloads, multi-action
			// turns, complete reasoning chains. 64K matches the frontier
			// context class; per-turn caps belong to the driver plumbing.
			// Flash-tier turns: 8K caps generation latency (64K let the
			// model ramble for 40+s per turn); reasoning fits comfortably,
			// payloads stream via multiple ACTION turns anyway.
			MaxTokens:            8192,
			RequiredCapabilities: []string{"chat"},
			Stream:               false,
			CognitiveTier:        cognitiveTier,
		})
		callCancel()
		subLoopsMu.Lock()
		state.Turns = turn
		subLoopsMu.Unlock()
		if err != nil {
			// Transient provider errors: bounded retry (was unbounded —
			// loops burned all 40 turns silently on provider failure and
			// surfaced as running-with-empty-replies).
			state.Error = err.Error()
			if providerFails < 6 {
				providerFails++
				time.Sleep(time.Duration(providerFails) * 5 * time.Second)
				continue
			}
			state.Status = "failed"
			break
		}
		if strings.TrimSpace(result.Text) == "" {
			// Empty reply (thinking consumed the budget): treat as a
			// provider failure too, not a productive turn.
			state.Error = "empty provider reply"
			if providerFails < 6 {
				providerFails++
				time.Sleep(time.Duration(providerFails) * 5 * time.Second)
				continue
			}
			state.Status = "failed"
			break
		}
		providerFails = 0
		reply := strings.TrimSpace(result.Text)
		subLoopsMu.Lock()
		state.LastReply = reply
		subLoopsMu.Unlock()

		// Rolling digest: capture the model's own PROGRESS: lines so later
		// turns (and the parent, via COLLECT) keep the investigation state.
		if digest := extractProgressDigest(reply); digest != "" {
			progressDigest = digest
			subLoopsMu.Lock()
			state.Progress = digest
			subLoopsMu.Unlock()
		}

		// Execute the sub-loop's action lines FIRST: a completion reply
		// often carries its final actions (submit/close) alongside DONE —
		// breaking before executing them would swallow the flag submission.
		actionOutput, actionRefs := s.executeSubLoopActions(reply)
		_ = actionRefs
		if strings.Contains(reply, "\nDONE") || strings.HasPrefix(reply, "DONE") {
			// DONE rejection gate (GateMesh L2): reject the first premature
			// DONE while the goal is unreached — turns remain and no flag
			// fact landed on the blackboard since the loop started.
			if !state.DoneRejected && turn < turnBudget && !s.hasNewFlagFactSince(state.StartedAt.Unix()) {
				subLoopsMu.Lock()
				state.DoneRejected = true
				subLoopsMu.Unlock()
				userPrompt = doneRejectionPrompt()
				flagshipTurn = true // L2 flagship: terminal-decision turn
				s.persistSubLoop(state)
				continue
			}
			state.Status = "done"
			break
		}
		nextPrompt := fmt.Sprintf("本轮全部动作结果：\n%s\n\n继续（第 %d/%d 轮）。下一轮同样先规划一个完整工作单元再批量出手（多行 ACTION 一起发）。完成时单独一行 DONE。", truncateForAI(actionOutput, 8000), turn+1, subLoopTurnLimit)
		// Backend digest fallback: if the model skipped its PROGRESS:
		// lines at a checkpoint (english replies, drifted focus), synthesize
		// the digest mechanically from blackboard deltas + recent actions so
		// later turns never reason from one-turn memory.
		if turn%5 == 0 && progressDigest == "" {
			progressDigest = s.mechanicalSubLoopDigest(state, actionOutput)
			subLoopsMu.Lock()
			if strings.TrimSpace(state.Progress) == "" {
				state.Progress = progressDigest
			}
			subLoopsMu.Unlock()
		}
		if progressDigest != "" {
			nextPrompt = "你此前写下的进展摘要：\n" + progressDigest + "\n\n" + nextPrompt
		}
		if turn%5 == 0 {
			nextPrompt += "\n（本轮为检查点：回复开头先写 2-4 行 PROGRESS: 摘要——已确认事实/已排除方向/当前假设，然后照常给动作。）"
		}
		userPrompt = nextPrompt
		// IGR stagnation gate: zero new blackboard conclusions this turn
		// increments the stall counter; a new conclusion resets it. At
		// stall>=5 with pivots remaining, the next turn's prompt head
		// carries a forced-pivot instruction.
		factCount := len(s.loadBlackboardFacts())
		if factCount > lastFactCount {
			stall = 0
		} else {
			stall++
		}
		lastFactCount = factCount
		if stall >= 5 && state.PivotCount < 2 {
			subLoopsMu.Lock()
			state.PivotCount++
			subLoopsMu.Unlock()
			userPrompt = progressGatePrompt(stall) + s.hyperSliceBlock(state.Goal+" "+state.LastReply) + s.kbStuckRecipeBlock(state.Goal) + userPrompt
			flagshipTurn = true // L2 flagship: forced-pivot turn
			s.persistSubLoop(state)
		}
		// Consume the one-shot flagship marker: only the turn immediately
		// following a gate injection runs on tier 2; subsequent routine
		// turns return to the tactical model.
		flagshipTurn = false
	}
	if state.Status == "running" {
		state.Status = "done"
	}
	state.EndedAt = time.Now()
	// subloop-result.v1: the machine-readable outcome (driver reads status/
	// flag/failure_code; LastReply stays for audit only).
	state.LastResult = s.buildSubloopResultV1(state)
	// persist final state (registry survives restarts)
	s.persistSubLoop(state)
	// Notify the parent conversation: the orchestrator sees this digest on
	// its next turn (sub-loop finished silently otherwise).
	digest := fmt.Sprintf("[子循环 %s 完成] 状态=%s 轮数=%d 用时=%s\n进展摘要：%s\n最后回复（截断）：%s",
		state.Name, state.Status, state.Turns,
		state.EndedAt.Sub(state.StartedAt).Round(time.Second),
		truncateForAI(strings.ReplaceAll(state.Progress, "\n", " | "), 600),
		truncateForAI(strings.ReplaceAll(state.LastReply, "\n", " | "), 800))
	if state.Status == "hibernated" {
		digest += "\n（该题连续脉冲零产出已休眠——close 释放槽位或换题。）"
	}
	if state.Turns >= turnBudget && state.Status != "failed" {
		digest += "\n（该子循环耗尽全部轮次仍未完成——考虑止损：close 该题释放名额换更简单的题，或带着已有结论换角度重新 SPAWN，不要原地重复。）"
	}
	s.appendAIConversation(ChatMessage{
		ID:        fmt.Sprintf("subloop-%s-%d", state.Name, state.EndedAt.Unix()),
		Role:      "assistant",
		Content:   digest,
		Timestamp: state.EndedAt.Unix(),
	})
	select {
	case subLoopConveyor <- state.Name:
	default:
	}
}

// mechanicalSubLoopDigest synthesizes a progress digest without model
// cooperation: blackboard facts that appeared since this loop started,
// plus the last action summary. It is intentionally mechanical — the model
// remains the reasoner; this only guarantees the loop never forgets.
func (s *Server) mechanicalSubLoopDigest(state *subLoopState, lastActions string) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("[轮 %d 自动摘要] 已落黑板的结论：", state.Turns))
	facts := s.loadBlackboardFacts()
	count := 0
	for i := len(facts) - 1; i >= 0 && count < 5; i-- {
		fact := facts[i]
		if fact.CreatedAt >= state.StartedAt.Unix() {
			builder.WriteString("\n- " + truncateForAI(strings.TrimSpace(fact.Statement), 160))
			count++
		}
	}
	if count == 0 {
		builder.WriteString("（尚无——把已验证结论用 ACTION:BLACKBOARD 落盘）")
	}
	compact := strings.Join(strings.Fields(strings.ReplaceAll(lastActions, "\n", " ")), " ")
	builder.WriteString("\n最近动作：" + truncateForAI(compact, 240))
	return builder.String()
}

// splitSpawnSpec parses "<name> : <goal>".
func splitSpawnSpec(spec string) (string, string, bool) {
	spec = strings.TrimSpace(spec)
	idx := strings.Index(spec, ":")
	if idx <= 0 {
		return "", "", false
	}
	name := strings.TrimSpace(spec[:idx])
	goal := strings.TrimSpace(spec[idx+1:])
	if name == "" || goal == "" {
		return "", "", false
	}
	return name, goal, true
}

// extractProgressDigest pulls the model-authored PROGRESS: lines out of a
// reply (checkpoint turns start with them). Returns "" when absent.
func extractProgressDigest(reply string) string {
	var lines []string
	for _, line := range strings.Split(reply, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "PROGRESS:") {
			lines = append(lines, strings.TrimSpace(strings.TrimPrefix(trimmed, "PROGRESS:")))
		}
	}
	return strings.Join(lines, "\n")
}

func (s *Server) executeSubLoopActions(reply string) (string, []string) {
	var output strings.Builder
	refs, results := s.parseAndExecuteAIActions(reply)
	for _, result := range results {
		summary := result.Summary
		if result.Output != "" {
			summary += "\n" + truncateForAI(distillObservationForAI(result.Output), 800)
		}
		output.WriteString(fmt.Sprintf("[%s] %s\n\n", result.Kind, summary))
	}
	return output.String(), refs
}

// collectSubLoops reports the state of all sub-loops (digest form).
func (s *Server) collectSubLoops(name string) (string, error) {
	s.loadPersistedSubLoops()
	subLoopsMu.Lock()
	defer subLoopsMu.Unlock()
	if len(globalSubLoops.loops) == 0 {
		return "当前没有子循环。用 ACTION:SPAWN <名字> : <目标> 启动并行调查。", nil
	}
	var builder strings.Builder
	for _, state := range globalSubLoops.loops {
		if name != "" && state.Name != name {
			continue
		}
		status := state.Status
		if status == "running" {
			status = fmt.Sprintf("running（%d 轮，已运行 %s）", state.Turns, time.Since(state.StartedAt).Round(time.Second))
		}
		builder.WriteString(fmt.Sprintf("- %s [%s] 目标: %.100s\n", state.Name, status, state.Goal))
		if state.Progress != "" {
			builder.WriteString(fmt.Sprintf("  进展摘要: %.500s\n", strings.ReplaceAll(strings.TrimSpace(state.Progress), "\n", " | ")))
		}
		if state.LastReply != "" {
			builder.WriteString(fmt.Sprintf("  最后回复: %.400s\n", strings.ReplaceAll(strings.TrimSpace(state.LastReply), "\n", " | ")))
		}
		if state.Error != "" {
			builder.WriteString(fmt.Sprintf("  错误: %.200s\n", state.Error))
		}
	}
	if builder.Len() == 0 {
		return fmt.Sprintf("没有名为 %q 的子循环。", name), nil
	}
	return builder.String(), nil
}

// getEnvDefault reads an env var with fallback (no package caching).
func getEnvDefault(name string, fallback string) string {
	if value := strings.TrimSpace(osGetenv(name)); value != "" {
		return value
	}
	return fallback
}

func atoiSafe(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	negative := false
	if strings.HasPrefix(value, "-") {
		negative = true
		value = value[1:]
	}
	if value == "" {
		return 0, false
	}
	result := 0
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return 0, false
		}
		result = result*10 + int(ch-'0')
		if result > 1<<30 {
			return 0, false
		}
	}
	if negative {
		result = -result
	}
	return result, true
}

// handleSubLoops exposes sub-loop states as an observation endpoint: any
// driver, frontend, or the model itself can read spawn/collect facts the
// same way it reads any other evidence (architecture: state is observable,
// never narrated).
func (s *Server) handleSubLoops(w http.ResponseWriter, r *http.Request) {
	subLoopsMu.Lock()
	states := make([]map[string]any, 0, len(globalSubLoops.loops))
	for _, state := range globalSubLoops.loops {
		states = append(states, map[string]any{
			"name":      state.Name,
			"goal":      state.Goal,
			"status":    state.Status,
			"turns":     state.Turns,
			"startedAt": state.StartedAt.Unix(),
			"endedAt":   state.EndedAt.Unix(),
			"error":     state.Error,
			"lastReply": truncateForAI(state.LastReply, 1000),
		})
	}
	subLoopsMu.Unlock()
	respondJSON(w, http.StatusOK, map[string]any{
		"maxConcurrent": maxConcurrentSubLoops(),
		"subloops":      states,
	})
}

// subLoopRegistryPath is the durable registry journal (one line per state
// transition, last-write-wins per name on load). Containers restart; the
// orchestrator's view of what has been tried must not.
func (s *Server) subLoopRegistryPath() string {
	return filepath.Join(s.workspace, "runtime", "subloops.jsonl")
}

// persistSubLoop appends the current state as one journal line.
func (s *Server) persistSubLoop(state *subLoopState) {
	if s == nil || s.workspace == "" {
		return
	}
	// DEADLOCK FIX: callers (spawnSubLoop) already hold subLoopsMu —
	// re-locking here wedged EVERY spawn after the first (mutex is not
	// reentrant). Snapshot without the lock: the caller's critical section
	// already guarantees exclusivity for this state.
	snapshot := *state
	line, err := json.Marshal(map[string]any{
		"name":         snapshot.Name,
		"goal":         snapshot.Goal,
		"status":       snapshot.Status,
		"turns":        snapshot.Turns,
		"progress":     snapshot.Progress,
		"error":        snapshot.Error,
		"pivotCount":   snapshot.PivotCount,
		"doneRejected": snapshot.DoneRejected,
		"startedAt":    snapshot.StartedAt.Unix(),
		"endedAt":      snapshot.EndedAt.Unix(),
	})
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(s.subLoopRegistryPath()), 0o755)
	file, err := os.OpenFile(s.subLoopRegistryPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.Write(append(line, '\n'))
}

// loadPersistedSubLoops merges the durable journal into the in-memory
// registry (last state per name). Non-running historical entries let the
// parent COLLECT past attempts after a restart instead of re-spawning the
// same failed investigation.
func (s *Server) loadPersistedSubLoops() {
	if s == nil || s.workspace == "" {
		return
	}
	data, err := os.ReadFile(s.subLoopRegistryPath())
	if err != nil {
		return
	}
	type journalEntry struct {
		Name         string `json:"name"`
		Goal         string `json:"goal"`
		Status       string `json:"status"`
		Turns        int    `json:"turns"`
		Progress     string `json:"progress"`
		Error        string `json:"error"`
		PivotCount   int    `json:"pivotCount"`
		DoneRejected bool   `json:"doneRejected"`
		StartedAt    int64  `json:"startedAt"`
		EndedAt      int64  `json:"endedAt"`
	}
	latest := map[string]journalEntry{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry journalEntry
		if json.Unmarshal([]byte(line), &entry) != nil || entry.Name == "" {
			continue
		}
		latest[entry.Name] = entry
	}
	subLoopsMu.Lock()
	defer subLoopsMu.Unlock()
	for name, entry := range latest {
		if _, exists := globalSubLoops.loops[name]; exists {
			continue // live state wins over journal
		}
		state := &subLoopState{
			Name:         entry.Name,
			Goal:         entry.Goal,
			Status:       entry.Status,
			Turns:        entry.Turns,
			Progress:     entry.Progress,
			Error:        entry.Error,
			PivotCount:   entry.PivotCount,
			DoneRejected: entry.DoneRejected,
		}
		if entry.StartedAt > 0 {
			state.StartedAt = time.Unix(entry.StartedAt, 0)
		}
		if entry.EndedAt > 0 {
			state.EndedAt = time.Unix(entry.EndedAt, 0)
		}
		// A journaled "running" loop is dead after restart; mark it so the
		// parent sees the truth and can decide (respawn / close).
		if state.Status == "running" {
			state.Status = "cancelled"
			state.Error = "backend restart interrupted this loop"
		}
		globalSubLoops.loops[name] = state
	}
}

// generateFalsificationReceipt asks the model (flagship tier) for a
// structured receipt at a pulse boundary. Falls back to a mechanical
// receipt from blackboard deltas when the model call fails.
type pulseReceipt struct {
	PulseOutcome          string   `json:"pulse_outcome"`
	FalsifiedVector       string   `json:"falsified_vector"`
	BlockingEvidenceIDs   []string `json:"blocking_evidence_ids"`
	NewDiscoveredSurfaces []string `json:"new_discovered_surfaces"`
	NegativeConstraint    string   `json:"negative_constraint"`
	CandidateFlag         string   `json:"candidate_flag"`
}

func (s *Server) generateFalsificationReceipt(state *subLoopState, ctx context.Context) pulseReceipt {
	callCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	result, err := app.GenerateProviderMessage(callCtx, s.workspace, app.ProviderGenerationRequest{
		SystemPrompt: "你是脉冲收据生成器。只输出一个 JSON 对象，无其他文本。",
		UserPrompt: fmt.Sprintf("调查目标：%s\n本脉冲最后进展：%s\n输出收据 JSON（字段说明：pulse_outcome 取 HYPOTHESIS_FALSIFIED/PARTIAL/SOLVED；falsified_vector 填【你自己调查里的真实证伪结论原文摘要】，禁止照抄本说明里的任何示例词；blocking_evidence_ids 填引用过的 obs-id；new_discovered_surfaces 填新发现攻击面；negative_constraint 填给下一脉冲的具体禁令；candidate_flag 有真实 flag 原文才填否则 null）",
			state.Goal, truncateForAI(state.Progress+" "+state.LastReply, 1200)),
		MaxTokens:            300,
		RequiredCapabilities: []string{"chat"},
		Stream:               false,
		CognitiveTier:        2, // flagship: epistemic summarization turn
	})
	if err == nil {
		var receipt pulseReceipt
		text := strings.TrimSpace(result.Text)
		if idx := strings.Index(text, "{"); idx >= 0 {
			if end := strings.LastIndex(text, "}"); end > idx {
				if json.Unmarshal([]byte(text[idx:end+1]), &receipt) == nil && receipt.PulseOutcome != "" {
					// Echo guard: the model sometimes copies the prompt's
					// field-description placeholders verbatim into values
					// ("一句话已证伪路径" etc.) — those pollute the board.
					if strings.Contains(receipt.FalsifiedVector, "一句话") ||
						strings.Contains(receipt.NegativeConstraint, "下一步禁令") ||
						strings.Contains(receipt.FalsifiedVector, "一句话已证伪路径") {
						// salvage: keep only genuinely-populated fields
						receipt.FalsifiedVector = truncateForAI(strings.TrimSpace(state.Progress), 120)
						if receipt.FalsifiedVector == "" {
							receipt.FalsifiedVector = "本脉冲无新进展记录（收据解析净化）"
						}
					}
					return receipt
				}
			}
		}
	}
	// Mechanical fallback from board deltas.
	return pulseReceipt{
		PulseOutcome:       "PARTIAL",
		FalsifiedVector:    truncateForAI(state.Progress, 120),
		NegativeConstraint: "换正交攻击面（不要重复本脉冲手法）",
	}
}

// recordReceiptToBoard writes the receipt's falsified vector as a negative
// conclusion (E2) so every future pulse and sibling loop sees it.
func (s *Server) recordReceiptToBoard(state *subLoopState, receipt pulseReceipt) {
	if strings.TrimSpace(receipt.FalsifiedVector) == "" {
		return
	}
	statement := "已证伪：" + receipt.FalsifiedVector
	if len(receipt.BlockingEvidenceIDs) > 0 {
		statement += "（证据 " + strings.Join(receipt.BlockingEvidenceIDs, " ") + "）"
	}
	if receipt.NegativeConstraint != "" {
		statement += "；下一步约束：" + receipt.NegativeConstraint
	}
	_, _ = s.appendBlackboardFact("negative_conclusion", statement)
	if receipt.NegativeConstraint != "" {
		subLoopsMu.Lock()
		state.NegativeConstraints = append(state.NegativeConstraints, receipt.NegativeConstraint)
		subLoopsMu.Unlock()
	}
}

// freshPulsePrompt builds the clean-fork context for the next pulse: no
// dialogue history, just seeds + accumulated constraints + receipt digest.
func (s *Server) freshPulsePrompt(state *subLoopState, receipt pulseReceipt, nextPulse int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "目标：%s\n", state.Goal)
	fmt.Fprintf(&b, "你在第 %d 个脉冲（12 轮预算）。此前脉冲的结论已进黑板，对话历史已清零——你只携带结论，不携带失败过程。\n", nextPulse)
	if state.NegativeConstraints != nil && len(state.NegativeConstraints) > 0 {
		b.WriteString("已积累禁令（禁止重复这些路径）：\n")
		for i, c := range state.NegativeConstraints {
			if i >= 6 {
				break
			}
			b.WriteString("- " + c + "\n")
		}
	}
	if receipt.PulseOutcome != "" {
		fmt.Fprintf(&b, "上一脉冲结果：%s（%s）", receipt.PulseOutcome, truncateForAI(receipt.FalsifiedVector, 100))
		if len(receipt.NewDiscoveredSurfaces) > 0 {
			b.WriteString("；新发现攻击面：" + strings.Join(receipt.NewDiscoveredSurfaces, "、"))
		}
		b.WriteString("\n")
	}
	b.WriteString("黑板种子：\n" + strings.Join(s.relevantBlackboardFacts(state.Goal, 4), "\n") + "\n\n开始本脉冲的观测-推理循环。")
	return b.String()
}

// buildSubloopResultV1 renders the structured result (subloop-result.v1)
// for the registry and COLLECT: machine fields for the driver, summary for
// audit only.
func (s *Server) buildSubloopResultV1(state *subLoopState) json.RawMessage {
	status := state.Status
	failure := ""
	switch status {
	case "done", "cancelled":
		if !s.hasNewFlagFactSince(state.StartedAt.Unix()) {
			status = "stuck"
			failure = "no_flag_landed"
		}
	case "failed":
		failure = state.Error
	case "hibernated":
		failure = "sterile_pulses"
	}
	flag := ""
	for _, fact := range s.loadBlackboardFacts() {
		if fact.CreatedAt >= state.StartedAt.Unix() && strings.Contains(fact.Statement, "flag{") {
			if i := strings.Index(fact.Statement, "flag{"); i >= 0 {
				if j := strings.Index(fact.Statement[i:], "}"); j > 0 {
					flag = fact.Statement[i : i+j+1]
				}
			}
		}
	}
	result := map[string]any{
		"status":       status,
		"flag":         flag,
		"evidence_ids": state.Progress,
		"failure_code": failure,
		"summary":      truncateForAI(strings.ReplaceAll(state.Progress, "\n", " | "), 120),
		"pulses":       state.Pulse,
	}
	encoded, _ := json.Marshal(result)
	return encoded
}

// handleSpawnDirect spawns a sub-loop WITHOUT a model turn: the driver's
// default policy calls this when both MBEE and the chat path have timed
// out — the architecture keeps working while the model is slow. Zero
// LLM dependency for the spawn itself; the sub-loop's own turns use the
// model as normal.
func (s *Server) handleSpawnDirect(w http.ResponseWriter, r *http.Request) {
	log.Printf("[spawn-direct] handler entered")
	defer log.Printf("[spawn-direct] handler done")
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var request struct {
		Name string `json:"name"`
		Goal string `json:"goal"`
		// Tier 2 = general-assault: the sub-loop runs on the flagship
		// model (CognitiveTier 2) for hard/stuck codes (sapper ladder).
		Tier int `json:"tier"`
		// Turns caps the loop budget (micro-task pipeline: 5-8).
		Turns int `json:"turns"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondError(w, http.StatusBadRequest, "decode: "+err.Error())
		return
	}
	if strings.TrimSpace(request.Name) == "" || strings.TrimSpace(request.Goal) == "" {
		respondError(w, http.StatusBadRequest, "name and goal are required")
		return
	}
	// Placeholder guard (defense in depth): names like <code> come from
	// models echoing documentation syntax — refuse, with a correction.
	if strings.ContainsAny(request.Name, "<>{}") {
		respondError(w, http.StatusBadRequest, "name 含尖括号占位符（未替换）。用真实循环名，如 solve-a-05 / audit-a-05")
		return
	}
	// General-assault tier is carried in the loop state; runSubLoop reads
	// it every turn (CognitiveTier: 2 = forced flagship).
	baseTier := 1
	if request.Tier == 2 {
		baseTier = 2
	}
	summary, name, err := s.spawnSubLoopTiered(strings.TrimSpace(request.Name), strings.TrimSpace(request.Goal), baseTier, request.Turns)
	_ = name
	if err != nil {
		respondError(w, http.StatusConflict, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"spawned": true, "summary": summary})
}
