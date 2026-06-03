package modelruntime

import "testing"

func TestEstimateTokenCostUsesPerMillionTokenRates(t *testing.T) {
	cost := EstimateTokenCost(1000, 500, 2.50, 10.00)

	if cost != 0.0075 {
		t.Fatalf("expected token cost 0.0075, got %.8f", cost)
	}
}

func TestEstimateTokenCostTreatsNegativeUsageAsZero(t *testing.T) {
	cost := EstimateTokenCost(-1000, 500, 2.50, 10.00)

	if cost != 0.005 {
		t.Fatalf("expected negative input usage to be ignored, got %.8f", cost)
	}
}
