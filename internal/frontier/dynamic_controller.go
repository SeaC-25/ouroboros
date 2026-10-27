package frontier

type Strategy string

const (
	StrategyBreadth = "breadth"
	StrategyDepth   = "depth"
	StrategyMixed   = "mixed"
)

const (
	breadthCoverageThreshold = 0.30
	depthCoverageThreshold   = 0.70
)

type DynamicController struct {
	strategy string // "breadth" or "depth"
}

type dynamicControllerState struct {
	matrix   *CoverageMatrix
	switched bool
}

var dynamicControllerStates = map[*DynamicController]dynamicControllerState{}

func NewDynamicController(matrix *CoverageMatrix, queue *Queue) *DynamicController {
	controller := &DynamicController{
		strategy: string(StrategyBreadth),
	}
	dynamicControllerStates[controller] = dynamicControllerState{matrix: matrix}
	return controller
}

func (c *DynamicController) AdjustStrategy(coverage float64) {
	if c == nil {
		return
	}

	coverage = normalizeCoverage(coverage)
	previous := c.currentStrategy()

	switch {
	case coverage < breadthCoverageThreshold:
		c.strategy = string(StrategyBreadth)
	case coverage > depthCoverageThreshold:
		c.strategy = string(StrategyDepth)
	default:
		c.strategy = previous
	}

	c.setSwitched(previous != c.currentStrategy())
}

func (c *DynamicController) Strategy() Strategy {
	if c == nil {
		return StrategyBreadth
	}
	return Strategy(c.currentStrategy())
}

func (c *DynamicController) Recompute() Strategy {
	if c == nil {
		return StrategyBreadth
	}

	coverage := c.coverage()
	switch {
	case coverage < breadthCoverageThreshold:
		c.AdjustStrategy(coverage)
		return StrategyBreadth
	case coverage > depthCoverageThreshold:
		c.AdjustStrategy(coverage)
		return StrategyDepth
	default:
		c.strategy = c.currentStrategy()
		c.setSwitched(false)
		return StrategyMixed
	}
}

func (c *DynamicController) ShouldWiden() bool {
	return c != nil && c.currentStrategy() == string(StrategyBreadth)
}

func (c *DynamicController) ShouldDeepen() bool {
	return c != nil && c.currentStrategy() == string(StrategyDepth)
}

func (c *DynamicController) currentStrategy() string {
	if c.strategy == string(StrategyDepth) {
		return string(StrategyDepth)
	}
	return string(StrategyBreadth)
}

func (c *DynamicController) coverage() float64 {
	if c == nil {
		return 0
	}

	matrix := dynamicControllerStates[c].matrix
	if matrix == nil {
		return 0
	}

	totalDiscovered := 0
	totalTested := 0

	for _, domain := range []Domain{DomainWebAPI, DomainInfra, DomainCTF} {
		for _, dimension := range coverageDimensions(domain) {
			counters, ok := matrix.Counters(domain, dimension)
			if !ok {
				continue
			}
			totalDiscovered += counters.Discovered
			totalTested += counters.Tested
		}
	}

	if totalDiscovered == 0 {
		return 0
	}
	return float64(totalTested) / float64(totalDiscovered)
}

func coverageDimensions(domain Domain) []Dimension {
	switch domain {
	case DomainWebAPI:
		return []Dimension{DimensionRoutes, DimensionParams, DimensionAuthStates}
	case DomainInfra:
		return []Dimension{DimensionHosts, DimensionServices, DimensionIdentities}
	case DomainCTF:
		return []Dimension{DimensionHypotheses, DimensionArtifacts, DimensionProofs}
	default:
		return nil
	}
}

func normalizeCoverage(coverage float64) float64 {
	if coverage > 1 {
		coverage = coverage / 100
	}
	if coverage < 0 {
		return 0
	}
	if coverage > 1 {
		return 1
	}
	return coverage
}

func (c *DynamicController) setSwitched(switched bool) {
	state := dynamicControllerStates[c]
	state.switched = switched
	dynamicControllerStates[c] = state
}
