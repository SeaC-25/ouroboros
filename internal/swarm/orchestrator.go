package swarm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/ouroboros/ouroboros/internal/blackboard"
	"github.com/ouroboros/ouroboros/internal/evidence"
)

type Orchestrator struct {
	store    *blackboard.MemoryStore
	driver   Driver
	mu       sync.RWMutex
	progress Progress
}

type Progress struct {
	Total    int
	Queued   int
	Running  int
	Blocked  int
	Complete int
	Failed   int
}

func NewOrchestrator(store *blackboard.MemoryStore, driver Driver) *Orchestrator {
	return &Orchestrator{
		store:  store,
		driver: driver,
	}
}

func (o *Orchestrator) Execute(ctx context.Context, task AgentTask) (AgentResult, error) {
	if o.store == nil {
		return AgentResult{}, errors.New("blackboard store is required")
	}
	if o.driver == nil {
		return AgentResult{}, errors.New("agent driver is required")
	}

	o.recordState(AgentRunning)
	result, err := o.driver.Run(ctx, task)
	if err != nil {
		o.recordState(AgentFailed)
		return AgentResult{}, err
	}
	if err := ValidateStructuredOutput(result.Output); err != nil {
		o.recordState(AgentFailed)
		return AgentResult{}, err
	}
	if err := o.applyOutput(result.Output); err != nil {
		o.recordState(AgentFailed)
		return AgentResult{}, err
	}
	o.recordState(result.State)
	return result, nil
}

func (o *Orchestrator) EvidenceGraph() evidence.Graph {
	return evidence.Project(o.store.State())
}

func (o *Orchestrator) Progress() Progress {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.progress
}

func (o *Orchestrator) applyOutput(output StructuredOutput) error {
	for _, proposal := range output.Proposals {
		switch proposal.Type {
		case ProposalFact:
			var fact blackboard.Fact
			if err := json.Unmarshal(proposal.Payload, &fact); err != nil {
				return fmt.Errorf("decode fact proposal: %w", err)
			}
			if _, err := o.store.AppendFact(fact); err != nil {
				return err
			}
		case ProposalIntent:
			var intent blackboard.Intent
			if err := json.Unmarshal(proposal.Payload, &intent); err != nil {
				return fmt.Errorf("decode intent proposal: %w", err)
			}
			if _, err := o.store.OpenIntent(intent); err != nil {
				return err
			}
		case ProposalHint:
			var hint blackboard.Hint
			if err := json.Unmarshal(proposal.Payload, &hint); err != nil {
				return fmt.Errorf("decode hint proposal: %w", err)
			}
			if _, err := o.store.AppendHint(hint); err != nil {
				return err
			}
		case ProposalFinding:
			var fact blackboard.Fact
			if err := json.Unmarshal(proposal.Payload, &fact); err != nil {
				return fmt.Errorf("decode finding proposal: %w", err)
			}
			if fact.Kind == "" {
				fact.Kind = "finding"
			}
			if _, err := o.store.AppendFact(fact); err != nil {
				return err
			}
		case ProposalMutation:
			var hint blackboard.Hint
			if err := json.Unmarshal(proposal.Payload, &hint); err != nil {
				return fmt.Errorf("decode mutation proposal: %w", err)
			}
			if hint.Kind == "" {
				hint.Kind = "mutation"
			}
			if _, err := o.store.AppendHint(hint); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported proposal type: %s", proposal.Type)
		}
	}
	return nil
}

func (o *Orchestrator) recordState(state AgentState) {
	o.mu.Lock()
	defer o.mu.Unlock()

	switch state {
	case AgentRunning:
		return
	case AgentQueued:
		o.progress.Total++
		o.progress.Queued++
	case AgentBlocked:
		o.progress.Total++
		o.progress.Blocked++
	case AgentComplete:
		o.progress.Total++
		o.progress.Complete++
	case AgentFailed:
		o.progress.Total++
		o.progress.Failed++
	}
}
