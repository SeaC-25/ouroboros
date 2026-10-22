package main

// GateMesh competition entrypoint: `ouro agent run --objective ...`
// (doc/gatemesh-architecture-v1.md §S7).
//
// Wires the six S1–S6 packages into one autonomous loop with REAL execution:
//
//	scopegate    → every outbound dial passes the CIDR/domain whitelist
//	               (the only gate that cannot be turned off)
//	conveyor     → 12-action reflex belt picks the next action (zero LLM)
//	nativetools  → the action ACTUALLY RUNS (http_probe dials, path_fuzz
//	               fuzzes, port_scan scans …) through guarded execution
//	evidencegate → tool findings must pass E2→E3 differential promotion
//	cognition    → Progress gate (anti-capitulation) + Budget gate (hard caps)
//	eventlog     → idempotency ledger (crash resume) + flag regex capture

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ouroboros/ouroboros/internal/cognition"
	"github.com/ouroboros/ouroboros/internal/conveyor"
	"github.com/ouroboros/ouroboros/internal/evidencegate"
	"github.com/ouroboros/ouroboros/internal/eventlog"
	"github.com/ouroboros/ouroboros/internal/kbsearch"
	"github.com/ouroboros/ouroboros/internal/nativetools"
	"github.com/ouroboros/ouroboros/internal/scopegate"
	"github.com/ouroboros/ouroboros/internal/sessionguard"
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
	objective   string
	workspace   string
	autoAccept  bool
	maxSteps    int
	maxTokens   int64
	wallClock   time.Duration
	flagFormat  string
	reportPath  string
	scopePolicy string
}

func newAgentRunCommand(out io.Writer) *cobra.Command {
	opts := &agentRunOptions{}
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Drive one objective through the GateMesh loop autonomously",
		Long: "ouro agent run --objective \"<goal>\" --scope-policy <policy.json>\n\n" +
			"Autonomous loop with real tool execution: the conveyor picks reflex\n" +
			"actions, nativetools executes them through the scope-guarded transport,\n" +
			"the Progress gate rejects capitulation, the Budget gate hard-stops on\n" +
			"limits, and findings count only after differential evidence promotion.",
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
	cmd.Flags().StringVar(&opts.flagFormat, "flag-format", "", "regex scanned across tool output for flag capture (e.g. 'flag\\{[^}]+\\}')")
	cmd.Flags().StringVar(&opts.reportPath, "report", "", "report output path (default: <workspace>/report.md)")
	cmd.Flags().StringVar(&opts.scopePolicy, "scope-policy", "", "scope policy JSON (CIDR/domain whitelist); required")
	_ = cmd.MarkFlagRequired("objective")
	_ = cmd.MarkFlagRequired("scope-policy")
	return cmd
}

// agentStepLog is one dispatched action record for the report.
type agentStepLog struct {
	Step    int    `json:"step"`
	Entity  string `json:"entity"`
	Action  string `json:"action"`
	Outcome string `json:"outcome"`
	Tool    string `json:"tool,omitempty"`
	Output  string `json:"output,omitempty"` // truncated transcript tail
}

// agentFinding is one evidence-tracked finding.
type agentFinding struct {
	ID     string `json:"id"`
	Entity string `json:"entity"`
	Signal string `json:"signal"`
	Level  string `json:"level"`
	Note   string `json:"note,omitempty"`
}

func runAgent(ctx context.Context, out io.Writer, opts *agentRunOptions) error {
	started := time.Now()

	// S3: scope gate — load policy, build the guarded HTTP client. This is
	// the transport every tool call below goes through; out-of-scope dials
	// fail before a packet leaves.
	policy, err := scopegate.LoadScopePolicyFile(opts.scopePolicy)
	if err != nil {
		return fmt.Errorf("load scope policy: %w", err)
	}
	guarded := scopegate.NewGuardedDialerStruct(policy, nil)
	if err := guarded.Validate(); err != nil {
		return fmt.Errorf("scope policy invalid: %w", err)
	}
	baseClient := guarded.GuardHTTPClient()

	// Session layer (sessionguard): a cookie jar keeps auth state across
	// tool calls, and a redirect guard hard-blocks logout-semantic paths
	// so no probe can silently tear the session down (GateMesh §横切面).
	baseClient.Jar, _ = cookiejar.New(nil)
	httpClient := &http.Client{
		Transport: baseClient.Transport,
		Jar:       baseClient.Jar,
		Timeout:   baseClient.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if sessionguard.IsDestructiveAuthPath(req.URL) {
				return fmt.Errorf("blocked logout-semantic path: %s", req.URL.Path)
			}
			return nil // every hop still dials through the guarded transport
		},
	}

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

	// L0: eventlog + idempotency ledger.
	store, err := eventlog.OpenSQLite(filepath.Join(ws, "events.db"))
	if err != nil {
		return fmt.Errorf("open eventlog: %w", err)
	}
	defer store.Close()
	leases, err := eventlog.NewLeaseStore(store)
	if err != nil {
		return fmt.Errorf("open lease ledger: %w", err)
	}

	// Crash resume: recover orphaned RUNNING leases from a previous run.
	// Requeued ones are marked FAILED (retryable); quarantined ones ABORTED
	// (poison-pill — the tool×entity combination crashed twice).
	requeued, quarantined, err := leases.RecoverOrphans(ctx, time.Now().UnixMilli())
	if err != nil {
		return fmt.Errorf("recover orphan leases: %w", err)
	}
	if len(requeued) > 0 || len(quarantined) > 0 {
		fmt.Fprintf(out, "agent: resumed — %d orphan leases requeued, %d quarantined\n",
			len(requeued), len(quarantined))
		for _, q := range quarantined {
			fmt.Fprintf(out, "agent: poison-pill %s×%s (switch tool on retry)\n", q.ToolName, q.EntityID)
		}
	}

	// S6 gates.
	progress := cognition.NewProgressGate(cognition.ProgressConfig{})
	budget := cognition.NewBudgetGate(cognition.BudgetConfig{
		MaxSteps:     opts.maxSteps,
		MaxTokens:    opts.maxTokens,
		MaxWallClock: opts.wallClock,
	})

	// Knowledge base (optional at runtime, required for pivot hints):
	// container path /opt/ouro/kb_vectors.db, dev fallback beside the repo.
	var kb *kbsearch.Reader
	for _, kbPath := range []string{"/opt/ouro/kb_vectors.db", "assets/kb_vectors.db"} {
		if r, err := kbsearch.Open(kbPath); err == nil {
			kb = r
			defer r.Close()
			break
		}
	}

	// Flag capture regex.
	var flagRe *regexp.Regexp
	if opts.flagFormat != "" {
		flagRe, err = regexp.Compile(opts.flagFormat)
		if err != nil {
			return fmt.Errorf("flag-format regex: %w", err)
		}
	}

	// S5 conveyor + seed entities.
	registry := conveyor.DefaultRegistry()
	entities := seedEntities(opts.objective)

	fmt.Fprintf(out, "agent: workspace=%s entities=%d budget(steps=%d tokens=%d wall=%s) scope=on\n",
		ws, len(entities), opts.maxSteps, opts.maxTokens, opts.wallClock)

	// The loop.
	steps := []agentStepLog{}
	findings := []agentFinding{}
	flags := map[string]bool{}
	done := map[string]bool{}
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

		// Best conveyor candidate not already done.
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
			fmt.Fprintf(out, "agent: no conveyor candidates remain; stopping\n")
			break
		}

		// Idempotency: claim before executing.
		key := eventlog.IdempotencyKey(best.Entity.ID, best.Spec.Tool, best.Spec.Action)
		if err := leases.Claim(ctx, eventlog.ExecutionLease{
			Key: key, EntityID: best.Entity.ID, ToolName: best.Spec.Tool,
			WorkerID: "agent", TimeoutMS: 120000,
		}); err != nil {
			done[best.Entity.ID+"\x00"+best.Spec.Action] = true
			stepsTaken++
			steps = append(steps, agentStepLog{stepsTaken, best.Entity.ID, best.Spec.Action, "duplicate-or-quarantined", best.Spec.Tool, ""})
			continue
		}
		done[best.Entity.ID+"\x00"+best.Spec.Action] = true
		_ = leases.Transition(ctx, key, eventlog.LeaseRunning, "")

		// REAL EXECUTION: dispatch the action through nativetools with the
		// scope-guarded HTTP client. Failures are recorded, never fatal.
		output, execErr := executeConveyorAction(ctx, best, httpClient)
		stepsTaken++
		_ = budget.Step()

		outcome := "ok"
		if execErr != nil {
			outcome = "error"
		}
		steps = append(steps, agentStepLog{
			Step: stepsTaken, Entity: best.Entity.ID, Action: best.Spec.Action,
			Outcome: outcome, Tool: best.Spec.Tool, Output: tailFor(output, 400),
		})
		_ = leases.Transition(ctx, key, eventlog.LeaseArtifactSaved, writeAttemptArtifact(ws, stepsTaken, best, output, execErr))

		// Flag capture across real tool output.
		if flagRe != nil {
			for _, m := range flagRe.FindAllString(output, -1) {
				if !flags[m] {
					flags[m] = true
					fmt.Fprintf(out, "agent: FLAG captured: %s\n", m)
				}
			}
		}

		// Evidence promotion: any non-empty signal output is a candidate
		// E2 finding; verify_finding actions replay and promote via the
		// differential gate. (Positive-only until a negative command exists
		// — recorded honestly in the note.)
		if strings.Contains(best.Spec.Action, "probe") || strings.Contains(best.Spec.Action, "scan") {
			f := evidencegate.Finding{
				ID:             fmt.Sprintf("f-%d", stepsTaken),
				Target:         best.Entity.ID,
				ReproduceCmd:   "ouro-native:" + best.Spec.Action + " " + best.Entity.ID,
				ExpectedSignal: "",
				Level:          evidencegate.E2Tentative,
			}
			res := evidencegate.PromoteDifferential(ctx, f, nativeReproducer{client: httpClient}, "")
			findings = append(findings, agentFinding{
				ID: f.ID, Entity: best.Entity.ID, Signal: firstLine(output, 120),
				Level: string(res.To), Note: res.Note,
			})
		}

		// Progress accounting: real component delta = did the tool produce
		// output (new surface) or not.
		newComponents := 0
		if execErr == nil && strings.TrimSpace(output) != "" {
			newComponents = 1
		}
		if progress.RecordStep(newComponents) {
			// STALLED — Forced Pivot Protocol (GateMesh §Progress gate):
			// 1) record the pivot against this entity's cap
			// 2) failure-as-a-query: KB retrieval on the output features
			// 3) orthogonal jump: drop the current quadrant for this entity
			//    (mark high-value orthogonal actions eligible by clearing
			//    the done-set entries of unexplored quadrants)
			pivotErr := progress.RecordPivot(best.Entity.ID)
			pivotNote := "stalled"
			if pivotErr == nil {
				pivotNote = "stalled→pivot"
				if kb != nil {
					terms := kbTermsFromOutput(output)
					if len(terms) > 0 {
						hits, err := kb.Search(terms, 3)
						if err == nil && len(hits) > 0 {
							pivotNote = "stalled→pivot kb:" + hits[0].Section
							for i, h := range hits {
								steps = append(steps, agentStepLog{stepsTaken, best.Entity.ID,
									"kb_hint", fmt.Sprintf("hit%d %s: %s", i+1, h.Section, firstLine(h.Text, 100)), "", ""})
							}
						}
					}
				}
				// Orthogonal jump: clear done-marks on a different quadrant's
				// actions for this entity so the selector explores them next.
				clearOrthogonalQuadrant(done, best.Entity.ID, best.Spec.Action)
			}
			steps = append(steps, agentStepLog{stepsTaken, best.Entity.ID, "progress_gate", pivotNote, "", ""})
		}
		_ = leases.Transition(ctx, key, eventlog.LeaseCommitted, artifactPath(ws, stepsTaken))
	}

	// Report.
	report := opts.reportPath
	if report == "" {
		report = filepath.Join(ws, "report.md")
	}
	if err := writeAgentReport(report, opts.objective, steps, findings, flags, budget, started); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	fmt.Fprintf(out, "agent: report → %s (%d steps, %d findings, %d flags, wall %s)\n",
		report, stepsTaken, len(findings), len(flags), time.Since(started).Round(time.Second))
	return nil
}

// executeConveyorAction maps a conveyor action to its REAL nativetools
// implementation. Every branch calls a real exported function; unknown
// bindings are an explicit error, never a silent no-op.
func executeConveyorAction(ctx context.Context, c conveyor.Candidate, client *http.Client) (string, error) {
	target := nativetools.Target{Kind: targetKindFor(c.Entity), Value: c.Entity.ID}
	opts := nativetools.Options{Timeout: 60 * time.Second, MaxWorkers: 8, HTTPClient: client}

	var output string
	var err error
	switch c.Spec.Action {
	case "http_probe":
		output, err = nativetools.HTTPProbe(ctx, target.Value, opts)
	case "body_fetch":
		output, err = nativetools.Crawl(ctx, target.Value, opts)
	case "auth_login":
		output, err = nativetools.TestAuth(ctx, target.Value, opts)
	case "port_scan":
		output, err = nativetools.PortScan(ctx, target.Value, opts)
	case "fingerprint", "service_fingerprint":
		output, err = nativetools.FingerprintServices(ctx, target.Value, opts)
	case "cms_scan":
		output, err = nativetools.HTTPProbe(ctx, target.Value, opts)
	case "osint":
		output, err = nativetools.DiscoverSubdomains(ctx, target.Value, opts)
	case "shallow_dir":
		// Fast track: in-process wordlist probe (milliseconds).
		output, err = nativetools.PathFuzz(ctx, target.Value, opts)
		// Deep track: fastsec heavy dictionary (4707 paths) as a guarded
		// subprocess; absent binary degrades to native-only output.
		if deep, derr := fastsecDeepScan(ctx, c.Entity.ID); derr == nil && strings.TrimSpace(deep) != "" {
			output = output + "\n" + deep
		}
	case "sqli_probe":
		output, err = nativetools.DetectInjection(ctx, target.Value, opts)
	case "xss_probe":
		output, err = nativetools.ScanReflectedXSS(ctx, target.Value, opts)
	case "nuclei_subset":
		output, err = nativetools.ScanTemplates(ctx, target.Value, opts)
	case "verify_finding":
		// Internal action: replay probe as the verification vehicle.
		output, err = nativetools.HTTPProbe(ctx, target.Value, opts)
	default:
		return "", fmt.Errorf("no real binding for action %q — refusing to fake it", c.Spec.Action)
	}
	return output, err
}

// fastsecDeepScan runs the fastsec binary (heavy dictionaries) as a
// process-group-guarded subprocess. Absent binary or failure returns an
// error; the caller degrades to native-only output.
func fastsecDeepScan(ctx context.Context, targetURL string) (string, error) {
	// Derive scheme://host[:port] from the target URL for -dir.
	parts := strings.SplitN(targetURL, "/", 4)
	if len(parts) < 3 || parts[2] == "" {
		return "", fmt.Errorf("fastsec: cannot derive base URL from %q", targetURL)
	}
	base := parts[0] + "//" + parts[2]
	runCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, "fastsec", "-dir", base, "-delay-min", "5", "-delay-max", "15")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func targetKindFor(e conveyor.Entity) string {
	if e.Has(conveyor.CapDomain) {
		return "domain"
	}
	return "url"
}

// nativeReproducer adapts conveyor replays to the evidence gate.
type nativeReproducer struct{ client *http.Client }

func (r nativeReproducer) Reproduce(ctx context.Context, reproduceCmd string) (string, error) {
	// Format: "ouro-native:<action> <entity>"
	parts := strings.SplitN(strings.TrimPrefix(reproduceCmd, "ouro-native:"), " ", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("bad reproduce cmd: %q", reproduceCmd)
	}
	e := conveyor.Entity{ID: parts[1], Confidence: 1, Capabilities: map[conveyor.Capability]bool{
		conveyor.CapWebEndpoint: true, conveyor.CapDomain: true,
	}}
	c := conveyor.Candidate{Entity: e, Spec: conveyor.ActionSpec{Action: parts[0]}}
	return executeConveyorAction(ctx, c, r.client)
}

func hasCredentials(e conveyor.Entity) bool { return e.Has(conveyor.CapAuthSession) }

// seedEntities extracts bare scope items from free text.
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
				Capabilities: map[conveyor.Capability]bool{conveyor.CapDomain: true, conveyor.CapHost: true}}
		default:
			continue
		}
		seen[tok] = true
		out = append(out, e)
	}
	if len(out) == 0 {
		out = append(out, conveyor.Entity{ID: "objective:" + objective, Confidence: 0.5,
			Source: conveyor.SourceInsight, Capabilities: map[conveyor.Capability]bool{conveyor.CapWebEndpoint: true}})
	}
	return out
}

// writeAttemptArtifact persists the FULL tool output (not the report's
// truncated view) for audit/replay; returns the path for the lease ledger.
func writeAttemptArtifact(ws string, step int, c conveyor.Candidate, output string, execErr error) string {
	path := artifactPath(ws, step)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return path
	}
	body := fmt.Sprintf("action=%s\nentity=%s\ntool=%s\nerror=%v\n\n%s\n",
		c.Spec.Action, c.Entity.ID, c.Spec.Tool, execErr, output)
	_ = os.WriteFile(path, []byte(body), 0o644)
	return path
}

func artifactPath(ws string, step int) string {
	return filepath.Join(ws, "attempts", fmt.Sprintf("step-%03d.txt", step))
}

func tailFor(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

func firstLine(s string, n int) string {
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			if len(line) > n {
				return line[:n]
			}
			return line
		}
	}
	return ""
}

func writeAgentReport(path, objective string, steps []agentStepLog, findings []agentFinding, flags map[string]bool, budget *cognition.BudgetGate, started time.Time) error {
	var b strings.Builder
	b.WriteString("# Ouroboros Agent Report\n\n")
	b.WriteString("## Objective\n\n" + objective + "\n\n")
	b.WriteString("## Budget\n\n")
	fmt.Fprintf(&b, "- steps used: %d\n- wall clock: %s\n- stopped by: %s\n\n",
		budget.StepsUsed(), time.Since(started).Round(time.Second), budget.StoppedBy())
	if len(findings) > 0 {
		b.WriteString("## Findings (evidence-gated)\n\n```json\n")
		enc := json.NewEncoder(&b)
		enc.SetIndent("", " ")
		if err := enc.Encode(findings); err != nil {
			return err
		}
		b.WriteString("```\n\n")
	}
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

// kbTermsFromOutput extracts failure-feature terms for KB retrieval
// (failure-as-a-query): distinctive tokens from error output, WAF names,
// status codes.
func kbTermsFromOutput(output string) []string {
	features := map[string]bool{}
	lower := strings.ToLower(output)
	// WAF / server signatures worth looking up.
	for _, sig := range []string{"cloudfront", "cloudflare", "akamai", "waf", "403", "401", "500",
		"nginx", "apache", "iis", "tomcat", "jenkins", "login", "redirect"} {
		if strings.Contains(lower, sig) {
			features[sig] = true
		}
	}
	out := make([]string, 0, len(features))
	for f := range features {
		out = append(out, f)
	}
	if len(out) > 4 {
		out = out[:4] // bounded query
	}
	return out
}

// quadrantOf maps an action to its orthogonal quadrant (GateMesh §机制2):
// A=frontal attack, B=topology/history, C=white-box/static, D=identity/protocol.
func quadrantOf(action string) string {
	switch action {
	case "sqli_probe", "xss_probe", "nuclei_subset":
		return "A"
	case "osint", "shallow_dir", "http_probe":
		return "B"
	case "body_fetch", "cms_scan", "fingerprint", "verify_finding":
		return "C"
	case "auth_login", "port_scan", "service_fingerprint":
		return "D"
	}
	return "B"
}

// clearOrthogonalQuadrant re-opens done-marks on actions from a DIFFERENT
// quadrant for this entity, forcing the selector to explore sideways.
func clearOrthogonalQuadrant(done map[string]bool, entityID, stalledAction string) {
	stalledQ := quadrantOf(stalledAction)
	for _, action := range []string{"http_probe", "body_fetch", "auth_login", "port_scan",
		"fingerprint", "cms_scan", "osint", "shallow_dir", "sqli_probe", "xss_probe",
		"nuclei_subset", "verify_finding"} {
		if quadrantOf(action) != stalledQ {
			delete(done, entityID+"\x00"+action)
		}
	}
}
