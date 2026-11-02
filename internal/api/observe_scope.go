package api

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/ouroboros/ouroboros/internal/app"
)

// authorizeObserveTarget enforces the workspace scope for the chat-driven
// observation primitive. Plan-node tool executions pass
// requireCommandScopeTarget; ACTION:OBSERVE must not be a silent bypass of
// the same gate. Policy:
//
//   - target_policy: disabled        → everything allowed (operator opt-out)
//   - otherwise                      → the URL must match an in-scope target
//     with kind=url (scheme+host equality, the same anti-prefix-trick rule
//     the crawler uses) and must not hit an exclusion entry.
func (s *Server) authorizeObserveTarget(targetURL string) error {
	scopeConfig, err := app.LoadScopeConfig(filepath.Join(s.workspace, "scope.yaml"))
	if err != nil {
		// No readable scope config: fail closed — the chat action must not
		// widen the attack surface beyond what the operator configured.
		return fmt.Errorf("无法读取工作区 scope.yaml（%v）；观测需要有效的 scope 配置", err)
	}
	if strings.TrimSpace(scopeConfig.Scope.TargetPolicy) == "disabled" {
		// Even with the policy disabled, infrastructure endpoints stay
		// non-targets: reachability never implies authorization to attack.
		// The model gets the classification in the error and self-corrects.
		if infErr := rejectInfrastructureTarget(scopeConfig, targetURL); infErr != nil {
			return infErr
		}
		return nil
	}

	parsed, err := url.Parse(targetURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("观测目标必须是绝对 URL: %q", targetURL)
	}
	// Infrastructure check first: a misconfigured scope listing an infra
	// host as a target is still infrastructure.
	if infErr := rejectInfrastructureTarget(scopeConfig, targetURL); infErr != nil {
		return infErr
	}
	for _, target := range scopeConfig.Scope.Targets {
		if strings.TrimSpace(target.Kind) != "url" || strings.TrimSpace(target.Value) == "" {
			continue
		}
		scoped, parseErr := url.Parse(strings.TrimSpace(target.Value))
		if parseErr != nil || scoped.Scheme == "" || scoped.Host == "" {
			continue
		}
		if parsed.Scheme == scoped.Scheme && parsed.Host == scoped.Host {
			return s.checkObserveExclusion(parsed, scopeConfig.Scope.Exclusions)
		}
	}
	return fmt.Errorf("观测目标 %q 不在工作区 scope 的 URL 目标内（scope.yaml targets）", targetURL)
}

// checkObserveExclusion rejects URLs covered by an exclusion entry. An
// exclusion matches when scheme+host are equal AND the exclusion's path is
// a prefix of the target's path (excluding the whole host requires the
// entry's path to be "/" or empty) — host equality alone must not swallow
// sibling paths that are still in scope.
func (s *Server) checkObserveExclusion(parsed *url.URL, exclusions []string) error {
	for _, exclusion := range exclusions {
		exclusion = strings.TrimSpace(exclusion)
		if exclusion == "" {
			continue
		}
		scoped, err := url.Parse(exclusion)
		if err != nil || scoped.Scheme == "" || scoped.Host == "" {
			continue
		}
		if parsed.Scheme != scoped.Scheme || parsed.Host != scoped.Host {
			continue
		}
		excludedPath := scoped.Path
		if excludedPath == "" {
			excludedPath = "/"
		}
		if excludedPath == "/" || strings.HasPrefix(parsed.Path, excludedPath) {
			return fmt.Errorf("观测目标 %q 命中 scope 排除项 %q", parsed.String(), exclusion)
		}
	}
	return nil
}

// rejectInfrastructureTarget classifies a URL against the scope's
// infrastructure list and rejects observations aimed at it. The error
// carries the classification so the model can redirect without another
// round of guessing.
func rejectInfrastructureTarget(config app.ScopeConfig, targetURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(targetURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil // malformed URLs fail later shape checks, not here
	}
	// Health-check allowance: a bare root GET to an infra endpoint is the
	// documented connectivity probe (VPN preflight). Deep paths (anything
	// beyond "/" or a probe query) stay rejected — probing is allowed,
	// attacking infrastructure is not.
	isBareProbe := (parsed.Path == "" || parsed.Path == "/") && parsed.RawQuery == ""
	for _, infra := range config.Scope.Infrastructure {
		if strings.TrimSpace(infra.Kind) != "url" || strings.TrimSpace(infra.Value) == "" {
			continue
		}
		infraURL, parseErr := url.Parse(strings.TrimSpace(infra.Value))
		if parseErr != nil || infraURL.Scheme == "" || infraURL.Host == "" {
			continue
		}
		if parsed.Scheme == infraURL.Scheme && parsed.Host == infraURL.Host {
			if isBareProbe {
				return nil
			}
			return fmt.Errorf("观测目标 %q 是基础设施端点（scope.yaml infrastructure：%s），不是靶场目标——靶场地址只来自任务指定的 start API 响应（根路径连通性预检除外）", targetURL, strings.TrimSpace(infra.Value))
		}
	}
	return nil
}
