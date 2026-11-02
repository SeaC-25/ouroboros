package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ouroboros/ouroboros/internal/blackboard"
	"github.com/ouroboros/ouroboros/internal/nativetools"
)

// The evidence gate turns the observation-reasoning loop's tentative
// conclusions into verified ones: ACTION:VERIFY <obs-id> <expected signal>
// replays the cited raw observation and substring-checks the response
// against the signal. On a match every fact back-linking the observation is
// promoted from tentative (E2) to verified (E3) in runtime/blackboard.jsonl;
// on a miss nothing is promoted and the failure is reported verbatim.

// verifyReplayTimeout bounds one observation replay. It matches the
// ACTION:OBSERVE handler so a verification cannot hang longer than the
// observation it re-runs.
const verifyReplayTimeout = 30 * time.Second

// verifyObservationJournal is the workspace journal holding raw
// observations, mirrored here from Server.observationsPath for tests.
func (s *Server) verifyObservationJournal() string {
	return s.observationsPath()
}

// loadVerifyObservation returns the observation with the given ID from
// runtime/observations.jsonl. A missing journal or record is a structured
// error, never a panic.
func (s *Server) loadVerifyObservation(obsID string) (BlackboardObservation, error) {
	path := s.verifyObservationJournal()
	file, err := os.Open(path)
	if err != nil {
		return BlackboardObservation{}, fmt.Errorf("无法读取观测日志 %s: %w", filepath.ToSlash(path), err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	// 4MB max line, aligned with the other journal readers.
	scanner.Buffer(make([]byte, 0, 64*1024), 4<<20)
	for scanner.Scan() {
		var observation BlackboardObservation
		if json.Unmarshal(scanner.Bytes(), &observation) != nil {
			continue
		}
		if observation.ID == obsID {
			return observation, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return BlackboardObservation{}, fmt.Errorf("扫描观测日志失败: %w", err)
	}
	return BlackboardObservation{}, fmt.Errorf("观测 %s 不存在", obsID)
}

// verifyReplayTarget rebuilds the replay target from the observation
// record: the payload is the ObserveResult JSON, whose url field carries
// the full request URL the original observation hit. A payload without a
// usable URL falls back to the record's target field.
func verifyReplayTarget(observation BlackboardObservation) (string, error) {
	var payload struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(observation.Payload), &payload); err == nil && strings.TrimSpace(payload.URL) != "" {
		return strings.TrimSpace(payload.URL), nil
	}
	if strings.TrimSpace(observation.Target) != "" {
		return strings.TrimSpace(observation.Target), nil
	}
	return "", fmt.Errorf("观测 %s 的载荷中没有可重放的 URL", observation.ID)
}

// executeVerifyAction replays the cited observation and promotes or keeps
// the evidence level of the facts that back-link it. The returned summary
// is the distillation shown back to the model.
func (s *Server) executeVerifyAction(obsID string, expectedSignal string) (string, error) {
	obsID = strings.TrimSpace(obsID)
	if obsID == "" {
		return "", fmt.Errorf("观测ID 不能为空")
	}
	if strings.TrimSpace(expectedSignal) == "" {
		return "", fmt.Errorf("预期信号 不能为空")
	}

	observation, err := s.loadVerifyObservation(obsID)
	if err != nil {
		return "", err
	}
	replayTarget, err := verifyReplayTarget(observation)
	if err != nil {
		return "", err
	}

	// Scope gate: the replay issues a real HTTP request, so it passes the
	// same gate as ACTION:OBSERVE — VERIFY must not be a silent bypass.
	if err := s.authorizeObserveTarget(replayTarget); err != nil {
		return "", err
	}

	// Spec is not persisted with the record; replay as plain GET.
	replayOut, err := nativetools.ObserveHTTP(context.Background(), replayTarget, nativetools.ObserveSpec{}, nativetools.Options{Timeout: verifyReplayTimeout})
	if err != nil {
		return "", fmt.Errorf("重放观测失败: %w", err)
	}

	if !strings.Contains(replayOut, expectedSignal) {
		return fmt.Sprintf("验证失败：信号未复现。观测 %s 已重放（GET %s），响应中不含预期信号，相关结论保持 E2（待验证）", obsID, replayTarget), nil
	}

	promoted, err := s.promoteFactsByObservation(obsID)
	if err != nil {
		return "", fmt.Errorf("信号已复现但晋升黑板结论失败: %w", err)
	}
	return fmt.Sprintf("验证通过：obs %s 信号复现 → %d 条结论晋升 E3", obsID, promoted), nil
}

// promoteFactsByObservation rewrites runtime/blackboard.jsonl, setting
// EvidenceLevel=3 on every fact whose SourceObservations contain the
// observation ID. Facts already verified stay verified; the rewrite is
// serialized through stateMu against concurrent appends and GUI edits.
func (s *Server) promoteFactsByObservation(obsID string) (int, error) {
	facts := s.loadBlackboardFacts()
	promoted := 0
	changed := false
	for i := range facts {
		if facts[i].EvidenceLevel == blackboard.EvidenceVerified {
			continue
		}
		if !factCitesObservation(facts[i], obsID) {
			continue
		}
		facts[i].EvidenceLevel = blackboard.EvidenceVerified
		facts[i].UpdatedAt = time.Now().Unix()
		promoted++
		changed = true
	}
	if !changed {
		return 0, nil
	}
	if err := s.writeBlackboardFacts(facts); err != nil {
		return 0, err
	}
	return promoted, nil
}

// factCitesObservation reports whether the fact's source-observation
// back-links contain the observation ID (exact match).
func factCitesObservation(fact BlackboardFact, obsID string) bool {
	for _, id := range fact.SourceObservations {
		if id == obsID {
			return true
		}
	}
	return false
}
