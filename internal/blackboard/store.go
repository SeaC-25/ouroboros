package blackboard

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type MemoryStore struct {
	mu           sync.RWMutex
	nextID       int
	facts        []Fact
	intents      map[string]Intent
	hints        []Hint
	observations []Observation
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		intents: make(map[string]Intent),
	}
}

func (s *MemoryStore) AppendFact(fact Fact) (Fact, error) {
	if strings.TrimSpace(fact.Kind) == "" {
		return Fact{}, errors.New("fact kind is required")
	}
	if strings.TrimSpace(fact.Statement) == "" {
		return Fact{}, errors.New("fact statement is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	fact.ID = s.newID("fact")
	fact.CreatedAt = now
	// SourceObservations is caller-supplied evidence backlink metadata; it
	// passes through untouched so facts keep pointing at their observations.
	s.facts = append(s.facts, fact)
	return fact, nil
}

func (s *MemoryStore) Facts() []Fact {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return append([]Fact(nil), s.facts...)
}

func (s *MemoryStore) AppendHint(hint Hint) (Hint, error) {
	if strings.TrimSpace(hint.Kind) == "" {
		return Hint{}, errors.New("hint kind is required")
	}
	if strings.TrimSpace(hint.Message) == "" {
		return Hint{}, errors.New("hint message is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	hint.ID = s.newID("hint")
	hint.CreatedAt = time.Now().UTC()
	s.hints = append(s.hints, hint)
	return hint, nil
}

func (s *MemoryStore) OpenIntent(intent Intent) (Intent, error) {
	if strings.TrimSpace(intent.Kind) == "" {
		return Intent{}, errors.New("intent kind is required")
	}
	if strings.TrimSpace(intent.Goal) == "" {
		return Intent{}, errors.New("intent goal is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	intent.ID = s.newID("intent")
	intent.State = IntentOpen
	intent.SourceFacts = append([]string(nil), intent.SourceFacts...)
	intent.CreatedAt = now
	intent.UpdatedAt = now
	s.intents[intent.ID] = copyIntent(intent)
	return copyIntent(intent), nil
}

func (s *MemoryStore) Intent(id string) (Intent, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	intent, ok := s.intents[id]
	if !ok {
		return Intent{}, false
	}
	return copyIntent(intent), true
}

func (s *MemoryStore) ClaimIntent(intentID string, agentID string, ttl time.Duration) (Lease, error) {
	if strings.TrimSpace(agentID) == "" {
		return Lease{}, errors.New("agent id is required")
	}
	if ttl <= 0 {
		return Lease{}, errors.New("lease ttl must be positive")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	intent, ok := s.intents[intentID]
	if !ok {
		return Lease{}, fmt.Errorf("intent not found: %s", intentID)
	}
	if intent.State == IntentClaimed && intent.Lease != nil {
		// An existing lease only blocks re-claim while unexpired: a worker
		// that died mid-lease must not wedge the intent forever, so an
		// expired lease can be taken over with a fresh leaseID.
		if now := time.Now().UTC(); intent.Lease.ExpiresAt.After(now) {
			return Lease{}, fmt.Errorf("intent %s is claimed by %s until %s", intentID, intent.Lease.AgentID, intent.Lease.ExpiresAt.Format(time.RFC3339))
		}
	} else if intent.State != IntentOpen && intent.State != IntentReleased {
		return Lease{}, fmt.Errorf("intent %s cannot be claimed from state %s", intentID, intent.State)
	}

	now := time.Now().UTC()
	lease := Lease{
		ID:        s.newID("lease"),
		IntentID:  intentID,
		AgentID:   agentID,
		ExpiresAt: now.Add(ttl),
		UpdatedAt: now,
	}
	intent.State = IntentClaimed
	intent.Lease = &lease
	intent.UpdatedAt = now
	s.intents[intentID] = copyIntent(intent)
	return lease, nil
}

func (s *MemoryStore) Heartbeat(intentID string, leaseID string, ttl time.Duration) (Lease, error) {
	if ttl <= 0 {
		return Lease{}, errors.New("lease ttl must be positive")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	intent, ok := s.intents[intentID]
	if !ok {
		return Lease{}, fmt.Errorf("intent not found: %s", intentID)
	}
	if intent.State != IntentClaimed || intent.Lease == nil {
		return Lease{}, fmt.Errorf("intent %s is not claimed", intentID)
	}
	if intent.Lease.ID != leaseID {
		return Lease{}, fmt.Errorf("lease mismatch for intent %s", intentID)
	}
	// A heartbeat on an already-expired lease must not resurrect it: the
	// worker is presumed dead and another agent may have taken over.
	if now := time.Now().UTC(); !intent.Lease.ExpiresAt.After(now) {
		return Lease{}, fmt.Errorf("lease for intent %s expired at %s", intentID, intent.Lease.ExpiresAt.Format(time.RFC3339))
	}

	now := time.Now().UTC()
	intent.Lease.ExpiresAt = now.Add(ttl)
	intent.Lease.UpdatedAt = now
	intent.UpdatedAt = now
	s.intents[intentID] = copyIntent(intent)
	return *intent.Lease, nil
}

func (s *MemoryStore) ReleaseIntent(intentID string, leaseID string) (Intent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	intent, ok := s.intents[intentID]
	if !ok {
		return Intent{}, fmt.Errorf("intent not found: %s", intentID)
	}
	if intent.State != IntentClaimed || intent.Lease == nil {
		return Intent{}, fmt.Errorf("intent %s is not claimed", intentID)
	}
	if intent.Lease.ID != leaseID {
		return Intent{}, fmt.Errorf("lease mismatch for intent %s", intentID)
	}

	intent.State = IntentReleased
	intent.Lease = nil
	intent.UpdatedAt = time.Now().UTC()
	s.intents[intentID] = copyIntent(intent)
	return copyIntent(intent), nil
}

func (s *MemoryStore) ConcludeIntent(intentID string, leaseID string, fact Fact) (ConcludeResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	intent, ok := s.intents[intentID]
	if !ok {
		return ConcludeResult{}, fmt.Errorf("intent not found: %s", intentID)
	}
	if intent.State != IntentClaimed || intent.Lease == nil {
		return ConcludeResult{}, fmt.Errorf("intent %s is not claimed", intentID)
	}
	if intent.Lease.ID != leaseID {
		return ConcludeResult{}, fmt.Errorf("lease mismatch for intent %s", intentID)
	}
	if strings.TrimSpace(fact.Kind) == "" {
		return ConcludeResult{}, errors.New("fact kind is required")
	}
	if strings.TrimSpace(fact.Statement) == "" {
		return ConcludeResult{}, errors.New("fact statement is required")
	}

	now := time.Now().UTC()
	fact.ID = s.newID("fact")
	fact.CreatedAt = now
	s.facts = append(s.facts, fact)

	intent.State = IntentConcluded
	intent.ResultFact = fact.ID
	intent.Lease = nil
	intent.UpdatedAt = now
	s.intents[intentID] = copyIntent(intent)

	return ConcludeResult{Intent: copyIntent(intent), Fact: fact}, nil
}

func (s *MemoryStore) State() ProjectState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	intents := make([]Intent, 0, len(s.intents))
	for _, intent := range s.intents {
		intents = append(intents, copyIntent(intent))
	}

	return ProjectState{
		Facts:        append([]Fact(nil), s.facts...),
		Intents:      intents,
		Hints:        append([]Hint(nil), s.hints...),
		Observations: append([]Observation(nil), s.observations...),
	}
}

func (s *MemoryStore) newID(prefix string) string {
	s.nextID++
	return fmt.Sprintf("%s-%d", prefix, s.nextID)
}

func copyIntent(intent Intent) Intent {
	intent.SourceFacts = append([]string(nil), intent.SourceFacts...)
	if intent.Lease != nil {
		lease := *intent.Lease
		intent.Lease = &lease
	}
	return intent
}
