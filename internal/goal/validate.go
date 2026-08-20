package goal

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

const weightTolerance = 0.000001

func Validate(stack *GoalStack) error {
	var errs []error

	if stack == nil {
		return errors.New("goal stack is required")
	}
	if strings.TrimSpace(stack.Mission) == "" {
		errs = append(errs, errors.New("mission is required"))
	}
	if len(stack.SuccessCriteria) == 0 {
		errs = append(errs, errors.New("success criteria are required"))
	}
	if err := validateWeights(stack.Weights); err != nil {
		errs = append(errs, err)
	}

	for i, criterion := range stack.SuccessCriteria {
		if strings.TrimSpace(criterion) == "" {
			errs = append(errs, fmt.Errorf("success criteria %d is empty", i))
		}
	}

	return errors.Join(errs...)
}

func validateWeights(weights Weights) error {
	values := []float64{weights.Mission, weights.Coverage, weights.Evidence, weights.Report}
	var total float64
	for _, value := range values {
		if value < 0 {
			return fmt.Errorf("goal weights must be non-negative")
		}
		total += value
	}
	if math.Abs(total-1.0) > weightTolerance {
		return fmt.Errorf("goal weights must sum to 1.0, got %.6f", total)
	}
	return nil
}
