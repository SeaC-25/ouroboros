package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ouroboros/ouroboros/internal/nativetools"
	"github.com/ouroboros/ouroboros/internal/toolchain"
)

// nativeGoAdapterID marks plan nodes that execute an in-process Go handler
// instead of a managed external binary (see docs/toolchain-go-roadmap.md).
const nativeGoAdapterID = nativetools.AdapterID

// nativeGoToolNode reports whether the node is a native Go tool node and, when
// so, its operation id.
func nativeGoToolNode(request ToolExecutionRequest) (string, bool) {
	node := request.Node
	if strings.TrimSpace(node.AdapterID) != nativeGoAdapterID {
		return "", false
	}
	operation := strings.TrimSpace(node.Operation)
	if operation == "" {
		return "", false
	}
	if _, ok := toolchain.DefaultAuthoredToolRegistry().AdapterOperation(nativeGoAdapterID, operation); !ok {
		return "", false
	}
	return operation, true
}

// executeNativeGoTool runs the in-process handler for a native Go tool node.
//
// The same gates that guard managed binaries are applied here: the node must
// resolve a scope target, and that target must survive exclusion checks (both
// enforced by requireCommandScopeTarget). Only the execution step differs (no
// PATH lookup, no subprocess), and the handler emits output byte-compatible
// with the external tool it replaces so parsers and the evidence chain stay
// untouched.
//
// It returns runnable=false when the node cannot run in this workspace — an
// unknown adapter, or a node whose required scope target is absent. That
// mirrors the managed path, where a tool whose binary is not installed also
// degrades to the offline runner instead of failing the whole run.
func executeNativeGoTool(ctx context.Context, request ToolExecutionRequest) (ToolExecutionResult, bool, error) {
	executionID := "native-" + safeExecutionID(request.Node.ID)
	result := ToolExecutionResult{
		ExecutionID: executionID,
		Status:      "failed",
		AttemptDir:  filepath.Join(request.WorkspaceRoot, "attempts", request.RunID, executionID),
	}

	operation, ok := nativeGoToolNode(request)
	if !ok {
		return result, false, nil
	}
	if err := requireCommandScopeTarget(request); err != nil {
		return result, false, nil
	}
	target, ok := nativeGoTargetFor(request)
	if !ok {
		return result, false, nil
	}

	if err := os.MkdirAll(result.AttemptDir, 0o755); err != nil {
		result.Transcript = err.Error()
		return result, true, fmt.Errorf("create attempt directory: %w", err)
	}

	runCtx := ctx
	if request.Node.RunTimeoutMS > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(request.Node.RunTimeoutMS)*time.Millisecond)
		defer cancel()
	}

	output, err := nativetools.Run(runCtx, operation, nativetools.Target{Kind: target.Kind, Value: target.Value}, nativetools.Options{
		Timeout:    nativeToolTimeout(request),
		MaxWorkers: nativeToolWorkers(request),
		ParserID:   parserIDForRequest(request),
	})
	if err != nil {
		result.Transcript = err.Error()
		return result, true, err
	}
	if strings.TrimSpace(output) == "" {
		output = "native tool completed without output"
	}
	result.Transcript = output
	result.Status = "completed"
	return result, true, nil
}

// nativeGoTargetFor selects the scope target the handler operates on, using the
// operation's preferred target kinds first and falling back to any non-excluded
// in-scope target.
func nativeGoTargetFor(request ToolExecutionRequest) (ScopeTarget, bool) {
	if len(request.ScopeTargets) == 0 {
		return ScopeTarget{}, false
	}
	for _, kind := range nativeGoPreferredTargetKinds(request.Node.Operation) {
		for _, candidate := range request.ScopeTargets {
			if strings.TrimSpace(candidate.Kind) != kind || strings.TrimSpace(candidate.Value) == "" {
				continue
			}
			if scopeTargetExcludedForRequest(request, candidate) {
				continue
			}
			return candidate, true
		}
	}
	for _, candidate := range request.ScopeTargets {
		if strings.TrimSpace(candidate.Value) == "" {
			continue
		}
		if scopeTargetExcludedForRequest(request, candidate) {
			continue
		}
		return candidate, true
	}
	return ScopeTarget{}, false
}

func nativeGoPreferredTargetKinds(operation string) []string {
	switch strings.TrimSpace(operation) {
	case nativetools.OpPortScan:
		return []string{"host", "ip", "cidr"}
	case nativetools.OpDNSProbe, nativetools.OpSubdomainDiscovery, nativetools.OpArchiveDiscovery:
		return []string{"domain", "host"}
	case nativetools.OpMetadataExtract, nativetools.OpSecretScan:
		return []string{"artifact", "path", "file"}
	default:
		return []string{"url", "host", "domain"}
	}
}

func nativeToolTimeout(request ToolExecutionRequest) time.Duration {
	if request.Node.RunTimeoutMS > 0 {
		return time.Duration(request.Node.RunTimeoutMS) * time.Millisecond
	}
	return 30 * time.Second
}

// nativeToolWorkers returns the handler concurrency limit. PlanNode carries no
// per-node concurrency budget, so the process-wide default applies.
func nativeToolWorkers(ToolExecutionRequest) int {
	return 10
}
