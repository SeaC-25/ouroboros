package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ouroboros/ouroboros/internal/app"
	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
	"github.com/ouroboros/ouroboros/internal/frontier"
	"github.com/ouroboros/ouroboros/internal/tui"
	"github.com/spf13/cobra"
)

type interactiveModel interface {
	tea.Model
}

type interactiveSession interface {
	Run() error
	Send(tea.Msg)
	Quit()
}

type bubbleTeaSession struct {
	program *tea.Program
}

func (s bubbleTeaSession) Run() error {
	_, err := s.program.Run()
	return err
}

func (s bubbleTeaSession) Send(msg tea.Msg) {
	s.program.Send(msg)
}

func (s bubbleTeaSession) Quit() {
	s.program.Quit()
}

var newInteractiveSession = func(model interactiveModel) interactiveSession {
	return bubbleTeaSession{program: tea.NewProgram(model, tea.WithAltScreen())}
}

var runInteractiveTUI = func(model interactiveModel) error {
	return newInteractiveSession(model).Run()
}

func main() {
	cmd := newRootCommand(os.Stdout, os.Stderr)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCommand(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   app.Name,
		Short: "Ouroboros automated pentest compiler",
		Long:  app.Banner(),
	}
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	cmd.AddCommand(newInitCommand(out))
	cmd.AddCommand(newDoctorCommand(out))
	cmd.AddCommand(newRunCommand(out))
	cmd.AddCommand(newReportProjectCommand(out))
	cmd.AddCommand(newReportProjectDispatchCommand(out))
	cmd.AddCommand(newReportProjectDispatchesCommand(out))
	cmd.AddCommand(newExecuteReportProjectDispatchCommand(out))
	cmd.AddCommand(newReportProjectDispatchExecutionsCommand(out))
	cmd.AddCommand(newReportProjectDispatchExecutionAnnotateCommand(out))
	cmd.AddCommand(newReportProjectDispatchExecutionSelectCommand(out))
	cmd.AddCommand(newReportProjectDispatchExecutionSelectionCommand(out))
	cmd.AddCommand(newReportProjectDispatchExecutionSelectionClearCommand(out))
	cmd.AddCommand(newReplayCommand(out))
	cmd.AddCommand(newReplayWorkbenchCommand(out))
	cmd.AddCommand(newReplayWorkbenchSessionCommand(out))
	cmd.AddCommand(newReplayWorkbenchSessionAnnotateCommand(out))
	cmd.AddCommand(newReplayWorkbenchSessionSelectCommand(out))
	cmd.AddCommand(newReplayWorkbenchSessionSelectionCommand(out))
	cmd.AddCommand(newReplayWorkbenchSessionSelectionClearCommand(out))
	cmd.AddCommand(newReplayWorkbenchSessionsCommand(out))
	cmd.AddCommand(newReplayWorkbenchCollaborationCommand(out))
	cmd.AddCommand(newReplayWorkbenchCollaborationAnnotateCommand(out))
	cmd.AddCommand(newReplayWorkbenchCollaborationSelectCommand(out))
	cmd.AddCommand(newReplayWorkbenchCollaborationSelectionCommand(out))
	cmd.AddCommand(newReplayWorkbenchCollaborationSelectionClearCommand(out))
	cmd.AddCommand(newReplayWorkbenchCollaborationPresenceCommand(out))
	cmd.AddCommand(newReplayWorkbenchCollaborationPresencesCommand(out))
	cmd.AddCommand(newReplayWorkbenchCollaborationNoteCommand(out))
	cmd.AddCommand(newReplayWorkbenchCollaborationNoteAnnotateCommand(out))
	cmd.AddCommand(newReplayWorkbenchCollaborationNoteSelectCommand(out))
	cmd.AddCommand(newReplayWorkbenchCollaborationNoteSelectionCommand(out))
	cmd.AddCommand(newReplayWorkbenchCollaborationNoteSelectionClearCommand(out))
	cmd.AddCommand(newReplayWorkbenchCollaborationNotesCommand(out))
	cmd.AddCommand(newReplayWorkbenchCollaborationsCommand(out))
	cmd.AddCommand(newToolchainCommand(out))
	cmd.AddCommand(newRuntimeCommand(out))
	return cmd
}

func newInitCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "init <workspace>",
		Short: "Initialize a production Ouroboros workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.InitWorkspace(args[0]); err != nil {
				return err
			}
			_, err := fmt.Fprintf(out, "initialized workspace: %s\n", args[0])
			return err
		},
	}
}

func newDoctorCommand(out io.Writer) *cobra.Command {
	var requireSafetyAuthorization bool
	var outputFormat string
	cmd := &cobra.Command{
		Use:   "doctor <workspace>",
		Short: "Validate workspace configuration and required paths",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := app.DoctorWorkspaceWithOptions(args[0], app.DoctorOptions{
				RequireSafetyAuthorization: requireSafetyAuthorization,
			})
			if err != nil {
				return err
			}
			format := strings.ToLower(strings.TrimSpace(outputFormat))
			if format == "" {
				format = "text"
			}
			if format == "json" {
				if err := printDoctorJSON(out, args[0], report, requireSafetyAuthorization); err != nil {
					return err
				}
				if !report.Ready {
					return fmt.Errorf("workspace is not ready")
				}
				return nil
			}
			if format != "text" {
				return fmt.Errorf("unsupported doctor format %q", outputFormat)
			}
			if report.Ready {
				if _, err := fmt.Fprintln(out, "workspace: ready"); err != nil {
					return err
				}
				return printDoctorWarnings(out, report.Warnings)
			}
			for _, issue := range report.Errors {
				if _, err := fmt.Fprintf(out, "workspace: %s\n", issue); err != nil {
					return err
				}
			}
			if err := printDoctorWarnings(out, report.Warnings); err != nil {
				return err
			}
			return fmt.Errorf("workspace is not ready")
		},
	}
	cmd.Flags().BoolVar(&requireSafetyAuthorization, "require-safety-authorization", false, "fail doctor when disabled safety boundaries lack authorization.yaml metadata")
	cmd.Flags().StringVar(&outputFormat, "format", "text", "output format: text or json")
	return cmd
}

type doctorJSONReport struct {
	SchemaVersion              int                      `json:"schema_version"`
	Workspace                  string                   `json:"workspace"`
	Ready                      bool                     `json:"ready"`
	Errors                     []string                 `json:"errors"`
	Warnings                   []string                 `json:"warnings"`
	SafetyBypasses             []app.DoctorSafetyBypass `json:"safety_bypasses"`
	StrictGate                 doctorJSONStrictGate     `json:"strict_gate"`
	RequireSafetyAuthorization bool                     `json:"require_safety_authorization"`
}

type doctorJSONStrictGate struct {
	Mode       string `json:"mode"`
	Status     string `json:"status"`
	ErrorCount int    `json:"error_count"`
}

func printDoctorJSON(out io.Writer, workspace string, report app.DoctorReport, requireSafetyAuthorization bool) error {
	payload := doctorJSONReport{
		SchemaVersion:              1,
		Workspace:                  workspace,
		Ready:                      report.Ready,
		Errors:                     append([]string{}, report.Errors...),
		Warnings:                   append([]string{}, report.Warnings...),
		SafetyBypasses:             append([]app.DoctorSafetyBypass{}, report.SafetyBypasses...),
		StrictGate:                 doctorJSONStrictGateFor(report, requireSafetyAuthorization),
		RequireSafetyAuthorization: requireSafetyAuthorization,
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(payload)
}

func doctorJSONStrictGateFor(report app.DoctorReport, requireSafetyAuthorization bool) doctorJSONStrictGate {
	if !requireSafetyAuthorization {
		return doctorJSONStrictGate{Mode: "warning", Status: "ready"}
	}
	errorCount := 0
	for _, bypass := range report.SafetyBypasses {
		if bypass.MissingAuthorization || bypass.ExpiredAuthorization {
			errorCount++
		}
	}
	status := "ready"
	if errorCount > 0 {
		status = "blocked"
	}
	return doctorJSONStrictGate{
		Mode:       "strict",
		Status:     status,
		ErrorCount: errorCount,
	}
}

func printDoctorWarnings(out io.Writer, warnings []string) error {
	for _, warning := range warnings {
		if _, err := fmt.Fprintf(out, "workspace: warning: %s\n", warning); err != nil {
			return err
		}
	}
	return nil
}

func newRunCommand(out io.Writer) *cobra.Command {
	var printTUI bool
	var interactive bool
	var deferReportProjection bool
	var requireSafetyAuthorization bool
	cmd := &cobra.Command{
		Use:   "run <workspace>",
		Short: "Run the production execution path for a workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if interactive {
				return runInteractiveWorkspace(context.Background(), args[0], out, printTUI, deferReportProjection, requireSafetyAuthorization)
			}
			result, err := app.RunWorkspaceWithOptions(context.Background(), args[0], app.RunOptions{
				DeferReportProjection:      deferReportProjection,
				RequireSafetyAuthorization: requireSafetyAuthorization,
			})
			if err != nil {
				return err
			}
			return printRunResult(out, result, printTUI)
		},
	}
	cmd.Flags().BoolVar(&printTUI, "tui", false, "Print a non-interactive TUI snapshot after the run")
	cmd.Flags().BoolVar(&interactive, "interactive", false, "Launch an interactive TUI after the run")
	cmd.Flags().BoolVar(&deferReportProjection, "defer-report-projection", false, "Skip synchronous report projection so it can be projected later")
	cmd.Flags().BoolVar(&requireSafetyAuthorization, "require-safety-authorization", false, "Fail run when disabled safety boundaries lack current authorization.yaml metadata")
	return cmd
}

type runOutcome struct {
	result app.RunResult
	err    error
}

func runInteractiveWorkspace(ctx context.Context, root string, out io.Writer, printTUI bool, deferReportProjection bool, requireSafetyAuthorization bool) error {
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	pauseController := app.NewRunPauseController()
	model := app.NewTUIModelForWorkspace(root).WithRuntimeControlRequester(func(action tui.RuntimeControlAction) (tui.RuntimeControlResult, error) {
		switch strings.TrimSpace(action.Kind) {
		case "cancel-run":
			cancelRun()
			return tui.RuntimeControlResult{Status: "requested", Message: "run context canceled"}, nil
		case "pause-run":
			pauseController.Pause()
			return tui.RuntimeControlResult{Status: "paused", Message: "worker admission paused"}, nil
		case "resume-run":
			pauseController.Resume()
			return tui.RuntimeControlResult{Status: "resumed", Message: "worker admission resumed"}, nil
		default:
			return tui.RuntimeControlResult{Status: "failed", Message: "unsupported runtime control"}, fmt.Errorf("unsupported runtime control: %s", action.Kind)
		}
	})
	session := newInteractiveSession(model)
	outcomes := make(chan runOutcome, 1)
	go func() {
		result, err := app.RunWorkspaceWithOptions(runCtx, root, app.RunOptions{
			PauseController:            pauseController,
			DeferReportProjection:      deferReportProjection,
			RequireSafetyAuthorization: requireSafetyAuthorization,
			OnSnapshot: func(snapshot eventlog.ReplaySnapshot) error {
				session.Send(tui.ReplaySnapshotMsg{Snapshot: snapshot})
				return nil
			},
		})
		if err != nil {
			session.Quit()
		}
		outcomes <- runOutcome{result: result, err: err}
	}()

	runErr := session.Run()
	outcome := <-outcomes
	if outcome.err != nil {
		return outcome.err
	}
	if err := printRunResult(out, outcome.result, printTUI); err != nil {
		return err
	}
	return runErr
}

func printRunResult(out io.Writer, result app.RunResult, printTUI bool) error {
	if _, err := fmt.Fprintf(out, "run: %s\n", result.RunID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "events: %d\n", result.EventCount); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "report: %s\n", replayReportDisplay(result.ReportPath)); err != nil {
		return err
	}
	if printTUI {
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(out, result.TUIView); err != nil {
			return err
		}
	}
	return nil
}

func newReportProjectCommand(out io.Writer) *cobra.Command {
	var write bool
	cmd := &cobra.Command{
		Use:   "report-project <workspace> <run-id>",
		Short: "Project a recorded run into a Markdown report",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := app.ProjectRunReport(context.Background(), args[0], args[1], app.ReportProjectionOptions{Write: write})
			if err != nil {
				return err
			}
			return printReportProjectionResult(out, result)
		},
	}
	cmd.Flags().BoolVar(&write, "write", false, "write the report and report_ready replay event")
	return cmd
}

func printReportProjectionResult(out io.Writer, result app.ReportProjectionResult) error {
	if _, err := fmt.Fprintf(out, "report-project: %s status=%s mode=%s written=%t\n", result.RunID, result.Status, result.Mode, result.Written); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "events: %d\n", result.EventCount); err != nil {
		return err
	}
	reportLabel := "planned_report"
	if result.Written {
		reportLabel = "report"
	}
	if _, err := fmt.Fprintf(out, "%s: %s\n", reportLabel, replayReportDisplay(result.ReportPath)); err != nil {
		return err
	}
	return nil
}

func newReportProjectDispatchCommand(out io.Writer) *cobra.Command {
	var actor string
	var reason string
	cmd := &cobra.Command{
		Use:   "report-project-dispatch <workspace> <run-id>",
		Short: "Persist a metadata-only report projection dispatch artifact",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			dispatch, err := app.CreateReportProjectionDispatch(context.Background(), args[0], args[1], app.ReportProjectionDispatchOptions{
				Actor:  actor,
				Reason: reason,
			})
			if err != nil {
				return err
			}
			return printReportProjectionDispatchCreateResult(out, dispatch)
		},
	}
	cmd.Flags().StringVar(&actor, "actor", "", "operator recorded on the dispatch artifact")
	cmd.Flags().StringVar(&reason, "reason", "", "reason recorded on the dispatch artifact")
	return cmd
}

func newReportProjectDispatchesCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "report-project-dispatches <workspace> [dispatch]",
		Short: "List or inspect report projection dispatch artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				dispatch, err := app.ReadReportProjectionDispatch(args[0], args[1])
				if err != nil {
					return err
				}
				return printReportProjectionDispatchArtifact(out, dispatch)
			}
			dispatches, err := app.ListReportProjectionDispatches(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "report-project-dispatches: total=%d\n", len(dispatches)); err != nil {
				return err
			}
			for _, dispatch := range dispatches {
				if err := printReportProjectionDispatchArtifact(out, dispatch); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func printReportProjectionDispatchCreateResult(out io.Writer, dispatch app.ReportProjectionDispatch) error {
	if _, err := fmt.Fprintf(out, "report-project-dispatch: %s status=%s mode=%s actor=%s\n", dispatch.RunID, dispatch.Status, dispatch.Mode, reportProjectionDisplayValue(dispatch.Actor, "operator")); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "events: %d\n", dispatch.EventCount); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "planned_report: %s\n", replayReportDisplay(dispatch.ReportPath)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "artifact: %s\n", dispatch.Path); err != nil {
		return err
	}
	return nil
}

func printReportProjectionDispatchArtifact(out io.Writer, dispatch app.ReportProjectionDispatch) error {
	_, err := fmt.Fprintf(
		out,
		"report-project-dispatch-artifact: %s status=%s mode=%s run=%s events=%d planned_report=%s actor=%s created_at=%s reason=%s\n",
		dispatch.Path,
		reportProjectionDisplayValue(dispatch.Status, "unknown"),
		reportProjectionDisplayValue(dispatch.Mode, "metadata-only"),
		dispatch.RunID,
		dispatch.EventCount,
		replayReportDisplay(dispatch.ReportPath),
		reportProjectionDisplayValue(dispatch.Actor, "operator"),
		reportProjectionDisplayValue(dispatch.CreatedAt, "unknown"),
		reportProjectionDisplayValue(dispatch.Reason, "none"),
	)
	return err
}

func newExecuteReportProjectDispatchCommand(out io.Writer) *cobra.Command {
	var execute bool
	var actor string
	var reason string
	cmd := &cobra.Command{
		Use:   "execute-report-project-dispatch <workspace> <dispatch>",
		Short: "Execute a persisted report projection dispatch artifact",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			execution, err := app.ExecuteReportProjectionDispatch(context.Background(), args[0], args[1], app.ReportProjectionDispatchExecutionOptions{
				Execute: execute,
				Actor:   actor,
				Reason:  reason,
			})
			if err != nil {
				return err
			}
			return printReportProjectionDispatchExecutionCreateResult(out, execution)
		},
	}
	cmd.Flags().BoolVar(&execute, "execute", false, "write the report through the persisted dispatch artifact")
	cmd.Flags().StringVar(&actor, "actor", "", "operator recorded on the execution artifact")
	cmd.Flags().StringVar(&reason, "reason", "", "reason recorded on the execution artifact")
	return cmd
}

func newReportProjectDispatchExecutionsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "report-project-dispatch-executions <workspace> [execution]",
		Short: "List or inspect report projection dispatch execution audit artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				execution, err := app.ReadReportProjectionDispatchExecution(args[0], args[1])
				if err != nil {
					return err
				}
				return printReportProjectionDispatchExecutionArtifact(out, execution)
			}
			executions, err := app.ListReportProjectionDispatchExecutions(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "report-project-dispatch-executions: total=%d\n", len(executions)); err != nil {
				return err
			}
			for _, execution := range executions {
				if err := printReportProjectionDispatchExecutionArtifact(out, execution); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newReportProjectDispatchExecutionAnnotateCommand(out io.Writer) *cobra.Command {
	var decision string
	var reviewer string
	var reviewedAtArg string
	var reason string
	cmd := &cobra.Command{
		Use:   "report-project-dispatch-execution-annotate <workspace> <execution>",
		Short: "Annotate a report projection dispatch execution artifact",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var reviewedAt time.Time
			if strings.TrimSpace(reviewedAtArg) != "" {
				parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(reviewedAtArg))
				if err != nil {
					return fmt.Errorf("reviewed-at must be RFC3339: %w", err)
				}
				reviewedAt = parsed
			}
			execution, err := app.AnnotateReportProjectionDispatchExecution(args[0], args[1], app.ReportProjectionDispatchExecutionAnnotationOptions{
				Decision:   decision,
				Reason:     reason,
				Reviewer:   reviewer,
				ReviewedAt: reviewedAt,
			})
			if err != nil {
				return err
			}
			return printReportProjectionDispatchExecutionCreateResult(out, execution)
		},
	}
	cmd.Flags().StringVar(&decision, "decision", "", "review decision: accepted, deferred, or rejected")
	cmd.Flags().StringVar(&reviewer, "reviewer", "", "reviewer identity; defaults to operator")
	cmd.Flags().StringVar(&reviewedAtArg, "reviewed-at", "", "review timestamp in RFC3339; defaults to now")
	cmd.Flags().StringVar(&reason, "reason", "", "review decision reason")
	return cmd
}

func newReportProjectDispatchExecutionSelectCommand(out io.Writer) *cobra.Command {
	var selectedBy string
	var selectedAtArg string
	var reason string
	cmd := &cobra.Command{
		Use:   "report-project-dispatch-execution-select <workspace> <execution>",
		Short: "Mark a written report projection dispatch execution as current",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var selectedAt time.Time
			if strings.TrimSpace(selectedAtArg) != "" {
				parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(selectedAtArg))
				if err != nil {
					return fmt.Errorf("selected-at must be RFC3339: %w", err)
				}
				selectedAt = parsed
			}
			selection, err := app.SelectReportProjectionDispatchExecution(args[0], args[1], app.ReportProjectionDispatchExecutionSelectionOptions{
				SelectedBy: selectedBy,
				SelectedAt: selectedAt,
				Reason:     reason,
			})
			if err != nil {
				if strings.Contains(err.Error(), "must be completed and written before selection") {
					return fmt.Errorf("report projection dispatch execution selection requires completed written execution: %w", err)
				}
				return err
			}
			return printReportProjectionDispatchExecutionSelection(out, selection)
		},
	}
	cmd.Flags().StringVar(&selectedBy, "selected-by", "", "operator identity; defaults to operator")
	cmd.Flags().StringVar(&selectedAtArg, "selected-at", "", "selection timestamp in RFC3339; defaults to now")
	cmd.Flags().StringVar(&reason, "reason", "", "selection reason")
	return cmd
}

func newReportProjectDispatchExecutionSelectionCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "report-project-dispatch-execution-selection <workspace>",
		Short: "Inspect the current report projection execution selection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			selection, found, err := app.ReadReportProjectionDispatchExecutionSelection(args[0])
			if err != nil {
				return err
			}
			if !found {
				_, err := fmt.Fprintln(out, "report-project-dispatch-execution-selection: none")
				return err
			}
			return printReportProjectionDispatchExecutionSelection(out, selection)
		},
	}
}

func newReportProjectDispatchExecutionSelectionClearCommand(out io.Writer) *cobra.Command {
	var clearedBy string
	var clearedAtArg string
	var reason string
	cmd := &cobra.Command{
		Use:   "report-project-dispatch-execution-selection-clear <workspace>",
		Short: "Clear the current report projection execution selection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var clearedAt time.Time
			if strings.TrimSpace(clearedAtArg) != "" {
				parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(clearedAtArg))
				if err != nil {
					return fmt.Errorf("cleared-at must be RFC3339: %w", err)
				}
				clearedAt = parsed
			}
			selection, err := app.ClearReportProjectionDispatchExecutionSelection(args[0], app.ReportProjectionDispatchExecutionSelectionClearOptions{
				ClearedBy: clearedBy,
				ClearedAt: clearedAt,
				Reason:    reason,
			})
			if err != nil {
				return err
			}
			return printReportProjectionDispatchExecutionSelection(out, selection)
		},
	}
	cmd.Flags().StringVar(&clearedBy, "cleared-by", "", "operator identity; defaults to operator")
	cmd.Flags().StringVar(&clearedAtArg, "cleared-at", "", "clear timestamp in RFC3339; defaults to now")
	cmd.Flags().StringVar(&reason, "reason", "", "clear reason")
	return cmd
}

func printReportProjectionDispatchExecutionCreateResult(out io.Writer, execution app.ReportProjectionDispatchExecution) error {
	if _, err := fmt.Fprintf(
		out,
		"report-project-dispatch-execution: %s status=%s mode=%s dispatch=%s run=%s events=%d written=%t actor=%s executed_at=%s reason=%s\n",
		execution.Path,
		reportProjectionDisplayValue(execution.Status, "unknown"),
		reportProjectionDisplayValue(execution.Mode, "execute"),
		execution.DispatchPath,
		execution.RunID,
		execution.EventCount,
		execution.Written,
		reportProjectionDisplayValue(execution.Actor, "operator"),
		reportProjectionDisplayValue(execution.ExecutedAt, "unknown"),
		reportProjectionDisplayValue(execution.Reason, "none"),
	); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "report: %s\n", replayReportDisplay(execution.ReportPath)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "artifact: %s\n", execution.Path); err != nil {
		return err
	}
	if err := printReportProjectionDispatchExecutionAnnotation(out, execution.Annotation); err != nil {
		return err
	}
	return nil
}

func printReportProjectionDispatchExecutionArtifact(out io.Writer, execution app.ReportProjectionDispatchExecution) error {
	if _, err := fmt.Fprintf(
		out,
		"report-project-dispatch-execution-artifact: %s status=%s mode=%s dispatch=%s run=%s events=%d report=%s written=%t actor=%s executed_at=%s reason=%s\n",
		execution.Path,
		reportProjectionDisplayValue(execution.Status, "unknown"),
		reportProjectionDisplayValue(execution.Mode, "execute"),
		execution.DispatchPath,
		execution.RunID,
		execution.EventCount,
		replayReportDisplay(execution.ReportPath),
		execution.Written,
		reportProjectionDisplayValue(execution.Actor, "operator"),
		reportProjectionDisplayValue(execution.ExecutedAt, "unknown"),
		reportProjectionDisplayValue(execution.Reason, "none"),
	); err != nil {
		return err
	}
	return printReportProjectionDispatchExecutionAnnotation(out, execution.Annotation)
}

func printReportProjectionDispatchExecutionAnnotation(out io.Writer, annotation *app.ReportProjectionDispatchExecutionAnnotation) error {
	if annotation == nil {
		return nil
	}
	_, err := fmt.Fprintf(
		out,
		"report-project-dispatch-execution-annotation: decision=%s reviewer=%s reviewed_at=%s reason=%s\n",
		reportProjectionDisplayValue(annotation.Decision, "unknown"),
		reportProjectionDisplayValue(annotation.Reviewer, "operator"),
		reportProjectionDisplayValue(annotation.ReviewedAt, "unknown"),
		reportProjectionDisplayValue(annotation.Reason, "none"),
	)
	return err
}

func printReportProjectionDispatchExecutionSelection(out io.Writer, selection app.ReportProjectionDispatchExecutionSelection) error {
	actorFields := fmt.Sprintf("selected_by=%s selected_at=%s", reportProjectionDisplayValue(selection.SelectedBy, "operator"), reportProjectionDisplayValue(selection.SelectedAt, "unknown"))
	if selection.Status == "cleared" {
		actorFields = fmt.Sprintf("cleared_by=%s cleared_at=%s", reportProjectionDisplayValue(selection.ClearedBy, "operator"), reportProjectionDisplayValue(selection.ClearedAt, "unknown"))
	}
	if _, err := fmt.Fprintf(
		out,
		"report-project-dispatch-execution-selection: status=%s mode=%s execution=%s dispatch=%s run=%s report=%s written=%t events=%d superseded=%d %s reason=%s\n",
		reportProjectionDisplayValue(selection.Status, "unknown"),
		reportProjectionDisplayValue(selection.Mode, "metadata-only"),
		selection.ExecutionPath,
		selection.DispatchPath,
		selection.RunID,
		replayReportDisplay(selection.ReportPath),
		selection.Written,
		selection.EventCount,
		len(selection.SupersededPaths),
		actorFields,
		reportProjectionDisplayValue(selection.Reason, "none"),
	); err != nil {
		return err
	}
	for _, path := range selection.SupersededPaths {
		if _, err := fmt.Fprintf(out, "report-project-dispatch-execution-superseded: %s\n", path); err != nil {
			return err
		}
	}
	return nil
}

func reportProjectionDisplayValue(value string, fallback string) string {
	value = strings.Join(strings.Fields(value), "_")
	if value == "" {
		return fallback
	}
	return value
}

func newReplayCommand(out io.Writer) *cobra.Command {
	var printTUI bool
	var interactive bool
	var format string
	cmd := &cobra.Command{
		Use:   "replay <workspace> <run-id>",
		Short: "Replay a recorded Ouroboros run",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := app.ReplayWorkspace(context.Background(), args[0], args[1])
			if err != nil {
				return err
			}
			switch strings.TrimSpace(format) {
			case "", "text":
				return printReplayText(out, result, printTUI, interactive)
			case "json":
				if printTUI || interactive {
					return fmt.Errorf("replay --format json cannot be combined with --tui or --interactive")
				}
				return printReplayJSON(out, result)
			case "ndjson":
				if printTUI || interactive {
					return fmt.Errorf("replay --format ndjson cannot be combined with --tui or --interactive")
				}
				return printReplayNDJSON(out, result)
			case "html":
				if printTUI || interactive {
					return fmt.Errorf("replay --format html cannot be combined with --tui or --interactive")
				}
				return printReplayHTML(out, result)
			default:
				return fmt.Errorf("unsupported replay format %q; expected text, json, ndjson, or html", format)
			}
		},
	}
	cmd.Flags().BoolVar(&printTUI, "tui", false, "Print a non-interactive TUI snapshot for the replay")
	cmd.Flags().BoolVar(&interactive, "interactive", false, "Launch an interactive TUI for the replay")
	cmd.Flags().StringVar(&format, "format", "text", "Replay output format: text, json, ndjson, or html")
	return cmd
}

func newReplayWorkbenchCommand(out io.Writer) *cobra.Command {
	var listen string
	var maxRequests int
	var compareRuns []string
	var sessionPath string
	var currentSession bool
	var currentCollaboration bool
	var currentCollaborationNote bool
	var enableSessionSelectionWrites bool
	var enableCollaborationSelectionWrites bool
	var enableCollaborationPresenceWrites bool
	var enableCollaborationNoteWrites bool
	var enableCollaborationNoteSelectionWrites bool
	var enableReportSelectionWrites bool
	var enableCollaborationStream bool
	var collaborationStreamRefreshIntervalMS int
	var sessionSelectionPollIntervalMS int
	var collaborationSelectionPollIntervalMS int
	var collaborationPresencePollIntervalMS int
	var collaborationNotesPollIntervalMS int
	var collaborationNoteSelectionPollIntervalMS int
	cmd := &cobra.Command{
		Use:   "replay-workbench <workspace> <run-id>",
		Short: "Serve a local read-only replay workbench",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireNonNegativeRuntimeBound("max-requests", maxRequests); err != nil {
				return err
			}
			if sessionSelectionPollIntervalMS < 0 {
				return fmt.Errorf("--session-selection-poll-interval-ms cannot be negative")
			}
			if collaborationSelectionPollIntervalMS < 0 {
				return fmt.Errorf("--collaboration-selection-poll-interval-ms cannot be negative")
			}
			if collaborationPresencePollIntervalMS < 0 {
				return fmt.Errorf("--collaboration-presence-poll-interval-ms cannot be negative")
			}
			if collaborationNotesPollIntervalMS < 0 {
				return fmt.Errorf("--collaboration-notes-poll-interval-ms cannot be negative")
			}
			if collaborationNoteSelectionPollIntervalMS < 0 {
				return fmt.Errorf("--collaboration-note-selection-poll-interval-ms cannot be negative")
			}
			if collaborationStreamRefreshIntervalMS < 0 {
				return fmt.Errorf("--collaboration-stream-refresh-interval-ms cannot be negative")
			}
			if strings.TrimSpace(sessionPath) != "" && currentSession {
				return fmt.Errorf("--session and --current-session cannot be combined")
			}
			if strings.TrimSpace(sessionPath) != "" && currentCollaboration {
				return fmt.Errorf("--session and --current-collaboration cannot be combined")
			}
			if strings.TrimSpace(sessionPath) != "" && currentCollaborationNote {
				return fmt.Errorf("--session and --current-collaboration-note cannot be combined")
			}
			if currentSession && currentCollaboration {
				return fmt.Errorf("--current-session and --current-collaboration cannot be combined")
			}
			if currentSession && currentCollaborationNote {
				return fmt.Errorf("--current-session and --current-collaboration-note cannot be combined")
			}
			if currentCollaboration && currentCollaborationNote {
				return fmt.Errorf("--current-collaboration and --current-collaboration-note cannot be combined")
			}
			if currentSession {
				selection, found, err := app.ReadReplayWorkbenchSessionSelection(args[0])
				if err != nil {
					return err
				}
				if !found {
					return fmt.Errorf("current replay workbench session selection was not found")
				}
				if strings.TrimSpace(selection.Status) != "current" || strings.TrimSpace(selection.SessionPath) == "" {
					return fmt.Errorf("current replay workbench session selection is not current")
				}
				sessionPath = selection.SessionPath
			}
			if currentCollaboration {
				selection, found, err := app.ReadReplayWorkbenchCollaborationSelection(args[0])
				if err != nil {
					return err
				}
				if !found {
					return fmt.Errorf("current replay workbench collaboration selection was not found")
				}
				if strings.TrimSpace(selection.Status) != "current" || strings.TrimSpace(selection.CollaborationPath) == "" {
					return fmt.Errorf("current replay workbench collaboration selection is not current")
				}
				collaboration, err := app.ReadReplayWorkbenchCollaboration(args[0], selection.CollaborationPath)
				if err != nil {
					return err
				}
				if strings.TrimSpace(collaboration.SessionPath) == "" {
					return fmt.Errorf("current replay workbench collaboration session path is empty")
				}
				sessionPath = collaboration.SessionPath
			}
			if currentCollaborationNote {
				selection, found, err := app.ReadReplayWorkbenchCollaborationNoteSelection(args[0])
				if err != nil {
					return err
				}
				if !found {
					return fmt.Errorf("current replay workbench collaboration note selection was not found")
				}
				if strings.TrimSpace(selection.Status) != "current" || strings.TrimSpace(selection.NotePath) == "" {
					return fmt.Errorf("current replay workbench collaboration note selection is not current")
				}
				note, err := app.ReadReplayWorkbenchCollaborationNote(args[0], selection.NotePath)
				if err != nil {
					return err
				}
				if strings.TrimSpace(note.CollaborationPath) == "" {
					return fmt.Errorf("current replay workbench collaboration note collaboration path is empty")
				}
				collaboration, err := app.ReadReplayWorkbenchCollaboration(args[0], note.CollaborationPath)
				if err != nil {
					return err
				}
				if strings.TrimSpace(collaboration.SessionPath) == "" {
					return fmt.Errorf("current replay workbench collaboration note session path is empty")
				}
				sessionPath = collaboration.SessionPath
			}
			var defaultSession replayWorkbenchSession
			var savedSession app.ReplayWorkbenchSessionArtifact
			if strings.TrimSpace(sessionPath) != "" {
				var err error
				savedSession, err = app.ReadReplayWorkbenchSession(args[0], sessionPath)
				if err != nil {
					return err
				}
				if savedSession.PrimaryRunID != args[1] {
					return fmt.Errorf("saved replay workbench session primary run %s does not match requested run %s", savedSession.PrimaryRunID, args[1])
				}
				compareRuns = mergeReplayWorkbenchCompareRuns(savedSession, compareRuns)
				defaultSession = replayWorkbenchSessionFromArtifact(savedSession)
			}
			result, err := app.ReplayWorkspace(context.Background(), args[0], args[1])
			if err != nil {
				return err
			}
			comparisons := make([]app.ReplayResult, 0, len(compareRuns))
			for _, runID := range compareRuns {
				runID = strings.TrimSpace(runID)
				if runID == "" {
					return fmt.Errorf("--compare-run requires a run id")
				}
				compareResult, err := app.ReplayWorkspace(context.Background(), args[0], runID)
				if err != nil {
					return err
				}
				comparisons = append(comparisons, compareResult)
			}
			savedSessions, err := app.ListReplayWorkbenchSessions(args[0])
			if err != nil {
				return err
			}
			collaborations, err := app.ListReplayWorkbenchCollaborations(args[0])
			if err != nil {
				return err
			}
			collaborationPresence, err := app.ListReplayWorkbenchCollaborationPresence(args[0])
			if err != nil {
				return err
			}
			collaborationNotes, err := app.ListReplayWorkbenchCollaborationNotes(args[0])
			if err != nil {
				return err
			}
			collaborationSelection, collaborationSelectionFound, err := app.ReadReplayWorkbenchCollaborationSelection(args[0])
			if err != nil {
				return err
			}
			var collaborationSelectionPtr *app.ReplayWorkbenchCollaborationSelection
			if collaborationSelectionFound {
				collaborationSelectionPtr = &collaborationSelection
			}
			collaborationNoteSelection, collaborationNoteSelectionFound, err := app.ReadReplayWorkbenchCollaborationNoteSelection(args[0])
			if err != nil {
				return err
			}
			var collaborationNoteSelectionPtr *app.ReplayWorkbenchCollaborationNoteSelection
			if collaborationNoteSelectionFound {
				collaborationNoteSelectionPtr = &collaborationNoteSelection
			}
			selection, selectionFound, err := app.ReadReplayWorkbenchSessionSelection(args[0])
			if err != nil {
				return err
			}
			var sessionSelection *app.ReplayWorkbenchSessionSelection
			if selectionFound {
				sessionSelection = &selection
			}
			reportExecutions, err := app.ListReportProjectionDispatchExecutions(args[0])
			if err != nil {
				return err
			}
			reportSelection, reportSelectionFound, err := app.ReadReportProjectionDispatchExecutionSelection(args[0])
			if err != nil {
				return err
			}
			var reportSelectionPtr *app.ReportProjectionDispatchExecutionSelection
			if reportSelectionFound {
				reportSelectionPtr = &reportSelection
			}
			listener, err := net.Listen("tcp", listen)
			if err != nil {
				return fmt.Errorf("listen for replay workbench: %w", err)
			}
			baseHandler := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
				WorkspaceRoot:                            args[0],
				DefaultSession:                           defaultSession,
				SavedSessions:                            savedSessions,
				SessionSelection:                         sessionSelection,
				Collaborations:                           collaborations,
				CollaborationPresence:                    collaborationPresence,
				CollaborationNotes:                       collaborationNotes,
				CollaborationSelection:                   collaborationSelectionPtr,
				CollaborationNoteSelection:               collaborationNoteSelectionPtr,
				ReportProjectionExecutions:               reportExecutions,
				ReportProjectionSelection:                reportSelectionPtr,
				AllowReportSelectionWrites:               enableReportSelectionWrites,
				AllowSessionSelectionWrites:              enableSessionSelectionWrites,
				AllowCollaborationSelectionWrites:        enableCollaborationSelectionWrites,
				AllowCollaborationPresenceWrites:         enableCollaborationPresenceWrites,
				AllowCollaborationNoteWrites:             enableCollaborationNoteWrites,
				AllowCollaborationNoteSelectionWrites:    enableCollaborationNoteSelectionWrites,
				EnableCollaborationStream:                enableCollaborationStream,
				CollaborationStreamRefreshIntervalMS:     collaborationStreamRefreshIntervalMS,
				SessionSelectionPollIntervalMS:           sessionSelectionPollIntervalMS,
				CollaborationSelectionPollIntervalMS:     collaborationSelectionPollIntervalMS,
				CollaborationPresencePollIntervalMS:      collaborationPresencePollIntervalMS,
				CollaborationNotesPollIntervalMS:         collaborationNotesPollIntervalMS,
				CollaborationNoteSelectionPollIntervalMS: collaborationNoteSelectionPollIntervalMS,
			}, result, comparisons...)
			server := &http.Server{}
			var mu sync.Mutex
			accepted := 0
			server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				recorder := &statusCaptureResponseWriter{ResponseWriter: w, status: http.StatusOK}
				baseHandler.ServeHTTP(recorder, r)
				if recorder.status < 200 || recorder.status > 299 {
					return
				}
				mu.Lock()
				accepted++
				shouldStop := maxRequests > 0 && accepted >= maxRequests
				mu.Unlock()
				if shouldStop {
					go func() {
						_ = server.Shutdown(context.Background())
					}()
				}
			})
			if _, err := fmt.Fprintf(out, "replay-workbench: listen=http://%s\n", listener.Addr().String()); err != nil {
				_ = listener.Close()
				return err
			}
			if savedSession.Path != "" {
				if _, err := fmt.Fprintf(out, "replay-workbench: session=%s\n", savedSession.Path); err != nil {
					_ = listener.Close()
					return err
				}
			}
			err = server.Serve(listener)
			if errors.Is(err, http.ErrServerClosed) {
				err = nil
			}
			mu.Lock()
			finalAccepted := accepted
			mu.Unlock()
			if _, printErr := fmt.Fprintf(out, "replay-workbench: accepted=%d\n", finalAccepted); printErr != nil {
				return printErr
			}
			return err
		},
	}
	cmd.Flags().StringVar(&listen, "listen", "127.0.0.1:0", "TCP listen address for the local replay workbench")
	cmd.Flags().IntVar(&maxRequests, "max-requests", 0, "stop after serving this many successful requests; 0 runs until interrupted")
	cmd.Flags().StringArrayVar(&compareRuns, "compare-run", nil, "Additional run id to include in replay comparison data; repeatable")
	cmd.Flags().StringVar(&sessionPath, "session", "", "Saved replay workbench session artifact to restore default filters and comparison runs")
	cmd.Flags().BoolVar(&currentSession, "current-session", false, "Restore the current saved replay workbench session selection")
	cmd.Flags().BoolVar(&currentCollaboration, "current-collaboration", false, "Restore the saved replay workbench session referenced by the current collaboration selection")
	cmd.Flags().BoolVar(&currentCollaborationNote, "current-collaboration-note", false, "Restore the saved replay workbench session referenced by the current collaboration note selection")
	cmd.Flags().BoolVar(&enableSessionSelectionWrites, "enable-session-selection-writes", false, "Allow local browser POST updates to saved session selection metadata")
	cmd.Flags().BoolVar(&enableCollaborationSelectionWrites, "enable-collaboration-selection-writes", false, "Allow local browser POST updates to collaboration selection metadata")
	cmd.Flags().BoolVar(&enableCollaborationPresenceWrites, "enable-collaboration-presence-writes", false, "Allow local browser POST updates to collaboration presence metadata")
	cmd.Flags().BoolVar(&enableCollaborationNoteWrites, "enable-collaboration-note-writes", false, "Allow local browser POST updates to collaboration note metadata")
	cmd.Flags().BoolVar(&enableCollaborationNoteSelectionWrites, "enable-collaboration-note-selection-writes", false, "Allow local browser POST updates to collaboration note selection metadata")
	cmd.Flags().BoolVar(&enableReportSelectionWrites, "enable-report-selection-writes", false, "Allow local browser POST updates to report selection metadata")
	cmd.Flags().BoolVar(&enableCollaborationStream, "enable-collaboration-stream", false, "Allow local browser EventSource streaming of collaboration metadata snapshots")
	cmd.Flags().IntVar(&collaborationStreamRefreshIntervalMS, "collaboration-stream-refresh-interval-ms", 0, "Refresh collaboration stream snapshots from workspace metadata at this interval in milliseconds; 0 disables server-side refresh")
	cmd.Flags().IntVar(&sessionSelectionPollIntervalMS, "session-selection-poll-interval-ms", 0, "Refresh saved session selection from the local read-only API at this interval in milliseconds; 0 disables polling")
	cmd.Flags().IntVar(&collaborationSelectionPollIntervalMS, "collaboration-selection-poll-interval-ms", 0, "Refresh collaboration selection from the local read-only API at this interval in milliseconds; 0 disables polling")
	cmd.Flags().IntVar(&collaborationPresencePollIntervalMS, "collaboration-presence-poll-interval-ms", 0, "Refresh collaboration presence from the local read-only API at this interval in milliseconds; 0 disables polling")
	cmd.Flags().IntVar(&collaborationNotesPollIntervalMS, "collaboration-notes-poll-interval-ms", 0, "Refresh collaboration notes from the local read-only API at this interval in milliseconds; 0 disables polling")
	cmd.Flags().IntVar(&collaborationNoteSelectionPollIntervalMS, "collaboration-note-selection-poll-interval-ms", 0, "Refresh collaboration note selection from the local read-only API at this interval in milliseconds; 0 disables polling")
	return cmd
}

func newReplayWorkbenchSessionCommand(out io.Writer) *cobra.Command {
	var compareRuns []string
	var selectedRun string
	var lane string
	var query string
	var node string
	var edge string
	var createdBy string
	var reason string
	var write bool
	cmd := &cobra.Command{
		Use:   "replay-workbench-session <workspace> <run-id>",
		Short: "Create or preview a saved replay workbench session",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			options := app.ReplayWorkbenchSessionOptions{
				PrimaryRunID:  args[1],
				CompareRunIDs: compareRuns,
				SelectedRunID: selectedRun,
				Filters: map[string]string{
					"lane": lane,
					"q":    query,
					"node": node,
					"edge": edge,
				},
				CreatedBy: createdBy,
				Reason:    reason,
			}
			var (
				session app.ReplayWorkbenchSessionArtifact
				err     error
			)
			if write {
				session, err = app.CreateReplayWorkbenchSession(context.Background(), args[0], options)
			} else {
				session, err = app.PreviewReplayWorkbenchSession(context.Background(), args[0], options)
			}
			if err != nil {
				return err
			}
			return printReplayWorkbenchSessionArtifact(out, session, true)
		},
	}
	cmd.Flags().StringArrayVar(&compareRuns, "compare-run", nil, "Additional run id to include in saved replay comparison data; repeatable")
	cmd.Flags().StringVar(&selectedRun, "selected-run", "", "Run id selected by default when the workbench session opens")
	cmd.Flags().StringVar(&lane, "lane", "", "Saved timeline lane filter")
	cmd.Flags().StringVar(&query, "q", "", "Saved timeline or graph search query")
	cmd.Flags().StringVar(&node, "node", "", "Saved Evidence Graph node focus")
	cmd.Flags().StringVar(&edge, "edge", "", "Saved Evidence Graph edge focus")
	cmd.Flags().StringVar(&createdBy, "created-by", "", "Actor recorded on written session artifacts")
	cmd.Flags().StringVar(&reason, "reason", "", "Reason recorded on written session artifacts")
	cmd.Flags().BoolVar(&write, "write", false, "Persist the session under workspace/replay/workbench-sessions/")
	return cmd
}

func newReplayWorkbenchSessionAnnotateCommand(out io.Writer) *cobra.Command {
	var decision string
	var reason string
	var reviewer string
	var reviewedAtArg string
	cmd := &cobra.Command{
		Use:   "replay-workbench-session-annotate <workspace> <artifact>",
		Short: "Annotate a saved metadata-only replay workbench session",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var reviewedAt time.Time
			if strings.TrimSpace(reviewedAtArg) != "" {
				parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(reviewedAtArg))
				if err != nil {
					return fmt.Errorf("reviewed-at must be RFC3339: %w", err)
				}
				reviewedAt = parsed
			}
			session, err := app.AnnotateReplayWorkbenchSession(args[0], args[1], app.ReplayWorkbenchSessionAnnotationOptions{
				Decision:   decision,
				Reason:     reason,
				Reviewer:   reviewer,
				ReviewedAt: reviewedAt,
			})
			if err != nil {
				return err
			}
			return printReplayWorkbenchSessionArtifact(out, session, true)
		},
	}
	cmd.Flags().StringVar(&decision, "decision", "", "review decision: accepted, deferred, or rejected")
	cmd.Flags().StringVar(&reason, "reason", "", "review decision reason")
	cmd.Flags().StringVar(&reviewer, "reviewer", "", "reviewer identity; defaults to operator")
	cmd.Flags().StringVar(&reviewedAtArg, "reviewed-at", "", "review timestamp in RFC3339; defaults to now")
	return cmd
}

func newReplayWorkbenchSessionSelectCommand(out io.Writer) *cobra.Command {
	var selectedBy string
	var selectedAtArg string
	var reason string
	cmd := &cobra.Command{
		Use:   "replay-workbench-session-select <workspace> <artifact>",
		Short: "Mark a saved replay workbench session as the current metadata-only selection",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var selectedAt time.Time
			if strings.TrimSpace(selectedAtArg) != "" {
				parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(selectedAtArg))
				if err != nil {
					return fmt.Errorf("selected-at must be RFC3339: %w", err)
				}
				selectedAt = parsed
			}
			selection, err := app.SelectReplayWorkbenchSession(args[0], args[1], app.ReplayWorkbenchSessionSelectionOptions{
				SelectedBy: selectedBy,
				SelectedAt: selectedAt,
				Reason:     reason,
			})
			if err != nil {
				return err
			}
			return printReplayWorkbenchSessionSelection(out, selection)
		},
	}
	cmd.Flags().StringVar(&selectedBy, "selected-by", "", "operator identity; defaults to operator")
	cmd.Flags().StringVar(&selectedAtArg, "selected-at", "", "selection timestamp in RFC3339; defaults to now")
	cmd.Flags().StringVar(&reason, "reason", "", "selection reason")
	return cmd
}

func newReplayWorkbenchSessionSelectionCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "replay-workbench-session-selection <workspace>",
		Short: "Inspect the current metadata-only replay workbench session selection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			selection, found, err := app.ReadReplayWorkbenchSessionSelection(args[0])
			if err != nil {
				return err
			}
			if !found {
				_, err := fmt.Fprintln(out, "replay-workbench-session-selection: none")
				return err
			}
			return printReplayWorkbenchSessionSelection(out, selection)
		},
	}
}

func newReplayWorkbenchSessionSelectionClearCommand(out io.Writer) *cobra.Command {
	var clearedBy string
	var clearedAtArg string
	var reason string
	cmd := &cobra.Command{
		Use:   "replay-workbench-session-selection-clear <workspace>",
		Short: "Clear the current metadata-only replay workbench session selection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var clearedAt time.Time
			if strings.TrimSpace(clearedAtArg) != "" {
				parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(clearedAtArg))
				if err != nil {
					return fmt.Errorf("cleared-at must be RFC3339: %w", err)
				}
				clearedAt = parsed
			}
			selection, err := app.ClearReplayWorkbenchSessionSelection(args[0], app.ReplayWorkbenchSessionSelectionClearOptions{
				ClearedBy: clearedBy,
				ClearedAt: clearedAt,
				Reason:    reason,
			})
			if err != nil {
				return err
			}
			return printReplayWorkbenchSessionSelection(out, selection)
		},
	}
	cmd.Flags().StringVar(&clearedBy, "cleared-by", "", "operator identity; defaults to operator")
	cmd.Flags().StringVar(&clearedAtArg, "cleared-at", "", "clear timestamp in RFC3339; defaults to now")
	cmd.Flags().StringVar(&reason, "reason", "", "clear reason")
	return cmd
}

func newReplayWorkbenchSessionsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "replay-workbench-sessions <workspace> [artifact]",
		Short: "List or inspect saved replay workbench sessions",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				session, err := app.ReadReplayWorkbenchSession(args[0], args[1])
				if err != nil {
					return err
				}
				return printReplayWorkbenchSessionArtifact(out, session, true)
			}
			sessions, err := app.ListReplayWorkbenchSessions(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "replay-workbench-sessions: total=%d\n", len(sessions)); err != nil {
				return err
			}
			for _, session := range sessions {
				if err := printReplayWorkbenchSessionArtifact(out, session, false); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newReplayWorkbenchCollaborationCommand(out io.Writer) *cobra.Command {
	var participants []string
	var createdBy string
	var createdAtArg string
	var reason string
	var write bool
	cmd := &cobra.Command{
		Use:   "replay-workbench-collaboration <workspace> <session-artifact>",
		Short: "Create or preview a metadata-only replay workbench collaboration session",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var createdAt time.Time
			if strings.TrimSpace(createdAtArg) != "" {
				parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(createdAtArg))
				if err != nil {
					return fmt.Errorf("created-at must be RFC3339: %w", err)
				}
				createdAt = parsed
			}
			options := app.ReplayWorkbenchCollaborationOptions{
				SessionPath:  args[1],
				Participants: participants,
				CreatedBy:    createdBy,
				CreatedAt:    createdAt,
				Reason:       reason,
			}
			var (
				collaboration app.ReplayWorkbenchCollaborationArtifact
				err           error
			)
			if write {
				collaboration, err = app.CreateReplayWorkbenchCollaboration(context.Background(), args[0], options)
			} else {
				collaboration, err = app.PreviewReplayWorkbenchCollaboration(context.Background(), args[0], options)
			}
			if err != nil {
				return err
			}
			return printReplayWorkbenchCollaborationArtifact(out, collaboration, true)
		},
	}
	cmd.Flags().StringArrayVar(&participants, "participant", nil, "Participant identity to include in the collaboration metadata; repeatable")
	cmd.Flags().StringVar(&createdBy, "created-by", "", "Actor recorded on written collaboration artifacts")
	cmd.Flags().StringVar(&createdAtArg, "created-at", "", "Creation timestamp in RFC3339; defaults to now")
	cmd.Flags().StringVar(&reason, "reason", "", "Reason recorded on written collaboration artifacts")
	cmd.Flags().BoolVar(&write, "write", false, "Persist the collaboration under workspace/replay/workbench-collaborations/")
	return cmd
}

func newReplayWorkbenchCollaborationAnnotateCommand(out io.Writer) *cobra.Command {
	var decision string
	var reason string
	var reviewer string
	var reviewedAtArg string
	cmd := &cobra.Command{
		Use:   "replay-workbench-collaboration-annotate <workspace> <artifact>",
		Short: "Annotate a saved metadata-only replay workbench collaboration",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var reviewedAt time.Time
			if strings.TrimSpace(reviewedAtArg) != "" {
				parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(reviewedAtArg))
				if err != nil {
					return fmt.Errorf("reviewed-at must be RFC3339: %w", err)
				}
				reviewedAt = parsed
			}
			collaboration, err := app.AnnotateReplayWorkbenchCollaboration(args[0], args[1], app.ReplayWorkbenchCollaborationAnnotationOptions{
				Decision:   decision,
				Reason:     reason,
				Reviewer:   reviewer,
				ReviewedAt: reviewedAt,
			})
			if err != nil {
				return err
			}
			return printReplayWorkbenchCollaborationArtifact(out, collaboration, true)
		},
	}
	cmd.Flags().StringVar(&decision, "decision", "", "review decision: accepted, deferred, or rejected")
	cmd.Flags().StringVar(&reason, "reason", "", "review decision reason")
	cmd.Flags().StringVar(&reviewer, "reviewer", "", "reviewer identity; defaults to operator")
	cmd.Flags().StringVar(&reviewedAtArg, "reviewed-at", "", "review timestamp in RFC3339; defaults to now")
	return cmd
}

func newReplayWorkbenchCollaborationSelectCommand(out io.Writer) *cobra.Command {
	var selectedBy string
	var selectedAtArg string
	var reason string
	cmd := &cobra.Command{
		Use:   "replay-workbench-collaboration-select <workspace> <artifact>",
		Short: "Mark a replay workbench collaboration as the current metadata-only selection",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var selectedAt time.Time
			if strings.TrimSpace(selectedAtArg) != "" {
				parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(selectedAtArg))
				if err != nil {
					return fmt.Errorf("selected-at must be RFC3339: %w", err)
				}
				selectedAt = parsed
			}
			selection, err := app.SelectReplayWorkbenchCollaboration(args[0], args[1], app.ReplayWorkbenchCollaborationSelectionOptions{
				SelectedBy: selectedBy,
				SelectedAt: selectedAt,
				Reason:     reason,
			})
			if err != nil {
				return err
			}
			return printReplayWorkbenchCollaborationSelection(out, selection)
		},
	}
	cmd.Flags().StringVar(&selectedBy, "selected-by", "", "operator identity; defaults to operator")
	cmd.Flags().StringVar(&selectedAtArg, "selected-at", "", "selection timestamp in RFC3339; defaults to now")
	cmd.Flags().StringVar(&reason, "reason", "", "selection reason")
	return cmd
}

func newReplayWorkbenchCollaborationSelectionCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "replay-workbench-collaboration-selection <workspace>",
		Short: "Inspect the current metadata-only replay workbench collaboration selection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			selection, found, err := app.ReadReplayWorkbenchCollaborationSelection(args[0])
			if err != nil {
				return err
			}
			if !found {
				_, err := fmt.Fprintln(out, "replay-workbench-collaboration-selection: none")
				return err
			}
			return printReplayWorkbenchCollaborationSelection(out, selection)
		},
	}
}

func newReplayWorkbenchCollaborationSelectionClearCommand(out io.Writer) *cobra.Command {
	var clearedBy string
	var clearedAtArg string
	var reason string
	cmd := &cobra.Command{
		Use:   "replay-workbench-collaboration-selection-clear <workspace>",
		Short: "Clear the current metadata-only replay workbench collaboration selection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var clearedAt time.Time
			if strings.TrimSpace(clearedAtArg) != "" {
				parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(clearedAtArg))
				if err != nil {
					return fmt.Errorf("cleared-at must be RFC3339: %w", err)
				}
				clearedAt = parsed
			}
			selection, err := app.ClearReplayWorkbenchCollaborationSelection(args[0], app.ReplayWorkbenchCollaborationSelectionClearOptions{
				ClearedBy: clearedBy,
				ClearedAt: clearedAt,
				Reason:    reason,
			})
			if err != nil {
				return err
			}
			return printReplayWorkbenchCollaborationSelection(out, selection)
		},
	}
	cmd.Flags().StringVar(&clearedBy, "cleared-by", "", "operator identity; defaults to operator")
	cmd.Flags().StringVar(&clearedAtArg, "cleared-at", "", "clear timestamp in RFC3339; defaults to now")
	cmd.Flags().StringVar(&reason, "reason", "", "clear reason")
	return cmd
}

func newReplayWorkbenchCollaborationPresenceCommand(out io.Writer) *cobra.Command {
	var participant string
	var status string
	var seenBy string
	var seenAtArg string
	var reason string
	var write bool
	cmd := &cobra.Command{
		Use:   "replay-workbench-collaboration-presence <workspace> <collaboration-artifact>",
		Short: "Create or preview metadata-only replay workbench collaboration presence",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var seenAt time.Time
			if strings.TrimSpace(seenAtArg) != "" {
				parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(seenAtArg))
				if err != nil {
					return fmt.Errorf("seen-at must be RFC3339: %w", err)
				}
				seenAt = parsed
			}
			options := app.ReplayWorkbenchCollaborationPresenceOptions{
				CollaborationPath: args[1],
				Participant:       participant,
				Status:            status,
				SeenBy:            seenBy,
				SeenAt:            seenAt,
				Reason:            reason,
			}
			var (
				presence app.ReplayWorkbenchCollaborationPresence
				err      error
			)
			if write {
				presence, err = app.UpdateReplayWorkbenchCollaborationPresence(context.Background(), args[0], options)
			} else {
				presence, err = app.PreviewReplayWorkbenchCollaborationPresence(context.Background(), args[0], options)
			}
			if err != nil {
				return err
			}
			return printReplayWorkbenchCollaborationPresence(out, presence, true)
		},
	}
	cmd.Flags().StringVar(&participant, "participant", "", "Participant identity for the presence metadata")
	cmd.Flags().StringVar(&status, "status", "online", "Presence status: online, away, or offline")
	cmd.Flags().StringVar(&seenBy, "seen-by", "", "Actor recording the presence update; defaults to participant")
	cmd.Flags().StringVar(&seenAtArg, "seen-at", "", "Presence timestamp in RFC3339; defaults to now")
	cmd.Flags().StringVar(&reason, "reason", "", "Reason recorded on written presence artifacts")
	cmd.Flags().BoolVar(&write, "write", false, "Persist the presence under workspace/replay/workbench-collaboration-presence/")
	return cmd
}

func newReplayWorkbenchCollaborationPresencesCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "replay-workbench-collaboration-presences <workspace> [artifact]",
		Short: "List or inspect replay workbench collaboration presence artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				presence, err := app.ReadReplayWorkbenchCollaborationPresence(args[0], args[1])
				if err != nil {
					return err
				}
				return printReplayWorkbenchCollaborationPresence(out, presence, true)
			}
			presence, err := app.ListReplayWorkbenchCollaborationPresence(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "replay-workbench-collaboration-presences: total=%d\n", len(presence)); err != nil {
				return err
			}
			for _, item := range presence {
				if err := printReplayWorkbenchCollaborationPresence(out, item, false); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newReplayWorkbenchCollaborationNoteCommand(out io.Writer) *cobra.Command {
	var author string
	var body string
	var targetKind string
	var targetID string
	var createdAtArg string
	var reason string
	var write bool
	cmd := &cobra.Command{
		Use:   "replay-workbench-collaboration-note <workspace> <collaboration-artifact>",
		Short: "Create or preview a metadata-only replay workbench collaboration note",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var createdAt time.Time
			if strings.TrimSpace(createdAtArg) != "" {
				parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(createdAtArg))
				if err != nil {
					return fmt.Errorf("created-at must be RFC3339: %w", err)
				}
				createdAt = parsed
			}
			options := app.ReplayWorkbenchCollaborationNoteOptions{
				CollaborationPath: args[1],
				Author:            author,
				Body:              body,
				TargetKind:        targetKind,
				TargetID:          targetID,
				CreatedAt:         createdAt,
				Reason:            reason,
			}
			var (
				note app.ReplayWorkbenchCollaborationNote
				err  error
			)
			if write {
				note, err = app.CreateReplayWorkbenchCollaborationNote(context.Background(), args[0], options)
			} else {
				note, err = app.PreviewReplayWorkbenchCollaborationNote(context.Background(), args[0], options)
			}
			if err != nil {
				return err
			}
			return printReplayWorkbenchCollaborationNote(out, note, true)
		},
	}
	cmd.Flags().StringVar(&author, "author", "", "Note author; defaults to operator")
	cmd.Flags().StringVar(&body, "body", "", "Note body")
	cmd.Flags().StringVar(&targetKind, "target-kind", "", "Optional note target kind such as graph-node, graph-edge, timeline-event, or session")
	cmd.Flags().StringVar(&targetID, "target-id", "", "Optional note target identifier")
	cmd.Flags().StringVar(&createdAtArg, "created-at", "", "Note timestamp in RFC3339; defaults to now")
	cmd.Flags().StringVar(&reason, "reason", "", "Reason recorded on written note artifacts")
	cmd.Flags().BoolVar(&write, "write", false, "Persist the note under workspace/replay/workbench-collaboration-notes/")
	return cmd
}

func newReplayWorkbenchCollaborationNoteAnnotateCommand(out io.Writer) *cobra.Command {
	var decision string
	var reason string
	var reviewer string
	var reviewedAtArg string
	cmd := &cobra.Command{
		Use:   "replay-workbench-collaboration-note-annotate <workspace> <artifact>",
		Short: "Annotate a saved metadata-only replay workbench collaboration note",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var reviewedAt time.Time
			if strings.TrimSpace(reviewedAtArg) != "" {
				parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(reviewedAtArg))
				if err != nil {
					return fmt.Errorf("reviewed-at must be RFC3339: %w", err)
				}
				reviewedAt = parsed
			}
			note, err := app.AnnotateReplayWorkbenchCollaborationNote(args[0], args[1], app.ReplayWorkbenchCollaborationNoteAnnotationOptions{
				Decision:   decision,
				Reason:     reason,
				Reviewer:   reviewer,
				ReviewedAt: reviewedAt,
			})
			if err != nil {
				return err
			}
			return printReplayWorkbenchCollaborationNote(out, note, true)
		},
	}
	cmd.Flags().StringVar(&decision, "decision", "", "review decision: accepted, deferred, or rejected")
	cmd.Flags().StringVar(&reason, "reason", "", "review decision reason")
	cmd.Flags().StringVar(&reviewer, "reviewer", "", "reviewer identity; defaults to operator")
	cmd.Flags().StringVar(&reviewedAtArg, "reviewed-at", "", "review timestamp in RFC3339; defaults to now")
	return cmd
}

func newReplayWorkbenchCollaborationNoteSelectCommand(out io.Writer) *cobra.Command {
	var selectedBy string
	var selectedAtArg string
	var reason string
	cmd := &cobra.Command{
		Use:   "replay-workbench-collaboration-note-select <workspace> <artifact>",
		Short: "Mark a replay workbench collaboration note as the current metadata-only selection",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var selectedAt time.Time
			if strings.TrimSpace(selectedAtArg) != "" {
				parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(selectedAtArg))
				if err != nil {
					return fmt.Errorf("selected-at must be RFC3339: %w", err)
				}
				selectedAt = parsed
			}
			selection, err := app.SelectReplayWorkbenchCollaborationNote(args[0], args[1], app.ReplayWorkbenchCollaborationNoteSelectionOptions{
				SelectedBy: selectedBy,
				SelectedAt: selectedAt,
				Reason:     reason,
			})
			if err != nil {
				return err
			}
			return printReplayWorkbenchCollaborationNoteSelection(out, selection)
		},
	}
	cmd.Flags().StringVar(&selectedBy, "selected-by", "", "operator identity; defaults to operator")
	cmd.Flags().StringVar(&selectedAtArg, "selected-at", "", "selection timestamp in RFC3339; defaults to now")
	cmd.Flags().StringVar(&reason, "reason", "", "selection reason")
	return cmd
}

func newReplayWorkbenchCollaborationNoteSelectionCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "replay-workbench-collaboration-note-selection <workspace>",
		Short: "Inspect the current metadata-only replay workbench collaboration note selection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			selection, found, err := app.ReadReplayWorkbenchCollaborationNoteSelection(args[0])
			if err != nil {
				return err
			}
			if !found {
				_, err := fmt.Fprintln(out, "replay-workbench-collaboration-note-selection: none")
				return err
			}
			return printReplayWorkbenchCollaborationNoteSelection(out, selection)
		},
	}
}

func newReplayWorkbenchCollaborationNoteSelectionClearCommand(out io.Writer) *cobra.Command {
	var clearedBy string
	var clearedAtArg string
	var reason string
	cmd := &cobra.Command{
		Use:   "replay-workbench-collaboration-note-selection-clear <workspace>",
		Short: "Clear the current metadata-only replay workbench collaboration note selection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var clearedAt time.Time
			if strings.TrimSpace(clearedAtArg) != "" {
				parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(clearedAtArg))
				if err != nil {
					return fmt.Errorf("cleared-at must be RFC3339: %w", err)
				}
				clearedAt = parsed
			}
			selection, err := app.ClearReplayWorkbenchCollaborationNoteSelection(args[0], app.ReplayWorkbenchCollaborationNoteSelectionClearOptions{
				ClearedBy: clearedBy,
				ClearedAt: clearedAt,
				Reason:    reason,
			})
			if err != nil {
				return err
			}
			return printReplayWorkbenchCollaborationNoteSelection(out, selection)
		},
	}
	cmd.Flags().StringVar(&clearedBy, "cleared-by", "", "operator identity; defaults to operator")
	cmd.Flags().StringVar(&clearedAtArg, "cleared-at", "", "clear timestamp in RFC3339; defaults to now")
	cmd.Flags().StringVar(&reason, "reason", "", "clear reason")
	return cmd
}

func newReplayWorkbenchCollaborationNotesCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "replay-workbench-collaboration-notes <workspace> [artifact]",
		Short: "List or inspect replay workbench collaboration note artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				note, err := app.ReadReplayWorkbenchCollaborationNote(args[0], args[1])
				if err != nil {
					return err
				}
				return printReplayWorkbenchCollaborationNote(out, note, true)
			}
			notes, err := app.ListReplayWorkbenchCollaborationNotes(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "replay-workbench-collaboration-notes: total=%d\n", len(notes)); err != nil {
				return err
			}
			for _, note := range notes {
				if err := printReplayWorkbenchCollaborationNote(out, note, false); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newReplayWorkbenchCollaborationsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "replay-workbench-collaborations <workspace> [artifact]",
		Short: "List or inspect replay workbench collaboration sessions",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				collaboration, err := app.ReadReplayWorkbenchCollaboration(args[0], args[1])
				if err != nil {
					return err
				}
				return printReplayWorkbenchCollaborationArtifact(out, collaboration, true)
			}
			collaborations, err := app.ListReplayWorkbenchCollaborations(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "replay-workbench-collaborations: total=%d\n", len(collaborations)); err != nil {
				return err
			}
			for _, collaboration := range collaborations {
				if err := printReplayWorkbenchCollaborationArtifact(out, collaboration, false); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

type replayWorkbenchHandlerOptions struct {
	WorkspaceRoot                            string
	DefaultSession                           replayWorkbenchSession
	SavedSessions                            []app.ReplayWorkbenchSessionArtifact
	SessionSelection                         *app.ReplayWorkbenchSessionSelection
	Collaborations                           []app.ReplayWorkbenchCollaborationArtifact
	CollaborationPresence                    []app.ReplayWorkbenchCollaborationPresence
	CollaborationNotes                       []app.ReplayWorkbenchCollaborationNote
	CollaborationSelection                   *app.ReplayWorkbenchCollaborationSelection
	CollaborationNoteSelection               *app.ReplayWorkbenchCollaborationNoteSelection
	ReportProjectionExecutions               []app.ReportProjectionDispatchExecution
	ReportProjectionSelection                *app.ReportProjectionDispatchExecutionSelection
	AllowReportSelectionWrites               bool
	AllowSessionSelectionWrites              bool
	AllowCollaborationSelectionWrites        bool
	AllowCollaborationPresenceWrites         bool
	AllowCollaborationNoteWrites             bool
	AllowCollaborationNoteSelectionWrites    bool
	EnableCollaborationStream                bool
	CollaborationStreamRefreshIntervalMS     int
	SessionSelectionPollIntervalMS           int
	CollaborationSelectionPollIntervalMS     int
	CollaborationPresencePollIntervalMS      int
	CollaborationNotesPollIntervalMS         int
	CollaborationNoteSelectionPollIntervalMS int
}

type replayWorkbenchCollaborationEventSnapshot struct {
	Collaborations             []app.ReplayWorkbenchCollaborationArtifact     `json:"collaborations"`
	CollaborationSelection     *app.ReplayWorkbenchCollaborationSelection     `json:"collaboration_selection"`
	CollaborationNoteSelection *app.ReplayWorkbenchCollaborationNoteSelection `json:"collaboration_note_selection"`
	CollaborationPresence      []app.ReplayWorkbenchCollaborationPresence     `json:"collaboration_presence"`
	CollaborationNotes         []app.ReplayWorkbenchCollaborationNote         `json:"collaboration_notes"`
}

type replayWorkbenchCollaborationEventBroker struct {
	mu          sync.Mutex
	subscribers map[chan replayWorkbenchCollaborationEventSnapshot]struct{}
}

func newReplayWorkbenchCollaborationEventBroker() *replayWorkbenchCollaborationEventBroker {
	return &replayWorkbenchCollaborationEventBroker{subscribers: map[chan replayWorkbenchCollaborationEventSnapshot]struct{}{}}
}

func (b *replayWorkbenchCollaborationEventBroker) subscribe() (chan replayWorkbenchCollaborationEventSnapshot, func()) {
	ch := make(chan replayWorkbenchCollaborationEventSnapshot, 1)
	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		if _, ok := b.subscribers[ch]; ok {
			delete(b.subscribers, ch)
			close(ch)
		}
		b.mu.Unlock()
	}
}

func (b *replayWorkbenchCollaborationEventBroker) publish(snapshot replayWorkbenchCollaborationEventSnapshot) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subscribers {
		select {
		case ch <- snapshot:
		default:
			select {
			case <-ch:
			default:
			}
			ch <- snapshot
		}
	}
}

func newReplayWorkbenchHandler(result app.ReplayResult, comparisons ...app.ReplayResult) http.Handler {
	return newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{}, result, comparisons...)
}

func newReplayWorkbenchHandlerWithDefaultSession(defaultSession replayWorkbenchSession, result app.ReplayResult, comparisons ...app.ReplayResult) http.Handler {
	return newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{DefaultSession: defaultSession}, result, comparisons...)
}

func newReplayWorkbenchHandlerWithOptions(options replayWorkbenchHandlerOptions, result app.ReplayResult, comparisons ...app.ReplayResult) http.Handler {
	results := append([]app.ReplayResult{result}, comparisons...)
	defaultSession := options.DefaultSession
	workspaceRoot := strings.TrimSpace(options.WorkspaceRoot)
	allowReportSelectionWrites := options.AllowReportSelectionWrites
	allowSessionSelectionWrites := options.AllowSessionSelectionWrites
	allowCollaborationSelectionWrites := options.AllowCollaborationSelectionWrites
	allowCollaborationPresenceWrites := options.AllowCollaborationPresenceWrites
	allowCollaborationNoteWrites := options.AllowCollaborationNoteWrites
	allowCollaborationNoteSelectionWrites := options.AllowCollaborationNoteSelectionWrites
	enableCollaborationStream := options.EnableCollaborationStream
	collaborationStreamRefreshIntervalMS := options.CollaborationStreamRefreshIntervalMS
	if collaborationStreamRefreshIntervalMS < 0 {
		collaborationStreamRefreshIntervalMS = 0
	}
	sessionSelectionPollIntervalMS := options.SessionSelectionPollIntervalMS
	if sessionSelectionPollIntervalMS < 0 {
		sessionSelectionPollIntervalMS = 0
	}
	collaborationSelectionPollIntervalMS := options.CollaborationSelectionPollIntervalMS
	if collaborationSelectionPollIntervalMS < 0 {
		collaborationSelectionPollIntervalMS = 0
	}
	collaborationPresencePollIntervalMS := options.CollaborationPresencePollIntervalMS
	if collaborationPresencePollIntervalMS < 0 {
		collaborationPresencePollIntervalMS = 0
	}
	collaborationNotesPollIntervalMS := options.CollaborationNotesPollIntervalMS
	if collaborationNotesPollIntervalMS < 0 {
		collaborationNotesPollIntervalMS = 0
	}
	collaborationNoteSelectionPollIntervalMS := options.CollaborationNoteSelectionPollIntervalMS
	if collaborationNoteSelectionPollIntervalMS < 0 {
		collaborationNoteSelectionPollIntervalMS = 0
	}
	savedSessions := append([]app.ReplayWorkbenchSessionArtifact(nil), options.SavedSessions...)
	collaborations := append([]app.ReplayWorkbenchCollaborationArtifact(nil), options.Collaborations...)
	var collaborationsMu sync.Mutex
	collaborationPresence := append([]app.ReplayWorkbenchCollaborationPresence(nil), options.CollaborationPresence...)
	var collaborationPresenceMu sync.Mutex
	collaborationNotes := append([]app.ReplayWorkbenchCollaborationNote(nil), options.CollaborationNotes...)
	var collaborationNotesMu sync.Mutex
	var sessionSelection *app.ReplayWorkbenchSessionSelection
	if options.SessionSelection != nil {
		selectionCopy := *options.SessionSelection
		sessionSelection = &selectionCopy
	}
	var sessionSelectionMu sync.Mutex
	var collaborationSelection *app.ReplayWorkbenchCollaborationSelection
	if options.CollaborationSelection != nil {
		selectionCopy := *options.CollaborationSelection
		collaborationSelection = &selectionCopy
	}
	var collaborationSelectionMu sync.Mutex
	var collaborationNoteSelection *app.ReplayWorkbenchCollaborationNoteSelection
	if options.CollaborationNoteSelection != nil {
		selectionCopy := *options.CollaborationNoteSelection
		collaborationNoteSelection = &selectionCopy
	}
	var collaborationNoteSelectionMu sync.Mutex
	reportExecutions := append([]app.ReportProjectionDispatchExecution(nil), options.ReportProjectionExecutions...)
	var reportSelection *app.ReportProjectionDispatchExecutionSelection
	if options.ReportProjectionSelection != nil {
		selectionCopy := *options.ReportProjectionSelection
		reportSelection = &selectionCopy
	}
	var reportSelectionMu sync.Mutex
	collaborationEvents := newReplayWorkbenchCollaborationEventBroker()
	collaborationSnapshot := func() replayWorkbenchCollaborationEventSnapshot {
		collaborationsMu.Lock()
		collaborationsCopy := append([]app.ReplayWorkbenchCollaborationArtifact(nil), collaborations...)
		collaborationsMu.Unlock()
		collaborationSelectionMu.Lock()
		var selectionCopy *app.ReplayWorkbenchCollaborationSelection
		if collaborationSelection != nil {
			copied := *collaborationSelection
			selectionCopy = &copied
		}
		collaborationSelectionMu.Unlock()
		collaborationPresenceMu.Lock()
		presenceCopy := append([]app.ReplayWorkbenchCollaborationPresence(nil), collaborationPresence...)
		collaborationPresenceMu.Unlock()
		collaborationNotesMu.Lock()
		notesCopy := append([]app.ReplayWorkbenchCollaborationNote(nil), collaborationNotes...)
		collaborationNotesMu.Unlock()
		collaborationNoteSelectionMu.Lock()
		var noteSelectionCopy *app.ReplayWorkbenchCollaborationNoteSelection
		if collaborationNoteSelection != nil {
			copied := *collaborationNoteSelection
			noteSelectionCopy = &copied
		}
		collaborationNoteSelectionMu.Unlock()
		return replayWorkbenchCollaborationEventSnapshot{
			Collaborations:             collaborationsCopy,
			CollaborationSelection:     selectionCopy,
			CollaborationNoteSelection: noteSelectionCopy,
			CollaborationPresence:      presenceCopy,
			CollaborationNotes:         notesCopy,
		}
	}
	refreshCollaborationSnapshot := func() replayWorkbenchCollaborationEventSnapshot {
		if workspaceRoot == "" {
			return collaborationSnapshot()
		}
		if refreshedCollaborations, err := app.ListReplayWorkbenchCollaborations(workspaceRoot); err == nil {
			collaborationsMu.Lock()
			collaborations = refreshedCollaborations
			collaborationsMu.Unlock()
		}
		if refreshedPresence, err := app.ListReplayWorkbenchCollaborationPresence(workspaceRoot); err == nil {
			collaborationPresenceMu.Lock()
			collaborationPresence = refreshedPresence
			collaborationPresenceMu.Unlock()
		}
		if refreshedNotes, err := app.ListReplayWorkbenchCollaborationNotes(workspaceRoot); err == nil {
			collaborationNotesMu.Lock()
			collaborationNotes = refreshedNotes
			collaborationNotesMu.Unlock()
		}
		if refreshedSelection, found, err := app.ReadReplayWorkbenchCollaborationSelection(workspaceRoot); err == nil {
			collaborationSelectionMu.Lock()
			if found {
				selectionCopy := refreshedSelection
				collaborationSelection = &selectionCopy
			} else {
				collaborationSelection = nil
			}
			collaborationSelectionMu.Unlock()
		}
		if refreshedNoteSelection, found, err := app.ReadReplayWorkbenchCollaborationNoteSelection(workspaceRoot); err == nil {
			collaborationNoteSelectionMu.Lock()
			if found {
				selectionCopy := refreshedNoteSelection
				collaborationNoteSelection = &selectionCopy
			} else {
				collaborationNoteSelection = nil
			}
			collaborationNoteSelectionMu.Unlock()
		}
		return collaborationSnapshot()
	}
	publishCollaborationSnapshot := func() {
		if enableCollaborationStream {
			collaborationEvents.publish(collaborationSnapshot())
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			sessionSelectionWrite := r.URL.Path == "/api/session-selection" && r.Method == http.MethodPost && allowSessionSelectionWrites
			presenceWrite := r.URL.Path == "/api/collaboration-presence" && r.Method == http.MethodPost && allowCollaborationPresenceWrites
			selectionWrite := r.URL.Path == "/api/collaboration-selection" && r.Method == http.MethodPost && allowCollaborationSelectionWrites
			noteWrite := r.URL.Path == "/api/collaboration-notes" && r.Method == http.MethodPost && allowCollaborationNoteWrites
			noteSelectionWrite := r.URL.Path == "/api/collaboration-note-selection" && r.Method == http.MethodPost && allowCollaborationNoteSelectionWrites
			reportSelectionWrite := r.URL.Path == "/api/report-selection" && r.Method == http.MethodPost && allowReportSelectionWrites
			if !sessionSelectionWrite && !presenceWrite && !selectionWrite && !noteWrite && !noteSelectionWrite && !reportSelectionWrite {
				http.Error(w, "replay workbench is read-only", http.StatusMethodNotAllowed)
				return
			}
		}
		switch r.URL.Path {
		case "/", "/index.html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if r.Method == http.MethodHead {
				return
			}
			sessionSelectionMu.Lock()
			var sessionSelectionCopy *app.ReplayWorkbenchSessionSelection
			if sessionSelection != nil {
				copied := *sessionSelection
				sessionSelectionCopy = &copied
			}
			sessionSelectionMu.Unlock()
			collaborationState := collaborationSnapshot()
			collaborationNoteSelectionMu.Lock()
			var noteSelectionCopy *app.ReplayWorkbenchCollaborationNoteSelection
			if collaborationNoteSelection != nil {
				copied := *collaborationNoteSelection
				noteSelectionCopy = &copied
			}
			collaborationNoteSelectionMu.Unlock()
			reportSelectionMu.Lock()
			var reportSelectionCopy *app.ReportProjectionDispatchExecutionSelection
			if reportSelection != nil {
				copied := *reportSelection
				reportSelectionCopy = &copied
			}
			reportSelectionMu.Unlock()
			if err := printReplayWorkbenchHTML(w, results, replayWorkbenchSessionFromRequestWithDefault(r, results, defaultSession), savedSessions, sessionSelectionCopy, reportExecutions, reportSelectionCopy, collaborationState.Collaborations, collaborationState.CollaborationPresence, collaborationState.CollaborationNotes, collaborationState.CollaborationSelection, noteSelectionCopy, allowReportSelectionWrites, allowSessionSelectionWrites, allowCollaborationSelectionWrites, allowCollaborationPresenceWrites, allowCollaborationNoteWrites, allowCollaborationNoteSelectionWrites, enableCollaborationStream, sessionSelectionPollIntervalMS, collaborationSelectionPollIntervalMS, collaborationPresencePollIntervalMS, collaborationNotesPollIntervalMS, collaborationNoteSelectionPollIntervalMS); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/session":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			if r.Method == http.MethodHead {
				return
			}
			if err := printReplayWorkbenchSession(w, replayWorkbenchSessionFromRequestWithDefault(r, results, defaultSession)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/sessions":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			if r.Method == http.MethodHead {
				return
			}
			if err := printReplayWorkbenchSessions(w, savedSessions); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/reports":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			if r.Method == http.MethodHead {
				return
			}
			if err := printReplayWorkbenchReportProjectionExecutionsJSON(w, reportExecutions); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/report-selection":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			if r.Method == http.MethodHead {
				return
			}
			if r.Method == http.MethodPost {
				selection, err := updateReplayWorkbenchReportProjectionSelectionFromRequest(r, workspaceRoot)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				reportSelectionMu.Lock()
				selectionCopy := selection
				reportSelection = &selectionCopy
				reportSelectionMu.Unlock()
				if err := printReplayWorkbenchReportProjectionSelectionValueJSON(w, selection); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
				return
			}
			reportSelectionMu.Lock()
			var selectionCopy *app.ReportProjectionDispatchExecutionSelection
			if reportSelection != nil {
				copied := *reportSelection
				selectionCopy = &copied
			}
			reportSelectionMu.Unlock()
			if err := printReplayWorkbenchReportProjectionSelectionJSON(w, selectionCopy); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/session-selection":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			if r.Method == http.MethodHead {
				return
			}
			if r.Method == http.MethodPost {
				selection, err := updateReplayWorkbenchSessionSelectionFromRequest(r, workspaceRoot)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				sessionSelectionMu.Lock()
				selectionCopy := selection
				sessionSelection = &selectionCopy
				sessionSelectionMu.Unlock()
				if err := printReplayWorkbenchSessionSelectionValueJSON(w, selection); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
				return
			}
			sessionSelectionMu.Lock()
			var selectionCopy *app.ReplayWorkbenchSessionSelection
			if sessionSelection != nil {
				copied := *sessionSelection
				selectionCopy = &copied
			}
			sessionSelectionMu.Unlock()
			if err := printReplayWorkbenchSessionSelectionJSON(w, selectionCopy); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/collaborations":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			if r.Method == http.MethodHead {
				return
			}
			collaborationsMu.Lock()
			collaborationsCopy := append([]app.ReplayWorkbenchCollaborationArtifact(nil), collaborations...)
			collaborationsMu.Unlock()
			if err := printReplayWorkbenchCollaborations(w, collaborationsCopy); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/collaboration-presence":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			if r.Method == http.MethodHead {
				return
			}
			if r.Method == http.MethodPost {
				presence, err := updateReplayWorkbenchCollaborationPresenceFromRequest(r, workspaceRoot)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				refreshed, err := app.ListReplayWorkbenchCollaborationPresence(workspaceRoot)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				collaborationPresenceMu.Lock()
				collaborationPresence = refreshed
				collaborationPresenceMu.Unlock()
				publishCollaborationSnapshot()
				if err := printReplayWorkbenchCollaborationPresenceValueJSON(w, presence); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
				return
			}
			collaborationPresenceMu.Lock()
			presenceCopy := append([]app.ReplayWorkbenchCollaborationPresence(nil), collaborationPresence...)
			collaborationPresenceMu.Unlock()
			if err := printReplayWorkbenchCollaborationPresenceJSON(w, presenceCopy); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/collaboration-notes":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			if r.Method == http.MethodHead {
				return
			}
			if r.Method == http.MethodPost {
				note, err := updateReplayWorkbenchCollaborationNoteFromRequest(r, workspaceRoot)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				refreshed, err := app.ListReplayWorkbenchCollaborationNotes(workspaceRoot)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				collaborationNotesMu.Lock()
				collaborationNotes = refreshed
				collaborationNotesMu.Unlock()
				publishCollaborationSnapshot()
				if err := printReplayWorkbenchCollaborationNoteValueJSON(w, note); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
				return
			}
			collaborationNotesMu.Lock()
			notesCopy := append([]app.ReplayWorkbenchCollaborationNote(nil), collaborationNotes...)
			collaborationNotesMu.Unlock()
			if err := printReplayWorkbenchCollaborationNotesJSON(w, notesCopy); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/collaboration-note-selection":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			if r.Method == http.MethodHead {
				return
			}
			if r.Method == http.MethodPost {
				selection, err := updateReplayWorkbenchCollaborationNoteSelectionFromRequest(r, workspaceRoot)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				collaborationNoteSelectionMu.Lock()
				selectionCopy := selection
				collaborationNoteSelection = &selectionCopy
				collaborationNoteSelectionMu.Unlock()
				publishCollaborationSnapshot()
				if err := printReplayWorkbenchCollaborationNoteSelectionValueJSON(w, selection); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
				return
			}
			collaborationNoteSelectionMu.Lock()
			var selectionCopy *app.ReplayWorkbenchCollaborationNoteSelection
			if collaborationNoteSelection != nil {
				copied := *collaborationNoteSelection
				selectionCopy = &copied
			}
			collaborationNoteSelectionMu.Unlock()
			if err := printReplayWorkbenchCollaborationNoteSelectionJSON(w, selectionCopy); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/collaboration-selection":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			if r.Method == http.MethodHead {
				return
			}
			if r.Method == http.MethodPost {
				selection, err := updateReplayWorkbenchCollaborationSelectionFromRequest(r, workspaceRoot)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				collaborationSelectionMu.Lock()
				selectionCopy := selection
				collaborationSelection = &selectionCopy
				collaborationSelectionMu.Unlock()
				publishCollaborationSnapshot()
				if err := printReplayWorkbenchCollaborationSelectionValueJSON(w, selection); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
				return
			}
			collaborationSelectionMu.Lock()
			var selectionCopy *app.ReplayWorkbenchCollaborationSelection
			if collaborationSelection != nil {
				copied := *collaborationSelection
				selectionCopy = &copied
			}
			collaborationSelectionMu.Unlock()
			if err := printReplayWorkbenchCollaborationSelectionJSON(w, selectionCopy); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/collaboration-events":
			if !enableCollaborationStream {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			if r.Method == http.MethodHead {
				return
			}
			if err := streamReplayWorkbenchCollaborationEvents(w, r, collaborationSnapshot(), collaborationEvents, time.Duration(collaborationStreamRefreshIntervalMS)*time.Millisecond, refreshCollaborationSnapshot); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/replay":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			if r.Method == http.MethodHead {
				return
			}
			if err := printReplayJSONFiltered(w, result, replayFilterFromRequestWithDefault(r, defaultSession.Filters)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/replays":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			if r.Method == http.MethodHead {
				return
			}
			if err := printReplayJSONListFiltered(w, results, replayFilterFromRequestWithDefault(r, defaultSession.Filters)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/graph":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			if r.Method == http.MethodHead {
				return
			}
			if err := printReplayGraphJSONFiltered(w, result, replayFilterFromRequestWithDefault(r, defaultSession.Filters)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/graphs":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			if r.Method == http.MethodHead {
				return
			}
			if err := printReplayGraphJSONListFiltered(w, results, replayFilterFromRequestWithDefault(r, defaultSession.Filters)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/graph.ndjson":
			w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
			if r.Method == http.MethodHead {
				return
			}
			if err := printReplayGraphNDJSONFiltered(w, result, replayFilterFromRequestWithDefault(r, defaultSession.Filters)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/timeline.ndjson":
			w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
			if r.Method == http.MethodHead {
				return
			}
			if err := printReplayNDJSONFiltered(w, result, replayFilterFromRequestWithDefault(r, defaultSession.Filters)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		default:
			http.NotFound(w, r)
		}
	})
}

func printReplayText(out io.Writer, result app.ReplayResult, printTUI bool, interactive bool) error {
	if _, err := fmt.Fprintf(out, "replay: %s\n", result.RunID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "events: %d\n", result.EventCount); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "model: %s\n", result.Status.Model); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "tools: %s\n", result.Status.ToolHealth); err != nil {
		return err
	}
	for _, lane := range replayLaneOrder() {
		if _, err := fmt.Fprintf(out, "lane: %s %d\n", lane, result.LaneCounts[lane]); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(out, "report: %s\n", replayReportDisplay(result.ReportPath)); err != nil {
		return err
	}
	if printTUI {
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(out, result.TUIView); err != nil {
			return err
		}
	}
	if interactive {
		return runInteractiveTUI(result.TUIModel)
	}
	return nil
}

type replayExport struct {
	RunID      string                 `json:"run_id"`
	EventCount int                    `json:"event_count"`
	LaneCounts map[string]int         `json:"lane_counts"`
	Status     replayExportStatus     `json:"status"`
	ReportPath string                 `json:"report_path"`
	Timeline   []replayExportTimeline `json:"timeline"`
	Graph      replayGraphExport      `json:"graph"`
}

type replayExportStatus struct {
	Model          string  `json:"model"`
	ContextPercent int     `json:"context_percent"`
	AgentCount     int     `json:"agent_count"`
	BreadthPercent int     `json:"breadth_percent"`
	DepthChains    int     `json:"depth_chains"`
	ToolHealth     string  `json:"tool_health"`
	Cost           float64 `json:"cost"`
}

type replayExportTimeline struct {
	EventID   int64           `json:"event_id"`
	RunID     string          `json:"run_id"`
	Lane      string          `json:"lane"`
	Type      string          `json:"type"`
	CreatedAt time.Time       `json:"created_at"`
	Payload   json.RawMessage `json:"payload"`
}

type replayGraphExport struct {
	RunID          string            `json:"run_id"`
	NodeCount      int               `json:"node_count"`
	EdgeCount      int               `json:"edge_count"`
	NodeTypeCounts map[string]int    `json:"node_type_counts"`
	EdgeTypeCounts map[string]int    `json:"edge_type_counts"`
	Focus          *replayGraphFocus `json:"focus,omitempty"`
	Nodes          []replayGraphNode `json:"nodes"`
	Edges          []replayGraphEdge `json:"edges"`
}

type replayGraphNode struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Label      string `json:"label"`
	SourceKind string `json:"source_kind,omitempty"`
}

type replayGraphEdge struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

type replayGraphFocus struct {
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	NodeID    string `json:"node_id,omitempty"`
	EdgeID    string `json:"edge_id,omitempty"`
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
	Type      string `json:"type,omitempty"`
	NodeCount int    `json:"node_count"`
	EdgeCount int    `json:"edge_count"`
}

type replayTimelineFilter struct {
	Lane  string
	Query string
	Node  string
	Edge  string
}

type replayWorkbenchSession struct {
	PrimaryRunID  string            `json:"primary_run_id"`
	SelectedRunID string            `json:"selected_run_id"`
	RunIDs        []string          `json:"run_ids"`
	Filters       map[string]string `json:"filters"`
	Endpoints     map[string]string `json:"endpoints"`
}

func replayFilterFromRequest(r *http.Request) replayTimelineFilter {
	return replayFilterFromRequestWithDefault(r, nil)
}

func replayFilterFromRequestWithDefault(r *http.Request, defaults map[string]string) replayTimelineFilter {
	query := r.URL.Query()
	lane := replayWorkbenchQueryValue(query, defaults, "lane")
	if strings.EqualFold(lane, "all") {
		lane = ""
	}
	return replayTimelineFilter{
		Lane:  lane,
		Query: replayWorkbenchQueryValue(query, defaults, "q"),
		Node:  replayWorkbenchQueryValue(query, defaults, "node"),
		Edge:  replayWorkbenchQueryValue(query, defaults, "edge"),
	}
}

func replayWorkbenchSessionFromRequest(r *http.Request, results []app.ReplayResult) replayWorkbenchSession {
	return replayWorkbenchSessionFromRequestWithDefault(r, results, replayWorkbenchSession{})
}

func replayWorkbenchSessionFromRequestWithDefault(r *http.Request, results []app.ReplayResult, defaults replayWorkbenchSession) replayWorkbenchSession {
	runIDs := make([]string, 0, len(results))
	for _, result := range results {
		if strings.TrimSpace(result.RunID) != "" {
			runIDs = append(runIDs, result.RunID)
		}
	}
	primary := ""
	if len(runIDs) > 0 {
		primary = runIDs[0]
	}
	selected := primary
	requested := replayWorkbenchQueryValue(r.URL.Query(), map[string]string{"run": defaults.SelectedRunID}, "run")
	for _, runID := range runIDs {
		if requested == runID {
			selected = runID
			break
		}
	}
	filter := replayFilterFromRequestWithDefault(r, defaults.Filters)
	filters := map[string]string{}
	if filter.Lane != "" {
		filters["lane"] = filter.Lane
	}
	if filter.Query != "" {
		filters["q"] = filter.Query
	}
	if filter.Node != "" {
		filters["node"] = filter.Node
	}
	if filter.Edge != "" {
		filters["edge"] = filter.Edge
	}
	filterQuery := replayWorkbenchFilterQuery(filters)
	htmlQuery := replayWorkbenchFilterQuery(filters)
	if selected != "" {
		htmlQuery.Set("run", selected)
	}
	return replayWorkbenchSession{
		PrimaryRunID:  primary,
		SelectedRunID: selected,
		RunIDs:        runIDs,
		Filters:       filters,
		Endpoints: map[string]string{
			"html":            replayWorkbenchPathWithQuery("/", htmlQuery),
			"replay":          replayWorkbenchPathWithQuery("/api/replay", filterQuery),
			"replays":         replayWorkbenchPathWithQuery("/api/replays", filterQuery),
			"sessions":        "/api/sessions",
			"graph":           replayWorkbenchPathWithQuery("/api/graph", filterQuery),
			"graphs":          replayWorkbenchPathWithQuery("/api/graphs", filterQuery),
			"graph_ndjson":    replayWorkbenchPathWithQuery("/api/graph.ndjson", filterQuery),
			"timeline_ndjson": replayWorkbenchPathWithQuery("/api/timeline.ndjson", filterQuery),
		},
	}
}

func replayWorkbenchQueryValue(values url.Values, defaults map[string]string, key string) string {
	if raw, ok := values[key]; ok {
		if len(raw) == 0 {
			return ""
		}
		return strings.TrimSpace(raw[0])
	}
	return strings.TrimSpace(defaults[key])
}

func replayWorkbenchSessionFromArtifact(session app.ReplayWorkbenchSessionArtifact) replayWorkbenchSession {
	runIDs := append([]string(nil), session.RunIDs...)
	filters := map[string]string{}
	for key, value := range session.Filters {
		if strings.TrimSpace(value) != "" {
			filters[key] = value
		}
	}
	return replayWorkbenchSession{
		PrimaryRunID:  session.PrimaryRunID,
		SelectedRunID: session.SelectedRunID,
		RunIDs:        runIDs,
		Filters:       filters,
		Endpoints:     session.Endpoints,
	}
}

func mergeReplayWorkbenchCompareRuns(session app.ReplayWorkbenchSessionArtifact, explicit []string) []string {
	seen := map[string]bool{session.PrimaryRunID: true}
	merged := make([]string, 0, len(session.RunIDs)+len(explicit))
	appendRunID := func(runID string) {
		runID = strings.TrimSpace(runID)
		if runID == "" || seen[runID] {
			return
		}
		seen[runID] = true
		merged = append(merged, runID)
	}
	for _, runID := range session.RunIDs {
		appendRunID(runID)
	}
	appendRunID(session.SelectedRunID)
	for _, runID := range explicit {
		appendRunID(runID)
	}
	return merged
}

func replayWorkbenchFilterQuery(filters map[string]string) url.Values {
	values := url.Values{}
	if lane := strings.TrimSpace(filters["lane"]); lane != "" {
		values.Set("lane", lane)
	}
	if query := strings.TrimSpace(filters["q"]); query != "" {
		values.Set("q", query)
	}
	if node := strings.TrimSpace(filters["node"]); node != "" {
		values.Set("node", node)
	}
	if edge := strings.TrimSpace(filters["edge"]); edge != "" {
		values.Set("edge", edge)
	}
	return values
}

func replayWorkbenchPathWithQuery(path string, values url.Values) string {
	encoded := values.Encode()
	if encoded == "" {
		return path
	}
	return path + "?" + encoded
}

func printReplayWorkbenchSession(out io.Writer, session replayWorkbenchSession) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(session)
}

func printReplayWorkbenchSessions(out io.Writer, sessions []app.ReplayWorkbenchSessionArtifact) error {
	if sessions == nil {
		sessions = []app.ReplayWorkbenchSessionArtifact{}
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(sessions)
}

func printReplayWorkbenchReportProjectionExecutionsJSON(out io.Writer, executions []app.ReportProjectionDispatchExecution) error {
	if executions == nil {
		executions = []app.ReportProjectionDispatchExecution{}
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(executions)
}

func printReplayWorkbenchReportProjectionSelectionJSON(out io.Writer, selection *app.ReportProjectionDispatchExecutionSelection) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if selection == nil {
		return encoder.Encode(nil)
	}
	return encoder.Encode(selection)
}

func printReplayWorkbenchReportProjectionSelectionValueJSON(out io.Writer, selection app.ReportProjectionDispatchExecutionSelection) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(selection)
}

func printReplayWorkbenchCollaborations(out io.Writer, collaborations []app.ReplayWorkbenchCollaborationArtifact) error {
	if collaborations == nil {
		collaborations = []app.ReplayWorkbenchCollaborationArtifact{}
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(collaborations)
}

func printReplayWorkbenchCollaborationPresenceJSON(out io.Writer, presence []app.ReplayWorkbenchCollaborationPresence) error {
	if presence == nil {
		presence = []app.ReplayWorkbenchCollaborationPresence{}
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(presence)
}

func printReplayWorkbenchCollaborationPresenceValueJSON(out io.Writer, presence app.ReplayWorkbenchCollaborationPresence) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(presence)
}

func printReplayWorkbenchCollaborationNotesJSON(out io.Writer, notes []app.ReplayWorkbenchCollaborationNote) error {
	if notes == nil {
		notes = []app.ReplayWorkbenchCollaborationNote{}
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(notes)
}

func printReplayWorkbenchCollaborationNoteValueJSON(out io.Writer, note app.ReplayWorkbenchCollaborationNote) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(note)
}

func printReplayWorkbenchCollaborationNoteSelectionJSON(out io.Writer, selection *app.ReplayWorkbenchCollaborationNoteSelection) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if selection == nil {
		return encoder.Encode(nil)
	}
	return encoder.Encode(selection)
}

func printReplayWorkbenchCollaborationNoteSelectionValueJSON(out io.Writer, selection app.ReplayWorkbenchCollaborationNoteSelection) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(selection)
}

type replayWorkbenchCollaborationPresenceWriteRequest struct {
	CollaborationPath string `json:"collaboration_path"`
	Participant       string `json:"participant"`
	Status            string `json:"status"`
	SeenBy            string `json:"seen_by"`
	SeenAt            string `json:"seen_at"`
	Reason            string `json:"reason"`
}

type replayWorkbenchSessionSelectionWriteRequest struct {
	Action      string `json:"action"`
	SessionPath string `json:"session_path"`
	Actor       string `json:"actor"`
	At          string `json:"at"`
	Reason      string `json:"reason"`
}

type replayWorkbenchCollaborationSelectionWriteRequest struct {
	Action            string `json:"action"`
	CollaborationPath string `json:"collaboration_path"`
	Actor             string `json:"actor"`
	At                string `json:"at"`
	Reason            string `json:"reason"`
}

type replayWorkbenchCollaborationNoteSelectionWriteRequest struct {
	Action   string `json:"action"`
	NotePath string `json:"note_path"`
	Actor    string `json:"actor"`
	At       string `json:"at"`
	Reason   string `json:"reason"`
}

type replayWorkbenchReportSelectionWriteRequest struct {
	Action        string `json:"action"`
	ExecutionPath string `json:"execution_path"`
	Actor         string `json:"actor"`
	At            string `json:"at"`
	Reason        string `json:"reason"`
}

type replayWorkbenchCollaborationNoteWriteRequest struct {
	CollaborationPath string `json:"collaboration_path"`
	Author            string `json:"author"`
	Body              string `json:"body"`
	TargetKind        string `json:"target_kind"`
	TargetID          string `json:"target_id"`
	CreatedAt         string `json:"created_at"`
	Reason            string `json:"reason"`
}

func updateReplayWorkbenchCollaborationPresenceFromRequest(r *http.Request, workspaceRoot string) (app.ReplayWorkbenchCollaborationPresence, error) {
	if strings.TrimSpace(workspaceRoot) == "" {
		return app.ReplayWorkbenchCollaborationPresence{}, fmt.Errorf("replay workbench collaboration presence writes require a workspace root")
	}
	var request replayWorkbenchCollaborationPresenceWriteRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return app.ReplayWorkbenchCollaborationPresence{}, fmt.Errorf("decode collaboration presence write request: %w", err)
	}
	var seenAt time.Time
	if trimmed := strings.TrimSpace(request.SeenAt); trimmed != "" {
		parsed, err := time.Parse(time.RFC3339, trimmed)
		if err != nil {
			return app.ReplayWorkbenchCollaborationPresence{}, fmt.Errorf("parse collaboration presence seen_at: %w", err)
		}
		seenAt = parsed
	}
	return app.UpdateReplayWorkbenchCollaborationPresence(r.Context(), workspaceRoot, app.ReplayWorkbenchCollaborationPresenceOptions{
		CollaborationPath: request.CollaborationPath,
		Participant:       request.Participant,
		Status:            request.Status,
		SeenBy:            request.SeenBy,
		SeenAt:            seenAt,
		Reason:            request.Reason,
	})
}

func updateReplayWorkbenchCollaborationNoteFromRequest(r *http.Request, workspaceRoot string) (app.ReplayWorkbenchCollaborationNote, error) {
	if strings.TrimSpace(workspaceRoot) == "" {
		return app.ReplayWorkbenchCollaborationNote{}, fmt.Errorf("replay workbench collaboration note writes require a workspace root")
	}
	var request replayWorkbenchCollaborationNoteWriteRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return app.ReplayWorkbenchCollaborationNote{}, fmt.Errorf("decode collaboration note write request: %w", err)
	}
	var createdAt time.Time
	if trimmed := strings.TrimSpace(request.CreatedAt); trimmed != "" {
		parsed, err := time.Parse(time.RFC3339, trimmed)
		if err != nil {
			return app.ReplayWorkbenchCollaborationNote{}, fmt.Errorf("parse collaboration note created_at: %w", err)
		}
		createdAt = parsed
	}
	return app.CreateReplayWorkbenchCollaborationNote(r.Context(), workspaceRoot, app.ReplayWorkbenchCollaborationNoteOptions{
		CollaborationPath: request.CollaborationPath,
		Author:            request.Author,
		Body:              request.Body,
		TargetKind:        request.TargetKind,
		TargetID:          request.TargetID,
		CreatedAt:         createdAt,
		Reason:            request.Reason,
	})
}

func updateReplayWorkbenchSessionSelectionFromRequest(r *http.Request, workspaceRoot string) (app.ReplayWorkbenchSessionSelection, error) {
	if strings.TrimSpace(workspaceRoot) == "" {
		return app.ReplayWorkbenchSessionSelection{}, fmt.Errorf("replay workbench session selection writes require a workspace root")
	}
	var request replayWorkbenchSessionSelectionWriteRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return app.ReplayWorkbenchSessionSelection{}, fmt.Errorf("decode session selection write request: %w", err)
	}
	var at time.Time
	if trimmed := strings.TrimSpace(request.At); trimmed != "" {
		parsed, err := time.Parse(time.RFC3339, trimmed)
		if err != nil {
			return app.ReplayWorkbenchSessionSelection{}, fmt.Errorf("parse session selection at: %w", err)
		}
		at = parsed
	}
	switch strings.ToLower(strings.TrimSpace(request.Action)) {
	case "select":
		return app.SelectReplayWorkbenchSession(workspaceRoot, request.SessionPath, app.ReplayWorkbenchSessionSelectionOptions{
			SelectedBy: request.Actor,
			SelectedAt: at,
			Reason:     request.Reason,
		})
	case "clear":
		return app.ClearReplayWorkbenchSessionSelection(workspaceRoot, app.ReplayWorkbenchSessionSelectionClearOptions{
			ClearedBy: request.Actor,
			ClearedAt: at,
			Reason:    request.Reason,
		})
	default:
		return app.ReplayWorkbenchSessionSelection{}, fmt.Errorf("session selection action must be select or clear")
	}
}

func updateReplayWorkbenchCollaborationSelectionFromRequest(r *http.Request, workspaceRoot string) (app.ReplayWorkbenchCollaborationSelection, error) {
	if strings.TrimSpace(workspaceRoot) == "" {
		return app.ReplayWorkbenchCollaborationSelection{}, fmt.Errorf("replay workbench collaboration selection writes require a workspace root")
	}
	var request replayWorkbenchCollaborationSelectionWriteRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return app.ReplayWorkbenchCollaborationSelection{}, fmt.Errorf("decode collaboration selection write request: %w", err)
	}
	var at time.Time
	if trimmed := strings.TrimSpace(request.At); trimmed != "" {
		parsed, err := time.Parse(time.RFC3339, trimmed)
		if err != nil {
			return app.ReplayWorkbenchCollaborationSelection{}, fmt.Errorf("parse collaboration selection at: %w", err)
		}
		at = parsed
	}
	switch strings.ToLower(strings.TrimSpace(request.Action)) {
	case "select":
		return app.SelectReplayWorkbenchCollaboration(workspaceRoot, request.CollaborationPath, app.ReplayWorkbenchCollaborationSelectionOptions{
			SelectedBy: request.Actor,
			SelectedAt: at,
			Reason:     request.Reason,
		})
	case "clear":
		return app.ClearReplayWorkbenchCollaborationSelection(workspaceRoot, app.ReplayWorkbenchCollaborationSelectionClearOptions{
			ClearedBy: request.Actor,
			ClearedAt: at,
			Reason:    request.Reason,
		})
	default:
		return app.ReplayWorkbenchCollaborationSelection{}, fmt.Errorf("collaboration selection action must be select or clear")
	}
}

func updateReplayWorkbenchCollaborationNoteSelectionFromRequest(r *http.Request, workspaceRoot string) (app.ReplayWorkbenchCollaborationNoteSelection, error) {
	if strings.TrimSpace(workspaceRoot) == "" {
		return app.ReplayWorkbenchCollaborationNoteSelection{}, fmt.Errorf("replay workbench collaboration note selection writes require a workspace root")
	}
	var request replayWorkbenchCollaborationNoteSelectionWriteRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return app.ReplayWorkbenchCollaborationNoteSelection{}, fmt.Errorf("decode collaboration note selection write request: %w", err)
	}
	var at time.Time
	if trimmed := strings.TrimSpace(request.At); trimmed != "" {
		parsed, err := time.Parse(time.RFC3339, trimmed)
		if err != nil {
			return app.ReplayWorkbenchCollaborationNoteSelection{}, fmt.Errorf("parse collaboration note selection at: %w", err)
		}
		at = parsed
	}
	switch strings.ToLower(strings.TrimSpace(request.Action)) {
	case "select":
		return app.SelectReplayWorkbenchCollaborationNote(workspaceRoot, request.NotePath, app.ReplayWorkbenchCollaborationNoteSelectionOptions{
			SelectedBy: request.Actor,
			SelectedAt: at,
			Reason:     request.Reason,
		})
	case "clear":
		return app.ClearReplayWorkbenchCollaborationNoteSelection(workspaceRoot, app.ReplayWorkbenchCollaborationNoteSelectionClearOptions{
			ClearedBy: request.Actor,
			ClearedAt: at,
			Reason:    request.Reason,
		})
	default:
		return app.ReplayWorkbenchCollaborationNoteSelection{}, fmt.Errorf("collaboration note selection action must be select or clear")
	}
}

func updateReplayWorkbenchReportProjectionSelectionFromRequest(r *http.Request, workspaceRoot string) (app.ReportProjectionDispatchExecutionSelection, error) {
	if strings.TrimSpace(workspaceRoot) == "" {
		return app.ReportProjectionDispatchExecutionSelection{}, fmt.Errorf("replay workbench report selection writes require a workspace root")
	}
	var request replayWorkbenchReportSelectionWriteRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return app.ReportProjectionDispatchExecutionSelection{}, fmt.Errorf("decode report selection write request: %w", err)
	}
	var at time.Time
	if trimmed := strings.TrimSpace(request.At); trimmed != "" {
		parsed, err := time.Parse(time.RFC3339, trimmed)
		if err != nil {
			return app.ReportProjectionDispatchExecutionSelection{}, fmt.Errorf("parse report selection at: %w", err)
		}
		at = parsed
	}
	switch strings.ToLower(strings.TrimSpace(request.Action)) {
	case "select":
		return app.SelectReportProjectionDispatchExecution(workspaceRoot, request.ExecutionPath, app.ReportProjectionDispatchExecutionSelectionOptions{
			SelectedBy: request.Actor,
			SelectedAt: at,
			Reason:     request.Reason,
		})
	case "clear":
		return app.ClearReportProjectionDispatchExecutionSelection(workspaceRoot, app.ReportProjectionDispatchExecutionSelectionClearOptions{
			ClearedBy: request.Actor,
			ClearedAt: at,
			Reason:    request.Reason,
		})
	default:
		return app.ReportProjectionDispatchExecutionSelection{}, fmt.Errorf("report selection action must be select or clear")
	}
}

func printReplayWorkbenchSessionSelectionJSON(out io.Writer, selection *app.ReplayWorkbenchSessionSelection) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if selection == nil {
		return encoder.Encode(nil)
	}
	return encoder.Encode(selection)
}

func printReplayWorkbenchSessionSelectionValueJSON(out io.Writer, selection app.ReplayWorkbenchSessionSelection) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(selection)
}

func printReplayWorkbenchCollaborationSelectionJSON(out io.Writer, selection *app.ReplayWorkbenchCollaborationSelection) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if selection == nil {
		return encoder.Encode(nil)
	}
	return encoder.Encode(selection)
}

func printReplayWorkbenchCollaborationSelectionValueJSON(out io.Writer, selection app.ReplayWorkbenchCollaborationSelection) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(selection)
}

func printReplayWorkbenchSessionArtifact(out io.Writer, session app.ReplayWorkbenchSessionArtifact, includeDetails bool) error {
	if _, err := fmt.Fprintf(
		out,
		"replay-workbench-session: status=%s mode=%s primary=%s selected=%s runs=%d artifact=%s created_by=%s created_at=%s reason=%s\n",
		session.Status,
		session.Mode,
		session.PrimaryRunID,
		session.SelectedRunID,
		len(session.RunIDs),
		session.Path,
		session.CreatedBy,
		session.CreatedAt,
		session.Reason,
	); err != nil {
		return err
	}
	if !includeDetails {
		if session.Annotation != nil {
			if _, err := fmt.Fprintf(
				out,
				"replay-session-annotation: decision=%s reviewer=%s reviewed_at=%s reason=%s\n",
				session.Annotation.Decision,
				session.Annotation.Reviewer,
				session.Annotation.ReviewedAt,
				session.Annotation.Reason,
			); err != nil {
				return err
			}
		}
		return nil
	}
	if session.Annotation != nil {
		if _, err := fmt.Fprintf(
			out,
			"replay-session-annotation: decision=%s reviewer=%s reviewed_at=%s reason=%s\n",
			session.Annotation.Decision,
			session.Annotation.Reviewer,
			session.Annotation.ReviewedAt,
			session.Annotation.Reason,
		); err != nil {
			return err
		}
	}
	for _, runID := range session.RunIDs {
		if _, err := fmt.Fprintf(out, "run: %s\n", runID); err != nil {
			return err
		}
	}
	for _, key := range []string{"edge", "lane", "node", "q"} {
		if value := strings.TrimSpace(session.Filters[key]); value != "" {
			if _, err := fmt.Fprintf(out, "filter: %s=%s\n", key, value); err != nil {
				return err
			}
		}
	}
	for _, key := range []string{"html", "replay", "replays", "sessions", "graph", "graphs", "graph_ndjson", "timeline_ndjson"} {
		if value := strings.TrimSpace(session.Endpoints[key]); value != "" {
			if _, err := fmt.Fprintf(out, "endpoint: %s=%s\n", key, value); err != nil {
				return err
			}
		}
	}
	return nil
}

func printReplayWorkbenchCollaborationArtifact(out io.Writer, collaboration app.ReplayWorkbenchCollaborationArtifact, includeDetails bool) error {
	if _, err := fmt.Fprintf(
		out,
		"replay-workbench-collaboration: status=%s mode=%s session=%s primary=%s selected=%s runs=%d filters=%d participants=%d artifact=%s created_by=%s created_at=%s reason=%s\n",
		collaboration.Status,
		collaboration.Mode,
		collaboration.SessionPath,
		collaboration.PrimaryRunID,
		collaboration.SelectedRunID,
		collaboration.RunIDs,
		collaboration.Filters,
		len(collaboration.Participants),
		collaboration.Path,
		collaboration.CreatedBy,
		collaboration.CreatedAt,
		collaboration.Reason,
	); err != nil {
		return err
	}
	if !includeDetails {
		if collaboration.Annotation != nil {
			if _, err := fmt.Fprintf(
				out,
				"replay-collaboration-annotation: decision=%s reviewer=%s reviewed_at=%s reason=%s\n",
				collaboration.Annotation.Decision,
				collaboration.Annotation.Reviewer,
				collaboration.Annotation.ReviewedAt,
				collaboration.Annotation.Reason,
			); err != nil {
				return err
			}
		}
		return nil
	}
	if collaboration.Annotation != nil {
		if _, err := fmt.Fprintf(
			out,
			"replay-collaboration-annotation: decision=%s reviewer=%s reviewed_at=%s reason=%s\n",
			collaboration.Annotation.Decision,
			collaboration.Annotation.Reviewer,
			collaboration.Annotation.ReviewedAt,
			collaboration.Annotation.Reason,
		); err != nil {
			return err
		}
	}
	for _, participant := range collaboration.Participants {
		if _, err := fmt.Fprintf(out, "participant: %s\n", participant); err != nil {
			return err
		}
	}
	for _, key := range []string{"html", "session", "sessions", "collaborations", "replay", "replays", "graph", "graphs", "graph_ndjson", "timeline_ndjson"} {
		if value := strings.TrimSpace(collaboration.Endpoints[key]); value != "" {
			if _, err := fmt.Fprintf(out, "endpoint: %s=%s\n", key, value); err != nil {
				return err
			}
		}
	}
	return nil
}

func printReplayWorkbenchCollaborationPresence(out io.Writer, presence app.ReplayWorkbenchCollaborationPresence, includeDetails bool) error {
	if _, err := fmt.Fprintf(
		out,
		"replay-workbench-collaboration-presence: status=%s mode=%s collaboration=%s session=%s primary=%s selected=%s participant=%s seen_by=%s seen_at=%s artifact=%s reason=%s\n",
		presence.Status,
		presence.Mode,
		presence.CollaborationPath,
		presence.SessionPath,
		presence.PrimaryRunID,
		presence.SelectedRunID,
		presence.Participant,
		presence.SeenBy,
		presence.SeenAt,
		presence.Path,
		presence.Reason,
	); err != nil {
		return err
	}
	if !includeDetails {
		return nil
	}
	return nil
}

func printReplayWorkbenchCollaborationNote(out io.Writer, note app.ReplayWorkbenchCollaborationNote, includeDetails bool) error {
	target := "none"
	if strings.TrimSpace(note.TargetKind) != "" || strings.TrimSpace(note.TargetID) != "" {
		target = strings.TrimSpace(note.TargetKind) + ":" + strings.TrimSpace(note.TargetID)
	}
	if _, err := fmt.Fprintf(
		out,
		"replay-workbench-collaboration-note: status=%s mode=%s collaboration=%s session=%s primary=%s selected=%s author=%s target=%s created_at=%s artifact=%s reason=%s body=%s\n",
		note.Status,
		note.Mode,
		note.CollaborationPath,
		note.SessionPath,
		note.PrimaryRunID,
		note.SelectedRunID,
		note.Author,
		target,
		note.CreatedAt,
		note.Path,
		note.Reason,
		note.Body,
	); err != nil {
		return err
	}
	if !includeDetails {
		if note.Annotation != nil {
			if _, err := fmt.Fprintf(
				out,
				"replay-collaboration-note-annotation: decision=%s reviewer=%s reviewed_at=%s reason=%s\n",
				note.Annotation.Decision,
				note.Annotation.Reviewer,
				note.Annotation.ReviewedAt,
				note.Annotation.Reason,
			); err != nil {
				return err
			}
		}
		return nil
	}
	if note.Annotation != nil {
		if _, err := fmt.Fprintf(
			out,
			"replay-collaboration-note-annotation: decision=%s reviewer=%s reviewed_at=%s reason=%s\n",
			note.Annotation.Decision,
			note.Annotation.Reviewer,
			note.Annotation.ReviewedAt,
			note.Annotation.Reason,
		); err != nil {
			return err
		}
	}
	return nil
}

func printReplayWorkbenchCollaborationNoteSelection(out io.Writer, selection app.ReplayWorkbenchCollaborationNoteSelection) error {
	actorFields := fmt.Sprintf("selected_by=%s selected_at=%s", selection.SelectedBy, selection.SelectedAt)
	if selection.Status == "cleared" {
		actorFields = fmt.Sprintf("cleared_by=%s cleared_at=%s", selection.ClearedBy, selection.ClearedAt)
	}
	target := "none"
	if strings.TrimSpace(selection.TargetKind) != "" || strings.TrimSpace(selection.TargetID) != "" {
		target = strings.TrimSpace(selection.TargetKind) + ":" + strings.TrimSpace(selection.TargetID)
	}
	if _, err := fmt.Fprintf(
		out,
		"replay-workbench-collaboration-note-selection: status=%s mode=%s note=%s collaboration=%s session=%s primary=%s selected=%s author=%s target=%s superseded=%d %s reason=%s\n",
		selection.Status,
		selection.Mode,
		selection.NotePath,
		selection.CollaborationPath,
		selection.SessionPath,
		selection.PrimaryRunID,
		selection.SelectedRunID,
		selection.Author,
		target,
		len(selection.SupersededPaths),
		actorFields,
		selection.Reason,
	); err != nil {
		return err
	}
	for _, path := range selection.SupersededPaths {
		if _, err := fmt.Fprintf(out, "replay-collaboration-note-superseded: %s\n", path); err != nil {
			return err
		}
	}
	return nil
}

func printReplayWorkbenchSessionSelection(out io.Writer, selection app.ReplayWorkbenchSessionSelection) error {
	actorFields := fmt.Sprintf("selected_by=%s selected_at=%s", selection.SelectedBy, selection.SelectedAt)
	if selection.Status == "cleared" {
		actorFields = fmt.Sprintf("cleared_by=%s cleared_at=%s", selection.ClearedBy, selection.ClearedAt)
	}
	if _, err := fmt.Fprintf(
		out,
		"replay-workbench-session-selection: status=%s mode=%s session=%s primary=%s selected=%s runs=%d filters=%d endpoints=%d superseded=%d %s reason=%s\n",
		selection.Status,
		selection.Mode,
		selection.SessionPath,
		selection.PrimaryRunID,
		selection.SelectedRunID,
		selection.RunIDs,
		selection.Filters,
		selection.Endpoints,
		len(selection.SupersededPaths),
		actorFields,
		selection.Reason,
	); err != nil {
		return err
	}
	for _, path := range selection.SupersededPaths {
		if _, err := fmt.Fprintf(out, "replay-session-superseded: %s\n", path); err != nil {
			return err
		}
	}
	return nil
}

func printReplayWorkbenchCollaborationSelection(out io.Writer, selection app.ReplayWorkbenchCollaborationSelection) error {
	actorFields := fmt.Sprintf("selected_by=%s selected_at=%s", selection.SelectedBy, selection.SelectedAt)
	if selection.Status == "cleared" {
		actorFields = fmt.Sprintf("cleared_by=%s cleared_at=%s", selection.ClearedBy, selection.ClearedAt)
	}
	if _, err := fmt.Fprintf(
		out,
		"replay-workbench-collaboration-selection: status=%s mode=%s collaboration=%s session=%s primary=%s selected=%s runs=%d filters=%d endpoints=%d participants=%d superseded=%d %s reason=%s\n",
		selection.Status,
		selection.Mode,
		selection.CollaborationPath,
		selection.SessionPath,
		selection.PrimaryRunID,
		selection.SelectedRunID,
		selection.RunIDs,
		selection.Filters,
		selection.Endpoints,
		selection.Participants,
		len(selection.SupersededPaths),
		actorFields,
		selection.Reason,
	); err != nil {
		return err
	}
	for _, path := range selection.SupersededPaths {
		if _, err := fmt.Fprintf(out, "replay-collaboration-superseded: %s\n", path); err != nil {
			return err
		}
	}
	return nil
}

func printReplayJSON(out io.Writer, result app.ReplayResult) error {
	return printReplayJSONExport(out, replayExportFromResult(result))
}

func printReplayJSONFiltered(out io.Writer, result app.ReplayResult, filter replayTimelineFilter) error {
	return printReplayJSONExport(out, filter.apply(replayExportFromResult(result)))
}

func printReplayJSONExport(out io.Writer, export replayExport) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(export)
}

func printReplayJSONList(out io.Writer, results []app.ReplayResult) error {
	return printReplayJSONExports(out, replayExportsFromResults(results))
}

func printReplayJSONListFiltered(out io.Writer, results []app.ReplayResult, filter replayTimelineFilter) error {
	return printReplayJSONExports(out, filter.applyAll(replayExportsFromResults(results)))
}

func printReplayJSONExports(out io.Writer, exports []replayExport) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(exports)
}

func printReplayGraphJSONFiltered(out io.Writer, result app.ReplayResult, filter replayTimelineFilter) error {
	return printReplayGraphJSONExport(out, filter.applyGraph(replayGraphExportFromResult(result)))
}

func printReplayGraphJSONListFiltered(out io.Writer, results []app.ReplayResult, filter replayTimelineFilter) error {
	return printReplayGraphJSONExports(out, filter.applyGraphs(replayGraphExportsFromResults(results)))
}

func printReplayGraphJSONExport(out io.Writer, export replayGraphExport) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(export)
}

func printReplayGraphJSONExports(out io.Writer, exports []replayGraphExport) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(exports)
}

func printReplayGraphNDJSONFiltered(out io.Writer, result app.ReplayResult, filter replayTimelineFilter) error {
	return printReplayGraphNDJSONExport(out, filter.applyGraph(replayGraphExportFromResult(result)))
}

func printReplayGraphNDJSONExport(out io.Writer, export replayGraphExport) error {
	summary := struct {
		Kind           string            `json:"kind"`
		RunID          string            `json:"run_id"`
		NodeCount      int               `json:"node_count"`
		EdgeCount      int               `json:"edge_count"`
		NodeTypeCounts map[string]int    `json:"node_type_counts"`
		EdgeTypeCounts map[string]int    `json:"edge_type_counts"`
		Focus          *replayGraphFocus `json:"focus,omitempty"`
	}{
		Kind:           "summary",
		RunID:          export.RunID,
		NodeCount:      export.NodeCount,
		EdgeCount:      export.EdgeCount,
		NodeTypeCounts: export.NodeTypeCounts,
		EdgeTypeCounts: export.EdgeTypeCounts,
		Focus:          export.Focus,
	}
	if err := printJSONLine(out, summary); err != nil {
		return err
	}
	for _, node := range export.Nodes {
		record := struct {
			Kind       string `json:"kind"`
			RunID      string `json:"run_id"`
			ID         string `json:"id"`
			Type       string `json:"type"`
			Label      string `json:"label"`
			SourceKind string `json:"source_kind,omitempty"`
		}{
			Kind:       "node",
			RunID:      export.RunID,
			ID:         node.ID,
			Type:       node.Type,
			Label:      node.Label,
			SourceKind: node.SourceKind,
		}
		if err := printJSONLine(out, record); err != nil {
			return err
		}
	}
	for _, edge := range export.Edges {
		record := struct {
			Kind  string `json:"kind"`
			RunID string `json:"run_id"`
			ID    string `json:"id"`
			From  string `json:"from"`
			To    string `json:"to"`
			Type  string `json:"type"`
		}{
			Kind:  "edge",
			RunID: export.RunID,
			ID:    replayGraphEdgeID(edge),
			From:  edge.From,
			To:    edge.To,
			Type:  edge.Type,
		}
		if err := printJSONLine(out, record); err != nil {
			return err
		}
	}
	return nil
}

func printJSONLine(out io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(data))
	return err
}

func printReplayNDJSON(out io.Writer, result app.ReplayResult) error {
	return printReplayNDJSONFiltered(out, result, replayTimelineFilter{})
}

func printReplayNDJSONFiltered(out io.Writer, result app.ReplayResult, filter replayTimelineFilter) error {
	for _, event := range filter.apply(replayExportFromResult(result)).Timeline {
		data, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("encode replay timeline event: %w", err)
		}
		if _, err := fmt.Fprintln(out, string(data)); err != nil {
			return err
		}
	}
	return nil
}

func (filter replayTimelineFilter) applyAll(exports []replayExport) []replayExport {
	filtered := make([]replayExport, 0, len(exports))
	for _, export := range exports {
		filtered = append(filtered, filter.apply(export))
	}
	return filtered
}

func (filter replayTimelineFilter) applyGraphs(exports []replayGraphExport) []replayGraphExport {
	filtered := make([]replayGraphExport, 0, len(exports))
	for _, export := range exports {
		filtered = append(filtered, filter.applyGraph(export))
	}
	return filtered
}

func (filter replayTimelineFilter) apply(export replayExport) replayExport {
	if strings.TrimSpace(filter.Lane) == "" && strings.TrimSpace(filter.Query) == "" {
		return export
	}
	timeline := make([]replayExportTimeline, 0, len(export.Timeline))
	laneCounts := map[string]int{}
	for _, event := range export.Timeline {
		if !filter.matches(event) {
			continue
		}
		timeline = append(timeline, event)
		laneCounts[event.Lane]++
	}
	export.Timeline = timeline
	export.EventCount = len(timeline)
	export.LaneCounts = laneCounts
	return export
}

func (filter replayTimelineFilter) applyGraph(export replayGraphExport) replayGraphExport {
	if edgeID := strings.TrimSpace(filter.Edge); edgeID != "" {
		export = focusReplayGraphOnEdge(export, edgeID)
	} else if nodeID := strings.TrimSpace(filter.Node); nodeID != "" {
		export = focusReplayGraphOnNode(export, nodeID)
	}
	query := strings.ToLower(strings.TrimSpace(filter.Query))
	if query == "" {
		return export
	}
	keptNodes := map[string]bool{}
	for _, node := range export.Nodes {
		if replayGraphNodeMatches(node, query) {
			keptNodes[node.ID] = true
		}
	}
	edges := make([]replayGraphEdge, 0, len(export.Edges))
	for _, edge := range export.Edges {
		if replayGraphEdgeMatches(edge, query) {
			keptNodes[edge.From] = true
			keptNodes[edge.To] = true
			edges = append(edges, edge)
			continue
		}
		if keptNodes[edge.From] && keptNodes[edge.To] {
			edges = append(edges, edge)
		}
	}
	nodes := make([]replayGraphNode, 0, len(keptNodes))
	for _, node := range export.Nodes {
		if keptNodes[node.ID] {
			nodes = append(nodes, node)
		}
	}
	export.Nodes = nodes
	export.Edges = edges
	return replayGraphWithCounts(export)
}

func focusReplayGraphOnEdge(export replayGraphExport, edgeID string) replayGraphExport {
	edgeID = strings.TrimSpace(edgeID)
	if edgeID == "" {
		return export
	}
	var focused replayGraphEdge
	found := false
	for _, edge := range export.Edges {
		if replayGraphEdgeID(edge) == edgeID {
			focused = edge
			found = true
			break
		}
	}
	if !found {
		export.Nodes = nil
		export.Edges = nil
		export.Focus = &replayGraphFocus{
			Kind:   "edge",
			ID:     edgeID,
			EdgeID: edgeID,
		}
		return replayGraphWithCounts(export)
	}
	keptNodes := map[string]bool{
		focused.From: true,
		focused.To:   true,
	}
	nodes := make([]replayGraphNode, 0, len(keptNodes))
	for _, node := range export.Nodes {
		if keptNodes[node.ID] {
			nodes = append(nodes, node)
		}
	}
	export.Nodes = nodes
	export.Edges = []replayGraphEdge{focused}
	export.Focus = &replayGraphFocus{
		Kind:   "edge",
		ID:     replayGraphEdgeID(focused),
		EdgeID: replayGraphEdgeID(focused),
		From:   focused.From,
		To:     focused.To,
		Type:   focused.Type,
	}
	return replayGraphWithCounts(export)
}

func focusReplayGraphOnNode(export replayGraphExport, nodeID string) replayGraphExport {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return export
	}
	found := false
	for _, node := range export.Nodes {
		if node.ID == nodeID {
			found = true
			break
		}
	}
	if !found {
		export.Nodes = nil
		export.Edges = nil
		export.Focus = &replayGraphFocus{
			Kind:   "node",
			ID:     nodeID,
			NodeID: nodeID,
		}
		return replayGraphWithCounts(export)
	}
	keptNodes := map[string]bool{nodeID: true}
	edges := make([]replayGraphEdge, 0, len(export.Edges))
	for _, edge := range export.Edges {
		if edge.From != nodeID && edge.To != nodeID {
			continue
		}
		keptNodes[edge.From] = true
		keptNodes[edge.To] = true
		edges = append(edges, edge)
	}
	nodes := make([]replayGraphNode, 0, len(keptNodes))
	for _, node := range export.Nodes {
		if keptNodes[node.ID] {
			nodes = append(nodes, node)
		}
	}
	export.Nodes = nodes
	export.Edges = edges
	export.Focus = &replayGraphFocus{
		Kind:   "node",
		ID:     nodeID,
		NodeID: nodeID,
	}
	return replayGraphWithCounts(export)
}

func replayGraphNodeMatches(node replayGraphNode, query string) bool {
	haystack := strings.ToLower(strings.Join([]string{
		node.ID,
		node.Type,
		node.Label,
		node.SourceKind,
	}, " "))
	return strings.Contains(haystack, query)
}

func replayGraphEdgeMatches(edge replayGraphEdge, query string) bool {
	haystack := strings.ToLower(strings.Join([]string{
		replayGraphEdgeID(edge),
		edge.From,
		edge.To,
		edge.Type,
	}, " "))
	return strings.Contains(haystack, query)
}

func (filter replayTimelineFilter) matches(event replayExportTimeline) bool {
	if strings.TrimSpace(filter.Lane) != "" && !strings.EqualFold(strings.TrimSpace(filter.Lane), event.Lane) {
		return false
	}
	query := strings.ToLower(strings.TrimSpace(filter.Query))
	if query == "" {
		return true
	}
	haystack := strings.ToLower(strings.Join([]string{
		event.RunID,
		event.Lane,
		event.Type,
		fmt.Sprint(event.EventID),
		event.CreatedAt.Format(time.RFC3339Nano),
		string(event.Payload),
	}, " "))
	return strings.Contains(haystack, query)
}

func printReplayHTML(out io.Writer, result app.ReplayResult) error {
	exports := []replayExport{replayExportFromResult(result)}
	return printReplayHTMLDocument(out, exports, replayWorkbenchSessionFromExports(exports), nil, nil, nil, nil, nil, nil, nil, nil, nil, false, false, false, false, false, false, false, 0, 0, 0, 0, 0)
}

func printReplayWorkbenchHTML(out io.Writer, results []app.ReplayResult, session replayWorkbenchSession, savedSessions []app.ReplayWorkbenchSessionArtifact, sessionSelection *app.ReplayWorkbenchSessionSelection, reportExecutions []app.ReportProjectionDispatchExecution, reportSelection *app.ReportProjectionDispatchExecutionSelection, collaborations []app.ReplayWorkbenchCollaborationArtifact, collaborationPresence []app.ReplayWorkbenchCollaborationPresence, collaborationNotes []app.ReplayWorkbenchCollaborationNote, collaborationSelection *app.ReplayWorkbenchCollaborationSelection, collaborationNoteSelection *app.ReplayWorkbenchCollaborationNoteSelection, allowReportSelectionWrites bool, allowSessionSelectionWrites bool, allowCollaborationSelectionWrites bool, allowCollaborationPresenceWrites bool, allowCollaborationNoteWrites bool, allowCollaborationNoteSelectionWrites bool, enableCollaborationStream bool, sessionSelectionPollIntervalMS int, collaborationSelectionPollIntervalMS int, collaborationPresencePollIntervalMS int, collaborationNotesPollIntervalMS int, collaborationNoteSelectionPollIntervalMS int) error {
	return printReplayHTMLDocument(out, replayExportsFromResults(results), session, savedSessions, sessionSelection, reportExecutions, reportSelection, collaborations, collaborationPresence, collaborationNotes, collaborationSelection, collaborationNoteSelection, allowReportSelectionWrites, allowSessionSelectionWrites, allowCollaborationSelectionWrites, allowCollaborationPresenceWrites, allowCollaborationNoteWrites, allowCollaborationNoteSelectionWrites, enableCollaborationStream, sessionSelectionPollIntervalMS, collaborationSelectionPollIntervalMS, collaborationPresencePollIntervalMS, collaborationNotesPollIntervalMS, collaborationNoteSelectionPollIntervalMS)
}

func printReplayHTMLDocument(out io.Writer, exports []replayExport, session replayWorkbenchSession, savedSessions []app.ReplayWorkbenchSessionArtifact, sessionSelection *app.ReplayWorkbenchSessionSelection, reportExecutions []app.ReportProjectionDispatchExecution, reportSelection *app.ReportProjectionDispatchExecutionSelection, collaborations []app.ReplayWorkbenchCollaborationArtifact, collaborationPresence []app.ReplayWorkbenchCollaborationPresence, collaborationNotes []app.ReplayWorkbenchCollaborationNote, collaborationSelection *app.ReplayWorkbenchCollaborationSelection, collaborationNoteSelection *app.ReplayWorkbenchCollaborationNoteSelection, allowReportSelectionWrites bool, allowSessionSelectionWrites bool, allowCollaborationSelectionWrites bool, allowCollaborationPresenceWrites bool, allowCollaborationNoteWrites bool, allowCollaborationNoteSelectionWrites bool, enableCollaborationStream bool, sessionSelectionPollIntervalMS int, collaborationSelectionPollIntervalMS int, collaborationPresencePollIntervalMS int, collaborationNotesPollIntervalMS int, collaborationNoteSelectionPollIntervalMS int) error {
	if len(exports) == 0 {
		exports = []replayExport{{ReportPath: replayReportDisplay("")}}
	}
	if session.PrimaryRunID == "" {
		session = replayWorkbenchSessionFromExports(exports)
	}
	primary := exports[0]
	primaryData, err := json.Marshal(primary)
	if err != nil {
		return fmt.Errorf("encode replay html data: %w", err)
	}
	allData, err := json.Marshal(exports)
	if err != nil {
		return fmt.Errorf("encode replay html data: %w", err)
	}
	graphs := make([]replayGraphExport, 0, len(exports))
	for _, export := range exports {
		graphs = append(graphs, export.Graph)
	}
	graphData, err := json.Marshal(graphs)
	if err != nil {
		return fmt.Errorf("encode replay graph html data: %w", err)
	}
	sessionData, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("encode replay html session: %w", err)
	}
	if savedSessions == nil {
		savedSessions = []app.ReplayWorkbenchSessionArtifact{}
	}
	sessionListData, err := json.Marshal(savedSessions)
	if err != nil {
		return fmt.Errorf("encode replay html sessions: %w", err)
	}
	if reportExecutions == nil {
		reportExecutions = []app.ReportProjectionDispatchExecution{}
	}
	reportExecutionData, err := json.Marshal(reportExecutions)
	if err != nil {
		return fmt.Errorf("encode replay html reports: %w", err)
	}
	var reportSelectionData []byte
	if reportSelection == nil {
		reportSelectionData = []byte("null")
	} else {
		reportSelectionData, err = json.Marshal(reportSelection)
		if err != nil {
			return fmt.Errorf("encode replay html report selection: %w", err)
		}
	}
	if collaborations == nil {
		collaborations = []app.ReplayWorkbenchCollaborationArtifact{}
	}
	collaborationData, err := json.Marshal(collaborations)
	if err != nil {
		return fmt.Errorf("encode replay html collaborations: %w", err)
	}
	if collaborationPresence == nil {
		collaborationPresence = []app.ReplayWorkbenchCollaborationPresence{}
	}
	collaborationPresenceData, err := json.Marshal(collaborationPresence)
	if err != nil {
		return fmt.Errorf("encode replay html collaboration presence: %w", err)
	}
	if collaborationNotes == nil {
		collaborationNotes = []app.ReplayWorkbenchCollaborationNote{}
	}
	collaborationNotesData, err := json.Marshal(collaborationNotes)
	if err != nil {
		return fmt.Errorf("encode replay html collaboration notes: %w", err)
	}
	var collaborationSelectionData []byte
	if collaborationSelection == nil {
		collaborationSelectionData = []byte("null")
	} else {
		collaborationSelectionData, err = json.Marshal(collaborationSelection)
		if err != nil {
			return fmt.Errorf("encode replay html collaboration selection: %w", err)
		}
	}
	var collaborationNoteSelectionData []byte
	if collaborationNoteSelection == nil {
		collaborationNoteSelectionData = []byte("null")
	} else {
		collaborationNoteSelectionData, err = json.Marshal(collaborationNoteSelection)
		if err != nil {
			return fmt.Errorf("encode replay html collaboration note selection: %w", err)
		}
	}
	var selectionData []byte
	if sessionSelection == nil {
		selectionData = []byte("null")
	} else {
		selectionData, err = json.Marshal(sessionSelection)
		if err != nil {
			return fmt.Errorf("encode replay html session selection: %w", err)
		}
	}
	runID := html.EscapeString(primary.RunID)
	reportPath := html.EscapeString(primary.ReportPath)
	sessionSelectionWriteControlHTML := replayWorkbenchSessionSelectionWriteControlHTML(allowSessionSelectionWrites)
	sessionSelectionWriteScript := replayWorkbenchSessionSelectionWriteScript(allowSessionSelectionWrites)
	reportSelectionWriteControlHTML := replayWorkbenchReportSelectionWriteControlHTML(allowReportSelectionWrites)
	reportSelectionWriteScript := replayWorkbenchReportSelectionWriteScript(allowReportSelectionWrites)
	selectionWriteControlHTML := replayWorkbenchCollaborationSelectionWriteControlHTML(allowCollaborationSelectionWrites)
	selectionWriteScript := replayWorkbenchCollaborationSelectionWriteScript(allowCollaborationSelectionWrites)
	presenceWriteControlHTML := replayWorkbenchCollaborationPresenceWriteControlHTML(allowCollaborationPresenceWrites)
	presenceWriteScript := replayWorkbenchCollaborationPresenceWriteScript(allowCollaborationPresenceWrites)
	noteWriteControlHTML := replayWorkbenchCollaborationNoteWriteControlHTML(allowCollaborationNoteWrites)
	noteWriteScript := replayWorkbenchCollaborationNoteWriteScript(allowCollaborationNoteWrites)
	noteSelectionWriteControlHTML := replayWorkbenchCollaborationNoteSelectionWriteControlHTML(allowCollaborationNoteSelectionWrites)
	noteSelectionWriteScript := replayWorkbenchCollaborationNoteSelectionWriteScript(allowCollaborationNoteSelectionWrites)
	collaborationControlsHTML := sessionSelectionWriteControlHTML + reportSelectionWriteControlHTML + selectionWriteControlHTML + presenceWriteControlHTML + noteWriteControlHTML + noteSelectionWriteControlHTML
	if sessionSelectionPollIntervalMS < 0 {
		sessionSelectionPollIntervalMS = 0
	}
	if collaborationSelectionPollIntervalMS < 0 {
		collaborationSelectionPollIntervalMS = 0
	}
	if collaborationPresencePollIntervalMS < 0 {
		collaborationPresencePollIntervalMS = 0
	}
	if collaborationNotesPollIntervalMS < 0 {
		collaborationNotesPollIntervalMS = 0
	}
	if collaborationNoteSelectionPollIntervalMS < 0 {
		collaborationNoteSelectionPollIntervalMS = 0
	}
	sessionSelectionPollingScript := replayWorkbenchSessionSelectionPollingScript(sessionSelectionPollIntervalMS)
	selectionPollingScript := replayWorkbenchCollaborationSelectionPollingScript(collaborationSelectionPollIntervalMS)
	presencePollingScript := replayWorkbenchCollaborationPresencePollingScript(collaborationPresencePollIntervalMS)
	notesPollingScript := replayWorkbenchCollaborationNotesPollingScript(collaborationNotesPollIntervalMS)
	noteSelectionPollingScript := replayWorkbenchCollaborationNoteSelectionPollingScript(collaborationNoteSelectionPollIntervalMS)
	collaborationStreamScript := replayWorkbenchCollaborationStreamScript(enableCollaborationStream)
	_, err = fmt.Fprintf(out, `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Ouroboros Replay Timeline - %s</title>
  <style>
    :root { color-scheme: light dark; font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    body { margin: 0; background: #0f172a; color: #e5e7eb; }
    main { width: calc(100%% - 32px); max-width: 1240px; margin: 0 auto; padding: 24px 16px 40px; }
    h1 { margin: 0 0 8px; font-size: 24px; font-weight: 650; }
    p { margin: 0 0 18px; color: #cbd5e1; }
    .toolbar { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; margin: 0 0 14px; }
    label { color: #cbd5e1; font-size: 13px; }
    select, input, button { min-width: 220px; max-width: 100%%; border: 1px solid #334155; border-radius: 6px; background: #111827; color: #f8fafc; padding: 7px 10px; }
    button { min-width: auto; cursor: pointer; background: #0f766e; border-color: #14b8a6; }
    input { flex: 1 1 260px; }
    canvas { width: 100%%; min-height: 320px; border: 1px solid #334155; background: #111827; display: block; }
    h2 { margin: 22px 0 10px; font-size: 18px; font-weight: 650; }
    #ouro-replay-events { display: grid; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); gap: 8px; margin-top: 14px; }
    #ouro-evidence-nodes { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 8px; margin-top: 14px; }
    .event { border: 1px solid #334155; border-radius: 6px; padding: 8px 10px; background: #1f2937; }
    .event strong { display: block; color: #f8fafc; font-size: 13px; }
    .event span { color: #94a3b8; font-size: 12px; }
    .graph-node { border: 1px solid #334155; border-radius: 6px; padding: 8px 10px; background: #111827; }
    .graph-node strong { display: block; color: #f8fafc; font-size: 13px; }
    .graph-node span { color: #94a3b8; font-size: 12px; }
  </style>
</head>
<body>
 <main>
    <h1>Ouroboros Replay Timeline</h1>
    <p id="ouro-replay-summary">run %s · events %d · report %s</p>
    <div class="toolbar">
      <label for="ouro-replay-run-select">Run</label>
      <select id="ouro-replay-run-select" aria-label="Replay run selector"></select>
      <label for="ouro-replay-session-select">Session</label>
      <select id="ouro-replay-session-select" aria-label="Saved replay workbench session selector"></select>
      <label for="ouro-replay-lane-filter">Lane</label>
      <select id="ouro-replay-lane-filter" aria-label="Replay lane filter">
        <option value="">All</option>
        <option value="mission">mission</option>
        <option value="agent">agent</option>
        <option value="observer">observer</option>
        <option value="blackboard">blackboard</option>
        <option value="toolchain">toolchain</option>
        <option value="submission-report">submission-report</option>
      </select>
      <label for="ouro-replay-search">Search</label>
      <input id="ouro-replay-search" type="search" autocomplete="off" aria-label="Replay event search">
      <label for="ouro-graph-node-filter">Graph Node</label>
      <select id="ouro-graph-node-filter" aria-label="Evidence graph node focus"></select>
      <label for="ouro-graph-edge-filter">Graph Edge</label>
      <select id="ouro-graph-edge-filter" aria-label="Evidence graph edge focus"></select>
	    </div>
	    <section id="ouro-replay-session-selection-summary" aria-label="Replay workbench session selection"></section>
	    <section id="ouro-replay-reports-summary" aria-label="Replay workbench reports"></section>
	    <section id="ouro-replay-report-selection-summary" aria-label="Replay workbench report selection"></section>
	    <section id="ouro-replay-collaboration-selection-summary" aria-label="Replay workbench collaboration selection"></section>
	    <section id="ouro-replay-collaboration-summary" aria-label="Replay workbench collaboration sessions"></section>
	    <section id="ouro-replay-collaboration-presence-summary" aria-label="Replay workbench collaboration presence"></section>
	    <section id="ouro-replay-collaboration-notes-summary" aria-label="Replay workbench collaboration notes"></section>
	    <section id="ouro-replay-collaboration-note-selection-summary" aria-label="Replay workbench collaboration note selection"></section>
	    %s
	    <canvas id="ouro-replay-timeline" width="1200" height="520" aria-label="Ouroboros replay timeline lanes"></canvas>
    <section id="ouro-replay-events" aria-label="Replay timeline event list"></section>
    <h2>Evidence Graph</h2>
    <canvas id="ouro-evidence-graph" width="1200" height="520" aria-label="Ouroboros evidence graph"></canvas>
    <section id="ouro-evidence-summary" aria-label="Evidence graph summary"></section>
    <section id="ouro-evidence-detail" aria-label="Evidence graph detail"></section>
    <section id="ouro-evidence-nodes" aria-label="Evidence graph node list"></section>
  </main>
<script>
window.OURO_REPLAY = %s;
window.OURO_REPLAYS = %s;
window.OURO_REPLAY_GRAPHS = %s;
window.OURO_REPLAY_SESSION = %s;
window.OURO_REPLAY_SESSIONS = %s;
window.OURO_REPLAY_SESSION_SELECTION = %s;
window.OURO_REPLAY_REPORTS = %s;
window.OURO_REPLAY_REPORT_SELECTION = %s;
window.OURO_REPLAY_COLLABORATIONS = %s;
window.OURO_REPLAY_COLLABORATION_PRESENCE = %s;
window.OURO_REPLAY_COLLABORATION_NOTES = %s;
window.OURO_REPLAY_COLLABORATION_SELECTION = %s;
window.OURO_REPLAY_COLLABORATION_NOTE_SELECTION = %s;
window.OURO_REPLAY_REPORT_SELECTION_WRITES_ENABLED = %t;
window.OURO_REPLAY_SESSION_SELECTION_WRITES_ENABLED = %t;
window.OURO_REPLAY_COLLABORATION_SELECTION_WRITES_ENABLED = %t;
window.OURO_REPLAY_COLLABORATION_PRESENCE_WRITES_ENABLED = %t;
window.OURO_REPLAY_COLLABORATION_NOTE_WRITES_ENABLED = %t;
window.OURO_REPLAY_COLLABORATION_NOTE_SELECTION_WRITES_ENABLED = %t;
window.OURO_REPLAY_COLLABORATION_STREAM_ENABLED = %t;
window.OURO_REPLAY_SESSION_SELECTION_POLL_INTERVAL_MS = %d;
window.OURO_REPLAY_COLLABORATION_SELECTION_POLL_INTERVAL_MS = %d;
window.OURO_REPLAY_COLLABORATION_PRESENCE_POLL_INTERVAL_MS = %d;
window.OURO_REPLAY_COLLABORATION_NOTES_POLL_INTERVAL_MS = %d;
window.OURO_REPLAY_COLLABORATION_NOTE_SELECTION_POLL_INTERVAL_MS = %d;
(function drawReplayTimeline() {
  const replays = Array.isArray(window.OURO_REPLAYS) && window.OURO_REPLAYS.length ? window.OURO_REPLAYS : [window.OURO_REPLAY || {}];
  const graphs = Array.isArray(window.OURO_REPLAY_GRAPHS) ? window.OURO_REPLAY_GRAPHS : [];
  const session = window.OURO_REPLAY_SESSION || {};
  const savedSessions = Array.isArray(window.OURO_REPLAY_SESSIONS) ? window.OURO_REPLAY_SESSIONS : [];
  let sessionSelection = window.OURO_REPLAY_SESSION_SELECTION || null;
  const reportExecutions = Array.isArray(window.OURO_REPLAY_REPORTS) ? window.OURO_REPLAY_REPORTS : [];
  let reportSelection = window.OURO_REPLAY_REPORT_SELECTION || null;
  const collaborations = Array.isArray(window.OURO_REPLAY_COLLABORATIONS) ? window.OURO_REPLAY_COLLABORATIONS : [];
  let collaborationPresence = Array.isArray(window.OURO_REPLAY_COLLABORATION_PRESENCE) ? window.OURO_REPLAY_COLLABORATION_PRESENCE : [];
  let collaborationNotes = Array.isArray(window.OURO_REPLAY_COLLABORATION_NOTES) ? window.OURO_REPLAY_COLLABORATION_NOTES : [];
  let collaborationSelection = window.OURO_REPLAY_COLLABORATION_SELECTION || null;
  let collaborationNoteSelection = window.OURO_REPLAY_COLLABORATION_NOTE_SELECTION || null;
  const reportSelectionWritesEnabled = window.OURO_REPLAY_REPORT_SELECTION_WRITES_ENABLED === true;
  const sessionSelectionWritesEnabled = window.OURO_REPLAY_SESSION_SELECTION_WRITES_ENABLED === true;
  const collaborationSelectionWritesEnabled = window.OURO_REPLAY_COLLABORATION_SELECTION_WRITES_ENABLED === true;
  const collaborationPresenceWritesEnabled = window.OURO_REPLAY_COLLABORATION_PRESENCE_WRITES_ENABLED === true;
  const collaborationNoteWritesEnabled = window.OURO_REPLAY_COLLABORATION_NOTE_WRITES_ENABLED === true;
  const collaborationNoteSelectionWritesEnabled = window.OURO_REPLAY_COLLABORATION_NOTE_SELECTION_WRITES_ENABLED === true;
  const collaborationStreamEnabled = window.OURO_REPLAY_COLLABORATION_STREAM_ENABLED === true;
  const sessionSelectionPollIntervalMS = %d;
  const collaborationSelectionPollIntervalMS = %d;
  const collaborationPresencePollIntervalMS = %d;
  const collaborationNotesPollIntervalMS = %d;
  const collaborationNoteSelectionPollIntervalMS = %d;
  const selector = document.getElementById("ouro-replay-run-select");
  const sessionSelector = document.getElementById("ouro-replay-session-select");
  const laneFilter = document.getElementById("ouro-replay-lane-filter");
  const search = document.getElementById("ouro-replay-search");
  const nodeFilter = document.getElementById("ouro-graph-node-filter");
  const edgeFilter = document.getElementById("ouro-graph-edge-filter");
  const summary = document.getElementById("ouro-replay-summary");
  const selectionSummary = document.getElementById("ouro-replay-session-selection-summary");
  const reportsSummary = document.getElementById("ouro-replay-reports-summary");
  const reportSelectionSummary = document.getElementById("ouro-replay-report-selection-summary");
  const collaborationSelectionSummary = document.getElementById("ouro-replay-collaboration-selection-summary");
  const collaborationSummary = document.getElementById("ouro-replay-collaboration-summary");
  const collaborationPresenceSummary = document.getElementById("ouro-replay-collaboration-presence-summary");
  const collaborationNotesSummary = document.getElementById("ouro-replay-collaboration-notes-summary");
  const collaborationNoteSelectionSummary = document.getElementById("ouro-replay-collaboration-note-selection-summary");
  const selectionSessionPath = sessionSelection ? String(sessionSelection.session_path || "") : "";
  sessionSelector.replaceChildren(new Option("Saved sessions", ""));
  savedSessions.forEach((saved, index) => {
    const isCurrent = selectionSessionPath && String(saved.path || "") === selectionSessionPath && String(sessionSelection.status || "") === "current";
    const label = (isCurrent ? "current · " : "") + String(saved.reason || saved.selected_run_id || saved.path || "session");
    sessionSelector.add(new Option(label, String(index)));
    if (isCurrent) sessionSelector.value = String(index);
  });
  selector.innerHTML = replays.map((replay, index) => (
    "<option value=\"" + index + "\">" + String(replay.run_id || "run " + (index + 1)) + "</option>"
  )).join("");
  const selectedRun = String(session.selected_run_id || "");
  const selectedIndex = selectedRun ? replays.findIndex((replay) => String(replay.run_id || "") === selectedRun) : -1;
  if (selectedIndex >= 0) selector.value = String(selectedIndex);
  const filters = session.filters || {};
  laneFilter.value = String(filters.lane || "");
  search.value = String(filters.q || "");
  nodeFilter.dataset.initialNode = String(filters.node || "");
  edgeFilter.dataset.initialEdge = String(filters.edge || "");
  function filterReplayTimeline(replay) {
    const lane = String(laneFilter.value || "");
    const query = String(search.value || "").trim().toLowerCase();
    const original = Array.isArray(replay.timeline) ? replay.timeline : [];
    const timeline = original.filter((event) => {
      const eventLane = String(event.lane || "");
      if (lane && eventLane.toLowerCase() !== lane.toLowerCase()) return false;
      if (!query) return true;
      return [
        replay.run_id,
        event.event_id,
        eventLane,
        event.type,
        event.created_at,
        JSON.stringify(event.payload || {})
      ].join(" ").toLowerCase().includes(query);
    });
    const laneCounts = {};
    timeline.forEach((event) => {
      const eventLane = String(event.lane || "unknown");
      laneCounts[eventLane] = (laneCounts[eventLane] || 0) + 1;
    });
    return Object.assign({}, replay, { timeline: timeline, event_count: timeline.length, lane_counts: laneCounts });
  }
  function filterReplayGraph(graph) {
    graph = filterReplayGraphByEdge(graph);
    if (!String(edgeFilter.value || "").trim()) graph = filterReplayGraphByNode(graph);
    const query = String(search.value || "").trim().toLowerCase();
    const nodes = Array.isArray(graph.nodes) ? graph.nodes : [];
    const edges = Array.isArray(graph.edges) ? graph.edges : [];
    if (!query) return graphWithCounts(graph, nodes, edges);
    const kept = new Set();
    nodes.forEach((node) => {
      const haystack = [node.id, node.type, node.label, node.source_kind].join(" ").toLowerCase();
      if (haystack.includes(query)) kept.add(String(node.id || ""));
    });
    const filteredEdges = [];
    edges.forEach((edge) => {
      const haystack = [edge.from, edge.to, edge.type].join(" ").toLowerCase();
      if (haystack.includes(query)) {
        kept.add(String(edge.from || ""));
        kept.add(String(edge.to || ""));
        filteredEdges.push(edge);
      } else if (kept.has(String(edge.from || "")) && kept.has(String(edge.to || ""))) {
        filteredEdges.push(edge);
      }
    });
    const filteredNodes = nodes.filter((node) => kept.has(String(node.id || "")));
    return graphWithCounts(graph, filteredNodes, filteredEdges);
  }
  function graphWithCounts(graph, nodes, edges) {
    const next = Object.assign({}, graph, { nodes: nodes, edges: edges, node_count: nodes.length, edge_count: edges.length });
    if (next.focus) next.focus = Object.assign({}, next.focus, { node_count: nodes.length, edge_count: edges.length });
    return next;
  }
  function graphEdgeID(edge) {
    return String(edge.id || (String(edge.from || "") + ":" + String(edge.to || "") + ":" + String(edge.type || "")));
  }
  function filterReplayGraphByEdge(graph) {
    const edgeID = String(edgeFilter.value || "").trim();
    const nodes = Array.isArray(graph.nodes) ? graph.nodes : [];
    const edges = Array.isArray(graph.edges) ? graph.edges : [];
    if (!edgeID) return graphWithCounts(graph, nodes, edges);
    const selected = edges.find((edge) => graphEdgeID(edge) === edgeID);
    if (!selected) {
      return graphWithCounts(Object.assign({}, graph, { focus: { kind: "edge", id: edgeID, edge_id: edgeID } }), [], []);
    }
    const kept = new Set([String(selected.from || ""), String(selected.to || "")]);
    const focusedNodes = nodes.filter((node) => kept.has(String(node.id || "")));
    return graphWithCounts(Object.assign({}, graph, {
      focus: {
        kind: "edge",
        id: edgeID,
        edge_id: edgeID,
        from: String(selected.from || ""),
        to: String(selected.to || ""),
        type: String(selected.type || "")
      }
    }), focusedNodes, [selected]);
  }
  function filterReplayGraphByNode(graph) {
    const nodeID = String(nodeFilter.value || "").trim();
    const nodes = Array.isArray(graph.nodes) ? graph.nodes : [];
    const edges = Array.isArray(graph.edges) ? graph.edges : [];
    if (!nodeID) return graphWithCounts(graph, nodes, edges);
    if (!nodes.some((node) => String(node.id || "") === nodeID)) {
      return graphWithCounts(Object.assign({}, graph, { focus: { kind: "node", id: nodeID, node_id: nodeID } }), [], []);
    }
    const kept = new Set([nodeID]);
    const focusedEdges = [];
    edges.forEach((edge) => {
      const from = String(edge.from || "");
      const to = String(edge.to || "");
      if (from !== nodeID && to !== nodeID) return;
      kept.add(from);
      kept.add(to);
      focusedEdges.push(edge);
    });
    const focusedNodes = nodes.filter((node) => kept.has(String(node.id || "")));
    return graphWithCounts(Object.assign({}, graph, { focus: { kind: "node", id: nodeID, node_id: nodeID } }), focusedNodes, focusedEdges);
  }
  function populateGraphNodeFilter(graph) {
    const nodes = Array.isArray(graph.nodes) ? graph.nodes : [];
    const current = String(nodeFilter.value || nodeFilter.dataset.initialNode || "");
    nodeFilter.innerHTML = "<option value=\"\">All graph</option>" + nodes.map((node) => (
      "<option value=\"" + String(node.id || "") + "\">" +
      String(node.type || "Node") + " · " + String(node.id || "") + "</option>"
    )).join("");
    if (current && nodes.some((node) => String(node.id || "") === current)) {
      nodeFilter.value = current;
    } else {
      nodeFilter.value = "";
    }
    nodeFilter.dataset.initialNode = "";
  }
  function populateGraphEdgeFilter(graph) {
    const edges = Array.isArray(graph.edges) ? graph.edges : [];
    const current = String(edgeFilter.value || edgeFilter.dataset.initialEdge || "");
    edgeFilter.innerHTML = "<option value=\"\">All edges</option>" + edges.map((edge) => {
      const edgeID = graphEdgeID(edge);
      return "<option value=\"" + edgeID + "\">" +
      String(edge.type || "edge") + " · " + String(edge.from || "") + " -> " + String(edge.to || "") + "</option>";
    }).join("");
    if (current && edges.some((edge) => graphEdgeID(edge) === current)) {
      edgeFilter.value = current;
    } else {
      edgeFilter.value = "";
    }
    edgeFilter.dataset.initialEdge = "";
  }
  function updateReplayWorkbenchURL() {
    const params = new URLSearchParams(window.location.search || "");
    const replay = replays[Number(selector.value || 0)] || {};
    const runID = String(replay.run_id || "");
    if (runID) params.set("run", runID); else params.delete("run");
    if (laneFilter.value) params.set("lane", laneFilter.value); else params.delete("lane");
    if (search.value.trim()) params.set("q", search.value.trim()); else params.delete("q");
    if (nodeFilter.value) params.set("node", nodeFilter.value); else params.delete("node");
    if (edgeFilter.value) params.set("edge", edgeFilter.value); else params.delete("edge");
    const query = params.toString();
    const next = window.location.pathname + (query ? "?" + query : "");
    if (window.history && window.history.replaceState) window.history.replaceState(null, "", next);
  }
  function escapeReplayHTML(value) {
    return String(value || "").replace(/[&<>"']/g, (ch) => ({
      "&": "&amp;",
      "<": "&lt;",
      ">": "&gt;",
      "\"": "&quot;",
      "'": "&#39;"
    })[ch]);
  }
  function renderReplayWorkbenchSessionSelection() {
    if (!selectionSummary) return;
    if (!sessionSelection || !sessionSelection.status) {
      selectionSummary.innerHTML = "";
      return;
    }
    const status = String(sessionSelection.status || "unknown");
    const actor = status === "cleared" ? String(sessionSelection.cleared_by || "unknown") : String(sessionSelection.selected_by || "unknown");
    const at = status === "cleared" ? String(sessionSelection.cleared_at || "unknown") : String(sessionSelection.selected_at || "unknown");
    const reason = String(sessionSelection.reason || "none");
    selectionSummary.innerHTML =
      "<article class=\"event\"><strong>session selection " + escapeReplayHTML(status) +
      "</strong><span>" + escapeReplayHTML(actor) + " · " + escapeReplayHTML(at) +
      " · " + escapeReplayHTML(reason) + " · " + escapeReplayHTML(String(sessionSelection.session_path || "none")) +
      "</span></article>";
  }
  function renderReplayWorkbenchReports() {
    if (!reportsSummary) return;
    if (!reportExecutions.length) {
      reportsSummary.innerHTML = "";
      return;
    }
    reportsSummary.innerHTML = reportExecutions.map((report) => {
      const decision = report.annotation && report.annotation.decision ? String(report.annotation.decision) : "unreviewed";
      return "<article class=\"event\"><strong>report projection " + escapeReplayHTML(String(report.status || "unknown")) +
        "</strong><span>" + escapeReplayHTML(String(report.run_id || "unknown")) +
        " · " + escapeReplayHTML(decision) +
        " · written " + escapeReplayHTML(String(report.written === true)) +
        " · " + escapeReplayHTML(String(report.report_path || "none")) +
        "</span></article>";
    }).join("");
  }
  function renderReplayWorkbenchReportSelection() {
    if (!reportSelectionSummary) return;
    if (!reportSelection || !reportSelection.status) {
      reportSelectionSummary.innerHTML = "";
      return;
    }
    const status = String(reportSelection.status || "unknown");
    const actor = status === "cleared" ? String(reportSelection.cleared_by || "unknown") : String(reportSelection.selected_by || "unknown");
    const at = status === "cleared" ? String(reportSelection.cleared_at || "unknown") : String(reportSelection.selected_at || "unknown");
    reportSelectionSummary.innerHTML =
      "<article class=\"event\"><strong>report selection " + escapeReplayHTML(status) +
      "</strong><span>" + escapeReplayHTML(actor) +
      " · " + escapeReplayHTML(at) +
      " · " + escapeReplayHTML(String(reportSelection.reason || "none")) +
      " · " + escapeReplayHTML(String(reportSelection.report_path || "none")) +
      "</span></article>";
  }
  function renderReplayWorkbenchCollaborationSelection() {
    if (!collaborationSelectionSummary) return;
    if (!collaborationSelection || !collaborationSelection.status) {
      collaborationSelectionSummary.innerHTML = "";
      return;
    }
    const status = String(collaborationSelection.status || "unknown");
    const actor = status === "cleared" ? String(collaborationSelection.cleared_by || "unknown") : String(collaborationSelection.selected_by || "unknown");
    const at = status === "cleared" ? String(collaborationSelection.cleared_at || "unknown") : String(collaborationSelection.selected_at || "unknown");
    const reason = String(collaborationSelection.reason || "none");
    collaborationSelectionSummary.innerHTML =
      "<article class=\"event\"><strong>collaboration selection " + escapeReplayHTML(status) +
      "</strong><span>" + escapeReplayHTML(actor) + " · " + escapeReplayHTML(at) +
      " · " + escapeReplayHTML(reason) + " · " + escapeReplayHTML(String(collaborationSelection.collaboration_path || "none")) +
      "</span></article>";
  }
  function renderReplayWorkbenchCollaborations() {
    if (!collaborationSummary) return;
    if (!collaborations.length) {
      collaborationSummary.innerHTML = "";
      return;
    }
    collaborationSummary.innerHTML = collaborations.map((collaboration) => {
      const participants = Array.isArray(collaboration.participants) ? collaboration.participants.length : 0;
      return "<article class=\"event\"><strong>collaboration " + escapeReplayHTML(String(collaboration.status || "unknown")) +
        "</strong><span>" + escapeReplayHTML(String(collaboration.created_by || "unknown")) +
        " · participants " + String(participants) +
        " · " + escapeReplayHTML(String(collaboration.reason || "none")) +
        " · " + escapeReplayHTML(String(collaboration.session_path || "none")) +
      "</span></article>";
    }).join("");
  }
  function renderReplayWorkbenchCollaborationPresence() {
    if (!collaborationPresenceSummary) return;
    if (!collaborationPresence.length) {
      collaborationPresenceSummary.innerHTML = "";
      return;
    }
    collaborationPresenceSummary.innerHTML = collaborationPresence.map((presence) => {
      return "<article class=\"event\"><strong>presence " + escapeReplayHTML(String(presence.status || "unknown")) +
        "</strong><span>" + escapeReplayHTML(String(presence.participant || "unknown")) +
        " · " + escapeReplayHTML(String(presence.seen_by || "unknown")) +
        " · " + escapeReplayHTML(String(presence.seen_at || "unknown")) +
        " · " + escapeReplayHTML(String(presence.reason || "none")) +
        "</span></article>";
    }).join("");
  }
  function renderReplayWorkbenchCollaborationNotes() {
    if (!collaborationNotesSummary) return;
    if (!collaborationNotes.length) {
      collaborationNotesSummary.innerHTML = "";
      return;
    }
    collaborationNotesSummary.innerHTML = collaborationNotes.map((note) => {
      const target = String(note.target_kind || "note") + (note.target_id ? ":" + String(note.target_id || "") : "");
      return "<article class=\"event\"><strong>collaboration note " + escapeReplayHTML(String(note.status || "unknown")) +
        "</strong><span>" + escapeReplayHTML(String(note.author || "unknown")) +
        " · " + escapeReplayHTML(target) +
        " · " + escapeReplayHTML(String(note.body || "")) +
        " · " + escapeReplayHTML(String(note.reason || "none")) +
        "</span></article>";
    }).join("");
  }
  function renderReplayWorkbenchCollaborationNoteSelection() {
    if (!collaborationNoteSelectionSummary) return;
    if (!collaborationNoteSelection) {
      collaborationNoteSelectionSummary.innerHTML = "";
      return;
    }
    const status = String(collaborationNoteSelection.status || "unknown");
    const notePath = String(collaborationNoteSelection.note_path || "unknown");
    const author = String(collaborationNoteSelection.author || "unknown");
    const target = String(collaborationNoteSelection.target_kind || "note") +
      (collaborationNoteSelection.target_id ? ":" + String(collaborationNoteSelection.target_id || "") : "");
    const actor = status === "cleared" ? String(collaborationNoteSelection.cleared_by || "unknown") : String(collaborationNoteSelection.selected_by || "unknown");
    const at = status === "cleared" ? String(collaborationNoteSelection.cleared_at || "unknown") : String(collaborationNoteSelection.selected_at || "unknown");
    collaborationNoteSelectionSummary.innerHTML = "<article class=\"event\"><strong>collaboration note selection " +
      escapeReplayHTML(status) + "</strong><span>" + escapeReplayHTML(author) +
      " · " + escapeReplayHTML(target) +
      " · " + escapeReplayHTML(actor) +
      " · " + escapeReplayHTML(at) +
      " · " + escapeReplayHTML(String(collaborationNoteSelection.reason || "none")) +
      " · " + escapeReplayHTML(notePath) +
      "</span></article>";
  }
%s
%s
%s
%s
%s
%s
%s
%s
%s
%s
%s
%s
  function applyReplayWorkbenchSession(saved) {
    saved = saved || {};
    const selectedRunID = String(saved.selected_run_id || "");
    if (selectedRunID) {
      const nextIndex = replays.findIndex((replay) => String(replay.run_id || "") === selectedRunID);
      if (nextIndex >= 0) selector.value = String(nextIndex);
    }
    const filters = saved.filters || {};
    laneFilter.value = String(filters.lane || "");
    search.value = String(filters.q || "");
    nodeFilter.value = "";
    edgeFilter.value = "";
    nodeFilter.dataset.initialNode = String(filters.node || "");
    edgeFilter.dataset.initialEdge = String(filters.edge || "");
    renderCurrentReplay();
    updateReplayWorkbenchURL();
  }
  function renderReplay(replay) {
  const timeline = Array.isArray(replay.timeline) ? replay.timeline : [];
  const laneCounts = replay.lane_counts || {};
  const preferred = ["mission", "agent", "observer", "blackboard", "toolchain", "submission-report"];
  const present = Object.keys(laneCounts);
  const lanes = preferred.filter((lane) => present.includes(lane)).concat(
    present.filter((lane) => !preferred.includes(lane)).sort()
  );
  const canvas = document.getElementById("ouro-replay-timeline");
  const ctx = canvas.getContext("2d");
  const list = document.getElementById("ouro-replay-events");
  const left = 150;
  const top = 52;
  const rowHeight = 58;
  const right = 36;
  const width = canvas.width;
  summary.textContent = "run " + String(replay.run_id || "unknown") + " · events " + String(replay.event_count || 0) + " · report " + String(replay.report_path || "not-generated");
  canvas.height = Math.max(320, top + Math.max(1, lanes.length) * rowHeight + 54);
  ctx.clearRect(0, 0, canvas.width, canvas.height);
  ctx.fillStyle = "#111827";
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  ctx.font = "14px ui-sans-serif, system-ui, sans-serif";
  ctx.fillStyle = "#e5e7eb";
  ctx.fillText("Replay timeline events: " + timeline.length, left, 26);
  const laneIndex = new Map(lanes.map((lane, index) => [lane, index]));
  lanes.forEach((lane, index) => {
    const y = top + index * rowHeight;
    ctx.strokeStyle = "#334155";
    ctx.beginPath();
    ctx.moveTo(left, y);
    ctx.lineTo(width - right, y);
    ctx.stroke();
    ctx.fillStyle = "#cbd5e1";
    ctx.fillText(lane + " (" + (laneCounts[lane] || 0) + ")", 18, y + 5);
  });
  const span = Math.max(1, timeline.length - 1);
  timeline.forEach((event, index) => {
    const lane = String(event.lane || "unknown");
    const laneY = top + (laneIndex.get(lane) || 0) * rowHeight;
    const x = left + ((width - left - right) * index / span);
    ctx.fillStyle = "#38bdf8";
    ctx.beginPath();
    ctx.arc(x, laneY, 6, 0, Math.PI * 2);
    ctx.fill();
    ctx.fillStyle = "#e5e7eb";
    if (index === 0 || index === timeline.length - 1) {
      ctx.fillText("timeline event " + event.event_id + " " + event.type, Math.min(x + 8, width - 260), laneY - 10);
    }
  });
  list.innerHTML = timeline.map((event) => (
    "<article class=\"event\"><strong>timeline event " + event.event_id + "</strong><span>" +
    String(event.lane || "unknown") + " · " + String(event.type || "unknown") +
    "</span></article>"
  )).join("");
  }
  function renderReplayGraph(graph) {
    const nodes = Array.isArray(graph.nodes) ? graph.nodes : [];
    const edges = Array.isArray(graph.edges) ? graph.edges : [];
    const canvas = document.getElementById("ouro-evidence-graph");
    const ctx = canvas.getContext("2d");
    const list = document.getElementById("ouro-evidence-nodes");
    const radius = Math.min(210, Math.max(80, 34 * Math.max(1, nodes.length)));
    const centerX = canvas.width / 2;
    const centerY = Math.max(170, canvas.height / 2);
    canvas.height = Math.max(320, 220 + Math.ceil(nodes.length / 8) * 80);
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    ctx.fillStyle = "#111827";
    ctx.fillRect(0, 0, canvas.width, canvas.height);
    ctx.fillStyle = "#e5e7eb";
    ctx.font = "14px ui-sans-serif, system-ui, sans-serif";
    ctx.fillText("Evidence Graph nodes: " + nodes.length + " edges: " + edges.length, 24, 28);
    renderReplayGraphSummary({ nodes: nodes, edges: edges });
    renderReplayGraphDetail({ nodes: nodes, edges: edges, focus: graph.focus });
    const positions = {};
    nodes.forEach((node, index) => {
      const angle = (Math.PI * 2 * index / Math.max(1, nodes.length)) - Math.PI / 2;
      positions[String(node.id || "")] = {
        x: centerX + Math.cos(angle) * radius,
        y: centerY + Math.sin(angle) * radius
      };
    });
    edges.forEach((edge) => {
      const from = positions[String(edge.from || "")];
      const to = positions[String(edge.to || "")];
      if (!from || !to) return;
      ctx.strokeStyle = "#475569";
      ctx.beginPath();
      ctx.moveTo(from.x, from.y);
      ctx.lineTo(to.x, to.y);
      ctx.stroke();
    });
    nodes.forEach((node) => {
      const point = positions[String(node.id || "")] || { x: centerX, y: centerY };
      ctx.fillStyle = "#0f766e";
      ctx.beginPath();
      ctx.arc(point.x, point.y, 10, 0, Math.PI * 2);
      ctx.fill();
      ctx.fillStyle = "#e5e7eb";
      ctx.fillText(String(node.type || "Node"), point.x + 14, point.y + 4);
    });
    list.innerHTML = nodes.map((node) => (
      "<article class=\"graph-node\"><strong>" + String(node.type || "Node") + " · " + String(node.id || "") +
      "</strong><span>" + String(node.label || "") + "</span></article>"
    )).join("");
  }
  function renderReplayGraphSummary(graph) {
    const nodes = Array.isArray(graph.nodes) ? graph.nodes : [];
    const edges = Array.isArray(graph.edges) ? graph.edges : [];
    const nodeTypes = {};
    nodes.forEach((node) => {
      const kind = String(node.type || "Node");
      nodeTypes[kind] = (nodeTypes[kind] || 0) + 1;
    });
    const edgeTypes = {};
    edges.forEach((edge) => {
      const kind = String(edge.type || "edge");
      edgeTypes[kind] = (edgeTypes[kind] || 0) + 1;
    });
    const nodeText = Object.keys(nodeTypes).sort().map((kind) => kind + ":" + nodeTypes[kind]).join(" ");
    const edgeText = Object.keys(edgeTypes).sort().map((kind) => kind + ":" + edgeTypes[kind]).join(" ");
    document.getElementById("ouro-evidence-summary").innerHTML =
      "<article class=\"event\"><strong>graph summary</strong><span>nodes " + nodes.length +
      " · edges " + edges.length + " · node types " + (nodeText || "none") +
      " · edge types " + (edgeText || "none") + "</span></article>";
  }
  function renderReplayGraphDetail(graph) {
    const nodes = Array.isArray(graph.nodes) ? graph.nodes : [];
    const edges = Array.isArray(graph.edges) ? graph.edges : [];
    const focus = graph.focus || {};
    let detail = "showing " + nodes.length + " nodes and " + edges.length + " edges";
    if (focus.kind === "edge") {
      const edgeID = String(focus.edge_id || focus.id || "");
      const edge = edges.find((candidate) => graphEdgeID(candidate) === edgeID) || {};
      detail = "selected graph edge " + edgeID + " · " +
        String(focus.from || edge.from || "") + " -> " + String(focus.to || edge.to || "") +
        " · " + String(focus.type || edge.type || "edge") +
        " · nodes " + String(focus.node_count || nodes.length) +
        " · edges " + String(focus.edge_count || edges.length);
    } else if (focus.kind === "node") {
      const nodeID = String(focus.node_id || focus.id || "");
      const node = nodes.find((candidate) => String(candidate.id || "") === nodeID) || {};
      detail = "selected graph node " + nodeID + " · " +
        String(node.type || "Node") + " · " + String(node.label || "") +
        " · incident edges " + String(focus.edge_count || edges.length);
    }
    document.getElementById("ouro-evidence-detail").innerHTML =
      "<article class=\"event\"><strong>graph detail</strong><span>" + detail + "</span></article>";
  }
  function renderCurrentReplay() {
    const index = Number(selector.value || 0);
    const graph = graphs[index] || (replays[index] || {}).graph || {};
    populateGraphNodeFilter(graph);
    populateGraphEdgeFilter(graph);
    renderReplay(filterReplayTimeline(replays[index] || replays[0] || {}));
    renderReplayGraph(filterReplayGraph(graph));
  }
  selector.addEventListener("change", () => { renderCurrentReplay(); updateReplayWorkbenchURL(); });
  sessionSelector.addEventListener("change", () => {
    const index = Number(sessionSelector.value);
    if (!Number.isNaN(index) && savedSessions[index]) applyReplayWorkbenchSession(savedSessions[index]);
  });
  laneFilter.addEventListener("change", () => { renderCurrentReplay(); updateReplayWorkbenchURL(); });
  search.addEventListener("input", () => { renderCurrentReplay(); updateReplayWorkbenchURL(); });
  nodeFilter.addEventListener("change", () => { renderCurrentReplay(); updateReplayWorkbenchURL(); });
  edgeFilter.addEventListener("change", () => { renderCurrentReplay(); updateReplayWorkbenchURL(); });
  renderReplayWorkbenchSessionSelection();
  renderReplayWorkbenchReports();
  renderReplayWorkbenchReportSelection();
  renderReplayWorkbenchCollaborationSelection();
  renderReplayWorkbenchCollaborations();
  renderReplayWorkbenchCollaborationPresence();
  renderReplayWorkbenchCollaborationNotes();
  renderReplayWorkbenchCollaborationNoteSelection();
  renderCurrentReplay();
})();
  </script>
</body>
</html>
		`, runID, runID, primary.EventCount, reportPath, collaborationControlsHTML, string(primaryData), string(allData), string(graphData), string(sessionData), string(sessionListData), string(selectionData), string(reportExecutionData), string(reportSelectionData), string(collaborationData), string(collaborationPresenceData), string(collaborationNotesData), string(collaborationSelectionData), string(collaborationNoteSelectionData), allowReportSelectionWrites, allowSessionSelectionWrites, allowCollaborationSelectionWrites, allowCollaborationPresenceWrites, allowCollaborationNoteWrites, allowCollaborationNoteSelectionWrites, enableCollaborationStream, sessionSelectionPollIntervalMS, collaborationSelectionPollIntervalMS, collaborationPresencePollIntervalMS, collaborationNotesPollIntervalMS, collaborationNoteSelectionPollIntervalMS, sessionSelectionPollIntervalMS, collaborationSelectionPollIntervalMS, collaborationPresencePollIntervalMS, collaborationNotesPollIntervalMS, collaborationNoteSelectionPollIntervalMS, sessionSelectionPollingScript, collaborationStreamScript, selectionPollingScript, noteSelectionPollingScript, sessionSelectionWriteScript, reportSelectionWriteScript, selectionWriteScript, presencePollingScript, presenceWriteScript, noteWriteScript, noteSelectionWriteScript, notesPollingScript)
	return err
}

func streamReplayWorkbenchCollaborationEvents(w http.ResponseWriter, r *http.Request, initial replayWorkbenchCollaborationEventSnapshot, broker *replayWorkbenchCollaborationEventBroker, refreshInterval time.Duration, refreshSnapshot func() replayWorkbenchCollaborationEventSnapshot) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("collaboration event streaming is not supported by this response writer")
	}
	events, unsubscribe := broker.subscribe()
	defer unsubscribe()
	var refresh <-chan time.Time
	var ticker *time.Ticker
	if refreshInterval > 0 && refreshSnapshot != nil {
		ticker = time.NewTicker(refreshInterval)
		defer ticker.Stop()
		refresh = ticker.C
	}
	if err := writeReplayWorkbenchCollaborationEvent(w, initial); err != nil {
		return err
	}
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return nil
		case snapshot, ok := <-events:
			if !ok {
				return nil
			}
			if err := writeReplayWorkbenchCollaborationEvent(w, snapshot); err != nil {
				return err
			}
			flusher.Flush()
		case <-refresh:
			if err := writeReplayWorkbenchCollaborationEvent(w, refreshSnapshot()); err != nil {
				return err
			}
			flusher.Flush()
		}
	}
}

func writeReplayWorkbenchCollaborationEvent(out io.Writer, snapshot replayWorkbenchCollaborationEventSnapshot) error {
	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode collaboration stream snapshot: %w", err)
	}
	_, err = fmt.Fprintf(out, "event: collaboration-snapshot\ndata: %s\n\n", data)
	return err
}

func replayWorkbenchCollaborationStreamScript(enabled bool) string {
	if !enabled {
		return ""
	}
	return `  function applyReplayWorkbenchCollaborationSnapshot(snapshot) {
    snapshot = snapshot || {};
    collaborations.splice(0, collaborations.length, ...((Array.isArray(snapshot.collaborations) ? snapshot.collaborations : [])));
    collaborationSelection = snapshot.collaboration_selection && snapshot.collaboration_selection.status ? snapshot.collaboration_selection : null;
    collaborationNoteSelection = snapshot.collaboration_note_selection && snapshot.collaboration_note_selection.status ? snapshot.collaboration_note_selection : null;
    collaborationPresence = Array.isArray(snapshot.collaboration_presence) ? snapshot.collaboration_presence : [];
    collaborationNotes = Array.isArray(snapshot.collaboration_notes) ? snapshot.collaboration_notes : [];
    renderReplayWorkbenchCollaborations();
    renderReplayWorkbenchCollaborationSelection();
    renderReplayWorkbenchCollaborationPresence();
    renderReplayWorkbenchCollaborationNotes();
    renderReplayWorkbenchCollaborationNoteSelection();
  }
  function installReplayWorkbenchCollaborationStream() {
    if (!collaborationStreamEnabled || typeof EventSource === "undefined") return;
    const source = new EventSource("/api/collaboration-events");
    source.addEventListener("collaboration-snapshot", (event) => {
      try {
        applyReplayWorkbenchCollaborationSnapshot(JSON.parse(event.data || "{}"));
      } catch (error) {
        return;
      }
    });
  }
  installReplayWorkbenchCollaborationStream();
`
}

func replayWorkbenchSessionSelectionPollingScript(intervalMS int) string {
	if intervalMS <= 0 {
		return ""
	}
	return `  async function refreshReplayWorkbenchSessionSelection() {
    try {
      const response = await fetch("/api/session-selection");
      if (!response.ok) return;
      const nextSelection = await response.json();
      sessionSelection = nextSelection && nextSelection.status ? nextSelection : null;
      renderReplayWorkbenchSessionSelection();
    } catch (error) {
      return;
    }
  }
  function installReplayWorkbenchSessionSelectionPoller() {
    if (sessionSelectionPollIntervalMS <= 0) return;
    setInterval(refreshReplayWorkbenchSessionSelection, sessionSelectionPollIntervalMS);
  }
  installReplayWorkbenchSessionSelectionPoller();
`
}

func replayWorkbenchCollaborationSelectionPollingScript(intervalMS int) string {
	if intervalMS <= 0 {
		return ""
	}
	return `  async function refreshReplayWorkbenchCollaborationSelection() {
    try {
      const response = await fetch("/api/collaboration-selection");
      if (!response.ok) return;
      const nextSelection = await response.json();
      collaborationSelection = nextSelection && nextSelection.status ? nextSelection : null;
      renderReplayWorkbenchCollaborationSelection();
    } catch (error) {
      return;
    }
  }
  function installReplayWorkbenchCollaborationSelectionPoller() {
    if (collaborationSelectionPollIntervalMS <= 0) return;
    setInterval(refreshReplayWorkbenchCollaborationSelection, collaborationSelectionPollIntervalMS);
  }
  installReplayWorkbenchCollaborationSelectionPoller();
`
}

func replayWorkbenchCollaborationNoteSelectionPollingScript(intervalMS int) string {
	if intervalMS <= 0 {
		return ""
	}
	return `  async function refreshReplayWorkbenchCollaborationNoteSelection() {
    try {
      const response = await fetch("/api/collaboration-note-selection");
      if (!response.ok) return;
      const nextSelection = await response.json();
      collaborationNoteSelection = nextSelection && nextSelection.status ? nextSelection : null;
      renderReplayWorkbenchCollaborationNoteSelection();
    } catch (error) {
      return;
    }
  }
  function installReplayWorkbenchCollaborationNoteSelectionPoller() {
    if (collaborationNoteSelectionPollIntervalMS <= 0) return;
    setInterval(refreshReplayWorkbenchCollaborationNoteSelection, collaborationNoteSelectionPollIntervalMS);
  }
  installReplayWorkbenchCollaborationNoteSelectionPoller();
`
}

func replayWorkbenchCollaborationPresencePollingScript(intervalMS int) string {
	if intervalMS <= 0 {
		return ""
	}
	return `  async function refreshReplayWorkbenchCollaborationPresence() {
    try {
      const response = await fetch("/api/collaboration-presence");
      if (!response.ok) return;
      const nextPresence = await response.json();
      collaborationPresence = Array.isArray(nextPresence) ? nextPresence : [];
      renderReplayWorkbenchCollaborationPresence();
    } catch (error) {
      return;
    }
  }
  function installReplayWorkbenchCollaborationPresencePoller() {
    if (collaborationPresencePollIntervalMS <= 0) return;
    setInterval(refreshReplayWorkbenchCollaborationPresence, collaborationPresencePollIntervalMS);
  }
  installReplayWorkbenchCollaborationPresencePoller();
`
}

func replayWorkbenchCollaborationNotesPollingScript(intervalMS int) string {
	if intervalMS <= 0 {
		return ""
	}
	return `  async function refreshReplayWorkbenchCollaborationNotes() {
    try {
      const response = await fetch("/api/collaboration-notes");
      if (!response.ok) return;
      const nextNotes = await response.json();
      collaborationNotes = Array.isArray(nextNotes) ? nextNotes : [];
      renderReplayWorkbenchCollaborationNotes();
    } catch (error) {
      return;
    }
  }
  function installReplayWorkbenchCollaborationNotesPoller() {
    if (collaborationNotesPollIntervalMS <= 0) return;
    setInterval(refreshReplayWorkbenchCollaborationNotes, collaborationNotesPollIntervalMS);
  }
  installReplayWorkbenchCollaborationNotesPoller();
`
}

func replayWorkbenchSessionSelectionWriteControlHTML(enabled bool) string {
	if !enabled {
		return ""
	}
	return `<section id="ouro-replay-session-selection-write" aria-label="Replay workbench session selection update">
      <form id="ouro-replay-session-selection-form" class="toolbar">
        <label for="ouro-replay-session-selection-session">Session</label>
        <select id="ouro-replay-session-selection-session" aria-label="Session selection selector"></select>
        <label for="ouro-replay-session-selection-actor">Actor</label>
        <input id="ouro-replay-session-selection-actor" type="text" autocomplete="off" aria-label="Session selection actor">
        <label for="ouro-replay-session-selection-reason">Reason</label>
        <input id="ouro-replay-session-selection-reason" type="text" autocomplete="off" aria-label="Session selection reason">
        <button id="ouro-replay-session-selection-select" type="submit" name="action" value="select">Select</button>
        <button id="ouro-replay-session-selection-clear" type="submit" name="action" value="clear">Clear</button>
      </form>
      <section id="ouro-replay-session-selection-write-status" aria-label="Replay workbench session selection write status"></section>
    </section>`
}

func replayWorkbenchReportSelectionWriteControlHTML(enabled bool) string {
	if !enabled {
		return ""
	}
	return `<section id="ouro-replay-report-selection-write" aria-label="Replay workbench report selection update">
      <form id="ouro-replay-report-selection-form" class="toolbar">
        <label for="ouro-replay-report-selection-execution">Report</label>
        <select id="ouro-replay-report-selection-execution" aria-label="Report selection selector"></select>
        <label for="ouro-replay-report-selection-actor">Actor</label>
        <input id="ouro-replay-report-selection-actor" type="text" autocomplete="off" aria-label="Report selection actor">
        <label for="ouro-replay-report-selection-reason">Reason</label>
        <input id="ouro-replay-report-selection-reason" type="text" autocomplete="off" aria-label="Report selection reason">
        <button id="ouro-replay-report-selection-select" type="submit" name="action" value="select">Select</button>
        <button id="ouro-replay-report-selection-clear" type="submit" name="action" value="clear">Clear</button>
      </form>
      <section id="ouro-replay-report-selection-write-status" aria-label="Replay workbench report selection write status"></section>
    </section>`
}

func replayWorkbenchReportSelectionWriteScript(enabled bool) string {
	if !enabled {
		return ""
	}
	return `  function installReplayWorkbenchReportSelectionWriter() {
    if (!reportSelectionWritesEnabled) return;
    const form = document.getElementById("ouro-replay-report-selection-form");
    if (!form) return;
    const executionSelect = document.getElementById("ouro-replay-report-selection-execution");
    const actorInput = document.getElementById("ouro-replay-report-selection-actor");
    const reasonInput = document.getElementById("ouro-replay-report-selection-reason");
    const writeStatus = document.getElementById("ouro-replay-report-selection-write-status");
    executionSelect.replaceChildren(new Option("Report execution", ""));
    reportExecutions.forEach((execution, index) => {
      const accepted = execution.annotation && String(execution.annotation.decision || "") === "accepted";
      if (!accepted || execution.written !== true || String(execution.status || "") !== "completed") return;
      const option = new Option(String(execution.report_path || execution.path || "report"), String(execution.path || ""));
      executionSelect.add(option);
      if (index === 0) executionSelect.value = option.value;
    });
    actorInput.value = "browser";
    reasonInput.value = "browser report selection update";
    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      const action = event.submitter && event.submitter.value === "clear" ? "clear" : "select";
      const payload = {
        action: action,
        execution_path: action === "select" ? String(executionSelect.value || "") : "",
        actor: String(actorInput.value || "browser").trim(),
        at: new Date().toISOString(),
        reason: String(reasonInput.value || "browser report selection update").trim()
      };
      if (action === "select" && !payload.execution_path) {
        writeStatus.innerHTML = "<article class=\"event\"><strong>report selection skipped</strong><span>missing report execution</span></article>";
        return;
      }
      try {
        const response = await fetch("/api/report-selection", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(payload)
        });
        if (!response.ok) throw new Error(await response.text());
        const written = await response.json();
        reportSelection = written && written.status ? written : null;
        renderReplayWorkbenchReportSelection();
        writeStatus.innerHTML = "<article class=\"event\"><strong>report selection saved</strong><span>" +
          escapeReplayHTML(String(written.status || action)) + " · " +
          escapeReplayHTML(String(written.report_path || "none")) + " · " +
          escapeReplayHTML(String(written.reason || payload.reason)) + "</span></article>";
      } catch (error) {
        writeStatus.innerHTML = "<article class=\"event\"><strong>report selection failed</strong><span>" +
          escapeReplayHTML(error && error.message ? error.message : String(error)) + "</span></article>";
      }
    });
  }
  installReplayWorkbenchReportSelectionWriter();
`
}

func replayWorkbenchSessionSelectionWriteScript(enabled bool) string {
	if !enabled {
		return ""
	}
	return `  function installReplayWorkbenchSessionSelectionWriter() {
    if (!sessionSelectionWritesEnabled) return;
    const form = document.getElementById("ouro-replay-session-selection-form");
    if (!form) return;
    const sessionSelect = document.getElementById("ouro-replay-session-selection-session");
    const actorInput = document.getElementById("ouro-replay-session-selection-actor");
    const reasonInput = document.getElementById("ouro-replay-session-selection-reason");
    const writeStatus = document.getElementById("ouro-replay-session-selection-write-status");
    sessionSelect.replaceChildren(new Option("Session", ""));
    savedSessions.forEach((saved, index) => {
      const option = new Option(String(saved.reason || saved.path || "session"), String(saved.path || ""));
      sessionSelect.add(option);
      if (index === 0) sessionSelect.value = option.value;
    });
    actorInput.value = "browser";
    reasonInput.value = "browser session selection update";
    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      const action = event.submitter && event.submitter.value === "clear" ? "clear" : "select";
      const payload = {
        action: action,
        session_path: action === "select" ? String(sessionSelect.value || "") : "",
        actor: String(actorInput.value || "browser").trim(),
        at: new Date().toISOString(),
        reason: String(reasonInput.value || "browser session selection update").trim()
      };
      if (action === "select" && !payload.session_path) {
        writeStatus.innerHTML = "<article class=\"event\"><strong>session selection skipped</strong><span>missing session</span></article>";
        return;
      }
      try {
        const response = await fetch("/api/session-selection", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(payload)
        });
        if (!response.ok) throw new Error(await response.text());
        const written = await response.json();
        sessionSelection = written && written.status ? written : null;
        renderReplayWorkbenchSessionSelection();
        writeStatus.innerHTML = "<article class=\"event\"><strong>session selection saved</strong><span>" +
          escapeReplayHTML(String(written.status || action)) + " · " +
          escapeReplayHTML(String(written.session_path || "none")) + " · " +
          escapeReplayHTML(String(written.reason || payload.reason)) + "</span></article>";
      } catch (error) {
        writeStatus.innerHTML = "<article class=\"event\"><strong>session selection failed</strong><span>" +
          escapeReplayHTML(error && error.message ? error.message : String(error)) + "</span></article>";
      }
    });
  }
  installReplayWorkbenchSessionSelectionWriter();
`
}

func replayWorkbenchCollaborationSelectionWriteControlHTML(enabled bool) string {
	if !enabled {
		return ""
	}
	return `<section id="ouro-replay-collaboration-selection-write" aria-label="Replay workbench collaboration selection update">
      <form id="ouro-replay-collaboration-selection-form" class="toolbar">
        <label for="ouro-replay-collaboration-selection-collaboration">Collaboration</label>
        <select id="ouro-replay-collaboration-selection-collaboration" aria-label="Collaboration selection selector"></select>
        <label for="ouro-replay-collaboration-selection-actor">Actor</label>
        <input id="ouro-replay-collaboration-selection-actor" type="text" autocomplete="off" aria-label="Collaboration selection actor">
        <label for="ouro-replay-collaboration-selection-reason">Reason</label>
        <input id="ouro-replay-collaboration-selection-reason" type="text" autocomplete="off" aria-label="Collaboration selection reason">
        <button id="ouro-replay-collaboration-selection-select" type="submit" name="action" value="select">Select</button>
        <button id="ouro-replay-collaboration-selection-clear" type="submit" name="action" value="clear">Clear</button>
      </form>
      <section id="ouro-replay-collaboration-selection-write-status" aria-label="Replay workbench collaboration selection write status"></section>
    </section>`
}

func replayWorkbenchCollaborationSelectionWriteScript(enabled bool) string {
	if !enabled {
		return ""
	}
	return `  function installReplayWorkbenchCollaborationSelectionWriter() {
    if (!collaborationSelectionWritesEnabled) return;
    const form = document.getElementById("ouro-replay-collaboration-selection-form");
    if (!form) return;
    const collaborationSelect = document.getElementById("ouro-replay-collaboration-selection-collaboration");
    const actorInput = document.getElementById("ouro-replay-collaboration-selection-actor");
    const reasonInput = document.getElementById("ouro-replay-collaboration-selection-reason");
    const writeStatus = document.getElementById("ouro-replay-collaboration-selection-write-status");
    collaborationSelect.replaceChildren(new Option("Collaboration", ""));
    collaborations.forEach((collaboration, index) => {
      const option = new Option(String(collaboration.reason || collaboration.path || "collaboration"), String(collaboration.path || ""));
      collaborationSelect.add(option);
      if (index === 0) collaborationSelect.value = option.value;
    });
    actorInput.value = "browser";
    reasonInput.value = "browser collaboration selection update";
    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      const action = event.submitter && event.submitter.value === "clear" ? "clear" : "select";
      const payload = {
        action: action,
        collaboration_path: action === "select" ? String(collaborationSelect.value || "") : "",
        actor: String(actorInput.value || "browser").trim(),
        at: new Date().toISOString(),
        reason: String(reasonInput.value || "browser collaboration selection update").trim()
      };
      if (action === "select" && !payload.collaboration_path) {
        writeStatus.innerHTML = "<article class=\"event\"><strong>collaboration selection skipped</strong><span>missing collaboration</span></article>";
        return;
      }
      try {
        const response = await fetch("/api/collaboration-selection", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(payload)
        });
        if (!response.ok) throw new Error(await response.text());
        const written = await response.json();
        collaborationSelection = written && written.status ? written : null;
        renderReplayWorkbenchCollaborationSelection();
        writeStatus.innerHTML = "<article class=\"event\"><strong>collaboration selection saved</strong><span>" +
          escapeReplayHTML(String(written.status || action)) + " · " +
          escapeReplayHTML(String(written.collaboration_path || "none")) + " · " +
          escapeReplayHTML(String(written.reason || payload.reason)) + "</span></article>";
      } catch (error) {
        writeStatus.innerHTML = "<article class=\"event\"><strong>collaboration selection failed</strong><span>" +
          escapeReplayHTML(error && error.message ? error.message : String(error)) + "</span></article>";
      }
    });
  }
  installReplayWorkbenchCollaborationSelectionWriter();
`
}

func replayWorkbenchCollaborationPresenceWriteControlHTML(enabled bool) string {
	if !enabled {
		return ""
	}
	return `<section id="ouro-replay-collaboration-presence-write" aria-label="Replay workbench collaboration presence update">
      <form id="ouro-replay-collaboration-presence-form" class="toolbar">
        <label for="ouro-replay-presence-collaboration">Collaboration</label>
        <select id="ouro-replay-presence-collaboration" aria-label="Presence collaboration selector"></select>
        <label for="ouro-replay-presence-participant">Participant</label>
        <input id="ouro-replay-presence-participant" type="text" autocomplete="off" aria-label="Presence participant">
        <label for="ouro-replay-presence-status">Status</label>
        <select id="ouro-replay-presence-status" aria-label="Presence status">
          <option value="online">online</option>
          <option value="away">away</option>
          <option value="offline">offline</option>
        </select>
        <label for="ouro-replay-presence-seen-by">Seen By</label>
        <input id="ouro-replay-presence-seen-by" type="text" autocomplete="off" aria-label="Presence seen by">
        <label for="ouro-replay-presence-reason">Reason</label>
        <input id="ouro-replay-presence-reason" type="text" autocomplete="off" aria-label="Presence reason">
        <button id="ouro-replay-presence-submit" type="submit">Set presence</button>
      </form>
      <section id="ouro-replay-presence-write-status" aria-label="Replay workbench collaboration presence write status"></section>
    </section>`
}

func replayWorkbenchCollaborationPresenceWriteScript(enabled bool) string {
	if !enabled {
		return ""
	}
	return `  function installReplayWorkbenchCollaborationPresenceWriter() {
    if (!collaborationPresenceWritesEnabled) return;
    const form = document.getElementById("ouro-replay-collaboration-presence-form");
    if (!form) return;
    const collaborationSelect = document.getElementById("ouro-replay-presence-collaboration");
    const participantInput = document.getElementById("ouro-replay-presence-participant");
    const statusSelect = document.getElementById("ouro-replay-presence-status");
    const seenByInput = document.getElementById("ouro-replay-presence-seen-by");
    const reasonInput = document.getElementById("ouro-replay-presence-reason");
    const writeStatus = document.getElementById("ouro-replay-presence-write-status");
    collaborationSelect.replaceChildren(new Option("Collaboration", ""));
    collaborations.forEach((collaboration, index) => {
      const option = new Option(String(collaboration.reason || collaboration.path || "collaboration"), String(collaboration.path || ""));
      collaborationSelect.add(option);
      if (index === 0) collaborationSelect.value = option.value;
    });
    if (collaborations.length) {
      const participants = Array.isArray(collaborations[0].participants) ? collaborations[0].participants : [];
      participantInput.value = String(participants[0] || "");
    }
    seenByInput.value = "browser";
    reasonInput.value = "browser presence update";
    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      const payload = {
        collaboration_path: String(collaborationSelect.value || ""),
        participant: String(participantInput.value || "").trim(),
        status: String(statusSelect.value || "online"),
        seen_by: String(seenByInput.value || "browser").trim(),
        seen_at: new Date().toISOString(),
        reason: String(reasonInput.value || "browser presence update").trim()
      };
      if (!payload.collaboration_path || !payload.participant) {
        writeStatus.innerHTML = "<article class=\"event\"><strong>presence write skipped</strong><span>missing collaboration or participant</span></article>";
        return;
      }
      try {
        const response = await fetch("/api/collaboration-presence", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(payload)
        });
        if (!response.ok) throw new Error(await response.text());
        const written = await response.json();
        const refreshed = await fetch("/api/collaboration-presence");
        if (refreshed.ok) {
          const nextPresence = await refreshed.json();
          collaborationPresence = Array.isArray(nextPresence) ? nextPresence : [written];
        } else {
          collaborationPresence = [written];
        }
        renderReplayWorkbenchCollaborationPresence();
        writeStatus.innerHTML = "<article class=\"event\"><strong>presence write saved</strong><span>" +
          escapeReplayHTML(String(written.participant || payload.participant)) + " · " +
          escapeReplayHTML(String(written.status || payload.status)) + " · " +
          escapeReplayHTML(String(written.path || "metadata-only")) + "</span></article>";
      } catch (error) {
        writeStatus.innerHTML = "<article class=\"event\"><strong>presence write failed</strong><span>" +
          escapeReplayHTML(error && error.message ? error.message : String(error)) + "</span></article>";
      }
    });
  }
  installReplayWorkbenchCollaborationPresenceWriter();
`
}

func replayWorkbenchCollaborationNoteWriteControlHTML(enabled bool) string {
	if !enabled {
		return ""
	}
	return `<section id="ouro-replay-collaboration-note-write" aria-label="Replay workbench collaboration note update">
      <form id="ouro-replay-collaboration-note-form" class="toolbar">
        <label for="ouro-replay-note-collaboration">Collaboration</label>
        <select id="ouro-replay-note-collaboration" aria-label="Note collaboration selector"></select>
        <label for="ouro-replay-note-author">Author</label>
        <input id="ouro-replay-note-author" type="text" autocomplete="off" aria-label="Note author">
        <label for="ouro-replay-note-target-kind">Target</label>
        <select id="ouro-replay-note-target-kind" aria-label="Note target kind">
          <option value="note">note</option>
          <option value="graph-node">graph-node</option>
          <option value="graph-edge">graph-edge</option>
          <option value="timeline-event">timeline-event</option>
          <option value="session">session</option>
        </select>
        <label for="ouro-replay-note-target-id">Target ID</label>
        <input id="ouro-replay-note-target-id" type="text" autocomplete="off" aria-label="Note target id">
        <label for="ouro-replay-note-body">Body</label>
        <input id="ouro-replay-note-body" type="text" autocomplete="off" aria-label="Note body">
        <label for="ouro-replay-note-reason">Reason</label>
        <input id="ouro-replay-note-reason" type="text" autocomplete="off" aria-label="Note reason">
        <button id="ouro-replay-note-submit" type="submit">Add note</button>
      </form>
      <section id="ouro-replay-note-write-status" aria-label="Replay workbench collaboration note write status"></section>
    </section>`
}

func replayWorkbenchCollaborationNoteWriteScript(enabled bool) string {
	if !enabled {
		return ""
	}
	return `  function installReplayWorkbenchCollaborationNoteWriter() {
    if (!collaborationNoteWritesEnabled) return;
    const form = document.getElementById("ouro-replay-collaboration-note-form");
    if (!form) return;
    const collaborationSelect = document.getElementById("ouro-replay-note-collaboration");
    const authorInput = document.getElementById("ouro-replay-note-author");
    const targetKindSelect = document.getElementById("ouro-replay-note-target-kind");
    const targetIDInput = document.getElementById("ouro-replay-note-target-id");
    const bodyInput = document.getElementById("ouro-replay-note-body");
    const reasonInput = document.getElementById("ouro-replay-note-reason");
    const writeStatus = document.getElementById("ouro-replay-note-write-status");
    collaborationSelect.replaceChildren(new Option("Collaboration", ""));
    collaborations.forEach((collaboration, index) => {
      const option = new Option(String(collaboration.reason || collaboration.path || "collaboration"), String(collaboration.path || ""));
      collaborationSelect.add(option);
      if (index === 0) collaborationSelect.value = option.value;
    });
    authorInput.value = "browser";
    reasonInput.value = "browser note";
    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      const payload = {
        collaboration_path: String(collaborationSelect.value || ""),
        author: String(authorInput.value || "browser").trim(),
        body: String(bodyInput.value || "").trim(),
        target_kind: String(targetKindSelect.value || "note").trim(),
        target_id: String(targetIDInput.value || "").trim(),
        created_at: new Date().toISOString(),
        reason: String(reasonInput.value || "browser note").trim()
      };
      if (!payload.collaboration_path || !payload.body) {
        writeStatus.innerHTML = "<article class=\"event\"><strong>note write skipped</strong><span>missing collaboration or body</span></article>";
        return;
      }
      try {
        const response = await fetch("/api/collaboration-notes", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(payload)
        });
        if (!response.ok) throw new Error(await response.text());
        const written = await response.json();
        const refreshed = await fetch("/api/collaboration-notes");
        if (refreshed.ok) {
          const nextNotes = await refreshed.json();
          collaborationNotes = Array.isArray(nextNotes) ? nextNotes : [written];
        } else {
          collaborationNotes = [written];
        }
        renderReplayWorkbenchCollaborationNotes();
        bodyInput.value = "";
        writeStatus.innerHTML = "<article class=\"event\"><strong>note write saved</strong><span>" +
          escapeReplayHTML(String(written.author || payload.author)) + " · " +
          escapeReplayHTML(String(written.path || "metadata-only")) + " · " +
          escapeReplayHTML(String(written.reason || payload.reason)) + "</span></article>";
      } catch (error) {
        writeStatus.innerHTML = "<article class=\"event\"><strong>note write failed</strong><span>" +
          escapeReplayHTML(error && error.message ? error.message : String(error)) + "</span></article>";
      }
    });
  }
  installReplayWorkbenchCollaborationNoteWriter();
`
}

func replayWorkbenchCollaborationNoteSelectionWriteControlHTML(enabled bool) string {
	if !enabled {
		return ""
	}
	return `<section id="ouro-replay-collaboration-note-selection-write" aria-label="Replay workbench collaboration note selection update">
      <form id="ouro-replay-collaboration-note-selection-form" class="toolbar">
        <label for="ouro-replay-note-selection-action">Action</label>
        <select id="ouro-replay-note-selection-action" aria-label="Note selection action">
          <option value="select">select</option>
          <option value="clear">clear</option>
        </select>
        <label for="ouro-replay-note-selection-note">Note</label>
        <select id="ouro-replay-note-selection-note" aria-label="Note selection selector"></select>
        <label for="ouro-replay-note-selection-actor">Actor</label>
        <input id="ouro-replay-note-selection-actor" type="text" autocomplete="off" aria-label="Note selection actor">
        <label for="ouro-replay-note-selection-reason">Reason</label>
        <input id="ouro-replay-note-selection-reason" type="text" autocomplete="off" aria-label="Note selection reason">
        <button id="ouro-replay-note-selection-submit" type="submit">Update note focus</button>
      </form>
      <section id="ouro-replay-note-selection-write-status" aria-label="Replay workbench collaboration note selection write status"></section>
    </section>`
}

func replayWorkbenchCollaborationNoteSelectionWriteScript(enabled bool) string {
	if !enabled {
		return ""
	}
	return `  function installReplayWorkbenchCollaborationNoteSelectionWriter() {
    if (!collaborationNoteSelectionWritesEnabled) return;
    const form = document.getElementById("ouro-replay-collaboration-note-selection-form");
    if (!form) return;
    const actionSelect = document.getElementById("ouro-replay-note-selection-action");
    const noteSelect = document.getElementById("ouro-replay-note-selection-note");
    const actorInput = document.getElementById("ouro-replay-note-selection-actor");
    const reasonInput = document.getElementById("ouro-replay-note-selection-reason");
    const writeStatus = document.getElementById("ouro-replay-note-selection-write-status");
    noteSelect.replaceChildren(new Option("Note", ""));
    collaborationNotes.forEach((note, index) => {
      const label = String(note.author || "note") + " · " + String(note.target_kind || "note") +
        (note.target_id ? ":" + String(note.target_id || "") : "") + " · " + String(note.reason || note.path || "");
      const option = new Option(label, String(note.path || ""));
      noteSelect.add(option);
      if (index === 0) noteSelect.value = option.value;
    });
    actorInput.value = "browser";
    reasonInput.value = "browser note selection update";
    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      const action = String(actionSelect.value || "select").trim().toLowerCase();
      const payload = {
        action: action,
        note_path: action === "select" ? String(noteSelect.value || "") : "",
        actor: String(actorInput.value || "browser").trim(),
        at: new Date().toISOString(),
        reason: String(reasonInput.value || "browser note selection update").trim()
      };
      if (action === "select" && !payload.note_path) {
        writeStatus.innerHTML = "<article class=\"event\"><strong>note selection skipped</strong><span>missing note</span></article>";
        return;
      }
      try {
        const response = await fetch("/api/collaboration-note-selection", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(payload)
        });
        if (!response.ok) throw new Error(await response.text());
        const written = await response.json();
        collaborationNoteSelection = written && written.status ? written : null;
        renderReplayWorkbenchCollaborationNoteSelection();
        writeStatus.innerHTML = "<article class=\"event\"><strong>note selection saved</strong><span>" +
          escapeReplayHTML(String(written.status || action)) + " · " +
          escapeReplayHTML(String(written.note_path || "none")) + " · " +
          escapeReplayHTML(String(written.reason || payload.reason)) + "</span></article>";
      } catch (error) {
        writeStatus.innerHTML = "<article class=\"event\"><strong>note selection failed</strong><span>" +
          escapeReplayHTML(error && error.message ? error.message : String(error)) + "</span></article>";
      }
    });
  }
  installReplayWorkbenchCollaborationNoteSelectionWriter();
`
}

func replayWorkbenchSessionFromExports(exports []replayExport) replayWorkbenchSession {
	runIDs := make([]string, 0, len(exports))
	for _, export := range exports {
		if strings.TrimSpace(export.RunID) != "" {
			runIDs = append(runIDs, export.RunID)
		}
	}
	primary := ""
	if len(runIDs) > 0 {
		primary = runIDs[0]
	}
	return replayWorkbenchSession{
		PrimaryRunID:  primary,
		SelectedRunID: primary,
		RunIDs:        runIDs,
		Filters:       map[string]string{},
		Endpoints: map[string]string{
			"html":            "/",
			"replay":          "/api/replay",
			"replays":         "/api/replays",
			"sessions":        "/api/sessions",
			"graph":           "/api/graph",
			"graphs":          "/api/graphs",
			"graph_ndjson":    "/api/graph.ndjson",
			"timeline_ndjson": "/api/timeline.ndjson",
		},
	}
}

func replayExportsFromResults(results []app.ReplayResult) []replayExport {
	exports := make([]replayExport, 0, len(results))
	for _, result := range results {
		exports = append(exports, replayExportFromResult(result))
	}
	return exports
}

func replayGraphExportsFromResults(results []app.ReplayResult) []replayGraphExport {
	exports := make([]replayGraphExport, 0, len(results))
	for _, result := range results {
		exports = append(exports, replayGraphExportFromResult(result))
	}
	return exports
}

func replayExportFromResult(result app.ReplayResult) replayExport {
	laneCounts := map[string]int{}
	for lane, count := range result.LaneCounts {
		laneCounts[string(lane)] = count
	}
	timeline := make([]replayExportTimeline, 0, len(result.Timeline))
	for _, event := range result.Timeline {
		timeline = append(timeline, replayExportTimeline{
			EventID:   event.EventID,
			RunID:     event.RunID,
			Lane:      string(event.Lane),
			Type:      string(event.Type),
			CreatedAt: event.CreatedAt,
			Payload:   append(json.RawMessage(nil), event.Payload...),
		})
	}
	return replayExport{
		RunID:      result.RunID,
		EventCount: result.EventCount,
		LaneCounts: laneCounts,
		Status: replayExportStatus{
			Model:          result.Status.Model,
			ContextPercent: result.Status.ContextPercent,
			AgentCount:     result.Status.AgentCount,
			BreadthPercent: result.Status.BreadthPercent,
			DepthChains:    result.Status.DepthChains,
			ToolHealth:     result.Status.ToolHealth,
			Cost:           result.Status.Cost,
		},
		ReportPath: replayReportDisplay(result.ReportPath),
		Timeline:   timeline,
		Graph:      replayGraphExportFromResult(result),
	}
}

func replayGraphExportFromResult(result app.ReplayResult) replayGraphExport {
	evidenceNodes := result.Evidence.Nodes()
	sort.Slice(evidenceNodes, func(i, j int) bool {
		return evidenceNodes[i].ID < evidenceNodes[j].ID
	})
	nodes := make([]replayGraphNode, 0, len(evidenceNodes))
	for _, node := range evidenceNodes {
		nodes = append(nodes, replayGraphNode{
			ID:         node.ID,
			Type:       string(node.Type),
			Label:      node.Label,
			SourceKind: node.SourceKind,
		})
	}
	evidenceEdges := result.Evidence.Edges()
	sort.Slice(evidenceEdges, func(i, j int) bool {
		if evidenceEdges[i].From != evidenceEdges[j].From {
			return evidenceEdges[i].From < evidenceEdges[j].From
		}
		if evidenceEdges[i].To != evidenceEdges[j].To {
			return evidenceEdges[i].To < evidenceEdges[j].To
		}
		return evidenceEdges[i].Type < evidenceEdges[j].Type
	})
	edges := make([]replayGraphEdge, 0, len(evidenceEdges))
	for _, edge := range evidenceEdges {
		graphEdge := replayGraphEdge{
			From: edge.From,
			To:   edge.To,
			Type: string(edge.Type),
		}
		graphEdge.ID = replayGraphEdgeID(graphEdge)
		edges = append(edges, graphEdge)
	}
	return replayGraphWithCounts(replayGraphExport{
		RunID: result.RunID,
		Nodes: nodes,
		Edges: edges,
	})
}

func replayGraphEdgeID(edge replayGraphEdge) string {
	if strings.TrimSpace(edge.ID) != "" {
		return edge.ID
	}
	return edge.From + ":" + edge.To + ":" + edge.Type
}

func replayGraphWithCounts(export replayGraphExport) replayGraphExport {
	nodeTypeCounts := map[string]int{}
	for _, node := range export.Nodes {
		nodeTypeCounts[node.Type]++
	}
	edgeTypeCounts := map[string]int{}
	for i, edge := range export.Edges {
		export.Edges[i].ID = replayGraphEdgeID(edge)
		edgeTypeCounts[edge.Type]++
	}
	export.NodeCount = len(export.Nodes)
	export.EdgeCount = len(export.Edges)
	export.NodeTypeCounts = nodeTypeCounts
	export.EdgeTypeCounts = edgeTypeCounts
	if export.Focus != nil {
		export.Focus.NodeCount = export.NodeCount
		export.Focus.EdgeCount = export.EdgeCount
	}
	return export
}

func replayReportDisplay(path string) string {
	if path == "" {
		return "not-generated"
	}
	return path
}

func replayLaneOrder() []eventlog.ReplayLane {
	return []eventlog.ReplayLane{
		eventlog.ReplayLaneMission,
		eventlog.ReplayLaneAgent,
		eventlog.ReplayLaneObserver,
		eventlog.ReplayLaneBlackboard,
		eventlog.ReplayLaneToolchain,
		eventlog.ReplayLaneSubmissionReport,
	}
}

func newToolchainCommand(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "toolchain",
		Short: "Inspect and manage native toolchain manifests",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, _ = fmt.Fprintln(out, "toolchain command requires a subcommand")
			return fmt.Errorf("toolchain command requires a subcommand")
		},
	}
	cmd.AddCommand(newToolchainListCommand(out))
	cmd.AddCommand(newToolchainHealthCommand(out))
	cmd.AddCommand(newToolchainCompilePreviewCommand(out))
	cmd.AddCommand(newToolchainCompilePreviewReviewCommand(out))
	cmd.AddCommand(newToolchainCompilePreviewReviewAnnotateCommand(out))
	cmd.AddCommand(newToolchainCompilePreviewReviewCompareCommand(out))
	cmd.AddCommand(newToolchainCompilePreviewReviewSelectCommand(out))
	cmd.AddCommand(newToolchainCompilePreviewReviewSelectionCommand(out))
	cmd.AddCommand(newToolchainCompilePreviewReviewSelectionClearCommand(out))
	cmd.AddCommand(newToolchainCompilePreviewReviewsCommand(out))
	cmd.AddCommand(newToolchainCompilePreviewSignalsCommand(out))
	cmd.AddCommand(newToolchainCompilePreviewSignalsFromReplayCommand(out))
	cmd.AddCommand(newToolchainInstallPlanCommand(out))
	cmd.AddCommand(newToolchainInstallRequestCommand(out))
	cmd.AddCommand(newToolchainInstallRequestsCommand(out))
	cmd.AddCommand(newToolchainInstallDispatchPlanCommand(out))
	cmd.AddCommand(newToolchainInstallRetryPlanCommand(out))
	cmd.AddCommand(newToolchainInstallRetryDispatchCommand(out))
	cmd.AddCommand(newToolchainInstallRetryDispatchesCommand(out))
	cmd.AddCommand(newToolchainExecuteInstallRetryDispatchCommand(out))
	cmd.AddCommand(newToolchainInstallRetryExecutionsCommand(out))
	cmd.AddCommand(newToolchainInstallOrchestrateCommand(out))
	cmd.AddCommand(newToolchainInstallOrchestrationsCommand(out))
	cmd.AddCommand(newToolchainInstallDispatchCommand(out))
	cmd.AddCommand(newToolchainInstallDispatchesCommand(out))
	cmd.AddCommand(newToolchainExecuteInstallDispatchCommand(out))
	cmd.AddCommand(newToolchainInstallExecutionsCommand(out))
	cmd.AddCommand(newToolchainApproveInstallRequestCommand(out))
	cmd.AddCommand(newToolchainDenyInstallRequestCommand(out))
	return cmd
}

func newRuntimeCommand(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "runtime",
		Short: "Inspect and prepare runtime recovery artifacts",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, _ = fmt.Fprintln(out, "runtime command requires a subcommand")
			return fmt.Errorf("runtime command requires a subcommand")
		},
	}
	cmd.AddCommand(newRuntimeWorkerRegisterCommand(out))
	cmd.AddCommand(newRuntimeWorkerHeartbeatCommand(out))
	cmd.AddCommand(newRuntimeWorkersCommand(out))
	cmd.AddCommand(newRuntimeWorkerPlacementPlanCommand(out))
	cmd.AddCommand(newRuntimeWorkerPlacementPlansCommand(out))
	cmd.AddCommand(newRuntimeExecuteWorkerPlacementPlanCommand(out))
	cmd.AddCommand(newRuntimeWorkerPlacementClaimExecutionsCommand(out))
	cmd.AddCommand(newRuntimeWorkerPlacementDispatchPlanCommand(out))
	cmd.AddCommand(newRuntimeWorkerPlacementDispatchPlansCommand(out))
	cmd.AddCommand(newRuntimeExecuteWorkerPlacementDispatchPlanCommand(out))
	cmd.AddCommand(newRuntimeWorkerPlacementDispatchExecutionsCommand(out))
	cmd.AddCommand(newRuntimeWorkerPlacementDispatchBatchesCommand(out))
	cmd.AddCommand(newRuntimeWorkerOrchestrateCommand(out))
	cmd.AddCommand(newRuntimeWorkerOrchestrationsCommand(out))
	cmd.AddCommand(newRuntimeWorkerOrchestrationSupervisorCommand(out))
	cmd.AddCommand(newRuntimeWorkerOrchestrationSupervisorRunsCommand(out))
	cmd.AddCommand(newRuntimeWorkerQueueClaimCommand(out))
	cmd.AddCommand(newRuntimeWorkerQueueClaimsCommand(out))
	cmd.AddCommand(newRuntimeWorkerQueueClaimCancelCommand(out))
	cmd.AddCommand(newRuntimeWorkerQueueClaimExpireCommand(out))
	cmd.AddCommand(newRuntimeWorkerQueueClaimRenewCommand(out))
	cmd.AddCommand(newRuntimeExecuteWorkerQueueClaimCommand(out))
	cmd.AddCommand(newRuntimeWorkerQueueDispatchExecutionsCommand(out))
	cmd.AddCommand(newRuntimeWorkerTransportOutboxCommand(out))
	cmd.AddCommand(newRuntimeWorkerTransportAckCommand(out))
	cmd.AddCommand(newRuntimeWorkerTransportServeHTTPCommand(out))
	cmd.AddCommand(newRuntimeWorkerTransportRunOnceCommand(out))
	cmd.AddCommand(newRuntimeWorkerTransportRunLoopCommand(out))
	cmd.AddCommand(newRuntimeWorkerTransportRunDaemonCommand(out))
	cmd.AddCommand(newRuntimeWorkerTransportDaemonRunsCommand(out))
	cmd.AddCommand(newRuntimeWorkerTransportRunSupervisorCommand(out))
	cmd.AddCommand(newRuntimeWorkerTransportSupervisorRunsCommand(out))
	cmd.AddCommand(newRuntimeWorkerTransportDaemonControlCommand(out))
	cmd.AddCommand(newRuntimeWorkerTransportDaemonControlsCommand(out))
	cmd.AddCommand(newRuntimeWorkerQueueDispatchResultCommand(out))
	cmd.AddCommand(newRuntimeWorkerQueueDispatchResultServeHTTPCommand(out))
	cmd.AddCommand(newRuntimeWorkerQueueDispatchResultsCommand(out))
	cmd.AddCommand(newRuntimeWorkerDispatchRetryPlanCommand(out))
	cmd.AddCommand(newRuntimeWorkerDispatchRetryPlansCommand(out))
	cmd.AddCommand(newRuntimeWorkerDispatchRetryDispatchCommand(out))
	cmd.AddCommand(newRuntimeWorkerDispatchRetryDispatchesCommand(out))
	cmd.AddCommand(newRuntimeExecuteWorkerDispatchRetryDispatchCommand(out))
	cmd.AddCommand(newRuntimeWorkerDispatchRetryDispatchExecutionsCommand(out))
	cmd.AddCommand(newRuntimeWorkerDispatchRetrySchedulerRunCommand(out))
	cmd.AddCommand(newRuntimeWorkerDispatchRetrySchedulerRunsCommand(out))
	cmd.AddCommand(newRuntimeWorkerLeaseHeartbeatCommand(out))
	cmd.AddCommand(newRuntimeWorkerLeaseReleaseCommand(out))
	cmd.AddCommand(newRuntimeWorkerLifecycleAuditCommand(out))
	cmd.AddCommand(newRuntimeWorkerLifecycleAuditsCommand(out))
	cmd.AddCommand(newRuntimeWorkerLifecycleReviewCommand(out))
	cmd.AddCommand(newRuntimeWorkerLifecycleReviewsCommand(out))
	cmd.AddCommand(newRuntimeWorkerLifecycleActionPlanCommand(out))
	cmd.AddCommand(newRuntimeWorkerLifecycleActionPlanAnnotateCommand(out))
	cmd.AddCommand(newRuntimeWorkerLifecycleActionPlanSelectCommand(out))
	cmd.AddCommand(newRuntimeWorkerLifecycleActionPlanSelectionCommand(out))
	cmd.AddCommand(newRuntimeWorkerLifecycleActionPlanSelectionClearCommand(out))
	cmd.AddCommand(newRuntimeWorkerLifecycleActionPlansCommand(out))
	cmd.AddCommand(newRuntimeWorkerLifecycleMaintenanceCommand(out))
	cmd.AddCommand(newRuntimeWorkerLifecycleMaintenancesCommand(out))
	cmd.AddCommand(newRuntimeWorkerLifecycleMaintenanceSupervisorCommand(out))
	cmd.AddCommand(newRuntimeWorkerLifecycleMaintenanceSupervisorRunsCommand(out))
	cmd.AddCommand(newRuntimeWorkerLifecyclePolicyCommand(out))
	cmd.AddCommand(newRuntimeWorkerLifecyclePolicyRunCommand(out))
	cmd.AddCommand(newRuntimeWorkerLifecyclePolicyRunsCommand(out))
	cmd.AddCommand(newRuntimeWorkerLifecyclePolicySchedulerRunCommand(out))
	cmd.AddCommand(newRuntimeWorkerLifecyclePolicySchedulerRunsCommand(out))
	cmd.AddCommand(newRuntimeWorkerLifecyclePoliciesCommand(out))
	cmd.AddCommand(newRuntimeWorkerLifecyclePolicySelectCommand(out))
	cmd.AddCommand(newRuntimeWorkerLifecyclePolicySelectionCommand(out))
	cmd.AddCommand(newRuntimeWorkerLifecyclePolicySelectionClearCommand(out))
	cmd.AddCommand(newRuntimeWorkerLeaseResultReconcileCommand(out))
	cmd.AddCommand(newRuntimeWorkerLeaseResultReconciliationsCommand(out))
	cmd.AddCommand(newRuntimeExecuteWorkerLeaseResultReconciliationCommand(out))
	cmd.AddCommand(newRuntimeWorkerLeaseResultReconciliationExecutionsCommand(out))
	cmd.AddCommand(newRuntimeWorkerLeaseReclaimCommand(out))
	cmd.AddCommand(newRuntimeWorkerLeaseReclaimsCommand(out))
	cmd.AddCommand(newRuntimeExecuteWorkerLeaseReclaimCommand(out))
	cmd.AddCommand(newRuntimeWorkerLeaseReclaimExecutionsCommand(out))
	return cmd
}

func newRuntimeWorkerRegisterCommand(out io.Writer) *cobra.Command {
	var mode string
	var status string
	var endpoint string
	var capabilities []string
	var labels []string
	cmd := &cobra.Command{
		Use:   "worker-register <workspace> <worker-id>",
		Short: "Write or update a metadata-only runtime worker registration",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			parsedLabels, err := parseRuntimeWorkerLabels(labels)
			if err != nil {
				return err
			}
			worker, err := app.RegisterRuntimeWorker(args[0], app.RuntimeWorkerArtifact{
				WorkerID:     args[1],
				Mode:         mode,
				Status:       status,
				Endpoint:     endpoint,
				Capabilities: capabilities,
				Labels:       parsedLabels,
			}, app.RuntimeWorkerRegistrationOptions{Now: time.Now().UTC()})
			if err != nil {
				return err
			}
			return printRuntimeWorker(out, worker)
		},
	}
	cmd.Flags().StringVar(&mode, "mode", "local", "runtime worker mode: local or remote")
	cmd.Flags().StringVar(&status, "status", "available", "runtime worker status: available, busy, draining, or offline")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "optional remote worker endpoint metadata")
	cmd.Flags().StringArrayVar(&capabilities, "capability", nil, "worker capability metadata; may be repeated")
	cmd.Flags().StringArrayVar(&labels, "label", nil, "worker label metadata as key=value; may be repeated")
	return cmd
}

func newRuntimeWorkerHeartbeatCommand(out io.Writer) *cobra.Command {
	var status string
	cmd := &cobra.Command{
		Use:   "worker-heartbeat <workspace> <worker>",
		Short: "Update a runtime worker registration heartbeat",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			worker, err := app.UpdateRuntimeWorkerHeartbeat(args[0], args[1], app.RuntimeWorkerHeartbeatOptions{
				Now:    time.Now().UTC(),
				Status: status,
			})
			if err != nil {
				return err
			}
			return printRuntimeWorker(out, worker)
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "optional next runtime worker status: available, busy, draining, or offline")
	return cmd
}

func newRuntimeWorkersCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "workers <workspace> [worker]",
		Short: "List or inspect runtime worker registration artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				worker, err := app.ReadRuntimeWorker(args[0], args[1])
				if err != nil {
					return err
				}
				return printRuntimeWorker(out, worker)
			}
			workers, err := app.ListRuntimeWorkers(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "runtime-workers: total=%d\n", len(workers)); err != nil {
				return err
			}
			for _, worker := range workers {
				if err := printRuntimeWorker(out, worker); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRuntimeWorkerPlacementPlanCommand(out io.Writer) *cobra.Command {
	var runID string
	cmd := &cobra.Command{
		Use:   "worker-placement-plan <workspace>",
		Short: "Write a dry-run metadata-only runtime worker placement plan",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			plan, err := app.CreateRuntimeWorkerPlacementPlan(args[0], app.RuntimeWorkerPlacementPlanOptions{
				Now:   time.Now().UTC(),
				RunID: runID,
			})
			if err != nil {
				return err
			}
			return printRuntimeWorkerPlacementPlan(out, plan)
		},
	}
	cmd.Flags().StringVar(&runID, "run-id", "", "planned run id to carry into placement candidates")
	return cmd
}

func newRuntimeWorkerPlacementPlansCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-placement-plans <workspace> [plan]",
		Short: "List or inspect runtime worker placement plan artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				plan, err := app.ReadRuntimeWorkerPlacementPlan(args[0], args[1])
				if err != nil {
					return err
				}
				return printRuntimeWorkerPlacementPlan(out, plan)
			}
			plans, err := app.ListRuntimeWorkerPlacementPlans(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-placement-plans: total=%d\n", len(plans)); err != nil {
				return err
			}
			for _, plan := range plans {
				if err := printRuntimeWorkerPlacementPlan(out, plan); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRuntimeExecuteWorkerPlacementPlanCommand(out io.Writer) *cobra.Command {
	var execute bool
	var executionID string
	var workerID string
	var leaseSeconds int64
	cmd := &cobra.Command{
		Use:   "execute-worker-placement-plan <workspace> <plan>",
		Short: "Create a worker queue claim from a ready placement candidate with an explicit gate",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requirePositiveRuntimeBound64("lease-seconds", leaseSeconds); err != nil {
				return err
			}
			execution, err := app.ExecuteRuntimeWorkerPlacementClaim(args[0], args[1], app.RuntimeWorkerPlacementClaimExecutionOptions{
				Execute:              execute,
				Now:                  time.Now().UTC(),
				CandidateExecutionID: executionID,
				WorkerID:             workerID,
				LeaseDuration:        time.Duration(leaseSeconds) * time.Second,
			})
			if err != nil && strings.TrimSpace(execution.Path) == "" && strings.TrimSpace(execution.Status) == "" {
				return err
			}
			if printErr := printRuntimeWorkerPlacementClaimExecution(out, execution); printErr != nil {
				return printErr
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&execute, "execute", false, "create a queue claim from the selected placement candidate")
	cmd.Flags().StringVar(&executionID, "execution-id", "", "candidate execution id to claim")
	cmd.Flags().StringVar(&workerID, "worker", "", "candidate worker id to claim")
	cmd.Flags().Int64Var(&leaseSeconds, "lease-seconds", int64(app.DefaultRuntimeWorkerQueueClaimLeaseDuration/time.Second), "created queue claim lease duration in seconds")
	return cmd
}

func newRuntimeWorkerPlacementClaimExecutionsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-placement-claim-executions <workspace> [execution]",
		Short: "List or inspect worker placement claim execution artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				execution, err := app.ReadRuntimeWorkerPlacementClaimExecution(args[0], args[1])
				if err != nil {
					return err
				}
				return printRuntimeWorkerPlacementClaimExecution(out, execution)
			}
			executions, err := app.ListRuntimeWorkerPlacementClaimExecutions(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-placement-claim-executions: total=%d\n", len(executions)); err != nil {
				return err
			}
			for _, execution := range executions {
				if err := printRuntimeWorkerPlacementClaimExecution(out, execution); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRuntimeWorkerPlacementDispatchPlanCommand(out io.Writer) *cobra.Command {
	var transportMode string
	cmd := &cobra.Command{
		Use:   "worker-placement-dispatch-plan <workspace>",
		Short: "Write a dry-run metadata-only dispatch plan for placement-derived queue claims",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			plan, err := app.CreateRuntimeWorkerPlacementDispatchPlan(args[0], app.RuntimeWorkerPlacementDispatchPlanOptions{
				Now:           time.Now().UTC(),
				TransportMode: transportMode,
			})
			if err != nil {
				return err
			}
			return printRuntimeWorkerPlacementDispatchPlan(out, plan)
		},
	}
	cmd.Flags().StringVar(&transportMode, "transport", "metadata-only", "planned queue dispatch transport: metadata-only, file-outbox, or http-json")
	return cmd
}

func newRuntimeWorkerPlacementDispatchPlansCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-placement-dispatch-plans <workspace> [plan]",
		Short: "List or inspect worker placement dispatch plan artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				plan, err := app.ReadRuntimeWorkerPlacementDispatchPlan(args[0], args[1])
				if err != nil {
					return err
				}
				return printRuntimeWorkerPlacementDispatchPlan(out, plan)
			}
			plans, err := app.ListRuntimeWorkerPlacementDispatchPlans(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-placement-dispatch-plans: total=%d\n", len(plans)); err != nil {
				return err
			}
			for _, plan := range plans {
				if err := printRuntimeWorkerPlacementDispatchPlan(out, plan); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRuntimeExecuteWorkerPlacementDispatchPlanCommand(out io.Writer) *cobra.Command {
	var execute bool
	var all bool
	var claimPath string
	var executionID string
	var workerID string
	var tokenEnv string
	var resultCallback string
	var maxCandidates int
	cmd := &cobra.Command{
		Use:   "execute-worker-placement-dispatch-plan <workspace> <plan>",
		Short: "Execute a worker placement dispatch plan through the queue dispatch gate",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireNonNegativeRuntimeBound("max-candidates", maxCandidates); err != nil {
				return err
			}
			if !execute {
				if all {
					return fmt.Errorf("worker placement dispatch batch requires explicit execute")
				}
				return fmt.Errorf("worker placement dispatch requires explicit execute")
			}
			if all {
				if strings.TrimSpace(claimPath) != "" || strings.TrimSpace(executionID) != "" || strings.TrimSpace(workerID) != "" {
					return fmt.Errorf("--all cannot be combined with --claim, --execution-id, or --worker")
				}
			}
			token, err := runtimeTokenFromEnvForPlacementDispatch(args[0], args[1], tokenEnv, "worker placement dispatch transport token")
			if err != nil {
				return err
			}
			if all {
				batch, err := app.ExecuteRuntimeWorkerPlacementDispatchBatch(args[0], args[1], app.RuntimeWorkerPlacementDispatchBatchOptions{
					Execute:                execute,
					Now:                    time.Now().UTC(),
					MaxCandidates:          maxCandidates,
					TransportAuthToken:     token,
					ResultCallbackEndpoint: resultCallback,
				})
				if err != nil && strings.TrimSpace(batch.Path) == "" && strings.TrimSpace(batch.Status) == "" {
					return err
				}
				if printErr := printRuntimeWorkerPlacementDispatchBatch(out, batch); printErr != nil {
					return printErr
				}
				return err
			}
			execution, err := app.ExecuteRuntimeWorkerPlacementDispatch(args[0], args[1], app.RuntimeWorkerPlacementDispatchExecutionOptions{
				Execute:                execute,
				Now:                    time.Now().UTC(),
				CandidateClaimPath:     claimPath,
				CandidateExecutionID:   executionID,
				WorkerID:               workerID,
				TransportAuthToken:     token,
				ResultCallbackEndpoint: resultCallback,
			})
			if err != nil && strings.TrimSpace(execution.Path) == "" && strings.TrimSpace(execution.Status) == "" {
				return err
			}
			if printErr := printRuntimeWorkerPlacementDispatchExecution(out, execution); printErr != nil {
				return printErr
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&execute, "execute", false, "execute the placement dispatch plan through the queue dispatch gate")
	cmd.Flags().BoolVar(&all, "all", false, "execute all ready placement dispatch candidates through the queue dispatch gate")
	cmd.Flags().StringVar(&claimPath, "claim", "", "candidate queue claim id, basename, or path to dispatch")
	cmd.Flags().StringVar(&executionID, "execution-id", "", "candidate execution id to dispatch")
	cmd.Flags().StringVar(&workerID, "worker", "", "candidate worker id to dispatch")
	cmd.Flags().StringVar(&tokenEnv, "transport-token-env", "", "environment variable containing bearer token for http-json transport")
	cmd.Flags().StringVar(&resultCallback, "result-callback", "", "HTTP(S) callback endpoint to carry into queue dispatch requests")
	cmd.Flags().IntVar(&maxCandidates, "max-candidates", 0, "maximum ready candidates to execute when --all is set; 0 means all")
	return cmd
}

func newRuntimeWorkerPlacementDispatchExecutionsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-placement-dispatch-executions <workspace> [execution]",
		Short: "List or inspect worker placement dispatch execution artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				execution, err := app.ReadRuntimeWorkerPlacementDispatchExecution(args[0], args[1])
				if err != nil {
					return err
				}
				return printRuntimeWorkerPlacementDispatchExecution(out, execution)
			}
			executions, err := app.ListRuntimeWorkerPlacementDispatchExecutions(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-placement-dispatch-executions: total=%d\n", len(executions)); err != nil {
				return err
			}
			for _, execution := range executions {
				if err := printRuntimeWorkerPlacementDispatchExecution(out, execution); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRuntimeWorkerPlacementDispatchBatchesCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-placement-dispatch-batches <workspace> [batch]",
		Short: "List or inspect worker placement dispatch batch artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				batch, err := app.ReadRuntimeWorkerPlacementDispatchBatch(args[0], args[1])
				if err != nil {
					return err
				}
				return printRuntimeWorkerPlacementDispatchBatch(out, batch)
			}
			batches, err := app.ListRuntimeWorkerPlacementDispatchBatches(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-placement-dispatch-batches: total=%d\n", len(batches)); err != nil {
				return err
			}
			for _, batch := range batches {
				if err := printRuntimeWorkerPlacementDispatchBatch(out, batch); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRuntimeWorkerOrchestrateCommand(out io.Writer) *cobra.Command {
	var execute bool
	var runID string
	var transportMode string
	var lifecycleRetryTransportMode string
	var maxCandidates int
	var leaseSeconds int64
	var tokenEnv string
	var resultCallback string
	var includeLifecycleMaintenance bool
	var executeLifecycleRetryDispatches bool
	var dispatchLifecycleRetryClaims bool
	cmd := &cobra.Command{
		Use:   "worker-orchestrate <workspace>",
		Short: "Write a runtime worker orchestration audit and optionally compose worker gates",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireNonNegativeRuntimeBound("max-candidates", maxCandidates); err != nil {
				return err
			}
			if err := requirePositiveRuntimeBound64("lease-seconds", leaseSeconds); err != nil {
				return err
			}
			if executeLifecycleRetryDispatches && !execute {
				return fmt.Errorf("--execute-lifecycle-retry-dispatches requires --execute")
			}
			if executeLifecycleRetryDispatches && !includeLifecycleMaintenance {
				return fmt.Errorf("--execute-lifecycle-retry-dispatches requires --include-lifecycle-maintenance")
			}
			if dispatchLifecycleRetryClaims && !execute {
				return fmt.Errorf("--dispatch-lifecycle-retry-claims requires --execute")
			}
			if dispatchLifecycleRetryClaims && !includeLifecycleMaintenance {
				return fmt.Errorf("--dispatch-lifecycle-retry-claims requires --include-lifecycle-maintenance")
			}
			if dispatchLifecycleRetryClaims && !executeLifecycleRetryDispatches {
				return fmt.Errorf("--dispatch-lifecycle-retry-claims requires --execute-lifecycle-retry-dispatches")
			}
			if err := requireRuntimeLifecycleRetryTransportMode(lifecycleRetryTransportMode); err != nil {
				return err
			}
			token, err := runtimeTokenFromEnvForTransport(execute, transportMode, tokenEnv, "worker orchestration transport token")
			if err != nil {
				return err
			}
			orchestration, err := app.CreateRuntimeWorkerOrchestration(args[0], app.RuntimeWorkerOrchestrationOptions{
				Execute:                         execute,
				Now:                             time.Now().UTC(),
				RunID:                           runID,
				TransportMode:                   transportMode,
				MaxCandidates:                   maxCandidates,
				LeaseDuration:                   time.Duration(leaseSeconds) * time.Second,
				TransportAuthToken:              token,
				ResultCallbackEndpoint:          resultCallback,
				IncludeLifecycleMaintenance:     includeLifecycleMaintenance,
				ExecuteLifecycleRetryDispatches: executeLifecycleRetryDispatches,
				DispatchLifecycleRetryClaims:    dispatchLifecycleRetryClaims,
				LifecycleRetryTransportMode:     lifecycleRetryTransportMode,
			})
			if err != nil && strings.TrimSpace(orchestration.Path) == "" && strings.TrimSpace(orchestration.Status) == "" {
				return err
			}
			if printErr := printRuntimeWorkerOrchestration(out, orchestration); printErr != nil {
				return printErr
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&execute, "execute", false, "compose placement claim and dispatch gates")
	cmd.Flags().StringVar(&runID, "run-id", "", "planned run id to carry into worker orchestration")
	cmd.Flags().StringVar(&transportMode, "transport", "metadata-only", "planned queue dispatch transport: metadata-only, file-outbox, or http-json")
	cmd.Flags().StringVar(&lifecycleRetryTransportMode, "lifecycle-retry-transport", "metadata-only", "lifecycle retry claim handoff transport: metadata-only or file-outbox")
	cmd.Flags().IntVar(&maxCandidates, "max-candidates", 0, "maximum ready candidates to claim and dispatch; 0 means all")
	cmd.Flags().Int64Var(&leaseSeconds, "lease-seconds", int64(app.DefaultRuntimeWorkerQueueClaimLeaseDuration/time.Second), "created queue claim lease duration in seconds")
	cmd.Flags().StringVar(&tokenEnv, "transport-token-env", "", "environment variable containing bearer token for http-json transport")
	cmd.Flags().StringVar(&resultCallback, "result-callback", "", "HTTP(S) callback endpoint to carry into queue dispatch requests")
	cmd.Flags().BoolVar(&includeLifecycleMaintenance, "include-lifecycle-maintenance", false, "with --execute, append metadata-only worker lifecycle maintenance and audit artifacts")
	cmd.Flags().BoolVar(&executeLifecycleRetryDispatches, "execute-lifecycle-retry-dispatches", false, "with --execute and --include-lifecycle-maintenance, run ready lifecycle retry-dispatch gates to create metadata-only retry queue claims")
	cmd.Flags().BoolVar(&dispatchLifecycleRetryClaims, "dispatch-lifecycle-retry-claims", false, "with lifecycle retry execution, dispatch created retry claims through metadata-only or file-outbox handoff")
	return cmd
}

func newRuntimeWorkerOrchestrationsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-orchestrations <workspace> [orchestration]",
		Short: "List or inspect runtime worker orchestration artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				orchestration, err := app.ReadRuntimeWorkerOrchestration(args[0], args[1])
				if err != nil {
					return err
				}
				return printRuntimeWorkerOrchestration(out, orchestration)
			}
			orchestrations, err := app.ListRuntimeWorkerOrchestrations(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-orchestrations: total=%d\n", len(orchestrations)); err != nil {
				return err
			}
			for _, orchestration := range orchestrations {
				if err := printRuntimeWorkerOrchestration(out, orchestration); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRuntimeWorkerOrchestrationSupervisorCommand(out io.Writer) *cobra.Command {
	var execute bool
	var runIDPrefix string
	var transportMode string
	var lifecycleRetryTransportMode string
	var maxCandidates int
	var maxRuns int
	var leaseSeconds int64
	var tokenEnv string
	var resultCallback string
	var includeLifecycleMaintenance bool
	var executeLifecycleRetryDispatches bool
	var dispatchLifecycleRetryClaims bool
	cmd := &cobra.Command{
		Use:   "worker-orchestration-supervisor <workspace>",
		Short: "Create a bounded metadata-only worker orchestration supervisor run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requirePositiveRuntimeBound("max-runs", maxRuns); err != nil {
				return err
			}
			if err := requireNonNegativeRuntimeBound("max-candidates", maxCandidates); err != nil {
				return err
			}
			if err := requirePositiveRuntimeBound64("lease-seconds", leaseSeconds); err != nil {
				return err
			}
			if executeLifecycleRetryDispatches && !execute {
				return fmt.Errorf("--execute-lifecycle-retry-dispatches requires --execute")
			}
			if executeLifecycleRetryDispatches && !includeLifecycleMaintenance {
				return fmt.Errorf("--execute-lifecycle-retry-dispatches requires --include-lifecycle-maintenance")
			}
			if dispatchLifecycleRetryClaims && !execute {
				return fmt.Errorf("--dispatch-lifecycle-retry-claims requires --execute")
			}
			if dispatchLifecycleRetryClaims && !includeLifecycleMaintenance {
				return fmt.Errorf("--dispatch-lifecycle-retry-claims requires --include-lifecycle-maintenance")
			}
			if dispatchLifecycleRetryClaims && !executeLifecycleRetryDispatches {
				return fmt.Errorf("--dispatch-lifecycle-retry-claims requires --execute-lifecycle-retry-dispatches")
			}
			if err := requireRuntimeLifecycleRetryTransportMode(lifecycleRetryTransportMode); err != nil {
				return err
			}
			token, err := runtimeTokenFromEnvForTransport(execute, transportMode, tokenEnv, "worker orchestration supervisor transport token")
			if err != nil {
				return err
			}
			supervisor, err := app.CreateRuntimeWorkerOrchestrationSupervisor(args[0], app.RuntimeWorkerOrchestrationSupervisorOptions{
				Execute:                         execute,
				Now:                             time.Now().UTC(),
				RunIDPrefix:                     runIDPrefix,
				TransportMode:                   transportMode,
				MaxCandidates:                   maxCandidates,
				MaxRuns:                         maxRuns,
				LeaseDuration:                   time.Duration(leaseSeconds) * time.Second,
				TransportAuthToken:              token,
				ResultCallbackEndpoint:          resultCallback,
				IncludeLifecycleMaintenance:     includeLifecycleMaintenance,
				ExecuteLifecycleRetryDispatches: executeLifecycleRetryDispatches,
				DispatchLifecycleRetryClaims:    dispatchLifecycleRetryClaims,
				LifecycleRetryTransportMode:     lifecycleRetryTransportMode,
			})
			if err != nil && strings.TrimSpace(supervisor.Path) == "" && strings.TrimSpace(supervisor.Status) == "" {
				return err
			}
			if printErr := printRuntimeWorkerOrchestrationSupervisor(out, supervisor); printErr != nil {
				return printErr
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&execute, "execute", false, "run bounded worker orchestration gates")
	cmd.Flags().StringVar(&runIDPrefix, "run-id-prefix", "", "run id prefix for child worker orchestrations")
	cmd.Flags().StringVar(&transportMode, "transport", "metadata-only", "planned queue dispatch transport: metadata-only, file-outbox, or http-json")
	cmd.Flags().StringVar(&lifecycleRetryTransportMode, "lifecycle-retry-transport", "metadata-only", "lifecycle retry claim handoff transport: metadata-only or file-outbox")
	cmd.Flags().IntVar(&maxCandidates, "max-candidates", 0, "maximum ready candidates per child orchestration; 0 means all")
	cmd.Flags().IntVar(&maxRuns, "max-runs", 1, "maximum child orchestration runs before supervisor exits")
	cmd.Flags().Int64Var(&leaseSeconds, "lease-seconds", int64(app.DefaultRuntimeWorkerQueueClaimLeaseDuration/time.Second), "created queue claim lease duration in seconds")
	cmd.Flags().StringVar(&tokenEnv, "transport-token-env", "", "environment variable containing bearer token for http-json transport")
	cmd.Flags().StringVar(&resultCallback, "result-callback", "", "HTTP(S) callback endpoint to carry into queue dispatch requests")
	cmd.Flags().BoolVar(&includeLifecycleMaintenance, "include-lifecycle-maintenance", false, "with --execute, append metadata-only lifecycle maintenance and audit artifacts in each child orchestration")
	cmd.Flags().BoolVar(&executeLifecycleRetryDispatches, "execute-lifecycle-retry-dispatches", false, "with --execute and --include-lifecycle-maintenance, run ready lifecycle retry-dispatch gates to create metadata-only retry queue claims")
	cmd.Flags().BoolVar(&dispatchLifecycleRetryClaims, "dispatch-lifecycle-retry-claims", false, "with lifecycle retry execution, dispatch created retry claims through metadata-only or file-outbox handoff")
	return cmd
}

func newRuntimeWorkerOrchestrationSupervisorRunsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-orchestration-supervisor-runs <workspace> [artifact]",
		Short: "List or inspect worker orchestration supervisor run artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				supervisor, err := app.ReadRuntimeWorkerOrchestrationSupervisor(args[0], args[1])
				if err != nil {
					return err
				}
				return printRuntimeWorkerOrchestrationSupervisor(out, supervisor)
			}
			supervisors, err := app.ListRuntimeWorkerOrchestrationSupervisors(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-orchestration-supervisor-runs: total=%d\n", len(supervisors)); err != nil {
				return err
			}
			for _, supervisor := range supervisors {
				if err := printRuntimeWorkerOrchestrationSupervisor(out, supervisor); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRuntimeWorkerQueueClaimCommand(out io.Writer) *cobra.Command {
	var runID string
	var executionID string
	var tool string
	var capability string
	var action string
	var runner string
	var parser string
	var runCommand []string
	var runTimeoutMS int
	var inputNodeIDs []string
	var expectedEvidence []string
	var leaseSeconds int64
	cmd := &cobra.Command{
		Use:   "worker-queue-claim <workspace> <worker>",
		Short: "Write a metadata-only worker queue claim artifact",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requirePositiveRuntimeBound64("lease-seconds", leaseSeconds); err != nil {
				return err
			}
			if err := requireNonNegativeRuntimeBound("run-timeout-ms", runTimeoutMS); err != nil {
				return err
			}
			claim, err := app.CreateRuntimeWorkerQueueClaim(args[0], app.RuntimeWorkerQueueClaim{
				WorkerID:    args[1],
				RunID:       runID,
				ExecutionID: executionID,
				Tool:        tool,
				Capability:  capability,
				ToolPlan: app.RuntimeWorkerToolPlanPayload{
					Action:           action,
					ManifestName:     tool,
					Runner:           runner,
					RunCommand:       runCommand,
					RunTimeoutMS:     runTimeoutMS,
					Parser:           parser,
					InputNodeIDs:     inputNodeIDs,
					ExpectedEvidence: expectedEvidence,
				},
			}, app.RuntimeWorkerQueueClaimOptions{
				Now:           time.Now().UTC(),
				LeaseDuration: time.Duration(leaseSeconds) * time.Second,
			})
			if err != nil {
				return err
			}
			return printRuntimeWorkerQueueClaim(out, claim)
		},
	}
	cmd.Flags().StringVar(&runID, "run-id", "", "run id to claim")
	cmd.Flags().StringVar(&executionID, "execution-id", "", "execution id to claim")
	cmd.Flags().StringVar(&tool, "tool", "", "tool or pack name to claim")
	cmd.Flags().StringVar(&capability, "capability", "", "capability to claim")
	cmd.Flags().StringVar(&action, "action", "", "semantic action for the claimed tool plan")
	cmd.Flags().StringVar(&runner, "runner", "", "runner declared by the claimed tool plan")
	cmd.Flags().StringVar(&parser, "parser", "", "parser declared by the claimed tool plan")
	cmd.Flags().StringArrayVar(&runCommand, "run-command", nil, "run command argument declared by the tool plan; may be repeated")
	cmd.Flags().IntVar(&runTimeoutMS, "run-timeout-ms", 0, "run timeout in milliseconds declared by the tool plan")
	cmd.Flags().StringArrayVar(&inputNodeIDs, "input-node", nil, "input node id declared by the tool plan; may be repeated")
	cmd.Flags().StringArrayVar(&expectedEvidence, "expected-evidence", nil, "expected evidence type declared by the tool plan; may be repeated")
	cmd.Flags().Int64Var(&leaseSeconds, "lease-seconds", int64(app.DefaultRuntimeWorkerQueueClaimLeaseDuration/time.Second), "claim lease duration in seconds")
	return cmd
}

func newRuntimeWorkerQueueClaimsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-queue-claims <workspace> [claim]",
		Short: "List or inspect worker queue claim artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				claim, err := app.ReadRuntimeWorkerQueueClaim(args[0], args[1])
				if err != nil {
					return err
				}
				return printRuntimeWorkerQueueClaim(out, claim)
			}
			claims, err := app.ListRuntimeWorkerQueueClaims(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-queue-claims: total=%d\n", len(claims)); err != nil {
				return err
			}
			for _, claim := range claims {
				if err := printRuntimeWorkerQueueClaim(out, claim); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRuntimeWorkerQueueClaimCancelCommand(out io.Writer) *cobra.Command {
	var cancel bool
	var canceledBy string
	var reason string
	cmd := &cobra.Command{
		Use:   "worker-queue-claim-cancel <workspace> <claim>",
		Short: "Cancel a claimed worker queue claim with an explicit gate",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			claim, err := app.CancelRuntimeWorkerQueueClaim(args[0], args[1], app.RuntimeWorkerQueueClaimCancelOptions{
				Cancel:     cancel,
				Now:        time.Now().UTC(),
				CanceledBy: canceledBy,
				Reason:     reason,
			})
			if err != nil {
				return err
			}
			return printRuntimeWorkerQueueClaim(out, claim)
		},
	}
	cmd.Flags().BoolVar(&cancel, "cancel", false, "cancel the claimed worker queue claim")
	cmd.Flags().StringVar(&canceledBy, "canceled-by", "", "operator or subsystem requesting the queue claim cancel")
	cmd.Flags().StringVar(&reason, "reason", "", "reason recorded on the canceled queue claim")
	return cmd
}

func newRuntimeWorkerQueueClaimExpireCommand(out io.Writer) *cobra.Command {
	var expire bool
	var expiredBy string
	var reason string
	cmd := &cobra.Command{
		Use:   "worker-queue-claim-expire <workspace> <claim>",
		Short: "Mark an expired claimed worker queue claim with an explicit gate",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			claim, err := app.ExpireRuntimeWorkerQueueClaim(args[0], args[1], app.RuntimeWorkerQueueClaimExpireOptions{
				Expire:    expire,
				Now:       time.Now().UTC(),
				ExpiredBy: expiredBy,
				Reason:    reason,
			})
			if err != nil {
				return err
			}
			return printRuntimeWorkerQueueClaim(out, claim)
		},
	}
	cmd.Flags().BoolVar(&expire, "expire", false, "mark the expired worker queue claim")
	cmd.Flags().StringVar(&expiredBy, "expired-by", "", "operator or subsystem marking the queue claim expired")
	cmd.Flags().StringVar(&reason, "reason", "", "reason recorded on the expired queue claim")
	return cmd
}

func newRuntimeWorkerQueueClaimRenewCommand(out io.Writer) *cobra.Command {
	var renew bool
	var leaseSeconds int64
	var renewedBy string
	var reason string
	cmd := &cobra.Command{
		Use:   "worker-queue-claim-renew <workspace> <claim>",
		Short: "Renew a claimed worker queue claim with an explicit gate",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if renew {
				if err := requirePositiveRuntimeBound64("lease-seconds", leaseSeconds); err != nil {
					return err
				}
			} else if leaseSeconds < 0 {
				if err := requirePositiveRuntimeBound64("lease-seconds", leaseSeconds); err != nil {
					return err
				}
			}
			claim, err := app.RenewRuntimeWorkerQueueClaim(args[0], args[1], app.RuntimeWorkerQueueClaimRenewOptions{
				Renew:         renew,
				Now:           time.Now().UTC(),
				LeaseDuration: time.Duration(leaseSeconds) * time.Second,
				RenewedBy:     renewedBy,
				Reason:        reason,
			})
			if err != nil {
				return err
			}
			return printRuntimeWorkerQueueClaim(out, claim)
		},
	}
	cmd.Flags().BoolVar(&renew, "renew", false, "renew the claimed worker queue claim")
	cmd.Flags().Int64Var(&leaseSeconds, "lease-seconds", 0, "renewed queue claim lease duration in seconds")
	cmd.Flags().StringVar(&renewedBy, "renewed-by", "", "operator or subsystem requesting the queue claim renewal")
	cmd.Flags().StringVar(&reason, "reason", "", "reason recorded on the renewed queue claim")
	return cmd
}

func newRuntimeExecuteWorkerQueueClaimCommand(out io.Writer) *cobra.Command {
	var execute bool
	var transportMode string
	var transportTokenEnv string
	var resultCallbackEndpoint string
	cmd := &cobra.Command{
		Use:   "execute-worker-queue-claim <workspace> <claim>",
		Short: "Execute a worker queue claim dispatch with an explicit gate",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !execute {
				return fmt.Errorf("worker queue claim dispatch requires explicit execute")
			}
			transportAuthToken, err := runtimeTokenFromEnvForTransport(execute, transportMode, transportTokenEnv, "worker queue dispatch transport token")
			if err != nil {
				return err
			}
			execution, err := app.ExecuteRuntimeWorkerQueueClaimDispatch(args[0], args[1], app.RuntimeWorkerQueueDispatchExecutionOptions{
				Execute:                execute,
				TransportMode:          transportMode,
				TransportAuthToken:     transportAuthToken,
				ResultCallbackEndpoint: resultCallbackEndpoint,
			})
			if err != nil {
				return err
			}
			return printRuntimeWorkerQueueDispatchExecution(out, execution)
		},
	}
	cmd.Flags().BoolVar(&execute, "execute", false, "execute the worker queue claim dispatch")
	cmd.Flags().StringVar(&transportMode, "transport", "metadata-only", "worker queue dispatch transport: metadata-only, file-outbox, or http-json")
	cmd.Flags().StringVar(&transportTokenEnv, "transport-token-env", "", "environment variable containing the http-json bearer token")
	cmd.Flags().StringVar(&resultCallbackEndpoint, "result-callback", "", "HTTP(S) endpoint remote workers should POST dispatch results to")
	return cmd
}

func newRuntimeWorkerQueueDispatchExecutionsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-queue-dispatch-executions <workspace> [execution]",
		Short: "List or inspect worker queue dispatch execution audit artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				execution, err := app.ReadRuntimeWorkerQueueDispatchExecution(args[0], args[1])
				if err != nil {
					return err
				}
				return printRuntimeWorkerQueueDispatchExecution(out, execution)
			}
			executions, err := app.ListRuntimeWorkerQueueDispatchExecutions(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-queue-dispatch-executions: total=%d\n", len(executions)); err != nil {
				return err
			}
			for _, execution := range executions {
				if err := printRuntimeWorkerQueueDispatchExecution(out, execution); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRuntimeWorkerTransportOutboxCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-transport-outbox <workspace> <worker> [request]",
		Short: "List or inspect file-outbox worker transport requests",
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 3 {
				request, err := app.ReadRuntimeWorkerTransportDispatchRequest(args[0], args[1], args[2])
				if err != nil {
					return err
				}
				return printRuntimeWorkerTransportDispatchRequest(out, request)
			}
			requests, err := app.ListRuntimeWorkerTransportDispatchRequests(args[0], args[1])
			if err != nil {
				return err
			}
			statusCounts := map[string]int{}
			for _, request := range requests {
				statusCounts[strings.TrimSpace(request.Status)]++
			}
			if _, err := fmt.Fprintf(
				out,
				"worker-transport-requests: total=%d pending=%d acknowledged=%d completed=%d failed=%d\n",
				len(requests),
				statusCounts["pending"],
				statusCounts["acknowledged"],
				statusCounts["completed"],
				statusCounts["failed"],
			); err != nil {
				return err
			}
			for _, request := range requests {
				if err := printRuntimeWorkerTransportDispatchRequest(out, request); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRuntimeWorkerTransportAckCommand(out io.Writer) *cobra.Command {
	var ack bool
	cmd := &cobra.Command{
		Use:   "worker-transport-ack <workspace> <worker> <request>",
		Short: "Acknowledge a pending file-outbox worker transport request",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			request, err := app.AcknowledgeRuntimeWorkerTransportDispatchRequest(args[0], args[1], args[2], app.RuntimeWorkerTransportAckOptions{
				Ack: ack,
				Now: time.Now().UTC(),
			})
			if err != nil {
				return err
			}
			return printRuntimeWorkerTransportDispatchRequest(out, request)
		},
	}
	cmd.Flags().BoolVar(&ack, "ack", false, "acknowledge the worker transport request")
	return cmd
}

func newRuntimeWorkerTransportServeHTTPCommand(out io.Writer) *cobra.Command {
	var execute bool
	var listen string
	var maxRequests int
	var authTokenEnv string
	cmd := &cobra.Command{
		Use:   "worker-transport-serve-http <workspace> <worker>",
		Short: "Receive http-json worker transport requests into the file-outbox",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !execute {
				return fmt.Errorf("worker transport http receiver requires explicit execute")
			}
			if err := requireNonNegativeRuntimeBound("max-requests", maxRequests); err != nil {
				return err
			}
			authToken, err := runtimeTokenFromEnv(authTokenEnv, "worker transport receiver auth token")
			if err != nil {
				return err
			}
			listener, err := net.Listen("tcp", listen)
			if err != nil {
				return fmt.Errorf("listen for worker transport http receiver: %w", err)
			}
			baseHandler := app.NewRuntimeWorkerTransportHTTPHandlerWithOptions(args[0], args[1], app.RuntimeWorkerTransportHTTPHandlerOptions{
				AuthToken: authToken,
			})
			server := &http.Server{}
			var mu sync.Mutex
			accepted := 0
			server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				recorder := &statusCaptureResponseWriter{ResponseWriter: w, status: http.StatusOK}
				baseHandler.ServeHTTP(recorder, r)
				if recorder.status < 200 || recorder.status > 299 {
					return
				}
				mu.Lock()
				accepted++
				shouldStop := maxRequests > 0 && accepted >= maxRequests
				mu.Unlock()
				if shouldStop {
					go func() {
						_ = server.Shutdown(context.Background())
					}()
				}
			})
			if _, err := fmt.Fprintf(out, "worker-transport-serve-http: worker=%s listen=%s\n", args[1], listener.Addr().String()); err != nil {
				_ = listener.Close()
				return err
			}
			err = server.Serve(listener)
			if errors.Is(err, http.ErrServerClosed) {
				err = nil
			}
			mu.Lock()
			finalAccepted := accepted
			mu.Unlock()
			if _, printErr := fmt.Fprintf(out, "worker-transport-serve-http: accepted=%d\n", finalAccepted); printErr != nil {
				return printErr
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&execute, "execute", false, "start the worker transport HTTP receiver")
	cmd.Flags().StringVar(&listen, "listen", "127.0.0.1:0", "TCP listen address for the worker transport HTTP receiver")
	cmd.Flags().IntVar(&maxRequests, "max-requests", 0, "stop after accepting this many requests; 0 runs until interrupted")
	cmd.Flags().StringVar(&authTokenEnv, "auth-token-env", "", "environment variable containing the receiver bearer token")
	return cmd
}

func runtimeTokenFromEnv(envName, label string) (string, error) {
	envName = strings.TrimSpace(envName)
	if envName == "" {
		return "", nil
	}
	token := strings.TrimSpace(os.Getenv(envName))
	if token == "" {
		return "", fmt.Errorf("%s env %s is empty or not set", label, envName)
	}
	return token, nil
}

func runtimeTokenFromEnvForTransport(execute bool, transportMode, envName, label string) (string, error) {
	if !execute || strings.TrimSpace(transportMode) != "http-json" {
		return "", nil
	}
	return runtimeTokenFromEnv(envName, label)
}

func runtimeTokenFromEnvForPlacementDispatch(root, planPath, envName, label string) (string, error) {
	if strings.TrimSpace(envName) == "" {
		return "", nil
	}
	plan, err := app.ReadRuntimeWorkerPlacementDispatchPlan(root, planPath)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(plan.TransportMode) != "http-json" {
		return "", nil
	}
	return runtimeTokenFromEnv(envName, label)
}

type statusCaptureResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusCaptureResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func newRuntimeWorkerTransportRunOnceCommand(out io.Writer) *cobra.Command {
	var execute bool
	var useRequestCommand bool
	var heartbeatLease bool
	var heartbeatBy string
	var heartbeatReason string
	cmd := &cobra.Command{
		Use:   "worker-transport-run-once <workspace> <worker> <request> [--use-request-command | -- <command> [args...]]",
		Short: "Execute one acknowledged file-outbox request with an explicit worker command",
		Args:  cobra.MinimumNArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := app.ExecuteRuntimeWorkerTransportDispatchRequest(context.Background(), args[0], args[1], args[2], app.RuntimeWorkerTransportRunOptions{
				Execute:           execute,
				Now:               time.Now().UTC(),
				Command:           args[3:],
				UseRequestCommand: useRequestCommand,
				HeartbeatLease:    heartbeatLease,
				HeartbeatBy:       heartbeatBy,
				HeartbeatReason:   heartbeatReason,
			})
			if err != nil && strings.TrimSpace(result.Path) == "" && strings.TrimSpace(result.Status) == "" {
				return err
			}
			if printErr := printRuntimeWorkerQueueDispatchResult(out, result); printErr != nil {
				return printErr
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&execute, "execute", false, "execute the acknowledged worker transport request")
	cmd.Flags().BoolVar(&useRequestCommand, "use-request-command", false, "execute the run_command stored in the request tool_plan")
	cmd.Flags().BoolVar(&heartbeatLease, "heartbeat-lease", false, "refresh the matching active worker lease heartbeat before and after execution")
	cmd.Flags().StringVar(&heartbeatBy, "heartbeat-by", "", "identity to record when refreshing the worker lease heartbeat; defaults to worker id")
	cmd.Flags().StringVar(&heartbeatReason, "heartbeat-reason", "", "reason to record when refreshing the worker lease heartbeat")
	return cmd
}

func newRuntimeWorkerTransportRunLoopCommand(out io.Writer) *cobra.Command {
	var execute bool
	var maxRequests int
	var useRequestCommand bool
	var heartbeatLease bool
	var heartbeatBy string
	var heartbeatReason string
	cmd := &cobra.Command{
		Use:   "worker-transport-run-loop <workspace> <worker> [--use-request-command | -- <command> [args...]]",
		Short: "Execute a bounded batch of acknowledged file-outbox requests",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requirePositiveRuntimeBound("max-requests", maxRequests); err != nil {
				return err
			}
			loop, err := app.ExecuteRuntimeWorkerTransportDispatchLoop(context.Background(), args[0], args[1], app.RuntimeWorkerTransportRunLoopOptions{
				Execute:           execute,
				Now:               time.Now().UTC(),
				Command:           args[2:],
				MaxRequests:       maxRequests,
				UseRequestCommand: useRequestCommand,
				HeartbeatLease:    heartbeatLease,
				HeartbeatBy:       heartbeatBy,
				HeartbeatReason:   heartbeatReason,
			})
			if err != nil && strings.TrimSpace(loop.Status) == "" {
				return err
			}
			if printErr := printRuntimeWorkerTransportRunLoopResult(out, loop); printErr != nil {
				return printErr
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&execute, "execute", false, "execute acknowledged worker transport requests")
	cmd.Flags().IntVar(&maxRequests, "max-requests", 1, "maximum acknowledged requests to execute")
	cmd.Flags().BoolVar(&useRequestCommand, "use-request-command", false, "execute each request's stored tool_plan run_command")
	cmd.Flags().BoolVar(&heartbeatLease, "heartbeat-lease", false, "refresh matching active worker lease heartbeats before and after each execution")
	cmd.Flags().StringVar(&heartbeatBy, "heartbeat-by", "", "identity to record when refreshing worker lease heartbeats; defaults to worker id")
	cmd.Flags().StringVar(&heartbeatReason, "heartbeat-reason", "", "reason to record when refreshing worker lease heartbeats")
	return cmd
}

func newRuntimeWorkerTransportRunDaemonCommand(out io.Writer) *cobra.Command {
	var execute bool
	var maxRequests int
	var idleRounds int
	var pollIntervalMS int
	var useRequestCommand bool
	var enableControl bool
	var heartbeatLease bool
	var heartbeatBy string
	var heartbeatReason string
	cmd := &cobra.Command{
		Use:   "worker-transport-run-daemon <workspace> <worker> [--use-request-command | -- <command> [args...]]",
		Short: "Poll and execute acknowledged file-outbox requests for one worker",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requirePositiveRuntimeBound("max-requests", maxRequests); err != nil {
				return err
			}
			if err := requirePositiveRuntimeBound("idle-rounds", idleRounds); err != nil {
				return err
			}
			if err := requirePositiveRuntimeBound("poll-interval-ms", pollIntervalMS); err != nil {
				return err
			}
			daemon, err := app.ExecuteRuntimeWorkerTransportDispatchDaemon(context.Background(), args[0], args[1], app.RuntimeWorkerTransportRunDaemonOptions{
				Execute:               execute,
				Now:                   time.Now().UTC(),
				Command:               args[2:],
				MaxRequests:           maxRequests,
				MaxIdleScans:          idleRounds,
				PollInterval:          time.Duration(pollIntervalMS) * time.Millisecond,
				UseRequestCommand:     useRequestCommand,
				EnableControlRequests: enableControl,
				HeartbeatLease:        heartbeatLease,
				HeartbeatBy:           heartbeatBy,
				HeartbeatReason:       heartbeatReason,
			})
			if err != nil && strings.TrimSpace(daemon.Status) == "" {
				return err
			}
			if printErr := printRuntimeWorkerTransportRunDaemonResult(out, daemon); printErr != nil {
				return printErr
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&execute, "execute", false, "execute acknowledged worker transport requests")
	cmd.Flags().IntVar(&maxRequests, "max-requests", 1, "maximum acknowledged requests to execute before exiting")
	cmd.Flags().IntVar(&idleRounds, "idle-rounds", 1, "maximum idle polling rounds before exiting")
	cmd.Flags().IntVar(&pollIntervalMS, "poll-interval-ms", 1000, "milliseconds between idle polling rounds")
	cmd.Flags().BoolVar(&useRequestCommand, "use-request-command", false, "execute each request's stored tool_plan run_command")
	cmd.Flags().BoolVar(&enableControl, "enable-control", false, "consume pending daemon stop/restart control requests before polling work")
	cmd.Flags().BoolVar(&heartbeatLease, "heartbeat-lease", false, "refresh matching active worker lease heartbeats before and after each execution")
	cmd.Flags().StringVar(&heartbeatBy, "heartbeat-by", "", "identity to record when refreshing worker lease heartbeats; defaults to worker id")
	cmd.Flags().StringVar(&heartbeatReason, "heartbeat-reason", "", "reason to record when refreshing worker lease heartbeats")
	return cmd
}

func newRuntimeWorkerTransportDaemonRunsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-transport-daemon-runs <workspace> <worker> [run]",
		Short: "List or inspect worker transport daemon run artifacts",
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 3 {
				run, err := app.ReadRuntimeWorkerTransportDaemonRun(args[0], args[1], args[2])
				if err != nil {
					return err
				}
				return printRuntimeWorkerTransportDaemonRun(out, run)
			}
			runs, err := app.ListRuntimeWorkerTransportDaemonRuns(args[0], args[1])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-transport-daemon-runs: total=%d\n", len(runs)); err != nil {
				return err
			}
			for _, run := range runs {
				if err := printRuntimeWorkerTransportDaemonRun(out, run); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRuntimeWorkerTransportRunSupervisorCommand(out io.Writer) *cobra.Command {
	var execute bool
	var maxRuns int
	var maxRequests int
	var idleRounds int
	var pollIntervalMS int
	var useRequestCommand bool
	var enableControl bool
	var heartbeatLease bool
	var heartbeatBy string
	var heartbeatReason string
	cmd := &cobra.Command{
		Use:   "worker-transport-run-supervisor <workspace> <worker> [--use-request-command | -- <command> [args...]]",
		Short: "Run bounded worker transport daemon supervision with restart handling",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requirePositiveRuntimeBound("max-runs", maxRuns); err != nil {
				return err
			}
			if err := requirePositiveRuntimeBound("max-requests", maxRequests); err != nil {
				return err
			}
			if err := requirePositiveRuntimeBound("idle-rounds", idleRounds); err != nil {
				return err
			}
			if err := requirePositiveRuntimeBound("poll-interval-ms", pollIntervalMS); err != nil {
				return err
			}
			supervisor, err := app.ExecuteRuntimeWorkerTransportDaemonSupervisor(context.Background(), args[0], args[1], app.RuntimeWorkerTransportDaemonSupervisorOptions{
				Execute:               execute,
				Now:                   time.Now().UTC(),
				Command:               args[2:],
				MaxRuns:               maxRuns,
				MaxRequests:           maxRequests,
				MaxIdleScans:          idleRounds,
				PollInterval:          time.Duration(pollIntervalMS) * time.Millisecond,
				UseRequestCommand:     useRequestCommand,
				EnableControlRequests: enableControl,
				HeartbeatLease:        heartbeatLease,
				HeartbeatBy:           heartbeatBy,
				HeartbeatReason:       heartbeatReason,
			})
			if err != nil && strings.TrimSpace(supervisor.Status) == "" {
				return err
			}
			if printErr := printRuntimeWorkerTransportRunSupervisorResult(out, supervisor); printErr != nil {
				return printErr
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&execute, "execute", false, "execute bounded daemon supervisor runs")
	cmd.Flags().IntVar(&maxRuns, "max-runs", 1, "maximum daemon runs before supervisor exits")
	cmd.Flags().IntVar(&maxRequests, "max-requests", 1, "maximum acknowledged requests each daemon run can execute")
	cmd.Flags().IntVar(&idleRounds, "idle-rounds", 1, "maximum idle polling rounds for each daemon run")
	cmd.Flags().IntVar(&pollIntervalMS, "poll-interval-ms", 1000, "milliseconds between idle polling rounds")
	cmd.Flags().BoolVar(&useRequestCommand, "use-request-command", false, "execute each request's stored tool_plan run_command")
	cmd.Flags().BoolVar(&enableControl, "enable-control", false, "consume pending daemon stop/restart controls during supervised daemon runs")
	cmd.Flags().BoolVar(&heartbeatLease, "heartbeat-lease", false, "refresh matching active worker lease heartbeats before and after each execution")
	cmd.Flags().StringVar(&heartbeatBy, "heartbeat-by", "", "identity to record when refreshing worker lease heartbeats; defaults to worker id")
	cmd.Flags().StringVar(&heartbeatReason, "heartbeat-reason", "", "reason to record when refreshing worker lease heartbeats")
	return cmd
}

func requirePositiveRuntimeBound(flag string, value int) error {
	if value <= 0 {
		return fmt.Errorf("--%s must be positive", flag)
	}
	return nil
}

func requirePositiveRuntimeBound64(flag string, value int64) error {
	if value <= 0 {
		return fmt.Errorf("--%s must be positive", flag)
	}
	return nil
}

func requireNonNegativeRuntimeBound(flag string, value int) error {
	if value < 0 {
		return fmt.Errorf("--%s cannot be negative", flag)
	}
	return nil
}

func requireRuntimeLifecycleRetryTransportMode(transportMode string) error {
	trimmed := strings.TrimSpace(transportMode)
	switch trimmed {
	case "", "metadata-only", "file-outbox":
		return nil
	case "http-json":
		return fmt.Errorf("--lifecycle-retry-transport supports metadata-only or file-outbox")
	default:
		return fmt.Errorf("unsupported lifecycle retry transport: %s", transportMode)
	}
}

func requireRuntimeDispatchRetryTransportMode(transportMode string) error {
	trimmed := strings.TrimSpace(transportMode)
	switch trimmed {
	case "", "metadata-only", "file-outbox":
		return nil
	case "http-json":
		return fmt.Errorf("--retry-transport supports metadata-only or file-outbox; worker dispatch retry claim handoff supports metadata-only or file-outbox transport")
	default:
		return fmt.Errorf("unsupported worker dispatch retry transport: %s", transportMode)
	}
}

func newRuntimeWorkerTransportSupervisorRunsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-transport-supervisor-runs <workspace> <worker> [run]",
		Short: "List or inspect worker transport daemon supervisor run artifacts",
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 3 {
				run, err := app.ReadRuntimeWorkerTransportDaemonSupervisorRun(args[0], args[1], args[2])
				if err != nil {
					return err
				}
				return printRuntimeWorkerTransportSupervisorRun(out, run)
			}
			runs, err := app.ListRuntimeWorkerTransportDaemonSupervisorRuns(args[0], args[1])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-transport-supervisor-runs: total=%d\n", len(runs)); err != nil {
				return err
			}
			for _, run := range runs {
				if err := printRuntimeWorkerTransportSupervisorRun(out, run); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRuntimeWorkerTransportDaemonControlCommand(out io.Writer) *cobra.Command {
	var action string
	var reason string
	var requestedBy string
	cmd := &cobra.Command{
		Use:   "worker-transport-daemon-control <workspace> <worker>",
		Short: "Write a pending metadata-only worker transport daemon control request",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			control, err := app.CreateRuntimeWorkerTransportDaemonControlRequest(args[0], args[1], app.RuntimeWorkerTransportDaemonControlRequest{
				Action:      action,
				Reason:      reason,
				RequestedBy: requestedBy,
			}, app.RuntimeWorkerTransportDaemonControlRequestOptions{Now: time.Now().UTC()})
			if err != nil {
				return err
			}
			return printRuntimeWorkerTransportDaemonControlRequest(out, control)
		},
	}
	cmd.Flags().StringVar(&action, "action", "", "daemon control action: stop or restart")
	cmd.Flags().StringVar(&reason, "reason", "", "operator reason for the daemon control request")
	cmd.Flags().StringVar(&requestedBy, "requested-by", "", "operator identity for daemon control audit metadata")
	return cmd
}

func newRuntimeWorkerTransportDaemonControlsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-transport-daemon-controls <workspace> <worker> [control]",
		Short: "List or inspect worker transport daemon control request artifacts",
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 3 {
				control, err := app.ReadRuntimeWorkerTransportDaemonControlRequest(args[0], args[1], args[2])
				if err != nil {
					return err
				}
				return printRuntimeWorkerTransportDaemonControlRequest(out, control)
			}
			controls, err := app.ListRuntimeWorkerTransportDaemonControlRequests(args[0], args[1])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-transport-daemon-controls: total=%d\n", len(controls)); err != nil {
				return err
			}
			for _, control := range controls {
				if err := printRuntimeWorkerTransportDaemonControlRequest(out, control); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRuntimeWorkerQueueDispatchResultCommand(out io.Writer) *cobra.Command {
	var status string
	var output string
	var resultError string
	cmd := &cobra.Command{
		Use:   "worker-queue-dispatch-result <workspace> <execution>",
		Short: "Submit a worker queue dispatch result artifact",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := app.SubmitRuntimeWorkerQueueDispatchResult(args[0], args[1], app.RuntimeWorkerQueueDispatchResult{
				Status: status,
				Output: output,
				Error:  resultError,
			}, app.RuntimeWorkerQueueDispatchResultOptions{Now: time.Now().UTC()})
			if err != nil {
				return err
			}
			return printRuntimeWorkerQueueDispatchResult(out, result)
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "worker queue dispatch result status: completed or failed")
	cmd.Flags().StringVar(&output, "output", "", "worker queue dispatch result output summary")
	cmd.Flags().StringVar(&resultError, "error", "", "worker queue dispatch result error summary")
	return cmd
}

func newRuntimeWorkerQueueDispatchResultServeHTTPCommand(out io.Writer) *cobra.Command {
	var execute bool
	var listen string
	var maxResults int
	var authTokenEnv string
	cmd := &cobra.Command{
		Use:   "worker-queue-dispatch-result-serve-http <workspace>",
		Short: "Receive worker queue dispatch result callbacks over HTTP",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !execute {
				return fmt.Errorf("worker queue dispatch result http receiver requires explicit execute")
			}
			if err := requireNonNegativeRuntimeBound("max-results", maxResults); err != nil {
				return err
			}
			authToken, err := runtimeTokenFromEnv(authTokenEnv, "worker queue dispatch result receiver auth token")
			if err != nil {
				return err
			}
			listener, err := net.Listen("tcp", listen)
			if err != nil {
				return fmt.Errorf("listen for worker queue dispatch result http receiver: %w", err)
			}
			baseHandler := app.NewRuntimeWorkerQueueDispatchResultHTTPHandler(args[0], app.RuntimeWorkerQueueDispatchResultHTTPHandlerOptions{
				AuthToken: authToken,
			})
			server := &http.Server{}
			var mu sync.Mutex
			accepted := 0
			server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				recorder := &statusCaptureResponseWriter{ResponseWriter: w, status: http.StatusOK}
				baseHandler.ServeHTTP(recorder, r)
				if recorder.status < 200 || recorder.status > 299 {
					return
				}
				mu.Lock()
				accepted++
				shouldStop := maxResults > 0 && accepted >= maxResults
				mu.Unlock()
				if shouldStop {
					go func() {
						_ = server.Shutdown(context.Background())
					}()
				}
			})
			if _, err := fmt.Fprintf(out, "worker-queue-dispatch-result-serve-http: listen=%s\n", listener.Addr().String()); err != nil {
				_ = listener.Close()
				return err
			}
			err = server.Serve(listener)
			if errors.Is(err, http.ErrServerClosed) {
				err = nil
			}
			mu.Lock()
			finalAccepted := accepted
			mu.Unlock()
			if _, printErr := fmt.Fprintf(out, "worker-queue-dispatch-result-serve-http: accepted=%d\n", finalAccepted); printErr != nil {
				return printErr
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&execute, "execute", false, "start the worker queue dispatch result HTTP receiver")
	cmd.Flags().StringVar(&listen, "listen", "127.0.0.1:0", "TCP listen address for the worker queue dispatch result receiver")
	cmd.Flags().IntVar(&maxResults, "max-results", 0, "stop after accepting this many result callbacks; 0 runs until interrupted")
	cmd.Flags().StringVar(&authTokenEnv, "auth-token-env", "", "environment variable containing the result receiver bearer token")
	return cmd
}

func newRuntimeWorkerQueueDispatchResultsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-queue-dispatch-results <workspace> [result]",
		Short: "List or inspect worker queue dispatch result artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				result, err := app.ReadRuntimeWorkerQueueDispatchResult(args[0], args[1])
				if err != nil {
					return err
				}
				return printRuntimeWorkerQueueDispatchResult(out, result)
			}
			results, err := app.ListRuntimeWorkerQueueDispatchResults(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-queue-dispatch-results: total=%d\n", len(results)); err != nil {
				return err
			}
			for _, result := range results {
				if err := printRuntimeWorkerQueueDispatchResult(out, result); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRuntimeWorkerDispatchRetryPlanCommand(out io.Writer) *cobra.Command {
	var write bool
	cmd := &cobra.Command{
		Use:   "worker-dispatch-retry-plan <workspace>",
		Short: "Print or write metadata-only retry candidates for failed worker dispatch results",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if write {
				plan, err := app.CreateRuntimeWorkerDispatchRetryPlan(args[0], time.Now().UTC())
				if err != nil {
					return err
				}
				return printRuntimeWorkerDispatchRetryPlan(out, plan)
			}
			plan, err := app.PlanRuntimeWorkerDispatchRetries(args[0])
			if err != nil {
				return err
			}
			return printRuntimeWorkerDispatchRetryPlanSummary(out, plan)
		},
	}
	cmd.Flags().BoolVar(&write, "write", false, "write a dry-run metadata-only worker dispatch retry plan artifact")
	return cmd
}

func newRuntimeWorkerDispatchRetryPlansCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-dispatch-retry-plans <workspace> [plan]",
		Short: "List or inspect worker dispatch retry plan artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				plan, err := app.ReadRuntimeWorkerDispatchRetryPlan(args[0], args[1])
				if err != nil {
					return err
				}
				return printRuntimeWorkerDispatchRetryPlan(out, plan)
			}
			plans, err := app.ListRuntimeWorkerDispatchRetryPlans(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-dispatch-retry-plans: total=%d\n", len(plans)); err != nil {
				return err
			}
			for _, plan := range plans {
				if err := printRuntimeWorkerDispatchRetryPlan(out, plan); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRuntimeWorkerDispatchRetryDispatchCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-dispatch-retry-dispatch <workspace> <plan>",
		Short: "Write a dry-run metadata-only worker dispatch retry dispatch artifact",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			dispatch, err := app.CreateRuntimeWorkerDispatchRetryDispatch(args[0], args[1], time.Now().UTC())
			if err != nil {
				return err
			}
			return printRuntimeWorkerDispatchRetryDispatch(out, dispatch)
		},
	}
}

func newRuntimeWorkerDispatchRetryDispatchesCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-dispatch-retry-dispatches <workspace> [dispatch]",
		Short: "List or inspect worker dispatch retry dispatch artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				dispatch, err := app.ReadRuntimeWorkerDispatchRetryDispatch(args[0], args[1])
				if err != nil {
					return err
				}
				return printRuntimeWorkerDispatchRetryDispatch(out, dispatch)
			}
			dispatches, err := app.ListRuntimeWorkerDispatchRetryDispatches(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-dispatch-retry-dispatches: total=%d\n", len(dispatches)); err != nil {
				return err
			}
			for _, dispatch := range dispatches {
				if err := printRuntimeWorkerDispatchRetryDispatch(out, dispatch); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRuntimeExecuteWorkerDispatchRetryDispatchCommand(out io.Writer) *cobra.Command {
	var execute bool
	var leaseSeconds int64
	var dispatchClaims bool
	var transportMode string
	var resultCallbackEndpoint string
	cmd := &cobra.Command{
		Use:   "execute-worker-dispatch-retry-dispatch <workspace> <dispatch>",
		Short: "Create metadata-only retry queue claims from a worker dispatch retry dispatch artifact",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requirePositiveRuntimeBound64("lease-seconds", leaseSeconds); err != nil {
				return err
			}
			execution, err := app.ExecuteRuntimeWorkerDispatchRetryDispatch(args[0], args[1], app.RuntimeWorkerDispatchRetryDispatchExecutionOptions{
				Execute:                execute,
				Now:                    time.Now().UTC(),
				LeaseDuration:          time.Duration(leaseSeconds) * time.Second,
				DispatchClaims:         dispatchClaims,
				TransportMode:          transportMode,
				ResultCallbackEndpoint: resultCallbackEndpoint,
			})
			if err != nil {
				return err
			}
			return printRuntimeWorkerDispatchRetryDispatchExecution(out, execution)
		},
	}
	cmd.Flags().BoolVar(&execute, "execute", false, "create retry queue claims from the reviewed retry dispatch artifact")
	cmd.Flags().Int64Var(&leaseSeconds, "lease-seconds", int64(app.DefaultRuntimeWorkerQueueClaimLeaseDuration/time.Second), "created retry queue claim lease duration in seconds")
	cmd.Flags().BoolVar(&dispatchClaims, "dispatch-claims", false, "dispatch newly created retry queue claims through the explicit queue dispatch gate")
	cmd.Flags().StringVar(&transportMode, "transport", "metadata-only", "retry queue claim dispatch transport: metadata-only or file-outbox")
	cmd.Flags().StringVar(&resultCallbackEndpoint, "result-callback", "", "HTTP(S) endpoint remote workers should POST dispatch results to")
	return cmd
}

func newRuntimeWorkerDispatchRetryDispatchExecutionsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-dispatch-retry-dispatch-executions <workspace> [execution]",
		Short: "List or inspect worker dispatch retry dispatch execution artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				execution, err := app.ReadRuntimeWorkerDispatchRetryDispatchExecution(args[0], args[1])
				if err != nil {
					return err
				}
				return printRuntimeWorkerDispatchRetryDispatchExecution(out, execution)
			}
			executions, err := app.ListRuntimeWorkerDispatchRetryDispatchExecutions(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-dispatch-retry-dispatch-executions: total=%d\n", len(executions)); err != nil {
				return err
			}
			for _, execution := range executions {
				if err := printRuntimeWorkerDispatchRetryDispatchExecution(out, execution); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func requirePositiveWorkerDispatchRetrySchedulerRuns(value int) error {
	if value <= 0 {
		return fmt.Errorf("--execute requires --max-runs > 0")
	}
	return nil
}

func newRuntimeWorkerDispatchRetrySchedulerRunCommand(out io.Writer) *cobra.Command {
	var execute bool
	var maxRuns int
	var actor string
	var reason string
	var leaseSeconds int64 = int64(app.DefaultRuntimeWorkerQueueClaimLeaseDuration / time.Second)
	var dispatchRetryClaims bool
	var retryTransportMode string
	cmd := &cobra.Command{
		Use:   "worker-dispatch-retry-scheduler-run <workspace>",
		Short: "Create one bounded worker dispatch retry scheduler audit",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if execute {
				if err := requirePositiveWorkerDispatchRetrySchedulerRuns(maxRuns); err != nil {
					return err
				}
			} else if err := requireNonNegativeRuntimeBound("max-runs", maxRuns); err != nil {
				return err
			}
			if err := requirePositiveRuntimeBound64("lease-seconds", leaseSeconds); err != nil {
				return err
			}
			if dispatchRetryClaims && !execute {
				return fmt.Errorf("retry claim handoff requires execute (--dispatch-retry-claims requires --execute)")
			}
			if err := requireRuntimeDispatchRetryTransportMode(retryTransportMode); err != nil {
				return err
			}
			run, err := app.CreateRuntimeWorkerDispatchRetrySchedulerRun(args[0], app.RuntimeWorkerDispatchRetrySchedulerRunOptions{
				Execute:             execute,
				MaxRuns:             maxRuns,
				Now:                 time.Now().UTC(),
				Actor:               actor,
				Reason:              reason,
				LeaseDuration:       time.Duration(leaseSeconds) * time.Second,
				DispatchRetryClaims: dispatchRetryClaims,
				RetryTransportMode:  retryTransportMode,
			})
			if err != nil && strings.TrimSpace(run.Path) == "" && strings.TrimSpace(run.Status) == "" {
				return err
			}
			if printErr := printRuntimeWorkerDispatchRetrySchedulerRun(out, run); printErr != nil {
				return printErr
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&execute, "execute", false, "execute a finite number of dispatch retry scheduler slices")
	cmd.Flags().IntVar(&maxRuns, "max-runs", 0, "maximum dispatch retry scheduler slices to create; required with --execute")
	cmd.Flags().StringVar(&actor, "actor", "", "operator identity recorded on the scheduler-run artifact")
	cmd.Flags().StringVar(&reason, "reason", "", "operator-visible reason for the scheduler run")
	cmd.Flags().Int64Var(&leaseSeconds, "lease-seconds", int64(app.DefaultRuntimeWorkerQueueClaimLeaseDuration/time.Second), "created retry queue claim lease duration in seconds")
	cmd.Flags().BoolVar(&dispatchRetryClaims, "dispatch-retry-claims", false, "with --execute, dispatch created retry claims through metadata-only or file-outbox handoff")
	cmd.Flags().StringVar(&retryTransportMode, "retry-transport", "metadata-only", "retry claim handoff transport: metadata-only or file-outbox")
	return cmd
}

func newRuntimeWorkerDispatchRetrySchedulerRunsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-dispatch-retry-scheduler-runs <workspace> [artifact]",
		Short: "List or inspect worker dispatch retry scheduler-run artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				run, err := app.ReadRuntimeWorkerDispatchRetrySchedulerRun(args[0], args[1])
				if err != nil {
					return err
				}
				return printRuntimeWorkerDispatchRetrySchedulerRun(out, run)
			}
			runs, err := app.ListRuntimeWorkerDispatchRetrySchedulerRuns(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-dispatch-retry-scheduler-runs: total=%d\n", len(runs)); err != nil {
				return err
			}
			for _, run := range runs {
				if err := printRuntimeWorkerDispatchRetrySchedulerRun(out, run); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRuntimeWorkerLeaseReclaimCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-lease-reclaim <workspace>",
		Short: "Write a dry-run metadata-only stale worker lease reclaim artifact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dispatch, err := app.CreateWorkerLeaseReclaimDispatch(args[0], time.Now().UTC(), app.DefaultWorkerLeaseStaleAfter)
			if err != nil {
				return err
			}
			return printWorkerLeaseReclaimDispatch(out, dispatch)
		},
	}
}

func newRuntimeWorkerLeaseHeartbeatCommand(out io.Writer) *cobra.Command {
	var heartbeat bool
	var heartbeatBy string
	var reason string
	cmd := &cobra.Command{
		Use:   "worker-lease-heartbeat <workspace> <run-id> <execution-id>",
		Short: "Refresh an active worker lease heartbeat with an explicit gate",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			lease, err := app.UpdateWorkerLeaseHeartbeat(args[0], args[1], args[2], app.WorkerLeaseHeartbeatOptions{
				Heartbeat:   heartbeat,
				Now:         time.Now().UTC(),
				HeartbeatBy: heartbeatBy,
				Reason:      reason,
			})
			if err != nil {
				return err
			}
			return printWorkerLeaseHeartbeat(out, lease)
		},
	}
	cmd.Flags().BoolVar(&heartbeat, "heartbeat", false, "refresh the active worker lease heartbeat")
	cmd.Flags().StringVar(&heartbeatBy, "heartbeat-by", "", "identity that refreshed the worker lease heartbeat")
	cmd.Flags().StringVar(&reason, "reason", "", "operator-visible reason for the heartbeat refresh")
	return cmd
}

func newRuntimeWorkerLeaseReleaseCommand(out io.Writer) *cobra.Command {
	var release bool
	var status string
	var releasedBy string
	var reason string
	var releaseError string
	var resultExecutionID string
	var attemptDir string
	cmd := &cobra.Command{
		Use:   "worker-lease-release <workspace> <run-id> <execution-id>",
		Short: "Release an active worker lease as completed or failed with an explicit gate",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			lease, err := app.ReleaseWorkerLease(args[0], args[1], args[2], app.WorkerLeaseReleaseOptions{
				Release:           release,
				Status:            status,
				Now:               time.Now().UTC(),
				ReleasedBy:        releasedBy,
				Reason:            reason,
				Error:             releaseError,
				ResultExecutionID: resultExecutionID,
				AttemptDir:        attemptDir,
			})
			if err != nil {
				return err
			}
			return printWorkerLeaseRelease(out, lease)
		},
	}
	cmd.Flags().BoolVar(&release, "release", false, "release the active worker lease")
	cmd.Flags().StringVar(&status, "status", "", "release status: completed or failed")
	cmd.Flags().StringVar(&releasedBy, "released-by", "", "identity that released the worker lease")
	cmd.Flags().StringVar(&reason, "reason", "", "operator-visible reason for releasing the worker lease")
	cmd.Flags().StringVar(&releaseError, "error", "", "error summary for failed worker leases")
	cmd.Flags().StringVar(&resultExecutionID, "result-execution-id", "", "result execution id linked to the released lease")
	cmd.Flags().StringVar(&attemptDir, "attempt-dir", "", "attempt directory linked to the released lease")
	return cmd
}

func newRuntimeWorkerLifecycleAuditCommand(out io.Writer) *cobra.Command {
	var write bool
	cmd := &cobra.Command{
		Use:   "worker-lifecycle-audit <workspace>",
		Short: "Show or persist a metadata-only worker lifecycle audit report",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if write {
				audit, err := app.CreateWorkerLifecycleAudit(args[0], time.Now().UTC(), app.DefaultWorkerLeaseStaleAfter)
				if err != nil {
					return err
				}
				return printWorkerLifecycleAudit(out, audit)
			}
			audit, err := app.PlanWorkerLifecycleAudit(args[0], time.Now().UTC(), app.DefaultWorkerLeaseStaleAfter)
			if err != nil {
				return err
			}
			return printWorkerLifecycleAudit(out, audit)
		},
	}
	cmd.Flags().BoolVar(&write, "write", false, "write a dry-run metadata-only worker lifecycle audit artifact")
	return cmd
}

func newRuntimeWorkerLifecycleAuditsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-lifecycle-audits <workspace> [artifact]",
		Short: "List or inspect worker lifecycle audit artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				audit, err := app.ReadWorkerLifecycleAudit(args[0], args[1])
				if err != nil {
					return err
				}
				return printWorkerLifecycleAudit(out, audit)
			}
			audits, err := app.ListWorkerLifecycleAudits(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-lifecycle-audits: total=%d\n", len(audits)); err != nil {
				return err
			}
			for _, audit := range audits {
				if err := printWorkerLifecycleAudit(out, audit); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRuntimeWorkerLifecycleReviewCommand(out io.Writer) *cobra.Command {
	var write bool
	cmd := &cobra.Command{
		Use:   "worker-lifecycle-review <workspace>",
		Short: "Show or persist a metadata-only worker lifecycle review",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if write {
				review, err := app.CreateWorkerLifecycleReview(args[0], time.Now().UTC(), app.DefaultWorkerLeaseStaleAfter)
				if err != nil {
					return err
				}
				return printWorkerLifecycleReview(out, review)
			}
			plan, err := app.PlanWorkerLifecycleReview(args[0], time.Now().UTC(), app.DefaultWorkerLeaseStaleAfter)
			if err != nil {
				return err
			}
			return printWorkerLifecycleReviewPlan(out, plan)
		},
	}
	cmd.Flags().BoolVar(&write, "write", false, "write a dry-run metadata-only worker lifecycle review artifact")
	return cmd
}

func newRuntimeWorkerLifecycleReviewsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-lifecycle-reviews <workspace> [artifact]",
		Short: "List or inspect worker lifecycle review artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				review, err := app.ReadWorkerLifecycleReview(args[0], args[1])
				if err != nil {
					return err
				}
				return printWorkerLifecycleReview(out, review)
			}
			reviews, err := app.ListWorkerLifecycleReviews(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-lifecycle-reviews: total=%d\n", len(reviews)); err != nil {
				return err
			}
			for _, review := range reviews {
				if err := printWorkerLifecycleReview(out, review); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRuntimeWorkerLifecycleActionPlanCommand(out io.Writer) *cobra.Command {
	var write bool
	cmd := &cobra.Command{
		Use:   "worker-lifecycle-action-plan <workspace> [review]",
		Short: "Show or persist a metadata-only worker lifecycle action plan",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			reviewPath := ""
			if len(args) == 2 {
				reviewPath = args[1]
			}
			if write {
				plan, err := app.CreateWorkerLifecycleActionPlan(args[0], reviewPath, time.Now().UTC(), app.DefaultWorkerLeaseStaleAfter)
				if err != nil {
					return err
				}
				return printWorkerLifecycleActionPlan(out, plan)
			}
			plan, err := app.PlanWorkerLifecycleActionPlan(args[0], reviewPath, time.Now().UTC(), app.DefaultWorkerLeaseStaleAfter)
			if err != nil {
				return err
			}
			return printWorkerLifecycleActionPlan(out, plan)
		},
	}
	cmd.Flags().BoolVar(&write, "write", false, "write a dry-run metadata-only worker lifecycle action-plan artifact")
	return cmd
}

func newRuntimeWorkerLifecycleActionPlanAnnotateCommand(out io.Writer) *cobra.Command {
	var decision string
	var reviewer string
	var reviewedAtArg string
	var reason string
	cmd := &cobra.Command{
		Use:   "worker-lifecycle-action-plan-annotate <workspace> <artifact>",
		Short: "Annotate a worker lifecycle action-plan artifact",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var reviewedAt time.Time
			if strings.TrimSpace(reviewedAtArg) != "" {
				parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(reviewedAtArg))
				if err != nil {
					return fmt.Errorf("reviewed-at must be RFC3339: %w", err)
				}
				reviewedAt = parsed
			}
			plan, err := app.AnnotateWorkerLifecycleActionPlan(args[0], args[1], app.WorkerLifecycleActionPlanAnnotationOptions{
				Decision:   decision,
				Reviewer:   reviewer,
				ReviewedAt: reviewedAt,
				Reason:     reason,
			})
			if err != nil {
				return err
			}
			return printWorkerLifecycleActionPlan(out, plan)
		},
	}
	cmd.Flags().StringVar(&decision, "decision", "", "review decision: accepted, deferred, or rejected")
	cmd.Flags().StringVar(&reviewer, "reviewer", "", "reviewer identity; defaults to operator")
	cmd.Flags().StringVar(&reviewedAtArg, "reviewed-at", "", "review timestamp in RFC3339; defaults to now")
	cmd.Flags().StringVar(&reason, "reason", "", "review decision reason")
	return cmd
}

func newRuntimeWorkerLifecycleActionPlanSelectCommand(out io.Writer) *cobra.Command {
	var selectedBy string
	var selectedAtArg string
	var reason string
	cmd := &cobra.Command{
		Use:   "worker-lifecycle-action-plan-select <workspace> <artifact>",
		Short: "Mark an accepted worker lifecycle action plan as current",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var selectedAt time.Time
			if strings.TrimSpace(selectedAtArg) != "" {
				parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(selectedAtArg))
				if err != nil {
					return fmt.Errorf("selected-at must be RFC3339: %w", err)
				}
				selectedAt = parsed
			}
			selection, err := app.SelectWorkerLifecycleActionPlan(args[0], args[1], app.WorkerLifecycleActionPlanSelectionOptions{
				SelectedBy: selectedBy,
				SelectedAt: selectedAt,
				Reason:     reason,
			})
			if err != nil {
				return err
			}
			return printWorkerLifecycleActionPlanSelection(out, selection)
		},
	}
	cmd.Flags().StringVar(&selectedBy, "selected-by", "", "operator identity; defaults to operator")
	cmd.Flags().StringVar(&selectedAtArg, "selected-at", "", "selection timestamp in RFC3339; defaults to now")
	cmd.Flags().StringVar(&reason, "reason", "", "selection reason")
	return cmd
}

func newRuntimeWorkerLifecycleActionPlanSelectionCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-lifecycle-action-plan-selection <workspace>",
		Short: "Inspect the current worker lifecycle action-plan selection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			selection, found, err := app.ReadWorkerLifecycleActionPlanSelection(args[0])
			if err != nil {
				return err
			}
			if !found {
				_, err := fmt.Fprintln(out, "worker-lifecycle-action-plan-selection: none")
				return err
			}
			return printWorkerLifecycleActionPlanSelection(out, selection)
		},
	}
}

func newRuntimeWorkerLifecycleActionPlanSelectionClearCommand(out io.Writer) *cobra.Command {
	var clearedBy string
	var clearedAtArg string
	var reason string
	cmd := &cobra.Command{
		Use:   "worker-lifecycle-action-plan-selection-clear <workspace>",
		Short: "Clear the current worker lifecycle action-plan selection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var clearedAt time.Time
			if strings.TrimSpace(clearedAtArg) != "" {
				parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(clearedAtArg))
				if err != nil {
					return fmt.Errorf("cleared-at must be RFC3339: %w", err)
				}
				clearedAt = parsed
			}
			selection, err := app.ClearWorkerLifecycleActionPlanSelection(args[0], app.WorkerLifecycleActionPlanSelectionClearOptions{
				ClearedBy: clearedBy,
				ClearedAt: clearedAt,
				Reason:    reason,
			})
			if err != nil {
				return err
			}
			return printWorkerLifecycleActionPlanSelection(out, selection)
		},
	}
	cmd.Flags().StringVar(&clearedBy, "cleared-by", "", "operator identity; defaults to operator")
	cmd.Flags().StringVar(&clearedAtArg, "cleared-at", "", "clear timestamp in RFC3339; defaults to now")
	cmd.Flags().StringVar(&reason, "reason", "", "clear reason")
	return cmd
}

func newRuntimeWorkerLifecycleActionPlansCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-lifecycle-action-plans <workspace> [artifact]",
		Short: "List or inspect worker lifecycle action-plan artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				plan, err := app.ReadWorkerLifecycleActionPlan(args[0], args[1])
				if err != nil {
					return err
				}
				return printWorkerLifecycleActionPlan(out, plan)
			}
			plans, err := app.ListWorkerLifecycleActionPlans(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-lifecycle-action-plans: total=%d\n", len(plans)); err != nil {
				return err
			}
			for _, plan := range plans {
				if err := printWorkerLifecycleActionPlan(out, plan); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRuntimeWorkerLifecycleMaintenanceCommand(out io.Writer) *cobra.Command {
	var execute bool
	var executeRetryDispatches bool
	var dispatchLifecycleRetryClaims bool
	var lifecycleRetryTransportMode string
	var executeLeaseRecovery bool
	cmd := &cobra.Command{
		Use:   "worker-lifecycle-maintenance <workspace>",
		Short: "Create a bounded metadata-only worker lifecycle maintenance audit",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dispatchLifecycleRetryClaims && !execute {
				return fmt.Errorf("--dispatch-lifecycle-retry-claims requires --execute")
			}
			if executeRetryDispatches && !execute {
				return fmt.Errorf("worker lifecycle maintenance retry-dispatch execution requires explicit execute")
			}
			if dispatchLifecycleRetryClaims && !executeRetryDispatches {
				return fmt.Errorf("--dispatch-lifecycle-retry-claims requires --execute-retry-dispatches")
			}
			if err := requireRuntimeLifecycleRetryTransportMode(lifecycleRetryTransportMode); err != nil {
				return err
			}
			if executeLeaseRecovery && !execute {
				return fmt.Errorf("worker lifecycle maintenance lease recovery execution requires explicit execute")
			}
			maintenance, err := app.CreateWorkerLifecycleMaintenance(args[0], app.WorkerLifecycleMaintenanceOptions{
				Execute:                    execute,
				ExecuteRetryDispatches:     executeRetryDispatches,
				DispatchRetryClaims:        dispatchLifecycleRetryClaims,
				RetryDispatchTransportMode: lifecycleRetryTransportMode,
				ExecuteLeaseRecovery:       executeLeaseRecovery,
				Now:                        time.Now().UTC(),
			})
			if err != nil {
				return err
			}
			return printWorkerLifecycleMaintenance(out, maintenance)
		},
	}
	cmd.Flags().BoolVar(&execute, "execute", false, "write metadata-only lifecycle review/action/recovery review artifacts")
	cmd.Flags().BoolVar(&executeRetryDispatches, "execute-retry-dispatches", false, "with --execute, run ready worker retry-dispatch execution gates to create retry queue claims without dispatching or running tools")
	cmd.Flags().BoolVar(&dispatchLifecycleRetryClaims, "dispatch-lifecycle-retry-claims", false, "with retry-dispatch execution, dispatch created retry claims through metadata-only or file-outbox handoff")
	cmd.Flags().StringVar(&lifecycleRetryTransportMode, "lifecycle-retry-transport", "metadata-only", "lifecycle retry claim handoff transport: metadata-only or file-outbox")
	cmd.Flags().BoolVar(&executeLeaseRecovery, "execute-lease-recovery", false, "with --execute, run ready lease reclaim and lease-result reconciliation execution gates without dispatching workers or running tools")
	return cmd
}

func newRuntimeWorkerLifecycleMaintenancesCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-lifecycle-maintenances <workspace> [artifact]",
		Short: "List or inspect worker lifecycle maintenance audits",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				maintenance, err := app.ReadWorkerLifecycleMaintenance(args[0], args[1])
				if err != nil {
					return err
				}
				return printWorkerLifecycleMaintenance(out, maintenance)
			}
			maintenances, err := app.ListWorkerLifecycleMaintenances(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-lifecycle-maintenances: total=%d\n", len(maintenances)); err != nil {
				return err
			}
			for _, maintenance := range maintenances {
				if err := printWorkerLifecycleMaintenance(out, maintenance); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRuntimeWorkerLifecycleMaintenanceSupervisorCommand(out io.Writer) *cobra.Command {
	var execute bool
	var executeRetryDispatches bool
	var dispatchLifecycleRetryClaims bool
	var lifecycleRetryTransportMode string
	var executeLeaseRecovery bool
	var maxRuns int
	cmd := &cobra.Command{
		Use:   "worker-lifecycle-maintenance-supervisor <workspace>",
		Short: "Create a bounded metadata-only worker lifecycle maintenance supervisor run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requirePositiveRuntimeBound("max-runs", maxRuns); err != nil {
				return err
			}
			if dispatchLifecycleRetryClaims && !execute {
				return fmt.Errorf("--dispatch-lifecycle-retry-claims requires --execute")
			}
			if executeRetryDispatches && !execute {
				return fmt.Errorf("worker lifecycle maintenance supervisor retry-dispatch execution requires explicit execute")
			}
			if dispatchLifecycleRetryClaims && !executeRetryDispatches {
				return fmt.Errorf("--dispatch-lifecycle-retry-claims requires --execute-retry-dispatches")
			}
			if err := requireRuntimeLifecycleRetryTransportMode(lifecycleRetryTransportMode); err != nil {
				return err
			}
			if executeLeaseRecovery && !execute {
				return fmt.Errorf("worker lifecycle maintenance supervisor lease recovery execution requires explicit execute")
			}
			supervisor, err := app.CreateWorkerLifecycleMaintenanceSupervisor(args[0], app.WorkerLifecycleMaintenanceSupervisorOptions{
				Execute:                    execute,
				ExecuteRetryDispatches:     executeRetryDispatches,
				DispatchRetryClaims:        dispatchLifecycleRetryClaims,
				RetryDispatchTransportMode: lifecycleRetryTransportMode,
				ExecuteLeaseRecovery:       executeLeaseRecovery,
				Now:                        time.Now().UTC(),
				MaxRuns:                    maxRuns,
			})
			if err != nil {
				return err
			}
			return printWorkerLifecycleMaintenanceSupervisor(out, supervisor)
		},
	}
	cmd.Flags().BoolVar(&execute, "execute", false, "run bounded metadata-only lifecycle maintenance and audit passes")
	cmd.Flags().BoolVar(&executeRetryDispatches, "execute-retry-dispatches", false, "with --execute, run ready worker retry-dispatch execution gates to create retry queue claims without dispatching or running tools")
	cmd.Flags().BoolVar(&dispatchLifecycleRetryClaims, "dispatch-lifecycle-retry-claims", false, "with retry-dispatch execution, dispatch created retry claims through metadata-only or file-outbox handoff")
	cmd.Flags().StringVar(&lifecycleRetryTransportMode, "lifecycle-retry-transport", "metadata-only", "lifecycle retry claim handoff transport: metadata-only or file-outbox")
	cmd.Flags().BoolVar(&executeLeaseRecovery, "execute-lease-recovery", false, "with --execute, run ready lease reclaim and lease-result reconciliation execution gates without dispatching workers or running tools")
	cmd.Flags().IntVar(&maxRuns, "max-runs", 1, "maximum maintenance passes before supervisor exits")
	return cmd
}

func newRuntimeWorkerLifecycleMaintenanceSupervisorRunsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-lifecycle-maintenance-supervisor-runs <workspace> [artifact]",
		Short: "List or inspect worker lifecycle maintenance supervisor run artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				supervisor, err := app.ReadWorkerLifecycleMaintenanceSupervisor(args[0], args[1])
				if err != nil {
					return err
				}
				return printWorkerLifecycleMaintenanceSupervisor(out, supervisor)
			}
			supervisors, err := app.ListWorkerLifecycleMaintenanceSupervisors(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-lifecycle-maintenance-supervisor-runs: total=%d\n", len(supervisors)); err != nil {
				return err
			}
			for _, supervisor := range supervisors {
				if err := printWorkerLifecycleMaintenanceSupervisor(out, supervisor); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRuntimeWorkerLifecyclePolicyCommand(out io.Writer) *cobra.Command {
	var write bool
	var staleAfterSeconds int64 = int64(app.DefaultWorkerLeaseStaleAfter / time.Second)
	var workerStaleAfterSeconds int64 = int64(app.DefaultRuntimeWorkerStaleAfter / time.Second)
	var pollIntervalMS int
	var maxRuns int = 1
	var transportMode string
	var includeLifecycleMaintenance bool
	var executeLifecycleRetryDispatches bool
	var dispatchLifecycleRetryClaims bool
	var lifecycleRetryTransportMode string
	var executeLeaseRecovery bool
	cmd := &cobra.Command{
		Use:   "worker-lifecycle-policy <workspace>",
		Short: "Show or persist a metadata-only worker lifecycle runtime policy",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requirePositiveRuntimeBound64("stale-after-seconds", staleAfterSeconds); err != nil {
				return err
			}
			if err := requirePositiveRuntimeBound64("worker-stale-after-seconds", workerStaleAfterSeconds); err != nil {
				return err
			}
			if err := requireNonNegativeRuntimeBound("poll-interval-ms", pollIntervalMS); err != nil {
				return err
			}
			if err := requirePositiveRuntimeBound("max-runs", maxRuns); err != nil {
				return err
			}
			if dispatchLifecycleRetryClaims && !includeLifecycleMaintenance {
				return fmt.Errorf("--dispatch-lifecycle-retry-claims requires --include-lifecycle-maintenance")
			}
			if dispatchLifecycleRetryClaims && !executeLifecycleRetryDispatches {
				return fmt.Errorf("--dispatch-lifecycle-retry-claims requires --execute-lifecycle-retry-dispatches")
			}
			if err := requireRuntimeLifecycleRetryTransportMode(lifecycleRetryTransportMode); err != nil {
				return err
			}
			options := app.WorkerLifecyclePolicyOptions{
				Now:                          time.Now().UTC(),
				StaleAfter:                   time.Duration(staleAfterSeconds) * time.Second,
				WorkerStaleAfter:             time.Duration(workerStaleAfterSeconds) * time.Second,
				PollInterval:                 time.Duration(pollIntervalMS) * time.Millisecond,
				MaxSupervisorRuns:            maxRuns,
				TransportMode:                transportMode,
				IncludeLifecycleMaintenance:  includeLifecycleMaintenance,
				ExecuteLifecycleRetries:      executeLifecycleRetryDispatches,
				DispatchLifecycleRetryClaims: dispatchLifecycleRetryClaims,
				LifecycleRetryTransportMode:  lifecycleRetryTransportMode,
				ExecuteLeaseRecovery:         executeLeaseRecovery,
			}
			if write {
				policy, err := app.CreateWorkerLifecyclePolicy(args[0], options)
				if err != nil {
					return err
				}
				return printWorkerLifecyclePolicy(out, policy)
			}
			policy, err := app.PreviewWorkerLifecyclePolicy(args[0], options)
			if err != nil {
				return err
			}
			return printWorkerLifecyclePolicy(out, policy)
		},
	}
	cmd.Flags().BoolVar(&write, "write", false, "write a dry-run metadata-only worker lifecycle policy artifact")
	cmd.Flags().Int64Var(&staleAfterSeconds, "stale-after-seconds", staleAfterSeconds, "seconds before active worker leases are stale")
	cmd.Flags().Int64Var(&workerStaleAfterSeconds, "worker-stale-after-seconds", workerStaleAfterSeconds, "seconds before runtime worker registrations are stale")
	cmd.Flags().IntVar(&pollIntervalMS, "poll-interval-ms", 0, "milliseconds between future lifecycle policy polling rounds; 0 disables background polling")
	cmd.Flags().IntVar(&maxRuns, "max-runs", 1, "maximum future lifecycle supervisor runs before exiting")
	cmd.Flags().StringVar(&transportMode, "transport", "metadata-only", "planned queue dispatch transport: metadata-only, file-outbox, or http-json")
	cmd.Flags().BoolVar(&includeLifecycleMaintenance, "include-lifecycle-maintenance", false, "include metadata-only lifecycle maintenance in the future worker lifecycle schedule")
	cmd.Flags().BoolVar(&executeLifecycleRetryDispatches, "execute-lifecycle-retry-dispatches", false, "allow future lifecycle retry-dispatch execution gates to create metadata-only retry queue claims")
	cmd.Flags().BoolVar(&dispatchLifecycleRetryClaims, "dispatch-lifecycle-retry-claims", false, "allow future lifecycle retry-dispatch gates to hand retry claims to metadata-only or file-outbox dispatch")
	cmd.Flags().StringVar(&lifecycleRetryTransportMode, "lifecycle-retry-transport", "metadata-only", "future lifecycle retry claim handoff transport: metadata-only or file-outbox")
	cmd.Flags().BoolVar(&executeLeaseRecovery, "execute-lifecycle-lease-recovery", false, "allow future lifecycle lease recovery execution gates to reclaim stale leases and reconcile returned results")
	cmd.Flags().BoolVar(&executeLeaseRecovery, "execute-lease-recovery", false, "alias for --execute-lifecycle-lease-recovery")
	return cmd
}

func newRuntimeWorkerLifecyclePolicyRunCommand(out io.Writer) *cobra.Command {
	var execute bool
	var actor string
	var reason string
	cmd := &cobra.Command{
		Use:   "worker-lifecycle-policy-run <workspace>",
		Short: "Run the current worker lifecycle policy as a bounded metadata-only scheduler slice",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			selection, found, err := app.ReadWorkerLifecyclePolicySelection(args[0])
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("worker lifecycle policy selection marker is required")
			}
			if strings.TrimSpace(selection.Status) != "current" {
				return fmt.Errorf("worker lifecycle policy selection must be current")
			}
			run, err := app.CreateWorkerLifecyclePolicyRun(args[0], app.WorkerLifecyclePolicyRunOptions{
				Execute: execute,
				Now:     time.Now().UTC(),
				Actor:   actor,
				Reason:  reason,
			})
			if err != nil {
				return err
			}
			return printWorkerLifecyclePolicyRun(out, run)
		},
	}
	cmd.Flags().BoolVar(&execute, "execute", false, "execute the selected policy's bounded lifecycle maintenance supervisor")
	cmd.Flags().StringVar(&actor, "actor", "", "operator identity recorded on the policy-run artifact")
	cmd.Flags().StringVar(&reason, "reason", "", "operator-visible reason for the policy run")
	return cmd
}

func newRuntimeWorkerLifecyclePolicyRunsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-lifecycle-policy-runs <workspace> [artifact]",
		Short: "List or inspect worker lifecycle policy-run artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				run, err := app.ReadWorkerLifecyclePolicyRun(args[0], args[1])
				if err != nil {
					return err
				}
				return printWorkerLifecyclePolicyRun(out, run)
			}
			runs, err := app.ListWorkerLifecyclePolicyRuns(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-lifecycle-policy-runs: total=%d\n", len(runs)); err != nil {
				return err
			}
			for _, run := range runs {
				if err := printWorkerLifecyclePolicyRun(out, run); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func requirePositiveWorkerLifecyclePolicySchedulerRuns(value int) error {
	if value <= 0 {
		return fmt.Errorf("--execute requires --max-runs > 0")
	}
	return nil
}

func newRuntimeWorkerLifecyclePolicySchedulerRunCommand(out io.Writer) *cobra.Command {
	var execute bool
	var maxRuns int
	var actor string
	var reason string
	cmd := &cobra.Command{
		Use:   "worker-lifecycle-policy-scheduler-run <workspace>",
		Short: "Create one bounded metadata-only worker lifecycle policy scheduler audit",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if execute {
				if err := requirePositiveWorkerLifecyclePolicySchedulerRuns(maxRuns); err != nil {
					return err
				}
			} else if err := requireNonNegativeRuntimeBound("max-runs", maxRuns); err != nil {
				return err
			}
			run, err := app.CreateWorkerLifecyclePolicySchedulerRun(args[0], app.WorkerLifecyclePolicySchedulerRunOptions{
				Execute: execute,
				MaxRuns: maxRuns,
				Now:     time.Now().UTC(),
				Actor:   actor,
				Reason:  reason,
			})
			if err != nil && strings.TrimSpace(run.Path) == "" && strings.TrimSpace(run.Status) == "" {
				return err
			}
			if printErr := printWorkerLifecyclePolicySchedulerRun(out, run); printErr != nil {
				return printErr
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&execute, "execute", false, "execute a finite number of selected policy-run slices")
	cmd.Flags().IntVar(&maxRuns, "max-runs", 0, "maximum selected policy-run slices to create; required with --execute")
	cmd.Flags().StringVar(&actor, "actor", "", "operator identity recorded on the scheduler-run artifact")
	cmd.Flags().StringVar(&reason, "reason", "", "operator-visible reason for the scheduler run")
	return cmd
}

func newRuntimeWorkerLifecyclePolicySchedulerRunsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-lifecycle-policy-scheduler-runs <workspace> [artifact]",
		Short: "List or inspect worker lifecycle policy scheduler-run artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				run, err := app.ReadWorkerLifecyclePolicySchedulerRun(args[0], args[1])
				if err != nil {
					return err
				}
				return printWorkerLifecyclePolicySchedulerRun(out, run)
			}
			runs, err := app.ListWorkerLifecyclePolicySchedulerRuns(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-lifecycle-policy-scheduler-runs: total=%d\n", len(runs)); err != nil {
				return err
			}
			for _, run := range runs {
				if err := printWorkerLifecyclePolicySchedulerRun(out, run); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRuntimeWorkerLifecyclePoliciesCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-lifecycle-policies <workspace> [artifact]",
		Short: "List or inspect worker lifecycle policy artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				policy, err := app.ReadWorkerLifecyclePolicy(args[0], args[1])
				if err != nil {
					return err
				}
				return printWorkerLifecyclePolicy(out, policy)
			}
			policies, err := app.ListWorkerLifecyclePolicies(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-lifecycle-policies: total=%d\n", len(policies)); err != nil {
				return err
			}
			for _, policy := range policies {
				if err := printWorkerLifecyclePolicy(out, policy); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRuntimeWorkerLifecyclePolicySelectCommand(out io.Writer) *cobra.Command {
	var selectedBy string
	var selectedAtArg string
	var reason string
	cmd := &cobra.Command{
		Use:   "worker-lifecycle-policy-select <workspace> <artifact>",
		Short: "Mark a worker lifecycle policy as the current metadata-only selection",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var selectedAt time.Time
			if strings.TrimSpace(selectedAtArg) != "" {
				parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(selectedAtArg))
				if err != nil {
					return fmt.Errorf("selected-at must be RFC3339: %w", err)
				}
				selectedAt = parsed
			}
			selection, err := app.SelectWorkerLifecyclePolicy(args[0], args[1], app.WorkerLifecyclePolicySelectionOptions{
				SelectedBy: selectedBy,
				SelectedAt: selectedAt,
				Reason:     reason,
			})
			if err != nil {
				return err
			}
			return printWorkerLifecyclePolicySelection(out, selection)
		},
	}
	cmd.Flags().StringVar(&selectedBy, "selected-by", "", "operator identity; defaults to operator")
	cmd.Flags().StringVar(&selectedAtArg, "selected-at", "", "selection timestamp in RFC3339; defaults to now")
	cmd.Flags().StringVar(&reason, "reason", "", "selection reason")
	return cmd
}

func newRuntimeWorkerLifecyclePolicySelectionCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-lifecycle-policy-selection <workspace>",
		Short: "Inspect the current worker lifecycle policy selection marker",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			selection, found, err := app.ReadWorkerLifecyclePolicySelection(args[0])
			if err != nil {
				return err
			}
			if !found {
				_, err := fmt.Fprintln(out, "worker-lifecycle-policy-selection: none")
				return err
			}
			return printWorkerLifecyclePolicySelection(out, selection)
		},
	}
}

func newRuntimeWorkerLifecyclePolicySelectionClearCommand(out io.Writer) *cobra.Command {
	var clearedBy string
	var clearedAtArg string
	var reason string
	cmd := &cobra.Command{
		Use:   "worker-lifecycle-policy-selection-clear <workspace>",
		Short: "Clear the current worker lifecycle policy selection marker",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var clearedAt time.Time
			if strings.TrimSpace(clearedAtArg) != "" {
				parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(clearedAtArg))
				if err != nil {
					return fmt.Errorf("cleared-at must be RFC3339: %w", err)
				}
				clearedAt = parsed
			}
			selection, err := app.ClearWorkerLifecyclePolicySelection(args[0], app.WorkerLifecyclePolicySelectionClearOptions{
				ClearedBy: clearedBy,
				ClearedAt: clearedAt,
				Reason:    reason,
			})
			if err != nil {
				return err
			}
			return printWorkerLifecyclePolicySelection(out, selection)
		},
	}
	cmd.Flags().StringVar(&clearedBy, "cleared-by", "", "operator identity; defaults to operator")
	cmd.Flags().StringVar(&clearedAtArg, "cleared-at", "", "clear timestamp in RFC3339; defaults to now")
	cmd.Flags().StringVar(&reason, "reason", "", "clear reason")
	return cmd
}

func newRuntimeWorkerLeaseResultReconcileCommand(out io.Writer) *cobra.Command {
	var write bool
	cmd := &cobra.Command{
		Use:   "worker-lease-result-reconcile <workspace>",
		Short: "Show metadata-only worker lease and queue result reconciliation candidates",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if write {
				dispatch, err := app.CreateWorkerLeaseResultReconciliationDispatch(args[0], time.Now().UTC())
				if err != nil {
					return err
				}
				return printWorkerLeaseResultReconciliationDispatch(out, dispatch)
			}
			plan, err := app.PlanWorkerLeaseResultReconciliation(args[0])
			if err != nil {
				return err
			}
			return printWorkerLeaseResultReconciliationPlan(out, plan)
		},
	}
	cmd.Flags().BoolVar(&write, "write", false, "write a dry-run metadata-only reconciliation artifact")
	return cmd
}

func newRuntimeWorkerLeaseResultReconciliationsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-lease-result-reconciliations <workspace> [artifact]",
		Short: "List or inspect worker lease/result reconciliation artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				dispatch, err := app.ReadWorkerLeaseResultReconciliationDispatch(args[0], args[1])
				if err != nil {
					return err
				}
				return printWorkerLeaseResultReconciliationDispatch(out, dispatch)
			}
			dispatches, err := app.ListWorkerLeaseResultReconciliationDispatches(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-lease-result-reconciliations: total=%d\n", len(dispatches)); err != nil {
				return err
			}
			for _, dispatch := range dispatches {
				if err := printWorkerLeaseResultReconciliationDispatch(out, dispatch); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRuntimeExecuteWorkerLeaseResultReconciliationCommand(out io.Writer) *cobra.Command {
	var execute bool
	var releasedBy string
	var reason string
	cmd := &cobra.Command{
		Use:   "execute-worker-lease-result-reconciliation <workspace> <artifact>",
		Short: "Release active worker leases from a persisted reconciliation artifact with an explicit gate",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			execution, err := app.ExecuteWorkerLeaseResultReconciliationDispatch(args[0], args[1], app.WorkerLeaseResultReconciliationExecutionOptions{
				Execute:    execute,
				Now:        time.Now().UTC(),
				ReleasedBy: releasedBy,
				Reason:     reason,
			})
			if err != nil {
				return err
			}
			return printWorkerLeaseResultReconciliationExecution(out, execution)
		},
	}
	cmd.Flags().BoolVar(&execute, "execute", false, "execute the reconciliation release audit")
	cmd.Flags().StringVar(&releasedBy, "released-by", "", "identity that released the worker lease")
	cmd.Flags().StringVar(&reason, "reason", "", "operator-visible reason for releasing the worker lease")
	return cmd
}

func newRuntimeWorkerLeaseResultReconciliationExecutionsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-lease-result-reconciliation-executions <workspace> [execution]",
		Short: "List or inspect worker lease/result reconciliation execution audits",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				execution, err := app.ReadWorkerLeaseResultReconciliationExecution(args[0], args[1])
				if err != nil {
					return err
				}
				return printWorkerLeaseResultReconciliationExecution(out, execution)
			}
			executions, err := app.ListWorkerLeaseResultReconciliationExecutions(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-lease-result-reconciliation-executions: total=%d\n", len(executions)); err != nil {
				return err
			}
			for _, execution := range executions {
				if err := printWorkerLeaseResultReconciliationExecution(out, execution); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRuntimeWorkerLeaseReclaimsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-lease-reclaims <workspace> [dispatch]",
		Short: "List or inspect stale worker lease reclaim artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				dispatch, err := app.ReadWorkerLeaseReclaimDispatch(args[0], args[1])
				if err != nil {
					return err
				}
				return printWorkerLeaseReclaimDispatch(out, dispatch)
			}
			dispatches, err := app.ListWorkerLeaseReclaimDispatches(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-lease-reclaims: total=%d\n", len(dispatches)); err != nil {
				return err
			}
			for _, dispatch := range dispatches {
				if err := printWorkerLeaseReclaimDispatch(out, dispatch); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRuntimeExecuteWorkerLeaseReclaimCommand(out io.Writer) *cobra.Command {
	var execute bool
	cmd := &cobra.Command{
		Use:   "execute-worker-lease-reclaim <workspace> <dispatch>",
		Short: "Execute a stale worker lease reclaim dispatch with an explicit gate",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			execution, err := app.ExecuteWorkerLeaseReclaimDispatch(args[0], args[1], app.WorkerLeaseReclaimExecutionOptions{Execute: execute})
			if err != nil {
				return err
			}
			return printWorkerLeaseReclaimExecution(out, execution)
		},
	}
	cmd.Flags().BoolVar(&execute, "execute", false, "execute the reclaim dispatch")
	return cmd
}

func newRuntimeWorkerLeaseReclaimExecutionsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "worker-lease-reclaim-executions <workspace> [execution]",
		Short: "List or inspect worker lease reclaim execution audit artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				execution, err := app.ReadWorkerLeaseReclaimExecution(args[0], args[1])
				if err != nil {
					return err
				}
				return printWorkerLeaseReclaimExecution(out, execution)
			}
			executions, err := app.ListWorkerLeaseReclaimExecutions(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "worker-lease-reclaim-executions: total=%d\n", len(executions)); err != nil {
				return err
			}
			for _, execution := range executions {
				if err := printWorkerLeaseReclaimExecution(out, execution); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func printRuntimeWorker(out io.Writer, worker app.RuntimeWorkerArtifact) error {
	if _, err := fmt.Fprintf(
		out,
		"runtime-worker: %s worker=%s mode=%s status=%s capabilities=%d endpoint=%s\n",
		worker.Path,
		worker.WorkerID,
		worker.Mode,
		worker.Status,
		len(worker.Capabilities),
		worker.Endpoint,
	); err != nil {
		return err
	}
	for _, capability := range worker.Capabilities {
		if _, err := fmt.Fprintf(out, "runtime-worker-capability: worker=%s capability=%s\n", worker.WorkerID, capability); err != nil {
			return err
		}
	}
	for _, key := range sortedRuntimeWorkerLabelKeys(worker.Labels) {
		if _, err := fmt.Fprintf(out, "runtime-worker-label: worker=%s %s=%s\n", worker.WorkerID, key, worker.Labels[key]); err != nil {
			return err
		}
	}
	return nil
}

func printRuntimeWorkerPlacementPlan(out io.Writer, plan app.RuntimeWorkerPlacementPlan) error {
	if _, err := fmt.Fprintf(
		out,
		"worker-placement-plan: %s plan=%s status=%s mode=%s run=%s ready=%d skipped=%d\n",
		plan.Path,
		plan.PlanID,
		plan.Status,
		plan.Mode,
		plan.RunID,
		plan.ReadyCount,
		plan.SkippedCount,
	); err != nil {
		return err
	}
	for _, candidate := range plan.Candidates {
		if _, err := fmt.Fprintf(
			out,
			"worker-placement: status=%s reason=%s mode=%s worker=%s worker_mode=%s worker_status=%s planned=%s run=%s execution=%s tool=%s capability=%s endpoint=%s\n",
			candidate.Status,
			candidate.Reason,
			candidate.Mode,
			candidate.WorkerID,
			candidate.WorkerMode,
			candidate.WorkerStatus,
			candidate.PlannedWorkerID,
			candidate.RunID,
			candidate.ExecutionID,
			candidate.Tool,
			candidate.Capability,
			candidate.WorkerEndpoint,
		); err != nil {
			return err
		}
		if err := printRuntimeWorkerToolPlan(out, "worker-placement-tool-plan", candidate.ToolPlan); err != nil {
			return err
		}
	}
	return nil
}

func printRuntimeWorkerPlacementClaimExecution(out io.Writer, execution app.RuntimeWorkerPlacementClaimExecution) error {
	if _, err := fmt.Fprintf(
		out,
		"worker-placement-claim-execution: %s status=%s mode=%s plan=%s claim=%s worker=%s run=%s execution=%s tool=%s capability=%s skip_reason=%s error=%s\n",
		execution.Path,
		execution.Status,
		execution.Mode,
		execution.PlacementPlanPath,
		execution.ClaimPath,
		execution.WorkerID,
		execution.RunID,
		execution.ExecutionID,
		execution.Tool,
		execution.Capability,
		execution.SkipReason,
		execution.Error,
	); err != nil {
		return err
	}
	if err := printRuntimeWorkerToolPlan(out, "worker-placement-claim-tool-plan", execution.ToolPlan); err != nil {
		return err
	}
	return nil
}

func printRuntimeWorkerPlacementDispatchPlan(out io.Writer, plan app.RuntimeWorkerPlacementDispatchPlan) error {
	if _, err := fmt.Fprintf(
		out,
		"worker-placement-dispatch-plan: %s plan=%s status=%s mode=%s transport=%s ready=%d skipped=%d\n",
		plan.Path,
		plan.PlanID,
		plan.Status,
		plan.Mode,
		plan.TransportMode,
		plan.ReadyCount,
		plan.SkippedCount,
	); err != nil {
		return err
	}
	for _, candidate := range plan.Candidates {
		if _, err := fmt.Fprintf(
			out,
			"worker-placement-dispatch: status=%s reason=%s mode=%s transport=%s placement_claim=%s claim=%s worker=%s run=%s execution=%s tool=%s capability=%s endpoint=%s\n",
			candidate.Status,
			candidate.Reason,
			candidate.Mode,
			candidate.TransportMode,
			candidate.PlacementClaimExecutionPath,
			candidate.ClaimPath,
			candidate.WorkerID,
			candidate.RunID,
			candidate.ExecutionID,
			candidate.Tool,
			candidate.Capability,
			candidate.WorkerEndpoint,
		); err != nil {
			return err
		}
		if err := printRuntimeWorkerToolPlan(out, "worker-placement-dispatch-tool-plan", candidate.ToolPlan); err != nil {
			return err
		}
	}
	return nil
}

func printRuntimeWorkerPlacementDispatchExecution(out io.Writer, execution app.RuntimeWorkerPlacementDispatchExecution) error {
	if _, err := fmt.Fprintf(
		out,
		"worker-placement-dispatch-execution: %s status=%s mode=%s transport=%s plan=%s placement_claim=%s claim=%s queue_dispatch=%s worker=%s run=%s execution=%s tool=%s capability=%s transport_request=%s skip_reason=%s error=%s\n",
		execution.Path,
		execution.Status,
		execution.Mode,
		execution.TransportMode,
		execution.PlacementDispatchPlanPath,
		execution.PlacementClaimExecutionPath,
		execution.ClaimPath,
		execution.QueueDispatchExecutionPath,
		execution.WorkerID,
		execution.RunID,
		execution.ExecutionID,
		execution.Tool,
		execution.Capability,
		execution.TransportRequestPath,
		execution.SkipReason,
		execution.Error,
	); err != nil {
		return err
	}
	if err := printRuntimeWorkerToolPlan(out, "worker-placement-dispatch-execution-tool-plan", execution.ToolPlan); err != nil {
		return err
	}
	return nil
}

func printRuntimeWorkerPlacementDispatchBatch(out io.Writer, batch app.RuntimeWorkerPlacementDispatchBatch) error {
	if _, err := fmt.Fprintf(
		out,
		"worker-placement-dispatch-batch: %s status=%s mode=%s transport=%s plan=%s total=%d dispatched=%d skipped=%d failed=%d max_candidates=%d error=%s\n",
		batch.Path,
		batch.Status,
		batch.Mode,
		batch.TransportMode,
		batch.PlacementDispatchPlanPath,
		batch.TotalCount,
		batch.DispatchedCount,
		batch.SkippedCount,
		batch.FailedCount,
		batch.MaxCandidates,
		batch.Error,
	); err != nil {
		return err
	}
	for _, execution := range batch.Executions {
		if err := printRuntimeWorkerPlacementDispatchExecution(out, execution); err != nil {
			return err
		}
	}
	return nil
}

func printRuntimeWorkerOrchestration(out io.Writer, orchestration app.RuntimeWorkerOrchestration) error {
	if _, err := fmt.Fprintf(
		out,
		"worker-orchestration: %s status=%s mode=%s run=%s transport=%s placement_ready=%d placement_skipped=%d claimed=%d claim_skipped=%d claim_failed=%d dispatch_ready=%d dispatch_skipped=%d dispatched=%d failed=%d max_candidates=%d retry_dispatch_executions_created=%d retry_dispatch_claims_created=%d retry_dispatch_claims_dispatched=%d retry_dispatch_transport_requests_created=%d dispatch_lifecycle_retry_claims=%t lifecycle_retry_transport=%s\n",
		orchestration.Path,
		orchestration.Status,
		orchestration.Mode,
		orchestration.RunID,
		orchestration.TransportMode,
		orchestration.PlacementReady,
		orchestration.PlacementSkipped,
		orchestration.ClaimedCount,
		orchestration.ClaimSkipped,
		orchestration.ClaimFailed,
		orchestration.DispatchReady,
		orchestration.DispatchSkipped,
		orchestration.DispatchedCount,
		orchestration.DispatchFailed,
		orchestration.MaxCandidates,
		orchestration.RetryDispatchExecutionsCreated,
		orchestration.RetryDispatchClaimsCreated,
		orchestration.RetryDispatchClaimsDispatched,
		orchestration.RetryDispatchTransportRequestsCreated,
		orchestration.DispatchLifecycleRetryClaims,
		orchestration.LifecycleRetryTransportMode,
	); err != nil {
		return err
	}
	if strings.TrimSpace(orchestration.PlacementPlanPath) != "" {
		if _, err := fmt.Fprintf(out, "placement-plan=%s\n", orchestration.PlacementPlanPath); err != nil {
			return err
		}
	}
	for _, path := range orchestration.ClaimExecutionPaths {
		if _, err := fmt.Fprintf(out, "placement-claim-execution=%s\n", path); err != nil {
			return err
		}
	}
	if strings.TrimSpace(orchestration.PlacementDispatchPlanPath) != "" {
		if _, err := fmt.Fprintf(out, "placement-dispatch-plan=%s ready=%d skipped=%d\n", orchestration.PlacementDispatchPlanPath, orchestration.DispatchReady, orchestration.DispatchSkipped); err != nil {
			return err
		}
	}
	if strings.TrimSpace(orchestration.PlacementDispatchBatchPath) != "" {
		if _, err := fmt.Fprintf(out, "placement-dispatch-batch=%s status=%s dispatched=%d skipped=%d failed=%d\n", orchestration.PlacementDispatchBatchPath, orchestration.PlacementDispatchBatchState, orchestration.DispatchedCount, orchestration.DispatchSkipped, orchestration.DispatchFailed); err != nil {
			return err
		}
	}
	if strings.TrimSpace(orchestration.LifecycleMaintenancePath) != "" {
		if _, err := fmt.Fprintf(out, "lifecycle-maintenance=%s\n", orchestration.LifecycleMaintenancePath); err != nil {
			return err
		}
	}
	if strings.TrimSpace(orchestration.LifecycleAuditPath) != "" {
		if _, err := fmt.Fprintf(out, "lifecycle-audit=%s\n", orchestration.LifecycleAuditPath); err != nil {
			return err
		}
	}
	for _, path := range orchestration.RetryDispatchExecutionPaths {
		if _, err := fmt.Fprintf(out, "worker-orchestration-retry-dispatch-execution: %s\n", path); err != nil {
			return err
		}
	}
	if strings.TrimSpace(orchestration.Error) != "" {
		_, err := fmt.Fprintf(out, "error: %s\n", strings.Join(strings.Fields(orchestration.Error), " "))
		return err
	}
	return nil
}

func printRuntimeWorkerOrchestrationSupervisor(out io.Writer, supervisor app.RuntimeWorkerOrchestrationSupervisor) error {
	summary := supervisor.Summary
	prefix := "worker-orchestration-supervisor:"
	pathPart := ""
	if strings.TrimSpace(supervisor.Path) != "" {
		prefix = "worker-orchestration-supervisor-artifact:"
		pathPart = supervisor.Path + " "
	}
	if _, err := fmt.Fprintf(
		out,
		"%s %sstatus=%s mode=%s transport=%s runs=%d max_runs=%d orchestrations_created=%d claimed=%d dispatched=%d skipped=%d failed=%d max_candidates=%d lifecycle_maintenance=%d lifecycle_audit=%d retry_dispatch_executions_created=%d retry_dispatch_claims_created=%d retry_dispatch_claims_dispatched=%d retry_dispatch_transport_requests_created=%d dispatch_lifecycle_retry_claims=%t lifecycle_retry_transport=%s\n",
		prefix,
		pathPart,
		supervisor.Status,
		supervisor.Mode,
		supervisor.TransportMode,
		summary.Runs,
		summary.MaxRuns,
		summary.OrchestrationArtifactsCreated,
		summary.ClaimedCount,
		summary.DispatchedCount,
		summary.SkippedCount,
		summary.FailedCount,
		supervisor.MaxCandidates,
		summary.LifecycleMaintenanceArtifactsCreated,
		summary.LifecycleAuditArtifactsCreated,
		summary.RetryDispatchExecutionsCreated,
		summary.RetryDispatchClaimsCreated,
		summary.RetryDispatchClaimsDispatched,
		summary.RetryDispatchTransportRequestsCreated,
		supervisor.DispatchLifecycleRetryClaims,
		supervisor.LifecycleRetryTransportMode,
	); err != nil {
		return err
	}
	for _, path := range supervisor.OrchestrationPaths {
		if _, err := fmt.Fprintf(out, "worker-orchestration-supervisor-orchestration: %s\n", path); err != nil {
			return err
		}
	}
	for _, path := range supervisor.RetryDispatchExecutionPaths {
		if _, err := fmt.Fprintf(out, "worker-orchestration-supervisor-retry-dispatch-execution: %s\n", path); err != nil {
			return err
		}
	}
	if strings.TrimSpace(supervisor.Error) != "" {
		if _, err := fmt.Fprintf(out, "worker-orchestration-supervisor-error: %s\n", supervisor.Error); err != nil {
			return err
		}
	}
	return nil
}

func printRuntimeWorkerQueueClaim(out io.Writer, claim app.RuntimeWorkerQueueClaim) error {
	if _, err := fmt.Fprintf(
		out,
		"worker-queue-claim: %s claim=%s status=%s mode=%s worker=%s worker_mode=%s run=%s execution=%s tool=%s capability=%s lease_seconds=%d expires=%s renewed_at=%s renewed_by=%s renew_reason=%s canceled_at=%s canceled_by=%s cancel_reason=%s expired_at=%s expired_by=%s expire_reason=%s\n",
		claim.Path,
		claim.ClaimID,
		claim.Status,
		claim.Mode,
		claim.WorkerID,
		claim.WorkerMode,
		claim.RunID,
		claim.ExecutionID,
		claim.Tool,
		claim.Capability,
		claim.LeaseSeconds,
		claim.ExpiresAt,
		claim.RenewedAt,
		claim.RenewedBy,
		claim.RenewReason,
		claim.CanceledAt,
		claim.CanceledBy,
		claim.CancelReason,
		claim.ExpiredAt,
		claim.ExpiredBy,
		claim.ExpireReason,
	); err != nil {
		return err
	}
	if err := printRuntimeWorkerToolPlan(out, "worker-queue-claim-tool-plan", claim.ToolPlan); err != nil {
		return err
	}
	return nil
}

func printRuntimeWorkerQueueDispatchExecution(out io.Writer, execution app.RuntimeWorkerQueueDispatchExecution) error {
	if _, err := fmt.Fprintf(
		out,
		"worker-queue-dispatch-execution: %s status=%s mode=%s transport=%s transport_auth=%s transport_request=%s transport_endpoint=%s transport_http_status=%d transport_response=%s result_callback=%s claim=%s worker=%s run=%s execution=%s tool=%s capability=%s skip_reason=%s error=%s\n",
		execution.Path,
		execution.Status,
		execution.Mode,
		execution.TransportMode,
		execution.TransportAuth,
		execution.TransportRequestPath,
		execution.TransportEndpoint,
		execution.TransportHTTPStatus,
		execution.TransportResponseBody,
		execution.ResultCallbackEndpoint,
		execution.ClaimPath,
		execution.WorkerID,
		execution.RunID,
		execution.ExecutionID,
		execution.Tool,
		execution.Capability,
		execution.SkipReason,
		execution.Error,
	); err != nil {
		return err
	}
	if err := printRuntimeWorkerToolPlan(out, "worker-queue-dispatch-tool-plan", execution.ToolPlan); err != nil {
		return err
	}
	return nil
}

func printRuntimeWorkerTransportDispatchRequest(out io.Writer, request app.RuntimeWorkerTransportDispatchRequest) error {
	if _, err := fmt.Fprintf(
		out,
		"worker-transport-request: %s status=%s transport=%s worker=%s run=%s execution=%s tool=%s capability=%s dispatch=%s claim=%s acked_by=%s acked_at=%s result_status=%s result=%s\n",
		request.Path,
		request.Status,
		request.TransportMode,
		request.WorkerID,
		request.RunID,
		request.ExecutionID,
		request.Tool,
		request.Capability,
		request.DispatchExecutionPath,
		request.ClaimPath,
		request.AckedBy,
		request.AckedAt,
		request.ResultStatus,
		request.ResultPath,
	); err != nil {
		return err
	}
	if err := printRuntimeWorkerToolPlan(out, "worker-transport-request-tool-plan", request.ToolPlan); err != nil {
		return err
	}
	return nil
}

func printRuntimeWorkerToolPlan(out io.Writer, label string, payload app.RuntimeWorkerToolPlanPayload) error {
	if payload.IsZero() {
		return nil
	}
	_, err := fmt.Fprintf(
		out,
		"%s: action=%s manifest=%s runner=%s parser=%s run_timeout_ms=%d run_command=%s input_nodes=%s expected_evidence=%s\n",
		label,
		payload.Action,
		payload.ManifestName,
		payload.Runner,
		payload.Parser,
		payload.RunTimeoutMS,
		strings.Join(payload.RunCommand, " "),
		strings.Join(payload.InputNodeIDs, " "),
		strings.Join(payload.ExpectedEvidence, " "),
	)
	return err
}

func printRuntimeWorkerTransportRunLoopResult(out io.Writer, loop app.RuntimeWorkerTransportRunLoopResult) error {
	if _, err := fmt.Fprintf(
		out,
		"worker-transport-run-loop: worker=%s status=%s processed=%d completed=%d failed=%d skipped=%d\n",
		loop.WorkerID,
		loop.Status,
		loop.Processed,
		loop.Completed,
		loop.Failed,
		loop.Skipped,
	); err != nil {
		return err
	}
	for _, skipped := range loop.SkippedRequests {
		if _, err := fmt.Fprintf(
			out,
			"worker-transport-run-loop-skip: request=%s worker=%s execution=%s status=%s reason=%s\n",
			skipped.RequestPath,
			skipped.WorkerID,
			skipped.ExecutionID,
			skipped.Status,
			skipped.Reason,
		); err != nil {
			return err
		}
	}
	for _, result := range loop.Results {
		if err := printRuntimeWorkerQueueDispatchResult(out, result); err != nil {
			return err
		}
	}
	return nil
}

func printRuntimeWorkerTransportRunDaemonResult(out io.Writer, daemon app.RuntimeWorkerTransportRunDaemonResult) error {
	if _, err := fmt.Fprintf(
		out,
		"worker-transport-run-daemon: worker=%s status=%s processed=%d completed=%d failed=%d skipped=%d scans=%d idle_rounds=%d artifact=%s\n",
		daemon.WorkerID,
		daemon.Status,
		daemon.Processed,
		daemon.Completed,
		daemon.Failed,
		daemon.Skipped,
		daemon.Scans,
		daemon.IdleScans,
		daemon.Path,
	); err != nil {
		return err
	}
	if strings.TrimSpace(daemon.ControlAction) != "" || strings.TrimSpace(daemon.ControlRequestPath) != "" || strings.TrimSpace(daemon.StopReason) != "" {
		if _, err := fmt.Fprintf(
			out,
			"worker-transport-run-daemon-control: control=%s control_request=%s requested_by=%s stop_reason=%s\n",
			daemon.ControlAction,
			daemon.ControlRequestPath,
			daemon.ControlRequestedBy,
			daemon.StopReason,
		); err != nil {
			return err
		}
	}
	for _, skipped := range daemon.SkippedRequests {
		if _, err := fmt.Fprintf(
			out,
			"worker-transport-run-daemon-skip: request=%s worker=%s execution=%s status=%s reason=%s\n",
			skipped.RequestPath,
			skipped.WorkerID,
			skipped.ExecutionID,
			skipped.Status,
			skipped.Reason,
		); err != nil {
			return err
		}
	}
	for _, result := range daemon.Results {
		if err := printRuntimeWorkerQueueDispatchResult(out, result); err != nil {
			return err
		}
	}
	return nil
}

func printRuntimeWorkerTransportDaemonRun(out io.Writer, daemon app.RuntimeWorkerTransportRunDaemonResult) error {
	_, err := fmt.Fprintf(
		out,
		"worker-transport-daemon-run: %s worker=%s status=%s mode=%s source=%s processed=%d completed=%d failed=%d skipped=%d scans=%d idle_rounds=%d control=%s control_request=%s stop_reason=%s started_at=%s completed_at=%s error=%s\n",
		daemon.Path,
		daemon.WorkerID,
		daemon.Status,
		daemon.Mode,
		daemon.CommandSource,
		daemon.Processed,
		daemon.Completed,
		daemon.Failed,
		daemon.Skipped,
		daemon.Scans,
		daemon.IdleScans,
		daemon.ControlAction,
		daemon.ControlRequestPath,
		daemon.StopReason,
		daemon.StartedAt,
		daemon.CompletedAt,
		daemon.Error,
	)
	return err
}

func printRuntimeWorkerTransportRunSupervisorResult(out io.Writer, supervisor app.RuntimeWorkerTransportDaemonSupervisorRun) error {
	if _, err := fmt.Fprintf(
		out,
		"worker-transport-run-supervisor: worker=%s status=%s runs=%d restarts=%d processed=%d completed=%d failed=%d skipped=%d artifact=%s\n",
		supervisor.WorkerID,
		supervisor.Status,
		supervisor.Runs,
		supervisor.RestartCount,
		supervisor.Processed,
		supervisor.Completed,
		supervisor.Failed,
		supervisor.Skipped,
		supervisor.Path,
	); err != nil {
		return err
	}
	for _, daemon := range supervisor.DaemonRuns {
		if _, err := fmt.Fprintf(
			out,
			"worker-transport-run-supervisor-daemon: run=%s worker=%s status=%s processed=%d completed=%d failed=%d skipped=%d control=%s control_request=%s\n",
			daemon.Path,
			daemon.WorkerID,
			daemon.Status,
			daemon.Processed,
			daemon.Completed,
			daemon.Failed,
			daemon.Skipped,
			daemon.ControlAction,
			daemon.ControlRequestPath,
		); err != nil {
			return err
		}
		for _, result := range daemon.Results {
			if err := printRuntimeWorkerQueueDispatchResult(out, result); err != nil {
				return err
			}
		}
	}
	return nil
}

func printRuntimeWorkerTransportSupervisorRun(out io.Writer, supervisor app.RuntimeWorkerTransportDaemonSupervisorRun) error {
	_, err := fmt.Fprintf(
		out,
		"worker-transport-supervisor-run: %s worker=%s status=%s mode=%s source=%s runs=%d restarts=%d processed=%d completed=%d failed=%d skipped=%d max_runs=%d control_enabled=%t started_at=%s completed_at=%s error=%s\n",
		supervisor.Path,
		supervisor.WorkerID,
		supervisor.Status,
		supervisor.Mode,
		supervisor.CommandSource,
		supervisor.Runs,
		supervisor.RestartCount,
		supervisor.Processed,
		supervisor.Completed,
		supervisor.Failed,
		supervisor.Skipped,
		supervisor.MaxRuns,
		supervisor.EnableControlRequests,
		supervisor.StartedAt,
		supervisor.CompletedAt,
		supervisor.Error,
	)
	return err
}

func printRuntimeWorkerTransportDaemonControlRequest(out io.Writer, control app.RuntimeWorkerTransportDaemonControlRequest) error {
	_, err := fmt.Fprintf(
		out,
		"worker-transport-daemon-control: %s worker=%s status=%s mode=%s action=%s requested_at=%s requested_by=%s reason=%s\n",
		control.Path,
		control.WorkerID,
		control.Status,
		control.Mode,
		control.Action,
		control.RequestedAt,
		control.RequestedBy,
		control.Reason,
	)
	return err
}

func printRuntimeWorkerQueueDispatchResult(out io.Writer, result app.RuntimeWorkerQueueDispatchResult) error {
	if _, err := fmt.Fprintf(
		out,
		"worker-queue-dispatch-result: %s status=%s mode=%s dispatch=%s worker=%s run=%s execution=%s tool=%s capability=%s output=%s error=%s\n",
		result.Path,
		result.Status,
		result.Mode,
		result.DispatchExecutionPath,
		result.WorkerID,
		result.RunID,
		result.ExecutionID,
		result.Tool,
		result.Capability,
		result.Output,
		result.Error,
	); err != nil {
		return err
	}
	return nil
}

func printRuntimeWorkerDispatchRetryPlanSummary(out io.Writer, plan app.RuntimeWorkerDispatchRetryPlan) error {
	if _, err := fmt.Fprintf(
		out,
		"worker-dispatch-retry-plan: total=%d ready=%d skipped=%d\n",
		plan.Summary.Total,
		plan.Summary.Ready,
		plan.Summary.Skipped,
	); err != nil {
		return err
	}
	return printRuntimeWorkerDispatchRetryPlanItems(out, plan)
}

func printRuntimeWorkerDispatchRetryPlan(out io.Writer, plan app.RuntimeWorkerDispatchRetryPlan) error {
	if _, err := fmt.Fprintf(
		out,
		"worker-dispatch-retry-plan-artifact: %s status=%s mode=%s ready=%d skipped=%d created_at=%s\n",
		plan.Path,
		plan.Status,
		plan.Mode,
		plan.Summary.Ready,
		plan.Summary.Skipped,
		plan.CreatedAt,
	); err != nil {
		return err
	}
	return printRuntimeWorkerDispatchRetryPlanItems(out, plan)
}

func printRuntimeWorkerDispatchRetryPlanItems(out io.Writer, plan app.RuntimeWorkerDispatchRetryPlan) error {
	for _, candidate := range plan.Ready {
		if _, err := fmt.Fprintf(
			out,
			"retry-worker-dispatch: %s dispatch=%s claim=%s request=%s gate=%s mode=%s worker=%s run=%s execution=%s tool=%s capability=%s status=%s command=%s output=%s error=%s\n",
			candidate.ResultPath,
			candidate.DispatchPath,
			candidate.ClaimPath,
			candidate.TransportRequestPath,
			candidate.Gate,
			candidate.Mode,
			candidate.WorkerID,
			candidate.RunID,
			candidate.ExecutionID,
			candidate.Tool,
			candidate.Capability,
			candidate.LastStatus,
			formatCommandJSON(candidate.ToolPlan.RunCommand),
			candidate.LastOutput,
			candidate.LastError,
		); err != nil {
			return err
		}
	}
	for _, skipped := range plan.Skipped {
		if _, err := fmt.Fprintf(
			out,
			"skip-worker-dispatch-result: %s dispatch=%s claim=%s worker=%s run=%s execution=%s status=%s reason=%s\n",
			skipped.ResultPath,
			skipped.DispatchPath,
			skipped.ClaimPath,
			skipped.WorkerID,
			skipped.RunID,
			skipped.ExecutionID,
			skipped.Status,
			skipped.Reason,
		); err != nil {
			return err
		}
	}
	return nil
}

func printRuntimeWorkerDispatchRetryDispatch(out io.Writer, dispatch app.RuntimeWorkerDispatchRetryDispatch) error {
	if _, err := fmt.Fprintf(
		out,
		"worker-dispatch-retry-dispatch: %s status=%s mode=%s retry_plan=%s ready=%d skipped=%d created_at=%s\n",
		dispatch.Path,
		dispatch.Status,
		dispatch.Mode,
		dispatch.RetryPlanPath,
		dispatch.Summary.Ready,
		dispatch.Summary.Skipped,
		dispatch.CreatedAt,
	); err != nil {
		return err
	}
	for _, candidate := range dispatch.Ready {
		if _, err := fmt.Fprintf(
			out,
			"retry-worker-dispatch: %s dispatch=%s claim=%s request=%s gate=%s mode=%s worker=%s run=%s execution=%s tool=%s capability=%s status=%s command=%s output=%s error=%s\n",
			candidate.ResultPath,
			candidate.DispatchPath,
			candidate.ClaimPath,
			candidate.TransportRequestPath,
			candidate.Gate,
			candidate.Mode,
			candidate.WorkerID,
			candidate.RunID,
			candidate.ExecutionID,
			candidate.Tool,
			candidate.Capability,
			candidate.LastStatus,
			formatCommandJSON(candidate.ToolPlan.RunCommand),
			candidate.LastOutput,
			candidate.LastError,
		); err != nil {
			return err
		}
	}
	for _, skipped := range dispatch.Skipped {
		if _, err := fmt.Fprintf(
			out,
			"skip-worker-dispatch-result: %s dispatch=%s claim=%s worker=%s run=%s execution=%s status=%s reason=%s\n",
			skipped.ResultPath,
			skipped.DispatchPath,
			skipped.ClaimPath,
			skipped.WorkerID,
			skipped.RunID,
			skipped.ExecutionID,
			skipped.Status,
			skipped.Reason,
		); err != nil {
			return err
		}
	}
	return nil
}

func printRuntimeWorkerDispatchRetryDispatchExecution(out io.Writer, execution app.RuntimeWorkerDispatchRetryDispatchExecution) error {
	if _, err := fmt.Fprintf(
		out,
		"worker-dispatch-retry-dispatch-execution: %s status=%s mode=%s retry_dispatch=%s total=%d ready=%d claimed=%d dispatched=%d skipped=%d failed=%d executed_at=%s\n",
		execution.Path,
		execution.Status,
		execution.Mode,
		execution.RetryDispatchPath,
		execution.Summary.Total,
		execution.Summary.Ready,
		execution.Summary.Claimed,
		execution.Summary.Dispatched,
		execution.Summary.Skipped,
		execution.Summary.Failed,
		execution.ExecutedAt,
	); err != nil {
		return err
	}
	for _, result := range execution.Results {
		if _, err := fmt.Fprintf(
			out,
			"retry-worker-queue-claim: source_result=%s source_dispatch=%s source_claim=%s retry_claim=%s retry_queue_dispatch=%s retry_transport_request=%s transport=%s worker=%s run=%s execution=%s tool=%s capability=%s status=%s reason=%s error=%s\n",
			result.SourceResultPath,
			result.SourceDispatchPath,
			result.SourceClaimPath,
			result.RetryClaimPath,
			result.RetryQueueDispatchPath,
			result.RetryTransportRequestPath,
			result.RetryTransportMode,
			result.WorkerID,
			result.RunID,
			result.ExecutionID,
			result.Tool,
			result.Capability,
			result.Status,
			result.Reason,
			result.Error,
		); err != nil {
			return err
		}
	}
	for _, skipped := range execution.Skipped {
		if _, err := fmt.Fprintf(
			out,
			"skip-worker-dispatch-result: %s dispatch=%s claim=%s worker=%s run=%s execution=%s status=%s reason=%s\n",
			skipped.ResultPath,
			skipped.DispatchPath,
			skipped.ClaimPath,
			skipped.WorkerID,
			skipped.RunID,
			skipped.ExecutionID,
			skipped.Status,
			skipped.Reason,
		); err != nil {
			return err
		}
	}
	return nil
}

func printRuntimeWorkerDispatchRetrySchedulerRun(out io.Writer, run app.RuntimeWorkerDispatchRetrySchedulerRun) error {
	maxRuns := run.MaxRunsRequested
	if maxRuns == 0 {
		maxRuns = run.MaxRuns
	}
	if _, err := fmt.Fprintf(
		out,
		"worker-dispatch-retry-scheduler-run-artifact: %s status=%s mode=%s max_runs=%d runs_created=%d dispatch_retry_claims=%t retry_transport=%s transport=%s current_ready=%d retry_plans_created=%d retry_dispatches_created=%d retry_dispatch_executions_created=%d retry_dispatch_claims_created=%d retry_claims_created=%d retry_dispatch_claims_dispatched=%d retry_claims_dispatched=%d retry_dispatch_transport_requests_created=%d retry_transport_requests_created=%d\n",
		run.Path,
		run.Status,
		run.Mode,
		maxRuns,
		run.RunsCreated,
		run.DispatchRetryClaims,
		printRuntimeDispatchRetryTransport(run.RetryTransportMode),
		printRuntimeDispatchRetryTransport(run.RetryTransportMode),
		run.CurrentSummary.Ready,
		run.RetryPlansCreated,
		run.RetryDispatchesCreated,
		run.RetryDispatchExecutionsCreated,
		run.RetryClaimsCreated,
		run.RetryClaimsCreated,
		run.RetryClaimsDispatched,
		run.RetryClaimsDispatched,
		run.RetryTransportRequestsCreated,
		run.RetryTransportRequestsCreated,
	); err != nil {
		return err
	}
	if strings.TrimSpace(run.SkipReason) != "" {
		if _, err := fmt.Fprintf(out, "worker-dispatch-retry-scheduler-run-skip: %s\n", run.SkipReason); err != nil {
			return err
		}
	}
	if strings.TrimSpace(run.Error) != "" {
		if _, err := fmt.Fprintf(out, "worker-dispatch-retry-scheduler-run-error: %s\n", run.Error); err != nil {
			return err
		}
	}
	return nil
}

func printRuntimeDispatchRetryTransport(transport string) string {
	trimmed := strings.TrimSpace(transport)
	if trimmed == "" {
		return "metadata-only"
	}
	return trimmed
}

func parseRuntimeWorkerLabels(values []string) (map[string]string, error) {
	labels := map[string]string{}
	for _, value := range values {
		key, labelValue, ok := strings.Cut(value, "=")
		if !ok {
			return nil, fmt.Errorf("runtime worker label must be key=value: %s", value)
		}
		key = strings.TrimSpace(key)
		labelValue = strings.TrimSpace(labelValue)
		if key == "" || labelValue == "" {
			return nil, fmt.Errorf("runtime worker label must be key=value: %s", value)
		}
		labels[key] = labelValue
	}
	if len(labels) == 0 {
		return nil, nil
	}
	return labels, nil
}

func sortedRuntimeWorkerLabelKeys(labels map[string]string) []string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func printWorkerLeaseHeartbeat(out io.Writer, lease app.WorkerLeaseArtifact) error {
	_, err := fmt.Fprintf(
		out,
		"worker-lease-heartbeat: %s run=%s execution=%s status=%s worker=%s heartbeat_at=%s heartbeat_by=%s heartbeat_reason=%s\n",
		lease.Path,
		lease.RunID,
		lease.ExecutionID,
		lease.Status,
		lease.WorkerID,
		lease.HeartbeatAt,
		lease.HeartbeatBy,
		lease.HeartbeatReason,
	)
	return err
}

func printWorkerLeaseRelease(out io.Writer, lease app.WorkerLeaseArtifact) error {
	_, err := fmt.Fprintf(
		out,
		"worker-lease-release: %s run=%s execution=%s status=%s worker=%s released_at=%s released_by=%s release_reason=%s result_execution=%s error=%s\n",
		lease.Path,
		lease.RunID,
		lease.ExecutionID,
		lease.Status,
		lease.WorkerID,
		lease.ReleasedAt,
		lease.ReleasedBy,
		lease.ReleaseReason,
		lease.ResultExecutionID,
		lease.Error,
	)
	return err
}

func printWorkerLifecycleAudit(out io.Writer, audit app.WorkerLifecycleAudit) error {
	summary := audit.Summary
	prefix := "worker-lifecycle-audit:"
	pathPart := ""
	if strings.TrimSpace(audit.Path) != "" {
		prefix = "worker-lifecycle-audit-artifact:"
		pathPart = audit.Path + " "
	}
	if _, err := fmt.Fprintf(
		out,
		"%s %sstatus=%s mode=%s workers=%d stale_workers=%d offline_workers=%d placements=%d placement_claims=%d placement_dispatch_plans=%d placement_dispatches=%d placement_batches=%d worker_orchestrations=%d orchestration_supervisors=%d queue_claims=%d dispatches=%d transport_requests=%d transport_pending=%d transport_ack=%d transport_completed=%d transport_failed=%d results=%d retry_plans=%d retry_dispatches=%d retry_dispatch_executions=%d leases=%d active_leases=%d stale_leases=%d reclaims=%d reclaim_executions=%d reconciliations=%d reconciliation_executions=%d lifecycle_reviews=%d action_plans=%d action_plan_annotations=%d action_plan_selections=%d lifecycle_policies=%d policy_selections=%d policy_runs=%d policy_scheduler_runs=%d maintenances=%d maintenance_supervisors=%d actions=%d daemon_runs=%d supervisor_runs=%d controls=%d artifacts=%d\n",
		prefix,
		pathPart,
		audit.Status,
		audit.Mode,
		summary.WorkerRegistrations,
		summary.StaleWorkerRegistrations,
		summary.OfflineWorkerRegistrations,
		summary.PlacementPlans,
		summary.PlacementClaimExecutions,
		summary.PlacementDispatchPlans,
		summary.PlacementDispatchExecutions,
		summary.PlacementDispatchBatches,
		summary.WorkerOrchestrations,
		summary.WorkerOrchestrationSupervisors,
		summary.QueueClaims,
		summary.QueueDispatchExecutions,
		summary.TransportRequests,
		summary.TransportRequestsPending,
		summary.TransportRequestsAcknowledged,
		summary.TransportRequestsCompleted,
		summary.TransportRequestsFailed,
		summary.QueueDispatchResults,
		summary.DispatchRetryPlans,
		summary.DispatchRetryDispatches,
		summary.DispatchRetryDispatchExecutions,
		summary.WorkerLeases,
		summary.ActiveLeases,
		summary.StaleActiveLeases,
		summary.ReclaimReviews,
		summary.ReclaimExecutions,
		summary.ReconciliationReviews,
		summary.ReconciliationExecutions,
		summary.LifecycleReviews,
		summary.ActionPlans,
		summary.ActionPlanAnnotations,
		summary.ActionPlanSelections,
		summary.LifecyclePolicies,
		summary.LifecyclePolicySelections,
		summary.LifecyclePolicyRuns,
		summary.LifecyclePolicySchedulerRuns,
		summary.LifecycleMaintenances,
		summary.LifecycleMaintenanceSupervisors,
		summary.ActionPlanActions,
		summary.DaemonRuns,
		summary.DaemonSupervisorRuns,
		summary.DaemonControls,
		summary.ArtifactTotal,
	); err != nil {
		return err
	}
	for _, item := range audit.Items {
		if _, err := fmt.Fprintf(
			out,
			"worker-lifecycle-audit-item: kind=%s status=%s mode=%s gate=%s worker=%s run=%s execution=%s path=%s linked=%s reason=%s%s\n",
			item.Kind,
			item.Status,
			item.Mode,
			item.Gate,
			item.WorkerID,
			item.RunID,
			item.ExecutionID,
			item.ArtifactPath,
			item.LinkedPath,
			item.Reason,
			workerLifecycleAuditItemAggregateSuffix(item),
		); err != nil {
			return err
		}
	}
	return nil
}

func workerLifecycleAuditItemAggregateSuffix(item app.WorkerLifecycleAuditItem) string {
	parts := make([]string, 0, 10)
	if item.TotalCount > 0 {
		parts = append(parts, fmt.Sprintf("total=%d", item.TotalCount))
	}
	if item.ReadyCount > 0 {
		parts = append(parts, fmt.Sprintf("ready=%d", item.ReadyCount))
	}
	if item.ResultCount > 0 {
		parts = append(parts, fmt.Sprintf("results=%d", item.ResultCount))
	}
	if item.ActionsTotal > 0 {
		parts = append(parts, fmt.Sprintf("actions=%d", item.ActionsTotal))
	}
	if strings.TrimSpace(item.Decision) != "" {
		parts = append(parts, fmt.Sprintf("decision=%s", item.Decision))
	}
	if strings.TrimSpace(item.Reviewer) != "" {
		parts = append(parts, fmt.Sprintf("reviewer=%s", item.Reviewer))
	}
	if strings.TrimSpace(item.ReviewedAt) != "" {
		parts = append(parts, fmt.Sprintf("reviewed_at=%s", item.ReviewedAt))
	}
	if strings.TrimSpace(item.SelectedBy) != "" {
		parts = append(parts, fmt.Sprintf("selected_by=%s", item.SelectedBy))
	}
	if strings.TrimSpace(item.SelectedAt) != "" {
		parts = append(parts, fmt.Sprintf("selected_at=%s", item.SelectedAt))
	}
	if strings.TrimSpace(item.ClearedBy) != "" {
		parts = append(parts, fmt.Sprintf("cleared_by=%s", item.ClearedBy))
	}
	if strings.TrimSpace(item.ClearedAt) != "" {
		parts = append(parts, fmt.Sprintf("cleared_at=%s", item.ClearedAt))
	}
	if item.SupersededCount > 0 {
		parts = append(parts, fmt.Sprintf("superseded=%d", item.SupersededCount))
	}
	if item.AckActions > 0 {
		parts = append(parts, fmt.Sprintf("ack_actions=%d", item.AckActions))
	}
	if item.RunActions > 0 {
		parts = append(parts, fmt.Sprintf("run_actions=%d", item.RunActions))
	}
	if item.DaemonControlActions > 0 {
		parts = append(parts, fmt.Sprintf("daemon_control_actions=%d", item.DaemonControlActions))
	}
	if item.RetryDispatchExecuteActions > 0 {
		parts = append(parts, fmt.Sprintf("retry_dispatch_execute=%d", item.RetryDispatchExecuteActions))
	}
	if item.ReclaimActions > 0 {
		parts = append(parts, fmt.Sprintf("reclaim_actions=%d", item.ReclaimActions))
	}
	if item.ReconciliationActions > 0 {
		parts = append(parts, fmt.Sprintf("reconciliation_actions=%d", item.ReconciliationActions))
	}
	if item.SkippedUnsafeActions > 0 {
		parts = append(parts, fmt.Sprintf("skipped_unsafe=%d", item.SkippedUnsafeActions))
	}
	if item.PlacementReady > 0 {
		parts = append(parts, fmt.Sprintf("placement_ready=%d", item.PlacementReady))
	}
	if item.PlacementSkipped > 0 {
		parts = append(parts, fmt.Sprintf("placement_skipped=%d", item.PlacementSkipped))
	}
	if item.DispatchReady > 0 {
		parts = append(parts, fmt.Sprintf("dispatch_ready=%d", item.DispatchReady))
	}
	if item.DispatchSkipped > 0 {
		parts = append(parts, fmt.Sprintf("dispatch_skipped=%d", item.DispatchSkipped))
	}
	if item.OrchestrationRuns > 0 {
		parts = append(parts, fmt.Sprintf("runs=%d", item.OrchestrationRuns))
	}
	if item.MaxRuns > 0 {
		parts = append(parts, fmt.Sprintf("max_runs=%d", item.MaxRuns))
	}
	if item.OrchestrationsCreated > 0 {
		parts = append(parts, fmt.Sprintf("orchestrations_created=%d", item.OrchestrationsCreated))
	}
	if item.LifecycleMaintenanceCreated > 0 {
		parts = append(parts, fmt.Sprintf("lifecycle_maintenance=%d", item.LifecycleMaintenanceCreated))
	}
	if item.LifecycleAuditCreated > 0 {
		parts = append(parts, fmt.Sprintf("lifecycle_audit=%d", item.LifecycleAuditCreated))
	}
	if item.ClaimedCount > 0 {
		parts = append(parts, fmt.Sprintf("claimed=%d", item.ClaimedCount))
	}
	if item.DispatchedCount > 0 {
		parts = append(parts, fmt.Sprintf("dispatched=%d", item.DispatchedCount))
	}
	if item.SkippedCount > 0 {
		parts = append(parts, fmt.Sprintf("skipped=%d", item.SkippedCount))
	}
	if item.FailedCount > 0 {
		parts = append(parts, fmt.Sprintf("failed=%d", item.FailedCount))
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

func printWorkerLifecycleReviewPlan(out io.Writer, plan app.WorkerLifecycleReviewPlan) error {
	summary := plan.LeaseSummary
	queue := plan.QueueSummary
	transport := plan.TransportSummary
	if _, err := fmt.Fprintf(
		out,
		"worker-lifecycle-review: leases=%d active=%d stale=%d completed=%d failed=%d reclaimed=%d fresh_active=%d other=%d reclaim_ready=%d reclaim_skipped=%d reconciliation_ready=%d reconciliation_skipped=%d queue_claims=%d queue_claimed=%d queue_dispatched=%d queue_completed=%d queue_failed=%d queue_canceled=%d queue_expired=%d transport_requests=%d transport_pending=%d transport_ack=%d transport_completed=%d transport_failed=%d dispatch_results=%d result_completed=%d result_failed=%d mode=%s\n",
		summary.Total,
		summary.Active,
		summary.StaleActive,
		summary.Completed,
		summary.Failed,
		summary.Reclaimed,
		summary.FreshActive,
		summary.Other,
		len(plan.Reclaim.Ready),
		len(plan.Reclaim.Skipped),
		len(plan.Reconciliation.Ready),
		len(plan.Reconciliation.Skipped),
		queue.Total,
		queue.Claimed,
		queue.Dispatched,
		queue.Completed,
		queue.Failed,
		queue.Canceled,
		queue.Expired,
		transport.RequestsTotal,
		transport.Pending,
		transport.Acknowledged,
		transport.Completed,
		transport.Failed,
		transport.DispatchResultsTotal,
		transport.DispatchResultsCompleted,
		transport.DispatchResultsFailed,
		plan.Mode,
	); err != nil {
		return err
	}
	return printWorkerLifecycleReviewDetails(out, plan.Reclaim, plan.Reconciliation)
}

func printWorkerLifecycleReview(out io.Writer, review app.WorkerLifecycleReview) error {
	summary := review.LeaseSummary
	queue := review.QueueSummary
	transport := review.TransportSummary
	if _, err := fmt.Fprintf(
		out,
		"worker-lifecycle-review-artifact: %s status=%s mode=%s leases=%d active=%d stale=%d completed=%d failed=%d reclaimed=%d fresh_active=%d other=%d reclaim_ready=%d reclaim_skipped=%d reconciliation_ready=%d reconciliation_skipped=%d queue_claims=%d queue_claimed=%d queue_dispatched=%d queue_completed=%d queue_failed=%d queue_canceled=%d queue_expired=%d transport_requests=%d transport_pending=%d transport_ack=%d transport_completed=%d transport_failed=%d dispatch_results=%d result_completed=%d result_failed=%d\n",
		review.Path,
		review.Status,
		review.Mode,
		summary.Total,
		summary.Active,
		summary.StaleActive,
		summary.Completed,
		summary.Failed,
		summary.Reclaimed,
		summary.FreshActive,
		summary.Other,
		len(review.Reclaim.Ready),
		len(review.Reclaim.Skipped),
		len(review.Reconciliation.Ready),
		len(review.Reconciliation.Skipped),
		queue.Total,
		queue.Claimed,
		queue.Dispatched,
		queue.Completed,
		queue.Failed,
		queue.Canceled,
		queue.Expired,
		transport.RequestsTotal,
		transport.Pending,
		transport.Acknowledged,
		transport.Completed,
		transport.Failed,
		transport.DispatchResultsTotal,
		transport.DispatchResultsCompleted,
		transport.DispatchResultsFailed,
	); err != nil {
		return err
	}
	return printWorkerLifecycleReviewDetails(out, review.Reclaim, review.Reconciliation)
}

func printWorkerLifecycleActionPlan(out io.Writer, plan app.WorkerLifecycleActionPlan) error {
	summary := plan.Summary
	reviewPath := plan.ReviewPath
	if strings.TrimSpace(reviewPath) == "" {
		reviewPath = "none"
	}
	prefix := "worker-lifecycle-action-plan:"
	pathPart := ""
	if strings.TrimSpace(plan.Path) != "" {
		prefix = "worker-lifecycle-action-plan-artifact:"
		pathPart = plan.Path + " "
	}
	if _, err := fmt.Fprintf(
		out,
		"%s %sstatus=%s mode=%s actions=%d ack_pending=%d run_acknowledged=%d daemon_control=%d retry_dispatch_execute=%d reclaim=%d reconcile=%d review=%s\n",
		prefix,
		pathPart,
		plan.Status,
		plan.Mode,
		summary.Total,
		summary.AckPendingTransport,
		summary.RunAcknowledgedTransport,
		summary.ApplyDaemonControl,
		summary.ExecuteDispatchRetryDispatch,
		summary.ReclaimStaleLease,
		summary.ReconcileResultLease,
		reviewPath,
	); err != nil {
		return err
	}
	for _, action := range plan.Actions {
		if _, err := fmt.Fprintf(
			out,
			"worker-lifecycle-action: type=%s gate=%s worker=%s run=%s execution=%s status=%s mode=%s reason=%s request_path=%s lease_path=%s result_path=%s dispatch=%s recommended=%s\n",
			action.Type,
			action.Gate,
			action.WorkerID,
			action.RunID,
			action.ExecutionID,
			action.Status,
			action.Mode,
			action.Reason,
			action.RequestPath,
			action.LeasePath,
			action.ResultPath,
			action.DispatchPath,
			action.RecommendedStatus,
		); err != nil {
			return err
		}
	}
	return printWorkerLifecycleActionPlanAnnotation(out, plan.Annotation)
}

func printWorkerLifecycleActionPlanAnnotation(out io.Writer, annotation *app.WorkerLifecycleActionPlanAnnotation) error {
	if annotation == nil {
		return nil
	}
	_, err := fmt.Fprintf(
		out,
		"worker-lifecycle-action-plan-annotation: decision=%s reviewer=%s reviewed_at=%s reason=%s\n",
		reportProjectionDisplayValue(annotation.Decision, "unknown"),
		reportProjectionDisplayValue(annotation.Reviewer, "operator"),
		reportProjectionDisplayValue(annotation.ReviewedAt, "unknown"),
		reportProjectionDisplayValue(annotation.Reason, "none"),
	)
	return err
}

func printWorkerLifecycleActionPlanSelection(out io.Writer, selection app.WorkerLifecycleActionPlanSelection) error {
	actorFields := fmt.Sprintf("selected_by=%s selected_at=%s", reportProjectionDisplayValue(selection.SelectedBy, "operator"), reportProjectionDisplayValue(selection.SelectedAt, "unknown"))
	if selection.Status == "cleared" {
		actorParts := []string{}
		if strings.TrimSpace(selection.SelectedBy) != "" || strings.TrimSpace(selection.SelectedAt) != "" {
			actorParts = append(actorParts, fmt.Sprintf("selected_by=%s selected_at=%s", reportProjectionDisplayValue(selection.SelectedBy, "operator"), reportProjectionDisplayValue(selection.SelectedAt, "unknown")))
		}
		actorParts = append(actorParts, fmt.Sprintf("cleared_by=%s cleared_at=%s", reportProjectionDisplayValue(selection.ClearedBy, "operator"), reportProjectionDisplayValue(selection.ClearedAt, "unknown")))
		actorFields = strings.Join(actorParts, " ")
	}
	isCurrent := strings.TrimSpace(selection.Status) == "current"
	_, err := fmt.Fprintf(
		out,
		"worker-lifecycle-action-plan-selection: status=%s mode=%s current=%t selection_path=%s plan_path=%s review_path=%s selection=%s plan=%s review=%s actions=%d ack=%d run=%d daemon_control=%d retry_dispatch_execute=%d reclaim=%d reconcile=%d superseded=%d %s reason=%s\n",
		reportProjectionDisplayValue(selection.Status, "unknown"),
		reportProjectionDisplayValue(selection.Mode, "metadata-only"),
		isCurrent,
		reportProjectionDisplayValue(selection.Path, "unknown"),
		reportProjectionDisplayValue(selection.PlanPath, "unknown"),
		reportProjectionDisplayValue(selection.ReviewPath, "none"),
		reportProjectionDisplayValue(selection.Path, "unknown"),
		selection.PlanPath,
		reportProjectionDisplayValue(selection.ReviewPath, "none"),
		selection.Summary.Total,
		selection.Summary.AckPendingTransport,
		selection.Summary.RunAcknowledgedTransport,
		selection.Summary.ApplyDaemonControl,
		selection.Summary.ExecuteDispatchRetryDispatch,
		selection.Summary.ReclaimStaleLease,
		selection.Summary.ReconcileResultLease,
		len(selection.SupersededPlanPaths),
		actorFields,
		reportProjectionDisplayValue(selection.Reason, "none"),
	)
	return err
}

func printWorkerLifecycleMaintenance(out io.Writer, maintenance app.WorkerLifecycleMaintenance) error {
	summary := maintenance.Summary
	prefix := "worker-lifecycle-maintenance:"
	pathPart := ""
	if strings.TrimSpace(maintenance.Path) != "" {
		prefix = "worker-lifecycle-maintenance-artifact:"
		pathPart = maintenance.Path + " "
	}
	if _, err := fmt.Fprintf(
		out,
		"%s %sstatus=%s mode=%s actions=%d ack_actions=%d run_actions=%d daemon_control_actions=%d retry_dispatch_execute=%d reclaim_actions=%d reconciliation_actions=%d skipped_unsafe=%d review_created=%d action_plan_created=%d reclaim_created=%d reconciliation_created=%d retry_dispatch_executions_created=%d retry_dispatch_claims_created=%d retry_dispatch_claims_dispatched=%d retry_dispatch_transport_requests_created=%d dispatch_lifecycle_retry_claims=%t lifecycle_retry_transport=%s reclaim_executions_created=%d reclaim_results_created=%d reconciliation_executions_created=%d reconciliation_results_created=%d execute_lease_recovery=%t review=%s action_plan=%s reclaim=%s reconciliation=%s reclaim_execution=%s reconciliation_execution=%s\n",
		prefix,
		pathPart,
		maintenance.Status,
		maintenance.Mode,
		summary.ActionsTotal,
		summary.AckActions,
		summary.RunActions,
		summary.DaemonControlActions,
		summary.RetryDispatchExecuteActions,
		summary.ReclaimActions,
		summary.ReconciliationActions,
		summary.SkippedUnsafeActions,
		summary.ReviewArtifactsCreated,
		summary.ActionPlanArtifactsCreated,
		summary.ReclaimDispatchArtifactsCreated,
		summary.ReconciliationDispatchArtifactsCreated,
		summary.RetryDispatchExecutionsCreated,
		summary.RetryDispatchClaimsCreated,
		summary.RetryDispatchClaimsDispatched,
		summary.RetryDispatchTransportRequestsCreated,
		maintenance.DispatchRetryClaims,
		maintenance.RetryDispatchTransportMode,
		summary.ReclaimExecutionsCreated,
		summary.ReclaimResultsCreated,
		summary.ReconciliationExecutionsCreated,
		summary.ReconciliationResultsCreated,
		maintenance.ExecuteLeaseRecovery,
		printOptionalPath(maintenance.ReviewPath),
		printOptionalPath(maintenance.ActionPlanPath),
		printOptionalPath(maintenance.ReclaimDispatchPath),
		printOptionalPath(maintenance.ReconciliationDispatchPath),
		printOptionalPath(maintenance.ReclaimExecutionPath),
		printOptionalPath(maintenance.ReconciliationExecutionPath),
	); err != nil {
		return err
	}
	if strings.TrimSpace(maintenance.ReclaimExecutionPath) != "" {
		if _, err := fmt.Fprintf(out, "worker-lifecycle-maintenance-reclaim-execution: %s\n", maintenance.ReclaimExecutionPath); err != nil {
			return err
		}
	}
	if strings.TrimSpace(maintenance.ReconciliationExecutionPath) != "" {
		if _, err := fmt.Fprintf(out, "worker-lifecycle-maintenance-reconciliation-execution: %s\n", maintenance.ReconciliationExecutionPath); err != nil {
			return err
		}
	}
	for _, path := range maintenance.RetryDispatchExecutionPaths {
		if _, err := fmt.Fprintf(out, "worker-lifecycle-maintenance-retry-dispatch-execution: %s\n", path); err != nil {
			return err
		}
	}
	for _, action := range maintenance.SkippedActions {
		if _, err := fmt.Fprintf(
			out,
			"worker-lifecycle-maintenance-skip: type=%s gate=%s worker=%s run=%s execution=%s reason=%s request_path=%s lease_path=%s result_path=%s dispatch=%s\n",
			action.Type,
			action.Gate,
			action.WorkerID,
			action.RunID,
			action.ExecutionID,
			action.Reason,
			action.RequestPath,
			action.LeasePath,
			action.ResultPath,
			action.DispatchPath,
		); err != nil {
			return err
		}
	}
	return nil
}

func printWorkerLifecycleMaintenanceSupervisor(out io.Writer, supervisor app.WorkerLifecycleMaintenanceSupervisor) error {
	summary := supervisor.Summary
	prefix := "worker-lifecycle-maintenance-supervisor:"
	pathPart := ""
	if strings.TrimSpace(supervisor.Path) != "" {
		prefix = "worker-lifecycle-maintenance-supervisor-artifact:"
		pathPart = supervisor.Path + " "
	}
	if _, err := fmt.Fprintf(
		out,
		"%s %sstatus=%s mode=%s runs=%d max_runs=%d actions=%d ack_actions=%d run_actions=%d daemon_control_actions=%d retry_dispatch_execute=%d reclaim_actions=%d reconciliation_actions=%d skipped_unsafe=%d maintenance_created=%d audit_created=%d retry_dispatch_executions_created=%d retry_dispatch_claims_created=%d retry_dispatch_claims_dispatched=%d retry_dispatch_transport_requests_created=%d dispatch_lifecycle_retry_claims=%t lifecycle_retry_transport=%s reclaim_executions_created=%d reclaim_results_created=%d reconciliation_executions_created=%d reconciliation_results_created=%d execute_lease_recovery=%t\n",
		prefix,
		pathPart,
		supervisor.Status,
		supervisor.Mode,
		summary.Runs,
		summary.MaxRuns,
		summary.ActionsTotal,
		summary.AckActions,
		summary.RunActions,
		summary.DaemonControlActions,
		summary.RetryDispatchExecuteActions,
		summary.ReclaimActions,
		summary.ReconciliationActions,
		summary.SkippedUnsafeActions,
		summary.MaintenanceArtifactsCreated,
		summary.AuditArtifactsCreated,
		summary.RetryDispatchExecutionsCreated,
		summary.RetryDispatchClaimsCreated,
		summary.RetryDispatchClaimsDispatched,
		summary.RetryDispatchTransportRequestsCreated,
		supervisor.DispatchRetryClaims,
		supervisor.RetryDispatchTransportMode,
		summary.ReclaimExecutionsCreated,
		summary.ReclaimResultsCreated,
		summary.ReconciliationExecutionsCreated,
		summary.ReconciliationResultsCreated,
		supervisor.ExecuteLeaseRecovery,
	); err != nil {
		return err
	}
	for _, path := range supervisor.MaintenancePaths {
		if _, err := fmt.Fprintf(out, "worker-lifecycle-maintenance-supervisor-maintenance: %s\n", path); err != nil {
			return err
		}
	}
	for _, path := range supervisor.AuditPaths {
		if _, err := fmt.Fprintf(out, "worker-lifecycle-maintenance-supervisor-audit: %s\n", path); err != nil {
			return err
		}
	}
	for _, path := range supervisor.RetryDispatchExecutionPaths {
		if _, err := fmt.Fprintf(out, "worker-lifecycle-maintenance-supervisor-retry-dispatch-execution: %s\n", path); err != nil {
			return err
		}
	}
	for _, path := range supervisor.ReclaimExecutionPaths {
		if _, err := fmt.Fprintf(out, "worker-lifecycle-maintenance-supervisor-reclaim-execution: %s\n", path); err != nil {
			return err
		}
	}
	for _, path := range supervisor.ReconciliationExecutionPaths {
		if _, err := fmt.Fprintf(out, "worker-lifecycle-maintenance-supervisor-reconciliation-execution: %s\n", path); err != nil {
			return err
		}
	}
	if strings.TrimSpace(supervisor.Error) != "" {
		if _, err := fmt.Fprintf(out, "worker-lifecycle-maintenance-supervisor-error: %s\n", supervisor.Error); err != nil {
			return err
		}
	}
	return nil
}

func printWorkerLifecyclePolicy(out io.Writer, policy app.WorkerLifecyclePolicy) error {
	summary := policy.Summary
	prefix := "worker-lifecycle-policy:"
	pathPart := ""
	if strings.TrimSpace(policy.Path) != "" {
		prefix = "worker-lifecycle-policy-artifact:"
		pathPart = policy.Path + " "
	}
	_, err := fmt.Fprintf(
		out,
		"%s %sstatus=%s mode=%s stale_after_seconds=%d worker_stale_after_seconds=%d poll_interval_seconds=%d poll_interval_ms=%d max_runs=%d transport=%s include_lifecycle_maintenance=%t execute_lifecycle_retries=%t dispatch_lifecycle_retry_claims=%t lifecycle_retry_transport=%s execute_lease_recovery=%t workers=%d stale_workers=%d offline_workers=%d queue_claims=%d dispatches=%d transport_requests=%d transport_pending=%d transport_ack=%d transport_completed=%d transport_failed=%d results=%d retry_plans=%d retry_dispatches=%d retry_dispatch_executions=%d leases=%d active_leases=%d stale_leases=%d lifecycle_reviews=%d action_plans=%d maintenances=%d maintenance_supervisors=%d worker_orchestrations=%d orchestration_supervisors=%d daemon_runs=%d supervisor_runs=%d controls=%d artifacts=%d\n",
		prefix,
		pathPart,
		policy.Status,
		policy.Mode,
		policy.StaleAfterSeconds,
		policy.WorkerStaleAfterSeconds,
		policy.PollIntervalSeconds,
		policy.PollIntervalMilliseconds,
		policy.MaxSupervisorRuns,
		policy.TransportMode,
		policy.IncludeLifecycleMaintenance,
		policy.ExecuteLifecycleRetries,
		policy.DispatchLifecycleRetryClaims,
		policy.LifecycleRetryTransportMode,
		policy.ExecuteLeaseRecovery,
		summary.WorkerRegistrations,
		summary.StaleWorkerRegistrations,
		summary.OfflineWorkerRegistrations,
		summary.QueueClaims,
		summary.QueueDispatchExecutions,
		summary.TransportRequests,
		summary.TransportRequestsPending,
		summary.TransportRequestsAcknowledged,
		summary.TransportRequestsCompleted,
		summary.TransportRequestsFailed,
		summary.QueueDispatchResults,
		summary.DispatchRetryPlans,
		summary.DispatchRetryDispatches,
		summary.DispatchRetryDispatchExecutions,
		summary.WorkerLeases,
		summary.ActiveLeases,
		summary.StaleActiveLeases,
		summary.LifecycleReviews,
		summary.ActionPlans,
		summary.LifecycleMaintenances,
		summary.LifecycleMaintenanceSupervisors,
		summary.WorkerOrchestrations,
		summary.WorkerOrchestrationSupervisors,
		summary.DaemonRuns,
		summary.DaemonSupervisorRuns,
		summary.DaemonControls,
		summary.ArtifactTotal,
	)
	return err
}

func printWorkerLifecyclePolicyRun(out io.Writer, run app.WorkerLifecyclePolicyRun) error {
	supervisorCreated := 0
	if strings.TrimSpace(run.SupervisorPath) != "" {
		supervisorCreated = 1
	}
	if _, err := fmt.Fprintf(
		out,
		"worker-lifecycle-policy-run-artifact: %s status=%s mode=metadata-only policy=%s selection=%s max_runs=%d poll_interval_ms=%d transport=%s include_lifecycle_maintenance=%t execute_lifecycle_retries=%t dispatch_lifecycle_retry_claims=%t lifecycle_retry_transport=%s retry_dispatch_claims_dispatched=%d retry_dispatch_transport_requests_created=%d execute_lease_recovery=%t supervisor_created=%d\n",
		run.Path,
		run.Status,
		run.PolicyPath,
		run.SelectionPath,
		run.MaxRuns,
		run.PollIntervalMilliseconds,
		run.TransportMode,
		run.IncludeLifecycleMaintenance,
		run.ExecuteLifecycleRetries,
		run.DispatchLifecycleRetryClaims,
		run.LifecycleRetryTransportMode,
		run.SupervisorSummary.RetryDispatchClaimsDispatched,
		run.SupervisorSummary.RetryDispatchTransportRequestsCreated,
		run.ExecuteLeaseRecovery,
		supervisorCreated,
	); err != nil {
		return err
	}
	if strings.TrimSpace(run.SupervisorPath) != "" {
		if _, err := fmt.Fprintf(out, "worker-lifecycle-policy-run-supervisor: %s\n", run.SupervisorPath); err != nil {
			return err
		}
	}
	if strings.TrimSpace(run.SkipReason) != "" {
		if _, err := fmt.Fprintf(out, "worker-lifecycle-policy-run-skip: %s\n", run.SkipReason); err != nil {
			return err
		}
	}
	if strings.TrimSpace(run.Error) != "" {
		if _, err := fmt.Fprintf(out, "worker-lifecycle-policy-run-error: %s\n", run.Error); err != nil {
			return err
		}
	}
	return nil
}

func printWorkerLifecyclePolicySchedulerRun(out io.Writer, run app.WorkerLifecyclePolicySchedulerRun) error {
	maxRuns := run.MaxRunsRequested
	if maxRuns == 0 {
		maxRuns = run.MaxRuns
	}
	supervisorCreated := run.SupervisorsCreated
	if supervisorCreated == 0 && run.SupervisorCreated {
		supervisorCreated = 1
	}
	if _, err := fmt.Fprintf(
		out,
		"worker-lifecycle-policy-scheduler-run-artifact: %s status=%s mode=%s policy_basename=%s policy_path=%s selection_path=%s poll_interval_ms=%d max_runs=%d transport=%s include_lifecycle_maintenance=%t execute_lifecycle_retries=%t dispatch_lifecycle_retry_claims=%t lifecycle_retry_transport=%s execute_lease_recovery=%t policy_runs_created=%d supervisor_created=%d\n",
		run.Path,
		run.Status,
		run.Mode,
		run.PolicyBasename,
		run.PolicyPath,
		run.SelectionPath,
		run.PollIntervalMilliseconds,
		maxRuns,
		run.TransportMode,
		run.IncludeLifecycleMaintenance,
		run.ExecuteLifecycleRetries,
		run.DispatchLifecycleRetryClaims,
		run.LifecycleRetryTransportMode,
		run.ExecuteLeaseRecovery,
		run.PolicyRunsCreated,
		supervisorCreated,
	); err != nil {
		return err
	}
	if strings.TrimSpace(run.SkipReason) != "" {
		if _, err := fmt.Fprintf(out, "worker-lifecycle-policy-scheduler-run-skip: %s\n", run.SkipReason); err != nil {
			return err
		}
	}
	if strings.TrimSpace(run.Error) != "" {
		if _, err := fmt.Fprintf(out, "worker-lifecycle-policy-scheduler-run-error: %s\n", run.Error); err != nil {
			return err
		}
	}
	return nil
}

func printWorkerLifecyclePolicySelection(out io.Writer, selection app.WorkerLifecyclePolicySelection) error {
	summary := selection.Summary
	actorFields := fmt.Sprintf("selected_by=%s selected_at=%s", selection.SelectedBy, selection.SelectedAt)
	if strings.TrimSpace(selection.Status) == "cleared" {
		actorFields = fmt.Sprintf("cleared_by=%s cleared_at=%s", selection.ClearedBy, selection.ClearedAt)
	}
	if _, err := fmt.Fprintf(
		out,
		"worker-lifecycle-policy-selection: status=%s mode=%s policy=%s selection=%s stale_after_seconds=%d worker_stale_after_seconds=%d poll_interval_ms=%d max_runs=%d transport=%s include_lifecycle_maintenance=%t execute_lifecycle_retries=%t dispatch_lifecycle_retry_claims=%t lifecycle_retry_transport=%s execute_lease_recovery=%t workers=%d stale_workers=%d offline_workers=%d queue_claims=%d dispatches=%d transport_requests=%d results=%d retry_plans=%d retry_dispatches=%d retry_dispatch_executions=%d leases=%d active_leases=%d stale_leases=%d lifecycle_reviews=%d action_plans=%d maintenances=%d maintenance_supervisors=%d worker_orchestrations=%d orchestration_supervisors=%d artifacts=%d superseded=%d %s reason=%s\n",
		selection.Status,
		selection.Mode,
		selection.PolicyPath,
		selection.Path,
		selection.StaleAfterSeconds,
		selection.WorkerStaleAfterSeconds,
		selection.PollIntervalMilliseconds,
		selection.MaxSupervisorRuns,
		selection.TransportMode,
		selection.IncludeLifecycleMaintenance,
		selection.ExecuteLifecycleRetries,
		selection.DispatchLifecycleRetryClaims,
		selection.LifecycleRetryTransportMode,
		selection.ExecuteLeaseRecovery,
		summary.WorkerRegistrations,
		summary.StaleWorkerRegistrations,
		summary.OfflineWorkerRegistrations,
		summary.QueueClaims,
		summary.QueueDispatchExecutions,
		summary.TransportRequests,
		summary.QueueDispatchResults,
		summary.DispatchRetryPlans,
		summary.DispatchRetryDispatches,
		summary.DispatchRetryDispatchExecutions,
		summary.WorkerLeases,
		summary.ActiveLeases,
		summary.StaleActiveLeases,
		summary.LifecycleReviews,
		summary.ActionPlans,
		summary.LifecycleMaintenances,
		summary.LifecycleMaintenanceSupervisors,
		summary.WorkerOrchestrations,
		summary.WorkerOrchestrationSupervisors,
		summary.ArtifactTotal,
		len(selection.SupersededPolicyPaths),
		actorFields,
		selection.Reason,
	); err != nil {
		return err
	}
	for _, path := range selection.SupersededPolicyPaths {
		if _, err := fmt.Fprintf(out, "worker-lifecycle-policy-superseded: %s\n", path); err != nil {
			return err
		}
	}
	return nil
}

func printOptionalPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return "none"
	}
	return path
}

func printWorkerLifecycleReviewDetails(out io.Writer, reclaim app.WorkerLeaseReclaimPlan, reconciliation app.WorkerLeaseResultReconciliationPlan) error {
	for _, candidate := range reclaim.Ready {
		lease := candidate.Lease
		if _, err := fmt.Fprintf(
			out,
			"worker-lifecycle-reclaim-ready: run=%s execution=%s worker=%s status=%s mode=%s\n",
			lease.RunID,
			lease.ExecutionID,
			lease.WorkerID,
			lease.Status,
			candidate.Mode,
		); err != nil {
			return err
		}
	}
	for _, skipped := range reclaim.Skipped {
		lease := skipped.Lease
		if _, err := fmt.Fprintf(
			out,
			"worker-lifecycle-reclaim-skip: run=%s execution=%s worker=%s status=%s reason=%s\n",
			lease.RunID,
			lease.ExecutionID,
			lease.WorkerID,
			lease.Status,
			skipped.Reason,
		); err != nil {
			return err
		}
	}
	for _, candidate := range reconciliation.Ready {
		if _, err := fmt.Fprintf(
			out,
			"worker-lifecycle-reconciliation-ready: run=%s execution=%s worker=%s result=%s lease=%s recommended=%s mode=%s reason=%s\n",
			candidate.Result.RunID,
			candidate.Result.ExecutionID,
			candidate.Result.WorkerID,
			candidate.Result.Status,
			candidate.Lease.Status,
			candidate.RecommendedStatus,
			candidate.Mode,
			candidate.Reason,
		); err != nil {
			return err
		}
	}
	for _, skipped := range reconciliation.Skipped {
		if _, err := fmt.Fprintf(
			out,
			"worker-lifecycle-reconciliation-skip: run=%s execution=%s worker=%s result=%s lease=%s mode=%s reason=%s\n",
			skipped.Result.RunID,
			skipped.Result.ExecutionID,
			skipped.Result.WorkerID,
			skipped.Result.Status,
			skipped.Lease.Status,
			skipped.Mode,
			skipped.Reason,
		); err != nil {
			return err
		}
	}
	return nil
}

func printWorkerLeaseResultReconciliationPlan(out io.Writer, plan app.WorkerLeaseResultReconciliationPlan) error {
	if _, err := fmt.Fprintf(out, "worker-lease-result-reconcile: ready=%d skipped=%d mode=metadata-only\n", len(plan.Ready), len(plan.Skipped)); err != nil {
		return err
	}
	for _, candidate := range plan.Ready {
		if _, err := fmt.Fprintf(
			out,
			"lease-result-reconcile: run=%s execution=%s worker=%s result=%s lease=%s recommended=%s mode=%s reason=%s result_path=%s dispatch=%s lease_path=%s\n",
			candidate.Result.RunID,
			candidate.Result.ExecutionID,
			candidate.Result.WorkerID,
			candidate.Result.Status,
			candidate.Lease.Status,
			candidate.RecommendedStatus,
			candidate.Mode,
			candidate.Reason,
			workerLeaseResultReconciliationDisplayPath(candidate.ResultPath, candidate.Result.Path),
			workerLeaseResultReconciliationDisplayPath(candidate.DispatchPath, candidate.Dispatch.Path),
			workerLeaseResultReconciliationDisplayPath(candidate.LeasePath, candidate.Lease.Path),
		); err != nil {
			return err
		}
	}
	for _, skipped := range plan.Skipped {
		if _, err := fmt.Fprintf(
			out,
			"skip-lease-result-reconcile: run=%s execution=%s worker=%s result=%s lease=%s mode=%s reason=%s result_path=%s dispatch=%s lease_path=%s\n",
			skipped.Result.RunID,
			skipped.Result.ExecutionID,
			skipped.Result.WorkerID,
			skipped.Result.Status,
			skipped.Lease.Status,
			skipped.Mode,
			skipped.Reason,
			workerLeaseResultReconciliationDisplayPath(skipped.ResultPath, skipped.Result.Path),
			workerLeaseResultReconciliationDisplayPath(skipped.DispatchPath, skipped.Dispatch.Path),
			workerLeaseResultReconciliationDisplayPath(skipped.LeasePath, skipped.Lease.Path),
		); err != nil {
			return err
		}
	}
	return nil
}

func printWorkerLeaseResultReconciliationDispatch(out io.Writer, dispatch app.WorkerLeaseResultReconciliationDispatch) error {
	if _, err := fmt.Fprintf(
		out,
		"worker-lease-result-reconciliation: %s status=%s mode=%s ready=%d skipped=%d\n",
		dispatch.Path,
		dispatch.Status,
		dispatch.Mode,
		len(dispatch.Ready),
		len(dispatch.Skipped),
	); err != nil {
		return err
	}
	for _, candidate := range dispatch.Ready {
		if _, err := fmt.Fprintf(
			out,
			"lease-result-reconciliation: run=%s execution=%s worker=%s result=%s lease=%s recommended=%s mode=%s reason=%s result_path=%s dispatch=%s lease_path=%s\n",
			candidate.Result.RunID,
			candidate.Result.ExecutionID,
			candidate.Result.WorkerID,
			candidate.Result.Status,
			candidate.Lease.Status,
			candidate.RecommendedStatus,
			candidate.Mode,
			candidate.Reason,
			workerLeaseResultReconciliationDisplayPath(candidate.ResultPath, candidate.Result.Path),
			workerLeaseResultReconciliationDisplayPath(candidate.DispatchPath, candidate.Dispatch.Path),
			workerLeaseResultReconciliationDisplayPath(candidate.LeasePath, candidate.Lease.Path),
		); err != nil {
			return err
		}
	}
	for _, skipped := range dispatch.Skipped {
		if _, err := fmt.Fprintf(
			out,
			"skip-lease-result-reconciliation: run=%s execution=%s worker=%s result=%s lease=%s mode=%s reason=%s result_path=%s dispatch=%s lease_path=%s\n",
			skipped.Result.RunID,
			skipped.Result.ExecutionID,
			skipped.Result.WorkerID,
			skipped.Result.Status,
			skipped.Lease.Status,
			skipped.Mode,
			skipped.Reason,
			workerLeaseResultReconciliationDisplayPath(skipped.ResultPath, skipped.Result.Path),
			workerLeaseResultReconciliationDisplayPath(skipped.DispatchPath, skipped.Dispatch.Path),
			workerLeaseResultReconciliationDisplayPath(skipped.LeasePath, skipped.Lease.Path),
		); err != nil {
			return err
		}
	}
	return nil
}

func printWorkerLeaseResultReconciliationExecution(out io.Writer, execution app.WorkerLeaseResultReconciliationExecution) error {
	if _, err := fmt.Fprintf(
		out,
		"worker-lease-result-reconciliation-execution: %s status=%s mode=%s dispatch=%s results=%d skipped=%d\n",
		execution.Path,
		execution.Status,
		execution.Mode,
		execution.DispatchPath,
		len(execution.Results),
		len(execution.Skipped),
	); err != nil {
		return err
	}
	for _, result := range execution.Results {
		if _, err := fmt.Fprintf(
			out,
			"release-worker-lease-result-reconciliation: run=%s execution=%s worker=%s result=%s status=%s previous=%s recommended=%s result_path=%s lease_path=%s\n",
			result.RunID,
			result.ExecutionID,
			result.WorkerID,
			result.ResultStatus,
			result.Status,
			result.PreviousStatus,
			result.RecommendedStatus,
			result.ResultPath,
			result.LeasePath,
		); err != nil {
			return err
		}
	}
	for _, skipped := range execution.Skipped {
		if _, err := fmt.Fprintf(
			out,
			"skip-worker-lease-result-reconciliation-execute: run=%s execution=%s worker=%s result=%s lease=%s reason=%s\n",
			skipped.Result.RunID,
			skipped.Result.ExecutionID,
			skipped.Result.WorkerID,
			skipped.Result.Status,
			skipped.Lease.Status,
			skipped.Reason,
		); err != nil {
			return err
		}
	}
	return nil
}

func workerLeaseResultReconciliationDisplayPath(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}

func printWorkerLeaseReclaimDispatch(out io.Writer, dispatch app.WorkerLeaseReclaimDispatch) error {
	if _, err := fmt.Fprintf(
		out,
		"worker-lease-reclaim: %s status=%s mode=%s ready=%d skipped=%d\n",
		dispatch.Path,
		dispatch.Status,
		dispatch.Mode,
		len(dispatch.Ready),
		len(dispatch.Skipped),
	); err != nil {
		return err
	}
	for _, candidate := range dispatch.Ready {
		lease := candidate.Lease
		if _, err := fmt.Fprintf(
			out,
			"reclaim-worker-lease: run=%s execution=%s tool=%s capability=%s worker=%s mode=%s\n",
			lease.RunID,
			lease.ExecutionID,
			lease.Tool,
			lease.Capability,
			lease.WorkerID,
			candidate.Mode,
		); err != nil {
			return err
		}
	}
	for _, skipped := range dispatch.Skipped {
		lease := skipped.Lease
		if _, err := fmt.Fprintf(
			out,
			"skip-worker-lease: run=%s execution=%s tool=%s capability=%s worker=%s status=%s reason=%s\n",
			lease.RunID,
			lease.ExecutionID,
			lease.Tool,
			lease.Capability,
			lease.WorkerID,
			lease.Status,
			skipped.Reason,
		); err != nil {
			return err
		}
	}
	return nil
}

func printWorkerLeaseReclaimExecution(out io.Writer, execution app.WorkerLeaseReclaimExecution) error {
	if _, err := fmt.Fprintf(
		out,
		"worker-lease-reclaim-execution: %s status=%s mode=%s dispatch=%s results=%d skipped=%d\n",
		execution.Path,
		execution.Status,
		execution.Mode,
		execution.DispatchPath,
		len(execution.Results),
		len(execution.Skipped),
	); err != nil {
		return err
	}
	for _, result := range execution.Results {
		if _, err := fmt.Fprintf(
			out,
			"reclaim-worker-lease-execute: run=%s execution=%s status=%s worker=%s tool=%s capability=%s\n",
			result.RunID,
			result.ExecutionID,
			result.Status,
			result.WorkerID,
			result.Tool,
			result.Capability,
		); err != nil {
			return err
		}
	}
	for _, skipped := range execution.Skipped {
		lease := skipped.Lease
		if _, err := fmt.Fprintf(
			out,
			"skip-worker-lease-reclaim-execute: run=%s execution=%s status=%s reason=%s\n",
			lease.RunID,
			lease.ExecutionID,
			lease.Status,
			skipped.Reason,
		); err != nil {
			return err
		}
	}
	return nil
}

func newToolchainListCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "list <workspace>",
		Short: "List workspace toolchain manifests",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			statuses, err := app.InspectToolchains(args[0])
			if err != nil {
				return err
			}
			for _, status := range statuses {
				if _, err := fmt.Fprintf(
					out,
					"toolchain: %s version=%s runner=%s parser=%s capabilities=%d\n",
					status.Name,
					status.Version,
					status.Runner,
					status.Parser,
					status.CapabilityCount,
				); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newToolchainHealthCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "health <workspace>",
		Short: "Check workspace toolchain health",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			statuses, err := app.InspectToolchains(args[0])
			if err != nil {
				return err
			}
			for _, status := range statuses {
				if _, err := fmt.Fprintf(
					out,
					"toolchain: %s state=%s installed=%s latest=%s message=%s\n",
					status.Name,
					status.Health.State,
					status.Health.InstalledVersion,
					status.Health.LatestVersion,
					status.Health.Message,
				); err != nil {
					return err
				}
				for _, dependency := range status.Dependencies {
					if _, err := fmt.Fprintf(
						out,
						"dependency: %s/%s kind=%s state=%s installed=%s latest=%s required=%t source=%s digest=%s latest_digest=%s metadata_source=%s message=%s\n",
						status.Name,
						dependency.Name,
						dependency.Kind,
						dependency.State,
						dependency.InstalledVersion,
						dependency.LatestVersion,
						dependency.Required,
						dependency.Source,
						dependency.ImageDigest,
						dependency.LatestImageDigest,
						dependency.MetadataSource,
						dependency.Message,
					); err != nil {
						return err
					}
				}
				for _, step := range status.InstallPlan {
					if _, err := fmt.Fprintf(
						out,
						"install: %s/%s kind=%s state=%s required=%t command=%s hint=%s source=%s digest=%s depends_on=%s\n",
						status.Name,
						step.Name,
						step.Kind,
						step.State,
						step.Required,
						formatCommandJSON(step.Command),
						step.Hint,
						step.Source,
						step.ImageDigest,
						formatDependsOn(step.DependsOn),
					); err != nil {
						return err
					}
				}
			}
			return nil
		},
	}
}

func newToolchainCompilePreviewCommand(out io.Writer) *cobra.Command {
	var actionName string
	var maxPlanNodes int
	var domainPriorityArgs []string
	cmd := &cobra.Command{
		Use:   "compile-preview <workspace>",
		Short: "Print a metadata-only scored tool compile preview",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			action, err := parseCompilePreviewAction(actionName)
			if err != nil {
				return err
			}
			if maxPlanNodes < 0 {
				return fmt.Errorf("max-plan-nodes must be non-negative")
			}
			domainPriorities, err := parseCompilePreviewDomainPriorities(domainPriorityArgs)
			if err != nil {
				return err
			}
			preview, err := app.PreviewToolCompilePlan(args[0], app.ToolCompilePreviewOptions{
				Action: action,
				Budget: compiler.CompileBudget{
					MaxPlanNodes: maxPlanNodes,
				},
				DomainPriorities: domainPriorities,
			})
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(
				out,
				"compile-preview: workspace=%s action=%s mode=%s nodes=%d warnings=%d\n",
				preview.Workspace,
				preview.Action,
				preview.Mode,
				len(preview.Nodes),
				len(preview.Warnings),
			); err != nil {
				return err
			}
			for _, warning := range preview.Warnings {
				if _, err := fmt.Fprintf(out, "compile-warning: %s\n", warning); err != nil {
					return err
				}
			}
			for _, node := range preview.Nodes {
				if _, err := fmt.Fprintf(
					out,
					"compile-node: id=%s manifest=%s capability=%s runner=%s parser=%s score=%s signals=%s command=%s\n",
					node.ID,
					node.ManifestName,
					node.Capability,
					node.Runner,
					node.Parser,
					formatFloat(node.SelectionScore),
					formatSelectionSignals(node.SelectionSignals),
					formatCommandJSON(node.RunCommand),
				); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&actionName, "action", string(compiler.ActionDiscoverAssets), "semantic action to compile")
	cmd.Flags().IntVar(&maxPlanNodes, "max-plan-nodes", 0, "maximum plan nodes to print; 0 means unlimited")
	cmd.Flags().StringArrayVar(&domainPriorityArgs, "domain-priority", nil, "domain priority as domain=weight, e.g. infra_internal=5")
	return cmd
}

func newToolchainCompilePreviewReviewCommand(out io.Writer) *cobra.Command {
	var actionName string
	var maxPlanNodes int
	var domainPriorityArgs []string
	cmd := &cobra.Command{
		Use:   "compile-preview-review <workspace>",
		Short: "Persist a metadata-only scored tool compile preview review",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			action, err := parseCompilePreviewAction(actionName)
			if err != nil {
				return err
			}
			if maxPlanNodes < 0 {
				return fmt.Errorf("max-plan-nodes must be non-negative")
			}
			domainPriorities, err := parseCompilePreviewDomainPriorities(domainPriorityArgs)
			if err != nil {
				return err
			}
			review, err := app.CreateToolCompilePreviewReview(args[0], app.ToolCompilePreviewOptions{
				Action: action,
				Budget: compiler.CompileBudget{
					MaxPlanNodes: maxPlanNodes,
				},
				DomainPriorities: domainPriorities,
			})
			if err != nil {
				return err
			}
			return printToolCompilePreviewReview(out, review)
		},
	}
	cmd.Flags().StringVar(&actionName, "action", string(compiler.ActionDiscoverAssets), "semantic action to compile")
	cmd.Flags().IntVar(&maxPlanNodes, "max-plan-nodes", 0, "maximum plan nodes to persist; 0 means unlimited")
	cmd.Flags().StringArrayVar(&domainPriorityArgs, "domain-priority", nil, "domain priority as domain=weight, e.g. infra_internal=5")
	return cmd
}

func newToolchainCompilePreviewReviewAnnotateCommand(out io.Writer) *cobra.Command {
	var decision string
	var reason string
	var reviewer string
	var reviewedAtArg string
	cmd := &cobra.Command{
		Use:   "compile-preview-review-annotate <workspace> <artifact>",
		Short: "Annotate a persisted metadata-only compile preview review",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var reviewedAt time.Time
			if strings.TrimSpace(reviewedAtArg) != "" {
				parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(reviewedAtArg))
				if err != nil {
					return fmt.Errorf("reviewed-at must be RFC3339: %w", err)
				}
				reviewedAt = parsed
			}
			review, err := app.AnnotateToolCompilePreviewReview(args[0], args[1], app.ToolCompilePreviewReviewAnnotationOptions{
				Decision:   decision,
				Reason:     reason,
				Reviewer:   reviewer,
				ReviewedAt: reviewedAt,
			})
			if err != nil {
				return err
			}
			return printToolCompilePreviewReview(out, review)
		},
	}
	cmd.Flags().StringVar(&decision, "decision", "", "review decision: accepted, deferred, or rejected")
	cmd.Flags().StringVar(&reason, "reason", "", "review decision reason")
	cmd.Flags().StringVar(&reviewer, "reviewer", "", "reviewer identity; defaults to operator")
	cmd.Flags().StringVar(&reviewedAtArg, "reviewed-at", "", "review timestamp in RFC3339; defaults to now")
	return cmd
}

func newToolchainCompilePreviewReviewCompareCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "compile-preview-review-compare <workspace> <base-artifact> <candidate-artifact>",
		Short: "Compare two persisted metadata-only compile preview reviews",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			comparison, err := app.CompareToolCompilePreviewReviews(args[0], args[1], args[2])
			if err != nil {
				return err
			}
			return printToolCompilePreviewReviewComparison(out, comparison)
		},
	}
}

func newToolchainCompilePreviewReviewSelectCommand(out io.Writer) *cobra.Command {
	var selectedBy string
	var selectedAtArg string
	var reason string
	cmd := &cobra.Command{
		Use:   "compile-preview-review-select <workspace> <artifact>",
		Short: "Mark a persisted compile preview review as the current metadata-only selection",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var selectedAt time.Time
			if strings.TrimSpace(selectedAtArg) != "" {
				parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(selectedAtArg))
				if err != nil {
					return fmt.Errorf("selected-at must be RFC3339: %w", err)
				}
				selectedAt = parsed
			}
			selection, err := app.SelectToolCompilePreviewReview(args[0], args[1], app.ToolCompilePreviewReviewSelectionOptions{
				SelectedBy: selectedBy,
				SelectedAt: selectedAt,
				Reason:     reason,
			})
			if err != nil {
				return err
			}
			return printToolCompilePreviewReviewSelection(out, selection)
		},
	}
	cmd.Flags().StringVar(&selectedBy, "selected-by", "", "operator identity; defaults to operator")
	cmd.Flags().StringVar(&selectedAtArg, "selected-at", "", "selection timestamp in RFC3339; defaults to now")
	cmd.Flags().StringVar(&reason, "reason", "", "selection reason")
	return cmd
}

func newToolchainCompilePreviewReviewSelectionCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "compile-preview-review-selection <workspace>",
		Short: "Inspect the current metadata-only compile preview review selection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			selection, found, err := app.ReadToolCompilePreviewReviewSelection(args[0])
			if err != nil {
				return err
			}
			if !found {
				_, err := fmt.Fprintln(out, "compile-preview-review-selection: none")
				return err
			}
			return printToolCompilePreviewReviewSelection(out, selection)
		},
	}
}

func newToolchainCompilePreviewReviewSelectionClearCommand(out io.Writer) *cobra.Command {
	var clearedBy string
	var clearedAtArg string
	var reason string
	cmd := &cobra.Command{
		Use:   "compile-preview-review-selection-clear <workspace>",
		Short: "Clear the current metadata-only compile preview review selection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var clearedAt time.Time
			if strings.TrimSpace(clearedAtArg) != "" {
				parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(clearedAtArg))
				if err != nil {
					return fmt.Errorf("cleared-at must be RFC3339: %w", err)
				}
				clearedAt = parsed
			}
			selection, err := app.ClearToolCompilePreviewReviewSelection(args[0], app.ToolCompilePreviewReviewSelectionClearOptions{
				ClearedBy: clearedBy,
				ClearedAt: clearedAt,
				Reason:    reason,
			})
			if err != nil {
				return err
			}
			return printToolCompilePreviewReviewSelection(out, selection)
		},
	}
	cmd.Flags().StringVar(&clearedBy, "cleared-by", "", "operator identity; defaults to operator")
	cmd.Flags().StringVar(&clearedAtArg, "cleared-at", "", "clear timestamp in RFC3339; defaults to now")
	cmd.Flags().StringVar(&reason, "reason", "", "clear reason")
	return cmd
}

func newToolchainCompilePreviewReviewsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "compile-preview-reviews <workspace> [artifact]",
		Short: "List or inspect persisted metadata-only compile preview reviews",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				review, err := app.ReadToolCompilePreviewReview(args[0], args[1])
				if err != nil {
					return err
				}
				return printToolCompilePreviewReview(out, review)
			}
			reviews, err := app.ListToolCompilePreviewReviews(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "compile-preview-reviews: total=%d\n", len(reviews)); err != nil {
				return err
			}
			for _, review := range reviews {
				if err := printToolCompilePreviewReview(out, review); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func printToolCompilePreviewReview(out io.Writer, review app.ToolCompilePreviewReview) error {
	if _, err := fmt.Fprintf(
		out,
		"compile-preview-review: %s status=%s mode=%s action=%s nodes=%d warnings=%d\n",
		review.Path,
		review.Status,
		review.Mode,
		review.Preview.Action,
		len(review.Preview.Nodes),
		len(review.Preview.Warnings),
	); err != nil {
		return err
	}
	if review.Annotation != nil {
		if _, err := fmt.Fprintf(
			out,
			"compile-review-annotation: decision=%s reviewer=%s reviewed_at=%s reason=%s\n",
			review.Annotation.Decision,
			review.Annotation.Reviewer,
			review.Annotation.ReviewedAt,
			review.Annotation.Reason,
		); err != nil {
			return err
		}
	}
	for _, warning := range review.Preview.Warnings {
		if _, err := fmt.Fprintf(out, "compile-review-warning: %s\n", warning); err != nil {
			return err
		}
	}
	for _, node := range review.Preview.Nodes {
		if _, err := fmt.Fprintf(
			out,
			"compile-review-node: id=%s manifest=%s capability=%s runner=%s parser=%s score=%s signals=%s command=%s\n",
			node.ID,
			node.ManifestName,
			node.Capability,
			node.Runner,
			node.Parser,
			formatFloat(node.SelectionScore),
			formatSelectionSignals(node.SelectionSignals),
			formatCommandJSON(node.RunCommand),
		); err != nil {
			return err
		}
	}
	return nil
}

func printToolCompilePreviewReviewComparison(out io.Writer, comparison app.ToolCompilePreviewReviewComparison) error {
	if _, err := fmt.Fprintf(
		out,
		"compile-preview-review-compare: mode=%s base=%s candidate=%s action=%s node_delta=%d warning_delta=%d added=%d removed=%d changed=%d reordered=%d score_changed=%d signal_changed=%d\n",
		comparison.Mode,
		comparison.Base.Path,
		comparison.Candidate.Path,
		comparison.Candidate.Action,
		comparison.Summary.NodeDelta,
		comparison.Summary.WarningDelta,
		comparison.Summary.AddedNodes,
		comparison.Summary.RemovedNodes,
		comparison.Summary.ChangedNodes,
		comparison.Summary.ReorderedNodes,
		comparison.Summary.ScoreChangedNodes,
		comparison.Summary.SignalChangedNodes,
	); err != nil {
		return err
	}
	for _, diff := range comparison.NodeDiffs {
		if _, err := fmt.Fprintf(
			out,
			"compile-review-compare-node: change=%s manifest=%s capability=%s base_index=%d candidate_index=%d base_score=%s candidate_score=%s base_signals=%s candidate_signals=%s\n",
			diff.Change,
			diff.ManifestName,
			diff.Capability,
			diff.BaseIndex,
			diff.CandidateIndex,
			formatFloat(diff.BaseScore),
			formatFloat(diff.CandidateScore),
			formatSelectionSignals(diff.BaseSignals),
			formatSelectionSignals(diff.CandidateSignals),
		); err != nil {
			return err
		}
	}
	for _, diff := range comparison.WarningDiffs {
		if _, err := fmt.Fprintf(out, "compile-review-compare-warning: change=%s warning=%s\n", diff.Change, diff.Warning); err != nil {
			return err
		}
	}
	return nil
}

func printToolCompilePreviewReviewSelection(out io.Writer, selection app.ToolCompilePreviewReviewSelection) error {
	actorFields := fmt.Sprintf("selected_by=%s selected_at=%s", selection.SelectedBy, selection.SelectedAt)
	if selection.Status == "cleared" {
		actorFields = fmt.Sprintf("cleared_by=%s cleared_at=%s", selection.ClearedBy, selection.ClearedAt)
	}
	if _, err := fmt.Fprintf(
		out,
		"compile-preview-review-selection: status=%s mode=%s review=%s action=%s nodes=%d warnings=%d superseded=%d %s reason=%s\n",
		selection.Status,
		selection.Mode,
		selection.ReviewPath,
		selection.Action,
		selection.ReviewNodes,
		selection.ReviewWarnings,
		len(selection.SupersededPaths),
		actorFields,
		selection.Reason,
	); err != nil {
		return err
	}
	for _, path := range selection.SupersededPaths {
		if _, err := fmt.Fprintf(out, "compile-review-superseded: %s\n", path); err != nil {
			return err
		}
	}
	return nil
}

func newToolchainCompilePreviewSignalsCommand(out io.Writer) *cobra.Command {
	var write bool
	var maxPlanNodes int
	var domainPriorityArgs []string
	var frontierScoreArgs []string
	var historyArgs []string
	cmd := &cobra.Command{
		Use:   "compile-preview-signals <workspace>",
		Short: "Write or inspect metadata-only compile preview scoring signals",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if maxPlanNodes < 0 {
				return fmt.Errorf("max-plan-nodes must be non-negative")
			}
			if !write {
				if maxPlanNodes != 0 || len(domainPriorityArgs) > 0 || len(frontierScoreArgs) > 0 || len(historyArgs) > 0 {
					return fmt.Errorf("compile-preview-signals mutation requires --write")
				}
				signals, found, err := app.LoadToolCompilePreviewSignals(args[0])
				if err != nil {
					return err
				}
				status := "missing"
				if found {
					status = "present"
				}
				return printCompilePreviewSignalsSummary(out, filepath.Join(args[0], "toolchain", "compile-preview-signals.json"), status, "metadata-only", signals)
			}
			domainPriorities, err := parseCompilePreviewSignalDomainPriorities(domainPriorityArgs)
			if err != nil {
				return err
			}
			frontierScores, err := parseCompilePreviewSignalFrontierScores(frontierScoreArgs)
			if err != nil {
				return err
			}
			history, err := parseCompilePreviewSignalHistory(historyArgs)
			if err != nil {
				return err
			}
			artifact, err := app.WriteToolCompilePreviewSignals(args[0], app.ToolCompilePreviewSignals{
				Budget: app.ToolCompilePreviewSignalBudget{
					MaxPlanNodes: maxPlanNodes,
				},
				DomainPriorities: domainPriorities,
				FrontierScores:   frontierScores,
				HistoricalStats:  history,
			})
			if err != nil {
				return err
			}
			return printCompilePreviewSignalsSummary(out, artifact.Path, artifact.Status, artifact.Mode, artifact.Signals)
		},
	}
	cmd.Flags().BoolVar(&write, "write", false, "write compile preview signals metadata")
	cmd.Flags().IntVar(&maxPlanNodes, "max-plan-nodes", 0, "maximum plan nodes to persist; 0 means unlimited")
	cmd.Flags().StringArrayVar(&domainPriorityArgs, "domain-priority", nil, "domain priority as domain=weight, e.g. infra_internal=5")
	cmd.Flags().StringArrayVar(&frontierScoreArgs, "frontier-score", nil, "candidate frontier score as manifest:capability=metric=value,...")
	cmd.Flags().StringArrayVar(&historyArgs, "history", nil, "candidate history as manifest:capability=successes=n,failures=n")
	return cmd
}

func newToolchainCompilePreviewSignalsFromReplayCommand(out io.Writer) *cobra.Command {
	var write bool
	var maxPlanNodes int
	cmd := &cobra.Command{
		Use:   "compile-preview-signals-from-replay <workspace> <run-id>",
		Short: "Derive metadata-only compile preview signals from replay events",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if maxPlanNodes < 0 {
				return fmt.Errorf("max-plan-nodes must be non-negative")
			}
			derivation, err := app.DeriveToolCompilePreviewSignalsFromReplay(context.Background(), args[0], args[1], app.ToolCompilePreviewSignalDeriveOptions{
				Write:        write,
				MaxPlanNodes: maxPlanNodes,
			})
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(
				out,
				"compile-preview-signals-from-replay: workspace=%s run=%s status=%s mode=%s path=%s max_plan_nodes=%d historical_stats=%d successes=%d failures=%d ignored=%d frontier_scores=%d\n",
				args[0],
				derivation.RunID,
				derivation.Status,
				derivation.Mode,
				derivation.Path,
				derivation.Signals.Budget.MaxPlanNodes,
				len(derivation.Signals.HistoricalStats),
				derivation.Summary.Successes,
				derivation.Summary.Failures,
				derivation.Summary.Ignored,
				len(derivation.Signals.FrontierScores),
			); err != nil {
				return err
			}
			for _, history := range derivation.Signals.HistoricalStats {
				if _, err := fmt.Fprintf(
					out,
					"signal-history: manifest=%s capability=%s successes=%d failures=%d\n",
					history.ManifestName,
					history.Capability,
					history.Successes,
					history.Failures,
				); err != nil {
					return err
				}
			}
			for _, score := range derivation.Signals.FrontierScores {
				if _, err := fmt.Fprintf(
					out,
					"signal-frontier: manifest=%s capability=%s id=%s target=%s score=%s\n",
					score.ManifestName,
					score.Capability,
					score.Path.ID,
					score.Path.Target,
					formatFloat(compilePreviewSignalFrontierScore(score.Path)),
				); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&write, "write", false, "write derived compile preview signals metadata")
	cmd.Flags().IntVar(&maxPlanNodes, "max-plan-nodes", 0, "maximum plan nodes to persist when writing; 0 means unlimited")
	return cmd
}

func printCompilePreviewSignalsSummary(out io.Writer, path, status, mode string, signals app.ToolCompilePreviewSignals) error {
	_, err := fmt.Fprintf(
		out,
		"compile-preview-signals: path=%s status=%s mode=%s max_plan_nodes=%d domain_priorities=%d frontier_scores=%d historical_stats=%d\n",
		path,
		status,
		mode,
		signals.Budget.MaxPlanNodes,
		len(signals.DomainPriorities),
		len(signals.FrontierScores),
		len(signals.HistoricalStats),
	)
	return err
}

func compilePreviewSignalFrontierScore(path app.ToolCompilePreviewSignalPath) float64 {
	return frontier.Score(frontier.CandidatePath{
		ID:                 strings.TrimSpace(path.ID),
		Target:             strings.TrimSpace(path.Target),
		Hypothesis:         strings.TrimSpace(path.Hypothesis),
		ExpectedUnlock:     strings.TrimSpace(path.ExpectedUnlock),
		MissionValue:       path.MissionValue,
		UnlockPotential:    path.UnlockPotential,
		EvidenceConfidence: path.EvidenceConfidence,
		Novelty:            path.Novelty,
		Cost:               path.Cost,
		DuplicateRisk:      path.DuplicateRisk,
		Noise:              path.Noise,
		ExhaustedPenalty:   path.ExhaustedPenalty,
	})
}

func parseCompilePreviewAction(value string) (compiler.SemanticAction, error) {
	action := compiler.SemanticAction(strings.TrimSpace(value))
	switch action {
	case compiler.ActionDiscoverAssets,
		compiler.ActionFingerprintSurface,
		compiler.ActionEnumeratePaths,
		compiler.ActionGenerateCandidates,
		compiler.ActionVerifyCandidate,
		compiler.ActionCollectEvidence,
		compiler.ActionProduceReport:
		return action, nil
	default:
		return "", fmt.Errorf("unsupported compile action: %s", value)
	}
}

func parseCompilePreviewDomainPriorities(values []string) ([]compiler.DomainPriority, error) {
	priorities := make([]compiler.DomainPriority, 0, len(values))
	for _, value := range values {
		name, weightText, ok := strings.Cut(value, "=")
		if !ok {
			return nil, fmt.Errorf("domain-priority must be domain=weight: %s", value)
		}
		domain, err := parseCompilePreviewDomain(name)
		if err != nil {
			return nil, err
		}
		weight, err := strconv.ParseFloat(strings.TrimSpace(weightText), 64)
		if err != nil {
			return nil, fmt.Errorf("parse domain-priority weight %s: %w", value, err)
		}
		priorities = append(priorities, compiler.DomainPriority{
			Domain: domain,
			Weight: weight,
		})
	}
	return priorities, nil
}

func parseCompilePreviewSignalDomainPriorities(values []string) ([]app.ToolCompilePreviewSignalDomainPriority, error) {
	priorities, err := parseCompilePreviewDomainPriorities(values)
	if err != nil {
		return nil, err
	}
	signals := make([]app.ToolCompilePreviewSignalDomainPriority, 0, len(priorities))
	for _, priority := range priorities {
		signals = append(signals, app.ToolCompilePreviewSignalDomainPriority{
			Domain: string(priority.Domain),
			Weight: priority.Weight,
		})
	}
	return signals, nil
}

func parseCompilePreviewSignalFrontierScores(values []string) ([]app.ToolCompilePreviewSignalFrontierScore, error) {
	scores := make([]app.ToolCompilePreviewSignalFrontierScore, 0, len(values))
	for _, value := range values {
		manifestName, capability, metricText, err := parseCompilePreviewSignalSelector(value, "frontier-score")
		if err != nil {
			return nil, err
		}
		fields, err := parseCompilePreviewSignalFields(metricText, "frontier-score")
		if err != nil {
			return nil, err
		}
		path := app.ToolCompilePreviewSignalPath{}
		for key, text := range fields {
			switch key {
			case "id":
				path.ID = text
			case "target":
				path.Target = text
			case "hypothesis":
				path.Hypothesis = text
			case "expected_unlock":
				path.ExpectedUnlock = text
			case "mission_value":
				path.MissionValue, err = parseCompilePreviewSignalFloat(value, key, text)
			case "unlock_potential":
				path.UnlockPotential, err = parseCompilePreviewSignalFloat(value, key, text)
			case "evidence_confidence":
				path.EvidenceConfidence, err = parseCompilePreviewSignalFloat(value, key, text)
			case "novelty":
				path.Novelty, err = parseCompilePreviewSignalFloat(value, key, text)
			case "cost":
				path.Cost, err = parseCompilePreviewSignalFloat(value, key, text)
			case "duplicate_risk":
				path.DuplicateRisk, err = parseCompilePreviewSignalFloat(value, key, text)
			case "noise":
				path.Noise, err = parseCompilePreviewSignalFloat(value, key, text)
			case "exhausted_penalty":
				path.ExhaustedPenalty, err = parseCompilePreviewSignalFloat(value, key, text)
			default:
				return nil, fmt.Errorf("unsupported frontier-score metric %q in %s", key, value)
			}
			if err != nil {
				return nil, err
			}
		}
		scores = append(scores, app.ToolCompilePreviewSignalFrontierScore{
			ManifestName: manifestName,
			Capability:   capability,
			Path:         path,
		})
	}
	return scores, nil
}

func parseCompilePreviewSignalHistory(values []string) ([]app.ToolCompilePreviewSignalHistory, error) {
	history := make([]app.ToolCompilePreviewSignalHistory, 0, len(values))
	for _, value := range values {
		manifestName, capability, metricText, err := parseCompilePreviewSignalSelector(value, "history")
		if err != nil {
			return nil, err
		}
		fields, err := parseCompilePreviewSignalFields(metricText, "history")
		if err != nil {
			return nil, err
		}
		entry := app.ToolCompilePreviewSignalHistory{
			ManifestName: manifestName,
			Capability:   capability,
		}
		for key, text := range fields {
			switch key {
			case "successes":
				entry.Successes, err = parseCompilePreviewSignalInt(value, key, text)
			case "failures":
				entry.Failures, err = parseCompilePreviewSignalInt(value, key, text)
			default:
				return nil, fmt.Errorf("unsupported history metric %q in %s", key, value)
			}
			if err != nil {
				return nil, err
			}
		}
		history = append(history, entry)
	}
	return history, nil
}

func parseCompilePreviewSignalSelector(value, name string) (string, string, string, error) {
	selector, metricText, ok := strings.Cut(value, "=")
	if !ok {
		return "", "", "", fmt.Errorf("%s must be manifest:capability=key=value,...: %s", name, value)
	}
	manifestName, capability, ok := strings.Cut(strings.TrimSpace(selector), ":")
	if !ok {
		return "", "", "", fmt.Errorf("%s selector must be manifest:capability: %s", name, value)
	}
	manifestName = strings.TrimSpace(manifestName)
	capability = strings.TrimSpace(capability)
	if manifestName == "" || capability == "" {
		return "", "", "", fmt.Errorf("%s selector must include manifest and capability: %s", name, value)
	}
	return manifestName, capability, strings.TrimSpace(metricText), nil
}

func parseCompilePreviewSignalFields(value, name string) (map[string]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, fmt.Errorf("%s metrics must not be empty", name)
	}
	fields := map[string]string{}
	for _, field := range strings.Split(value, ",") {
		key, text, ok := strings.Cut(field, "=")
		if !ok {
			return nil, fmt.Errorf("%s metric must be key=value: %s", name, field)
		}
		key = strings.TrimSpace(key)
		text = strings.TrimSpace(text)
		if key == "" {
			return nil, fmt.Errorf("%s metric key must not be empty", name)
		}
		fields[key] = text
	}
	return fields, nil
}

func parseCompilePreviewSignalFloat(source, key, value string) (float64, error) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s metric %s in %s: %w", key, value, source, err)
	}
	return parsed, nil
}

func parseCompilePreviewSignalInt(source, key, value string) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("parse %s metric %s in %s: %w", key, value, source, err)
	}
	if parsed < 0 {
		return 0, fmt.Errorf("%s metric must be non-negative in %s", key, source)
	}
	return parsed, nil
}

func parseCompilePreviewDomain(value string) (frontier.Domain, error) {
	switch strings.TrimSpace(value) {
	case string(frontier.DomainWebAPI), "web":
		return frontier.DomainWebAPI, nil
	case string(frontier.DomainInfra), "infra":
		return frontier.DomainInfra, nil
	case string(frontier.DomainCTF), "ctf":
		return frontier.DomainCTF, nil
	default:
		return "", fmt.Errorf("unsupported compile-preview domain: %s", value)
	}
}

func formatSelectionSignals(signals []string) string {
	if len(signals) == 0 {
		return "-"
	}
	return strings.Join(signals, ",")
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}

func newToolchainInstallPlanCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "install-plan <workspace>",
		Short: "Print non-executing toolchain dependency install steps",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			statuses, err := app.InspectToolchains(args[0])
			if err != nil {
				return err
			}
			count := 0
			for _, status := range statuses {
				for _, step := range status.InstallPlan {
					count++
					if _, err := fmt.Fprintf(
						out,
						"install-plan: %s/%s kind=%s state=%s required=%t command=%s hint=%s source=%s digest=%s depends_on=%s\n",
						status.Name,
						step.Name,
						step.Kind,
						step.State,
						step.Required,
						formatCommandJSON(step.Command),
						step.Hint,
						step.Source,
						step.ImageDigest,
						formatDependsOn(step.DependsOn),
					); err != nil {
						return err
					}
				}
			}
			if count == 0 {
				_, err := fmt.Fprintln(out, "install-plan: empty")
				return err
			}
			return nil
		},
	}
}

func newToolchainInstallRequestCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "install-request <workspace> <pack> <dependency>",
		Short: "Write a pending metadata-only toolchain install request",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			request, err := app.CreateToolchainInstallRequest(args[0], args[1], args[2])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "install-request: %s\n", request.Path); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "status: %s\n", request.Status); err != nil {
				return err
			}
			_, err = fmt.Fprintf(out, "command=%s depends_on=%s\n", formatCommandJSON(request.Command), formatDependsOn(request.DependsOn))
			return err
		},
	}
}

func newToolchainInstallRequestsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "install-requests <workspace>",
		Short: "List pending metadata-only toolchain install requests",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			requests, err := app.ListToolchainInstallRequests(args[0])
			if err != nil {
				return err
			}
			summary := app.SummarizeToolchainInstallRequests(requests)
			if _, err := fmt.Fprintf(out, "install-requests: %s\n", summary.Line()); err != nil {
				return err
			}
			if len(requests) == 0 {
				return nil
			}
			for _, request := range requests {
				if _, err := fmt.Fprintf(
					out,
					"install-request: %s status=%s pack=%s dependency=%s command=%s depends_on=%s\n",
					request.Path,
					request.Status,
					request.Pack,
					request.Dependency,
					formatCommandJSON(request.Command),
					formatDependsOn(request.DependsOn),
				); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newToolchainInstallDispatchPlanCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "install-dispatch-plan <workspace>",
		Short: "Print approved metadata-only toolchain install dispatch candidates",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			plan, err := app.PlanApprovedToolchainInstalls(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(
				out,
				"install-dispatch-plan: total=%d ready=%d skipped=%d pending=%d approved=%d denied=%d\n",
				plan.Summary.Total,
				len(plan.Ready),
				len(plan.Skipped),
				plan.Summary.Pending,
				plan.Summary.Approved,
				plan.Summary.Denied,
			); err != nil {
				return err
			}
			for _, request := range plan.Ready {
				if _, err := fmt.Fprintf(
					out,
					"dispatch-install: %s status=%s pack=%s dependency=%s command=%s mode=metadata-only depends_on=%s\n",
					request.Path,
					request.Status,
					request.Pack,
					request.Dependency,
					formatCommandJSON(request.Command),
					formatDependsOn(request.DependsOn),
				); err != nil {
					return err
				}
			}
			for _, skipped := range plan.Skipped {
				request := skipped.Request
				if _, err := fmt.Fprintf(
					out,
					"skip-install-request: %s status=%s pack=%s dependency=%s depends_on=%s reason=%s\n",
					request.Path,
					request.Status,
					request.Pack,
					request.Dependency,
					formatDependsOn(request.DependsOn),
					skipped.Reason,
				); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newToolchainInstallDispatchCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "install-dispatch <workspace>",
		Short: "Write a dry-run metadata-only toolchain install dispatch artifact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dispatch, err := app.CreateToolchainInstallDispatch(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(
				out,
				"install-dispatch: %s status=%s mode=%s ready=%d skipped=%d\n",
				dispatch.Path,
				dispatch.Status,
				dispatch.Mode,
				len(dispatch.Ready),
				len(dispatch.Skipped),
			); err != nil {
				return err
			}
			for _, request := range dispatch.Ready {
				if _, err := fmt.Fprintf(
					out,
					"dispatch-install: %s status=%s pack=%s dependency=%s command=%s mode=%s depends_on=%s\n",
					request.Path,
					request.Status,
					request.Pack,
					request.Dependency,
					formatCommandJSON(request.Command),
					dispatch.Mode,
					formatDependsOn(request.DependsOn),
				); err != nil {
					return err
				}
			}
			for _, skipped := range dispatch.Skipped {
				request := skipped.Request
				if _, err := fmt.Fprintf(
					out,
					"skip-install-request: %s status=%s pack=%s dependency=%s depends_on=%s reason=%s\n",
					request.Path,
					request.Status,
					request.Pack,
					request.Dependency,
					formatDependsOn(request.DependsOn),
					skipped.Reason,
				); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newToolchainInstallRetryPlanCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "install-retry-plan <workspace>",
		Short: "Print metadata-only retry candidates for failed toolchain install executions",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			plan, err := app.PlanFailedToolchainInstallRetries(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(
				out,
				"install-retry-plan: total=%d ready=%d skipped=%d\n",
				plan.Summary.Total,
				plan.Summary.Ready,
				plan.Summary.Skipped,
			); err != nil {
				return err
			}
			for _, candidate := range plan.Ready {
				if _, err := fmt.Fprintf(
					out,
					"retry-install: %s dispatch=%s request=%s pack=%s dependency=%s status=%s command=%s mode=metadata-only error=%s\n",
					candidate.ExecutionPath,
					candidate.DispatchPath,
					candidate.RequestPath,
					candidate.Pack,
					candidate.Dependency,
					candidate.LastStatus,
					formatCommandJSON(candidate.Command),
					candidate.LastError,
				); err != nil {
					return err
				}
			}
			for _, skipped := range plan.Skipped {
				if _, err := fmt.Fprintf(
					out,
					"skip-install-result: %s dispatch=%s request=%s pack=%s dependency=%s status=%s reason=%s\n",
					skipped.ExecutionPath,
					skipped.DispatchPath,
					skipped.RequestPath,
					skipped.Pack,
					skipped.Dependency,
					skipped.Status,
					skipped.Reason,
				); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newToolchainInstallRetryDispatchCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "install-retry-dispatch <workspace>",
		Short: "Write a dry-run metadata-only retry dispatch artifact for failed installs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dispatch, err := app.CreateToolchainInstallRetryDispatch(args[0])
			if err != nil {
				return err
			}
			return printToolchainInstallRetryDispatch(out, dispatch)
		},
	}
}

func printToolchainInstallRetryDispatch(out io.Writer, dispatch app.ToolchainInstallRetryDispatch) error {
	if _, err := fmt.Fprintf(
		out,
		"install-retry-dispatch: %s status=%s mode=%s ready=%d skipped=%d\n",
		dispatch.Path,
		dispatch.Status,
		dispatch.Mode,
		len(dispatch.Ready),
		len(dispatch.Skipped),
	); err != nil {
		return err
	}
	for _, candidate := range dispatch.Ready {
		if _, err := fmt.Fprintf(
			out,
			"retry-install: %s dispatch=%s request=%s pack=%s dependency=%s status=%s command=%s mode=%s error=%s\n",
			candidate.ExecutionPath,
			candidate.DispatchPath,
			candidate.RequestPath,
			candidate.Pack,
			candidate.Dependency,
			candidate.LastStatus,
			formatCommandJSON(candidate.Command),
			dispatch.Mode,
			candidate.LastError,
		); err != nil {
			return err
		}
	}
	for _, skipped := range dispatch.Skipped {
		if _, err := fmt.Fprintf(
			out,
			"skip-install-result: %s dispatch=%s request=%s pack=%s dependency=%s status=%s reason=%s\n",
			skipped.ExecutionPath,
			skipped.DispatchPath,
			skipped.RequestPath,
			skipped.Pack,
			skipped.Dependency,
			skipped.Status,
			skipped.Reason,
		); err != nil {
			return err
		}
	}
	return nil
}

func newToolchainInstallRetryDispatchesCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "install-retry-dispatches <workspace> [dispatch]",
		Short: "List or inspect dry-run metadata-only retry dispatch artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				dispatch, err := app.ReadToolchainInstallRetryDispatch(args[0], args[1])
				if err != nil {
					return err
				}
				return printToolchainInstallRetryDispatch(out, dispatch)
			}
			dispatches, err := app.ListToolchainInstallRetryDispatches(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "install-retry-dispatches: total=%d\n", len(dispatches)); err != nil {
				return err
			}
			for _, dispatch := range dispatches {
				if err := printToolchainInstallRetryDispatch(out, dispatch); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newToolchainExecuteInstallRetryDispatchCommand(out io.Writer) *cobra.Command {
	var execute bool
	command := &cobra.Command{
		Use:   "execute-install-retry-dispatch <workspace> <dispatch>",
		Short: "Execute approved retry install commands from a dry-run retry dispatch artifact",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			execution, err := app.ExecuteToolchainInstallRetryDispatch(args[0], args[1], app.ToolchainInstallRetryDispatchExecutionOptions{Execute: execute})
			if err != nil {
				return err
			}
			return printToolchainInstallRetryDispatchExecution(out, execution)
		},
	}
	command.Flags().BoolVar(&execute, "execute", false, "actually execute approved retry install command arrays")
	return command
}

func printToolchainInstallRetryDispatchExecution(out io.Writer, execution app.ToolchainInstallRetryDispatchExecution) error {
	return printToolchainInstallRetryExecution(out, "install-retry-dispatch-execution", execution)
}

func newToolchainInstallRetryExecutionsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "install-retry-executions <workspace> [execution]",
		Short: "List or inspect toolchain install retry execution audit artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				execution, err := app.ReadToolchainInstallRetryDispatchExecution(args[0], args[1])
				if err != nil {
					return err
				}
				return printToolchainInstallRetryExecution(out, "install-retry-execution", execution)
			}
			executions, err := app.ListToolchainInstallRetryDispatchExecutions(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "install-retry-executions: total=%d\n", len(executions)); err != nil {
				return err
			}
			for _, execution := range executions {
				if err := printToolchainInstallRetryExecution(out, "install-retry-execution", execution); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newToolchainInstallOrchestrateCommand(out io.Writer) *cobra.Command {
	var execute bool
	command := &cobra.Command{
		Use:   "install-orchestrate <workspace>",
		Short: "Write a toolchain install orchestration audit and optionally execute approved installs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			orchestration, err := app.CreateToolchainInstallOrchestration(args[0], app.ToolchainInstallOrchestrationOptions{Execute: execute})
			if err != nil && strings.TrimSpace(orchestration.Path) == "" && strings.TrimSpace(orchestration.Status) == "" {
				return err
			}
			if printErr := printToolchainInstallOrchestration(out, orchestration); printErr != nil {
				return printErr
			}
			return err
		},
	}
	command.Flags().BoolVar(&execute, "execute", false, "actually execute already-approved install command arrays and write retry dispatches for failures")
	return command
}

func newToolchainInstallOrchestrationsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "install-orchestrations <workspace> [orchestration]",
		Short: "List or inspect toolchain install orchestration audit artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				orchestration, err := app.ReadToolchainInstallOrchestration(args[0], args[1])
				if err != nil {
					return err
				}
				return printToolchainInstallOrchestration(out, orchestration)
			}
			orchestrations, err := app.ListToolchainInstallOrchestrations(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "install-orchestrations: total=%d\n", len(orchestrations)); err != nil {
				return err
			}
			for _, orchestration := range orchestrations {
				if err := printToolchainInstallOrchestration(out, orchestration); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func printToolchainInstallOrchestration(out io.Writer, orchestration app.ToolchainInstallOrchestration) error {
	if _, err := fmt.Fprintf(
		out,
		"install-orchestration: %s status=%s mode=%s requests=%s dispatch_ready=%d dispatch_skipped=%d retry_ready=%d retry_skipped=%d results=%d skipped=%d\n",
		orchestration.Path,
		orchestration.Status,
		orchestration.Mode,
		orchestration.RequestSummary.Line(),
		orchestration.DispatchReady,
		orchestration.DispatchSkipped,
		orchestration.RetryReady,
		orchestration.RetrySkipped,
		orchestration.ExecutionResults,
		orchestration.ExecutionSkipped,
	); err != nil {
		return err
	}
	if strings.TrimSpace(orchestration.DispatchPath) != "" {
		if _, err := fmt.Fprintf(out, "install-dispatch=%s status=%s\n", orchestration.DispatchPath, orchestration.DispatchStatus); err != nil {
			return err
		}
	}
	if strings.TrimSpace(orchestration.ExecutionPath) != "" {
		if _, err := fmt.Fprintf(
			out,
			"install-execution=%s status=%s results=%d skipped=%d\n",
			orchestration.ExecutionPath,
			orchestration.ExecutionStatus,
			orchestration.ExecutionResults,
			orchestration.ExecutionSkipped,
		); err != nil {
			return err
		}
	}
	if strings.TrimSpace(orchestration.RetryDispatchPath) != "" {
		if _, err := fmt.Fprintf(out, "install-retry-dispatch=%s status=%s\n", orchestration.RetryDispatchPath, orchestration.RetryDispatchStatus); err != nil {
			return err
		}
	}
	for _, warning := range orchestration.Warnings {
		if _, err := fmt.Fprintf(out, "warning: %s\n", warning); err != nil {
			return err
		}
	}
	if strings.TrimSpace(orchestration.Error) != "" {
		_, err := fmt.Fprintf(out, "error: %s\n", strings.Join(strings.Fields(orchestration.Error), " "))
		return err
	}
	return nil
}

func printToolchainInstallRetryExecution(out io.Writer, label string, execution app.ToolchainInstallRetryDispatchExecution) error {
	if _, err := fmt.Fprintf(
		out,
		"%s: %s status=%s mode=%s retry_dispatch=%s results=%d skipped=%d\n",
		label,
		execution.Path,
		execution.Status,
		execution.Mode,
		execution.RetryDispatchPath,
		len(execution.Results),
		len(execution.Skipped),
	); err != nil {
		return err
	}
	for _, warning := range execution.Warnings {
		if _, err := fmt.Fprintf(out, "warning: %s\n", warning); err != nil {
			return err
		}
	}
	for _, result := range execution.Results {
		if _, err := fmt.Fprintf(
			out,
			"retry-install-execute: pack=%s dependency=%s status=%s command=%s source_execution=%s dispatch=%s output=%s error=%s\n",
			result.Pack,
			result.Dependency,
			result.Status,
			formatCommandJSON(result.Command),
			result.ExecutionPath,
			result.DispatchPath,
			strings.Join(strings.Fields(result.Output), " "),
			strings.Join(strings.Fields(result.Error), " "),
		); err != nil {
			return err
		}
	}
	for _, skipped := range execution.Skipped {
		if _, err := fmt.Fprintf(
			out,
			"skip-install-result: %s dispatch=%s request=%s pack=%s dependency=%s status=%s reason=%s\n",
			skipped.ExecutionPath,
			skipped.DispatchPath,
			skipped.RequestPath,
			skipped.Pack,
			skipped.Dependency,
			skipped.Status,
			skipped.Reason,
		); err != nil {
			return err
		}
	}
	return nil
}

func newToolchainInstallDispatchesCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "install-dispatches <workspace> [dispatch]",
		Short: "List or inspect dry-run metadata-only toolchain install dispatch artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				dispatch, err := app.ReadToolchainInstallDispatch(args[0], args[1])
				if err != nil {
					return err
				}
				return printToolchainInstallDispatch(out, dispatch)
			}
			dispatches, err := app.ListToolchainInstallDispatches(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "install-dispatches: total=%d\n", len(dispatches)); err != nil {
				return err
			}
			for _, dispatch := range dispatches {
				if err := printToolchainInstallDispatch(out, dispatch); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func printToolchainInstallDispatch(out io.Writer, dispatch app.ToolchainInstallDispatch) error {
	if _, err := fmt.Fprintf(
		out,
		"install-dispatch: %s status=%s mode=%s ready=%d skipped=%d\n",
		dispatch.Path,
		dispatch.Status,
		dispatch.Mode,
		len(dispatch.Ready),
		len(dispatch.Skipped),
	); err != nil {
		return err
	}
	for _, request := range dispatch.Ready {
		if _, err := fmt.Fprintf(
			out,
			"dispatch-install: %s status=%s pack=%s dependency=%s command=%s mode=%s depends_on=%s\n",
			request.Path,
			request.Status,
			request.Pack,
			request.Dependency,
			formatCommandJSON(request.Command),
			dispatch.Mode,
			formatDependsOn(request.DependsOn),
		); err != nil {
			return err
		}
	}
	for _, skipped := range dispatch.Skipped {
		request := skipped.Request
		if _, err := fmt.Fprintf(
			out,
			"skip-install-request: %s status=%s pack=%s dependency=%s depends_on=%s reason=%s\n",
			request.Path,
			request.Status,
			request.Pack,
			request.Dependency,
			formatDependsOn(request.DependsOn),
			skipped.Reason,
		); err != nil {
			return err
		}
	}
	return nil
}

func newToolchainExecuteInstallDispatchCommand(out io.Writer) *cobra.Command {
	var execute bool
	command := &cobra.Command{
		Use:   "execute-install-dispatch <workspace> <dispatch>",
		Short: "Execute approved install commands from a dry-run dispatch artifact",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			execution, err := app.ExecuteToolchainInstallDispatch(args[0], args[1], app.ToolchainInstallDispatchExecutionOptions{Execute: execute})
			if err != nil {
				return err
			}
			return printToolchainInstallDispatchExecution(out, execution)
		},
	}
	command.Flags().BoolVar(&execute, "execute", false, "actually execute approved install command arrays")
	return command
}

func printToolchainInstallDispatchExecution(out io.Writer, execution app.ToolchainInstallDispatchExecution) error {
	return printToolchainInstallExecution(out, "install-dispatch-execution", execution)
}

func newToolchainInstallExecutionsCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "install-executions <workspace> [execution]",
		Short: "List or inspect toolchain install execution audit artifacts",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				execution, err := app.ReadToolchainInstallDispatchExecution(args[0], args[1])
				if err != nil {
					return err
				}
				return printToolchainInstallExecution(out, "install-execution", execution)
			}
			executions, err := app.ListToolchainInstallDispatchExecutions(args[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "install-executions: total=%d\n", len(executions)); err != nil {
				return err
			}
			for _, execution := range executions {
				if err := printToolchainInstallExecution(out, "install-execution", execution); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func printToolchainInstallExecution(out io.Writer, label string, execution app.ToolchainInstallDispatchExecution) error {
	if _, err := fmt.Fprintf(
		out,
		"%s: %s status=%s mode=%s dispatch=%s results=%d skipped=%d\n",
		label,
		execution.Path,
		execution.Status,
		execution.Mode,
		execution.DispatchPath,
		len(execution.Results),
		len(execution.Skipped),
	); err != nil {
		return err
	}
	for _, warning := range execution.Warnings {
		if _, err := fmt.Fprintf(out, "warning: %s\n", warning); err != nil {
			return err
		}
	}
	for _, result := range execution.Results {
		if _, err := fmt.Fprintf(
			out,
			"install-execute: pack=%s dependency=%s status=%s command=%s\n",
			result.Pack,
			result.Dependency,
			result.Status,
			formatCommandJSON(result.Command),
		); err != nil {
			return err
		}
	}
	for _, skipped := range execution.Skipped {
		request := skipped.Request
		if _, err := fmt.Fprintf(
			out,
			"skip-install-request: %s status=%s pack=%s dependency=%s depends_on=%s reason=%s\n",
			request.Path,
			request.Status,
			request.Pack,
			request.Dependency,
			formatDependsOn(request.DependsOn),
			skipped.Reason,
		); err != nil {
			return err
		}
	}
	return nil
}

func newToolchainApproveInstallRequestCommand(out io.Writer) *cobra.Command {
	return newToolchainInstallRequestStatusCommand(
		out,
		"approve-install-request <workspace> <request>",
		"Mark a pending toolchain install request approved",
		"approved",
	)
}

func newToolchainDenyInstallRequestCommand(out io.Writer) *cobra.Command {
	return newToolchainInstallRequestStatusCommand(
		out,
		"deny-install-request <workspace> <request>",
		"Mark a pending toolchain install request denied",
		"denied",
	)
}

func newToolchainInstallRequestStatusCommand(out io.Writer, use string, short string, status string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			request, err := app.UpdateToolchainInstallRequestStatus(args[0], args[1], status)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(
				out,
				"install-request: %s status=%s pack=%s dependency=%s command=%s depends_on=%s\n",
				request.Path,
				request.Status,
				request.Pack,
				request.Dependency,
				formatCommandJSON(request.Command),
				formatDependsOn(request.DependsOn),
			)
			return err
		},
	}
}

func formatDependsOn(dependsOn []string) string {
	filtered := make([]string, 0, len(dependsOn))
	for _, dependency := range dependsOn {
		dependency = strings.TrimSpace(dependency)
		if dependency != "" {
			filtered = append(filtered, dependency)
		}
	}
	if len(filtered) == 0 {
		return "none"
	}
	return strings.Join(filtered, ",")
}

func formatCommandJSON(command []string) string {
	data, err := json.Marshal(command)
	if err != nil {
		return "[]"
	}
	return string(data)
}
