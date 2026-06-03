package modelruntime

func EstimateTokenCost(inputTokens, outputTokens int, inputPricePerMillionTokens, outputPricePerMillionTokens float64) float64 {
	if inputTokens < 0 {
		inputTokens = 0
	}
	if outputTokens < 0 {
		outputTokens = 0
	}
	return (float64(inputTokens) * inputPricePerMillionTokens / 1_000_000) +
		(float64(outputTokens) * outputPricePerMillionTokens / 1_000_000)
}
