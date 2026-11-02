package api

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ouroboros/ouroboros/internal/app"
)

// egressGateEnv is the operator's diagnostic kill switch for the execution
// egress gate. Only the exact value "0" disables the gate; every other value
// (including empty) keeps it armed. Diagnostics only — it is a physical-layer
// bypass, never a policy expression.
const egressGateEnv = "OURO_EGRESS_GATE"

// egressURLPattern extracts http(s) URLs from raw command text using the same
// shape as the OBSERVE gate: scheme + authority, stopping at the first
// character that cannot appear in a host or port (whitespace, quote, bracket,
// slash, shell metacharacter). Purely local commands contain no URL text and
// are never intercepted by this shape.
var egressURLPattern = regexp.MustCompile(`https?://[A-Za-z0-9.:-]+(/[A-Za-z0-9._~!$&*+,;=:@%/-]*)?`)

// extractEgressURLs returns the http(s) URLs embedded in raw command text,
// de-duplicated in first-appearance order. This is a textual scan of what the
// command visibly contains; matching still goes through url.Parse and
// scheme+host equality, the OBSERVE gate's rule. A command containing no URL
// text is pure local work and is out of this gate's jurisdiction.
func extractEgressURLs(command string) []string {
	matches := egressURLPattern.FindAllString(command, -1)
	seen := make(map[string]bool, len(matches))
	urls := make([]string, 0, len(matches))
	for _, match := range matches {
		if seen[match] {
			continue
		}
		seen[match] = true
		urls = append(urls, match)
	}
	return urls
}

// checkExecutionEgress enforces the workspace scope on the ACTION:EXECUTE
// path. Plan-node tool executions pass requireCommandScopeTarget; a shell
// command must not be a silent bypass of the same boundary. Policy (identical
// semantics to authorizeObserveTarget, S3 compliance gate):
//
//   - target_policy: disabled → non-infra commands pass (infra still rejected:
//     reachability never implies authorization to attack)
//   - otherwise (enforce)     → every http(s) URL in the command must match an
//     in-scope target with kind=url (scheme+host equality, the same
//     anti-prefix-trick rule the crawler uses)
//
// Commands without any URL text (pure local work) are always allowed: the
// gate audits egress, not the shell itself. The extraction runs before the
// scope config is even read, so an unreadable scope.yaml cannot block a
// purely local command.
func (s *Server) checkExecutionEgress(workspace, command string) error {
	if strings.TrimSpace(os.Getenv(egressGateEnv)) == "0" {
		return nil
	}

	urls := extractEgressURLs(command)
	if len(urls) == 0 {
		return nil
	}

	scopeConfig, err := app.LoadScopeConfig(filepath.Join(workspace, "scope.yaml"))
	if err != nil {
		// No readable scope config: fail closed — an arbitrary shell must not
		// widen the attack surface beyond what the operator configured.
		return fmt.Errorf("无法读取工作区 scope.yaml（%v）；执行命令前需要有效的 scope 配置", err)
	}

	policy := strings.TrimSpace(scopeConfig.Scope.TargetPolicy)
	if policy == "disabled" {
		// Even with the policy disabled, infrastructure endpoints stay
		// non-targets: reachability never implies authorization to attack.
		for _, rawURL := range urls {
			if infErr := rejectInfrastructureTarget(scopeConfig, rawURL); infErr != nil {
				return egressGateError(rawURL, "基础设施端点", infErr.Error())
			}
		}
		return nil
	}

	for _, rawURL := range urls {
		// Infrastructure check first: a misconfigured scope listing an infra
		// host as a target is still infrastructure.
		if infErr := rejectInfrastructureTarget(scopeConfig, rawURL); infErr != nil {
			return egressGateError(rawURL, "基础设施端点", infErr.Error())
		}
		parsed, parseErr := url.Parse(rawURL)
		if parseErr != nil || parsed.Scheme == "" || parsed.Host == "" {
			return fmt.Errorf("执行命令中的 URL %q 无法解析为绝对 URL，无法确认其在 scope 内", rawURL)
		}
		if !egressURLInScope(scopeConfig, parsed) {
			return egressGateError(rawURL, "域外端点",
				fmt.Sprintf("执行命令中的 URL %q 不在工作区 scope 的 URL 目标内（scope.yaml targets）", rawURL))
		}
		if exclErr := s.checkObserveExclusion(parsed, scopeConfig.Scope.Exclusions); exclErr != nil {
			return egressGateError(rawURL, "排除项端点", exclErr.Error())
		}
	}
	return nil
}

// egressURLInScope mirrors authorizeObserveTarget's in-scope check: kind=url
// targets with scheme+host equality, then exclusions.
func egressURLInScope(scopeConfig app.ScopeConfig, parsed *url.URL) bool {
	for _, target := range scopeConfig.Scope.Targets {
		if strings.TrimSpace(target.Kind) != "url" || strings.TrimSpace(target.Value) == "" {
			continue
		}
		scoped, parseErr := url.Parse(strings.TrimSpace(target.Value))
		if parseErr != nil || scoped.Scheme == "" || scoped.Host == "" {
			continue
		}
		if parsed.Scheme == scoped.Scheme && parsed.Host == scoped.Host {
			return true
		}
	}
	return false
}

// egressGateError renders one structured rejection: the refused host, why,
// and the operator's way out. Chat surfaces print it verbatim; the /api
// caller sees the same text in the HTTP error field.
func egressGateError(rawURL string, reason string, detail string) error {
	parsed, parseErr := url.Parse(rawURL)
	host := rawURL
	if parseErr == nil && parsed.Host != "" {
		host = parsed.Host
	}
	return fmt.Errorf("执行出站门拒绝: %s %q（host=%s）。%s。如需诊断可设置 OURO_EGRESS_GATE=0 临时放行", reason, rawURL, host, detail)
}
