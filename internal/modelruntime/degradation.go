package modelruntime

import "fmt"

const (
	ModelOpus47   = "Opus 4.7"
	ModelSonnet46 = "Sonnet 4.6"
	ModelHaiku45  = "Haiku 4.5"
)

var degradationChain = []string{
	ModelOpus47,
	ModelSonnet46,
	ModelHaiku45,
}

type DegradationManager struct {
	currentModel string
	budget       float64
}

func NewDegradationManager(currentModel string, budget float64) *DegradationManager {
	if currentModel == "" {
		currentModel = ModelOpus47
	}
	return &DegradationManager{
		currentModel: currentModel,
		budget:       budget,
	}
}

func (m *DegradationManager) ShouldDegrade() bool {
	if m == nil {
		return false
	}
	if m.budget > 0 {
		return false
	}
	_, ok := nextDegradedModel(m.model())
	return ok
}

func (m *DegradationManager) Downgrade() (newModel string, err error) {
	if m == nil {
		return "", fmt.Errorf("degradation manager is nil")
	}

	current := m.model()
	next, ok := nextDegradedModel(current)
	if !ok {
		if current == ModelHaiku45 {
			return current, fmt.Errorf("model %q is already the lowest tier", current)
		}
		return current, fmt.Errorf("model %q is not in the degradation chain", current)
	}

	m.currentModel = next
	return next, nil
}

func (m *DegradationManager) GetCurrentModel() string {
	if m == nil {
		return ""
	}
	return m.model()
}

func (m *DegradationManager) model() string {
	if m.currentModel == "" {
		return ModelOpus47
	}
	return m.currentModel
}

func nextDegradedModel(current string) (string, bool) {
	for i, model := range degradationChain {
		if model != current {
			continue
		}
		if i+1 >= len(degradationChain) {
			return "", false
		}
		return degradationChain[i+1], true
	}
	return "", false
}
