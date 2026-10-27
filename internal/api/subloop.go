package api

import (
	"context"
	"fmt"
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
	// subLoopTurnLimit caps a single sub-loop's model calls.
	subLoopTurnLimit = 40
	// subLoopTurnTimeout bounds one model call inside a sub-loop.
	subLoopTurnTimeout = 240 * time.Second
	// subLoopHardBudget bounds the whole sub-loop wall-clock.
	subLoopHardBudget = 40 * time.Minute
)

type subLoopState struct {
	Name      string
	Goal      string
	Status    string // running | done | failed | cancelled
	Turns     int
	LastReply string
	StartedAt time.Time
	EndedAt   time.Time
	Error     string
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
func (s *Server) spawnSubLoop(name string, goal string) (string, error) {
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

	systemPrompt := fmt.Sprintf(`你是 Ouroboros 的并行调查子智能体（%s）。你独立负责一个观测-推理循环。

你的目标： %s

工作方式：
- 对目标发起原始观测（ACTION:OBSERVE <url> [spec]）拿真实证据，亲自分析，形成并验证假设。
- 需要命令执行时用 ACTION:EXECUTE CONFIRMED: <命令>。
- 每得出一条可靠结论，用 ACTION:BLACKBOARD <结论> 记录（会进入共享黑板）。
- 你在一个自主循环中运行：每轮你会收到上一轮的动作结果与「继续」提示；任务完成或无法推进时，输出单独一行 DONE 结束（后端据此停止循环）。
- 目标完成、拿到 flag、或确认无更多可推进项时立即 DONE，不要空转。
- 所有回复使用中文。`, name, goal)

	go s.runSubLoop(state, systemPrompt, goal)
	return fmt.Sprintf("子循环 %s 已启动（目标: %.80s）", name, goal), nil
}

// runSubLoop drives one autonomous sub-loop: model call -> action execution
// -> "continue" nudge, until DONE / turn limit / budget / context cancel.
func (s *Server) runSubLoop(state *subLoopState, systemPrompt string, goal string) {
	ctx, cancel := context.WithTimeout(context.Background(), subLoopHardBudget)
	defer cancel()

	deadline := time.Now().Add(subLoopHardBudget)
	userPrompt := fmt.Sprintf("目标：%s\n开始你的观测-推理循环。", goal)
	for turn := 1; turn <= subLoopTurnLimit; turn++ {
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
		callCtx, callCancel := context.WithTimeout(ctx, subLoopTurnTimeout)
		result, err := app.GenerateProviderMessage(callCtx, s.workspace, app.ProviderGenerationRequest{
			SystemPrompt:         systemPrompt,
			UserPrompt:           userPrompt,
			MaxTokens:            2048,
			RequiredCapabilities: []string{"chat"},
			Stream:               false,
		})
		callCancel()
		subLoopsMu.Lock()
		state.Turns = turn
		subLoopsMu.Unlock()
		if err != nil {
			// Transient provider errors: retry once, then keep going —
			// sub-loops must survive provider hiccups.
			if turn < subLoopTurnLimit {
				time.Sleep(5 * time.Second)
				continue
			}
			state.Status = "failed"
			state.Error = err.Error()
			break
		}
		reply := strings.TrimSpace(result.Text)
		subLoopsMu.Lock()
		state.LastReply = reply
		subLoopsMu.Unlock()

		if strings.Contains(reply, "\nDONE") || strings.HasPrefix(reply, "DONE") {
			state.Status = "done"
			break
		}
		// Execute the sub-loop's action lines (OBSERVE/EXECUTE/BLACKBOARD/
		// READ/...) through the same parser, without persisting to the
		// parent conversation journal — results feed the next turn.
		actionOutput, actionRefs := s.executeSubLoopActions(reply)
		userPrompt = fmt.Sprintf("动作结果：\n%s\n\n继续（第 %d/%d 轮）。完成时单独一行 DONE。", truncateForAI(actionOutput, 6000), turn+1, subLoopTurnLimit)
		_ = actionRefs
	}
	if state.Status == "running" {
		state.Status = "done"
	}
	state.EndedAt = time.Now()
	select {
	case subLoopConveyor <- state.Name:
	default:
	}
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

func (s *Server) executeSubLoopActions(reply string) (string, []string) {
	var output strings.Builder
	refs, results := s.parseAndExecuteAIActions(reply)
	for _, result := range results {
		summary := result.Summary
		if result.Output != "" {
			summary += "\n" + truncateForAI(result.Output, 2400)
		}
		output.WriteString(fmt.Sprintf("[%s] %s\n\n", result.Kind, summary))
	}
	return output.String(), refs
}

// collectSubLoops reports the state of all sub-loops (digest form).
func (s *Server) collectSubLoops(name string) (string, error) {
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
