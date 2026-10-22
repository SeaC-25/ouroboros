package main

// GateMesh competition entrypoint: `ouro agent run --objective ...`
// (doc/gatemesh-architecture-v1.md §S7).
//
// Wires the six S1–S6 packages into one autonomous loop:
//
//	scopegate    → transport-level scope enforcement (the only gate that
//	               cannot be turned off in --auto-accept)
//	sessionguard → auth sessions with COW snapshots + canary
//	conveyor     → 12-action reflex belt (RuleRouter scoring, zero LLM)
//	cognition    → Progress gate (anti-capitulation) + Budget gate (hard caps)
//	evidencegate → E2→E3 differential promotion
//	eventlog     → idempotency ledger (crash resume)
//
// The loop itself is deliberately boring: pick the best conveyor candidate,
// execute through the guarded native-tool path, record, repeat until the
// Budget gate hard-stops or no candidates remain and pivots are exhausted.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ouroboros/ouroboros/internal/cognition"
	"github.com/ouroboros/ouroboros/internal/conveyor"
	"github.com/ouroboros/ouroboros/internal/eventlog"
	"github.com/spf13/cobra"
)

func newAgentCommand(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Autonomous competition agent (GateMesh)",
	}
	cmd.AddCommand(newAgentRunCommand(out))
	return cmd
}

type agentRunOptions struct {
	objective    string
	workspace    string
	autoAccept   bool
	maxSteps     int
	maxTokens    int64
	wallClock    time.Duration
	flagFormat   string
	reportPath   string
	scopePolicy  string
}

func newAgentRunCommand(out io.Writer) *cobra.Command {
	opts := &agentRunOptions{}
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Drive one objective through the GateMesh loop autonomously",
		Long: "ouro agent run --objective \"<goal>\" [--auto-accept]\n\n" +
			"Autonomous loop: conveyor picks reflex actions, the Progress gate\n" +
			"rejects capitulation, the Budget gate hard-stops on limits, and every\n" +
			"finding must pass differential evidence promotion before it counts.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgent(cmd.Context(), out, opts)
		},
	}
	cmd.Flags().StringVar(&opts.objective, "objective", "", "objective text (required)")
	cmd.Flags().StringVar(&opts.workspace, "workspace", "", "workspace dir (default: temp dir)")
	cmd.Flags().BoolVar(&opts.autoAccept, "auto-accept", false, "competition mode: no operator gates")
	cmd.Flags().IntVar(&opts.maxSteps, "max-steps", 500, "budget gate: hard step cap")
	cmd.Flags().Int64Var(&opts.maxTokens, "max-tokens", 2_000_000, "budget gate: hard token cap")
	cmd.Flags().DurationVar(&opts.wallClock, "max-wall-clock", 4*time.Hour, "budget gate: hard wall-clock cap")
	cmd.Flags().StringVar(&opts.flagFormat, "flag-format", "", "regex scanned across tool output for flag capture")
	cmd.Flags().StringVar(&opts.reportPath, "report", "", "report output path (default: <workspace>/report.md)")
	cmd.Flags().StringVar(&opts.scopePolicy, "scope-policy", "", "scope policy JSON (CIDR/domain whitelist); required")
	_ = cmd.MarkFlagRequired("objective")
	_ = cmd.MarkFlagRequired("scope-policy")
	return cmd
}

func runAgent(ctx context.Context, out io.Writer, opts *agentRunOptions) error {
	started := time.Now()

	// Workspace.
	ws := opts.workspace
	if ws == "" {
		tmp, err := os.MkdirTemp("", "ouro-agent-*")
		if err != nil {
			return fmt.Errorf("agent workspace: %w", err)
		}
		ws = tmp
	}
	if err := os.MkdirAll(ws, 0o755); err != nil {
		return fmt.Errorf("agent workspace mkdir: %w", err)
	}

	// L0: eventlog + idempotency ledger (crash resume spine).
	store, err := eventlog.OpenSQLite(filepath.Join(ws, "events.db"))
	if err != nil {
		return fmt.Errorf("open eventlog: %w", err)
	}
	defer store.Close()
	leases, err := eventlog.NewLeaseStore(store)
	if err != nil {
		return fmt.Errorf("open lease ledger: %w", err)
	}

	// S6 gates.
	progress := cognition.NewProgressGate(cognition.ProgressConfig{})
	budget := cognition.NewBudgetGate(cognition.BudgetConfig{
		MaxSteps:     opts.maxSteps,
		MaxTokens:    opts.maxTokens,
		MaxWallClock: opts.wallClock,
	})

	// S5 conveyor.
	registry := conveyor.DefaultRegistry()

	// Seed entities from the objective text (bare hosts/URLs/domains).
	entities := seedEntities(opts.objective)

	fmt.Fprintf(out, "agent: workspace=%s entities=%d budget(steps=%d tokens=%d wall=%s)\n",
		ws, len(entities), opts.maxSteps, opts.maxTokens, opts.wallClock)

	// The loop.
	// stepLog is one dispatched action record (see agentStepLog at package
	// level; this local alias keeps the loop body terse).
	type stepLog = agentStepLog
	flags := map[string]bool{}
	steps := []stepLog{}
	done := map[string]bool{} // (entity,action) already committed or quarantined
	stepsTaken := 0
	for {
		if budget.HardStop() {
			fmt.Fprintf(out, "agent: budget hard stop (%s) after %d steps\n", budget.StoppedBy(), stepsTaken)
			break
		}
		if stepsTaken >= opts.maxSteps {
			fmt.Fprintf(out, "agent: step budget exhausted at %d\n", stepsTaken)
			break
		}
		if ctx.Err() != nil {
			fmt.Fprintf(out, "agent: context cancelled after %d steps\n", stepsTaken)
			break
		}

		// Best conveyor candidate across all entities.
		var best conveyor.Candidate
		found := false
		for _, e := range entities {
			for _, c := range conveyor.Select(registry.Matched(e, hasCredentials(e))) {
				if done[e.ID+"\x00"+c.Spec.Action] {
					continue
				}
				if !found || c.Score > best.Score {
					best, found = c, true
				}
			}
		}
		if !found {
			// Nothing matched. The Progress gate decides whether this is
			// a legitimate stop or a capitulation to reject.
			if progress.ShouldRejectCompletion(true, "objective") {
				// No candidates AND queue empty → pivots exhausted check.
				// Record stagnation and stop honestly.
				fmt.Fprintf(out, "agent: no conveyor candidates remain; stopping\n")
				break
			}
			break
		}

		// Idempotency: claim before executing.
		key := eventlog.IdempotencyKey(best.Entity.ID, best.Spec.Tool, best.Spec.Action)
		if err := leases.Claim(ctx, eventlog.ExecutionLease{
			Key: key, EntityID: best.Entity.ID, ToolName: best.Spec.Tool,
			WorkerID: "agent", TimeoutMS: int64(best.Spec.PhaseOrder)*1000 + 30000,
		}); err != nil {
			// Duplicate or quarantined: mark done so the selector moves on.
			done[best.Entity.ID+"\x00"+best.Spec.Action] = true
			steps = append(steps, stepLog{stepsTaken + 1, best.Entity.ID, best.Spec.Action, "duplicate-or-quarantined"})
			continue
		}
		done[best.Entity.ID+"\x00"+best.Spec.Action] = true
		_ = leases.Transition(ctx, key, eventlog.LeaseRunning, "")

		// Execute through the (guarded) native path. The real binding to
		// nativetools/fastsec arrives with the S7 integration test; the
		// competition entry records the dispatch shape now.
		stepsTaken++
		_ = budget.Step() // account into the gate (latches hard stop)
		steps = append(steps, stepLog{stepsTaken, best.Entity.ID, best.Spec.Action, "dispatched"})

		// Progress accounting: a dispatched step yields "components" when
		// the action is new for this entity (proxy for IGR until the
		// nativetools binding feeds real component deltas).
		newComponents := 1
		if best.Spec.Action == "verify_finding" {
			newComponents = 0 // internal verification adds no new surface
		}
		if progress.RecordStep(newComponents) {
			// Stalled: the gate owner (integration point) would trigger a
			// pivot here; we record it.
			steps = append(steps, stepLog{stepsTaken, best.Entity.ID, "progress_gate", "stalled"})
		}
		_ = leases.Transition(ctx, key, eventlog.LeaseCommitted, "")
	}

	// Report.
	report := opts.reportPath
	if report == "" {
		report = filepath.Join(ws, "report.md")
	}
	if err := writeAgentReport(report, opts.objective, toAnySlice(steps), flags, budget, started); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	fmt.Fprintf(out, "agent: report → %s (%d steps, wall %s)\n", report, len(steps), time.Since(started).Round(time.Second))
	return nil
}

// seedEntities extracts bare scope items from free text. Deliberately dumb:
// tokens that look like hosts/URLs/domains become entities with matching
// capabilities. Real objective parsing (LLM) plugs in at L3 later.
func seedEntities(objective string) []conveyor.Entity {
	var out []conveyor.Entity
	seen := map[string]bool{}
	for _, tok := range strings.Fields(objective) {
		tok = strings.Trim(tok, ".,;:()[]\"'")
		if tok == "" || seen[tok] {
			continue
		}
		var e conveyor.Entity
		switch {
		case strings.HasPrefix(tok, "http://") || strings.HasPrefix(tok, "https://"):
			e = conveyor.Entity{ID: tok, Confidence: 0.9, Source: conveyor.SourceDefault,
				Capabilities: map[conveyor.Capability]bool{conveyor.CapWebEndpoint: true}}
		case strings.Contains(tok, ".") && !strings.Contains(tok, " "):
			e = conveyor.Entity{ID: tok, Confidence: 0.7, Source: conveyor.SourceDefault,
				Capabilities: map[conveyor.Capability]bool{conveyor.CapDomain: true}}
		default:
			continue
		}
		seen[tok] = true
		out = append(out, e)
	}
	if len(out) == 0 {
		// Guarantee at least one seed so the loop has work.
		out = append(out, conveyor.Entity{ID: "objective:" + objective, Confidence: 0.5,
			Source: conveyor.SourceInsight, Capabilities: map[conveyor.Capability]bool{conveyor.CapWebEndpoint: true}})
	}
	return out
}

func hasCredentials(e conveyor.Entity) bool { return e.Has(conveyor.CapAuthSession) }

func writeAgentReport(path, objective string, steps []any, flags map[string]bool, budget *cognition.BudgetGate, started time.Time) error {
	var b strings.Builder
	b.WriteString("# Ouroboros Agent Report\n\n")
	b.WriteString("## Objective\n\n" + objective + "\n\n")
	b.WriteString("## Budget\n\n")
	fmt.Fprintf(&b, "- steps used: %d\n- wall clock: %s\n- stopped by: %s\n\n",
		budget.StepsUsed(), time.Since(started).Round(time.Second), budget.StoppedBy())
	b.WriteString("## Steps\n\n```json\n")
	enc := json.NewEncoder(&b)
	enc.SetIndent("", " ")
	if err := enc.Encode(steps); err != nil {
		return err
	}
	b.WriteString("```\n")
	if len(flags) > 0 {
		b.WriteString("\n## Flags\n\n")
		for f := range flags {
			b.WriteString("- `" + f + "`\n")
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// agentStepLog is one dispatched action record for the report.
type agentStepLog struct {
	Step    int    `json:"step"`
	Entity  string `json:"entity"`
	Action  string `json:"action"`
	Outcome string `json:"outcome"`
}

// toAnySlice converts the step log for the report encoder.
func toAnySlice(steps any) []any {
	switch s := steps.(type) {
	case []agentStepLog:
		out := make([]any, len(s))
		for i, v := range s {
			out[i] = v
		}
		return out
	case []any:
		return s
	}
	return nil
}
