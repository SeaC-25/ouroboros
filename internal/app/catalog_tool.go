package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
	"github.com/ouroboros/ouroboros/internal/toolchain"
)

// CatalogToolInvocation is the stable input contract for starting a built-in
// binary catalog operation outside the full RunWorkspace pipeline.
type CatalogToolInvocation struct {
	WorkspaceRoot     string
	ToolID            string
	Operation         string
	Target            string
	TargetKind        string
	Args              []string
	RunID             string
	ExecutionID       string
	PlanNodeID        string
	ScopeExclusions   []string
	ScopeTargetPolicy string
}

// CatalogToolInvocationResult contains the resolved command plus enough
// catalog and output context to collect artifacts and dispatch parser events.
type CatalogToolInvocationResult struct {
	WorkspaceRoot      string
	Executable         string
	Args               []string
	Argv               []string
	ToolID             string
	PackID             string
	AdapterID          string
	Operation          string
	ParserID           string
	DependencyName     string
	DependencyKind     string
	Capability         string
	Capabilities       []string
	Target             string
	ResolvedTarget     string
	TargetKind         string
	RunID              string
	ExecutionID        string
	PlanNodeID         string
	ScopeExclusions    []string
	ScopeTargetPolicy  string
	ManagedOutputPaths []string
	ManagedOutputFiles []string
	ManagedOutputDirs  []string
}

// ResolveCatalogToolInvocation resolves one binary catalog operation into the
// same managed argv used by RunWorkspace. It checks the workspace toolchain
// first and falls back to the host PATH only when execution is requested.
func ResolveCatalogToolInvocation(input CatalogToolInvocation) (CatalogToolInvocationResult, error) {
	workspaceRoot := strings.TrimSpace(input.WorkspaceRoot)
	if workspaceRoot == "" {
		return CatalogToolInvocationResult{}, fmt.Errorf("catalog tool invocation requires workspace root")
	}
	absRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return CatalogToolInvocationResult{}, fmt.Errorf("resolve catalog workspace root: %w", err)
	}
	tool, found := builtInCatalogTool(input.ToolID)
	if !found {
		return CatalogToolInvocationResult{}, fmt.Errorf("catalog tool is not registered: %s", strings.TrimSpace(input.ToolID))
	}
	if strings.TrimSpace(tool.DependencyKind) != "binary" {
		return CatalogToolInvocationResult{}, fmt.Errorf("catalog entry %s is not executable", tool.Name)
	}
	operation := strings.TrimSpace(input.Operation)
	if operation == "" {
		return CatalogToolInvocationResult{}, fmt.Errorf("catalog tool %s operation is required", tool.Name)
	}
	if operation != strings.TrimSpace(tool.Operation) {
		return CatalogToolInvocationResult{}, fmt.Errorf("catalog tool %s does not support operation %s", tool.Name, operation)
	}
	registryOperation, ok := toolchain.DefaultAuthoredToolRegistry().AdapterOperation(tool.AdapterID, operation)
	if !ok || !containsTrimmedValue(registryOperation.DependencyNames, tool.Dependency) {
		return CatalogToolInvocationResult{}, fmt.Errorf("catalog tool %s operation %s has no managed binary contract", tool.Name, operation)
	}
	target := strings.TrimSpace(input.Target)
	if target == "" {
		return CatalogToolInvocationResult{}, fmt.Errorf("catalog tool %s target is required", tool.Name)
	}
	targetKind := strings.TrimSpace(input.TargetKind)
	if targetKind == "" {
		return CatalogToolInvocationResult{}, fmt.Errorf("catalog tool %s target kind is required", tool.Name)
	}

	planNodeID := strings.TrimSpace(input.PlanNodeID)
	if planNodeID == "" {
		planNodeID = catalogPlanNodeID(tool.Name, input.ExecutionID)
	}
	capabilities := append([]string(nil), tool.Capabilities...)
	capability := ""
	if len(capabilities) > 0 {
		capability = strings.TrimSpace(capabilities[0])
	}
	dependencies := catalogToolDependencies(tool)
	request := ToolExecutionRequest{
		RunID:             strings.TrimSpace(input.RunID),
		WorkspaceRoot:     absRoot,
		ScopeTargets:      []ScopeTarget{{Kind: targetKind, Value: target}},
		ScopeExclusions:   append([]string(nil), input.ScopeExclusions...),
		ScopeTargetPolicy: strings.TrimSpace(input.ScopeTargetPolicy),
		IsolationMode:     "process",
		Node: compiler.PlanNode{
			ID:             planNodeID,
			Capability:     capability,
			ManifestName:   strings.TrimSpace(tool.Pack),
			ToolID:         strings.TrimSpace(tool.Name),
			AdapterID:      strings.TrimSpace(tool.AdapterID),
			Operation:      operation,
			ParserID:       strings.TrimSpace(tool.Parser),
			TargetKind:     targetKind,
			DependencyName: strings.TrimSpace(tool.Dependency),
			DependencyKind: "binary",
			Runner:         "native",
			Parser:         strings.TrimSpace(tool.Parser),
			Dependencies:   dependencies,
		},
		catalogInvocation: true,
	}
	command, selectedTarget, executable, err := nativeManagedRunCommandWithTarget(request)
	if err != nil {
		return CatalogToolInvocationResult{}, err
	}
	if !executable || len(command) == 0 {
		return CatalogToolInvocationResult{}, fmt.Errorf("catalog tool %s cannot build operation %s for target kind %s", tool.Name, operation, targetKind)
	}
	managedPaths, managedFiles, managedDirs, err := catalogManagedOutputLocations(absRoot, command)
	if err != nil {
		return CatalogToolInvocationResult{}, err
	}
	command = append(append([]string(nil), command...), input.Args...)
	result := CatalogToolInvocationResult{
		WorkspaceRoot:      absRoot,
		Executable:         command[0],
		Args:               append([]string(nil), command[1:]...),
		Argv:               append([]string(nil), command...),
		ToolID:             strings.TrimSpace(tool.Name),
		PackID:             strings.TrimSpace(tool.Pack),
		AdapterID:          strings.TrimSpace(tool.AdapterID),
		Operation:          operation,
		ParserID:           strings.TrimSpace(tool.Parser),
		DependencyName:     strings.TrimSpace(tool.Dependency),
		DependencyKind:     "binary",
		Capability:         capability,
		Capabilities:       capabilities,
		Target:             target,
		ResolvedTarget:     strings.TrimSpace(selectedTarget.Value),
		TargetKind:         targetKind,
		RunID:              strings.TrimSpace(input.RunID),
		ExecutionID:        strings.TrimSpace(input.ExecutionID),
		PlanNodeID:         planNodeID,
		ScopeExclusions:    append([]string(nil), input.ScopeExclusions...),
		ScopeTargetPolicy:  strings.TrimSpace(input.ScopeTargetPolicy),
		ManagedOutputPaths: managedPaths,
		ManagedOutputFiles: managedFiles,
		ManagedOutputDirs:  managedDirs,
	}
	if result.ResolvedTarget == "" {
		result.ResolvedTarget = target
	}
	return result, nil
}

// CatalogToolParserEvents dispatches a completed catalog invocation through
// the same specialized parsers used by RunWorkspace.
func CatalogToolParserEvents(invocation CatalogToolInvocationResult, execution ToolExecutionResult) []eventlog.Event {
	request := catalogToolExecutionRequest(invocation)
	if strings.TrimSpace(execution.ExecutionID) == "" {
		execution.ExecutionID = strings.TrimSpace(invocation.ExecutionID)
	}
	if transcript, ok, err := managedOutputTranscriptForRequest(request); err != nil {
		execution.Transcript = err.Error()
	} else if ok {
		execution.Transcript = transcript
	}
	return parserEventsForToolExecution(strings.TrimSpace(invocation.RunID), request, execution)
}

func builtInCatalogTool(toolID string) (toolchain.BuiltInTool, bool) {
	toolID = strings.TrimSpace(toolID)
	for _, tool := range toolchain.BuiltInAppSecTools() {
		if strings.TrimSpace(tool.Name) == toolID {
			return tool, true
		}
	}
	return toolchain.BuiltInTool{}, false
}

func catalogToolDependencies(tool toolchain.BuiltInTool) []toolchain.Dependency {
	dependencies := []toolchain.Dependency{{Name: strings.TrimSpace(tool.Dependency), Kind: "binary"}}
	for _, candidate := range toolchain.BuiltInAppSecTools() {
		if strings.TrimSpace(candidate.DependencyKind) != "rule_bundle" || strings.TrimSpace(candidate.RuleBundleFor) != strings.TrimSpace(tool.Name) {
			continue
		}
		dependencies = append(dependencies, toolchain.Dependency{
			Name: strings.TrimSpace(candidate.Dependency),
			Kind: "rule_bundle",
		})
	}
	return dependencies
}

func catalogPlanNodeID(toolID string, executionID string) string {
	parts := []string{"catalog", safeFactIDPart(toolID)}
	if executionID = strings.TrimSpace(executionID); executionID != "" {
		parts = append(parts, safeFactIDPart(executionID))
	}
	return strings.Join(parts, "-")
}

func catalogManagedOutputLocations(workspaceRoot string, command []string) ([]string, []string, []string, error) {
	managedRoot, err := filepath.Abs(filepath.Join(workspaceRoot, "toolchain", "managed-outputs"))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolve managed output root: %w", err)
	}
	paths := []string{}
	files := []string{}
	dirs := []string{}
	seen := map[string]bool{}
	for _, argument := range command[1:] {
		for _, candidate := range catalogOutputArgumentPaths(argument) {
			absolute, err := filepath.Abs(candidate)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("resolve managed output path: %w", err)
			}
			relative, err := filepath.Rel(managedRoot, absolute)
			if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
				continue
			}
			if seen[absolute] {
				continue
			}
			seen[absolute] = true
			paths = append(paths, absolute)
			if info, statErr := os.Stat(absolute); statErr == nil && info.IsDir() {
				dirs = append(dirs, absolute)
			} else {
				files = append(files, absolute)
			}
		}
	}
	sort.Strings(paths)
	sort.Strings(files)
	sort.Strings(dirs)
	return paths, files, dirs, nil
}

func catalogOutputArgumentPaths(argument string) []string {
	argument = strings.TrimSpace(argument)
	if argument == "" {
		return nil
	}
	candidates := []string{argument}
	if _, value, ok := strings.Cut(argument, "="); ok {
		candidates = append(candidates, strings.TrimSpace(value))
	}
	paths := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if filepath.IsAbs(candidate) {
			paths = append(paths, candidate)
		}
	}
	return paths
}

func catalogToolExecutionRequest(invocation CatalogToolInvocationResult) ToolExecutionRequest {
	return ToolExecutionRequest{
		RunID:             strings.TrimSpace(invocation.RunID),
		WorkspaceRoot:     strings.TrimSpace(invocation.WorkspaceRoot),
		ScopeTargets:      []ScopeTarget{{Kind: strings.TrimSpace(invocation.TargetKind), Value: strings.TrimSpace(invocation.Target)}},
		ScopeExclusions:   append([]string(nil), invocation.ScopeExclusions...),
		ScopeTargetPolicy: strings.TrimSpace(invocation.ScopeTargetPolicy),
		IsolationMode:     "process",
		Node: compiler.PlanNode{
			ID:             strings.TrimSpace(invocation.PlanNodeID),
			Capability:     strings.TrimSpace(invocation.Capability),
			ManifestName:   strings.TrimSpace(invocation.PackID),
			ToolID:         strings.TrimSpace(invocation.ToolID),
			AdapterID:      strings.TrimSpace(invocation.AdapterID),
			Operation:      strings.TrimSpace(invocation.Operation),
			ParserID:       strings.TrimSpace(invocation.ParserID),
			TargetKind:     strings.TrimSpace(invocation.TargetKind),
			DependencyName: strings.TrimSpace(invocation.DependencyName),
			DependencyKind: strings.TrimSpace(invocation.DependencyKind),
			Runner:         "native",
			Parser:         strings.TrimSpace(invocation.ParserID),
			Dependencies: []toolchain.Dependency{{
				Name: strings.TrimSpace(invocation.DependencyName),
				Kind: strings.TrimSpace(invocation.DependencyKind),
			}},
		},
		catalogInvocation: true,
	}
}
