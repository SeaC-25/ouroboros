package frontier

type Domain string

const (
	DomainWebAPI Domain = "web_api"
	DomainInfra  Domain = "infra_internal"
	DomainCTF    Domain = "ctf_lab"
)

type Dimension string

const (
	DimensionRoutes     Dimension = "routes"
	DimensionParams     Dimension = "params"
	DimensionAuthStates Dimension = "auth_states"
	DimensionHosts      Dimension = "hosts"
	DimensionServices   Dimension = "services"
	DimensionIdentities Dimension = "identities"
	DimensionHypotheses Dimension = "hypotheses"
	DimensionArtifacts  Dimension = "artifacts"
	DimensionProofs     Dimension = "proofs"
)

type CoverageCounters struct {
	Discovered int
	Tested     int
	Verified   int
	Exhausted  int
}

type Bucket string

const (
	BucketUnknown   Bucket = "unknown"
	BucketShallow   Bucket = "shallow"
	BucketActive    Bucket = "active"
	BucketVerified  Bucket = "verified"
	BucketExhausted Bucket = "exhausted"
)

type CoverageMatrix struct {
	counters map[Domain]map[Dimension]CoverageCounters
}

func NewCoverageMatrix() *CoverageMatrix {
	matrix := &CoverageMatrix{
		counters: make(map[Domain]map[Dimension]CoverageCounters),
	}
	matrix.ensure(DomainWebAPI, DimensionRoutes)
	matrix.ensure(DomainWebAPI, DimensionParams)
	matrix.ensure(DomainWebAPI, DimensionAuthStates)
	matrix.ensure(DomainInfra, DimensionHosts)
	matrix.ensure(DomainInfra, DimensionServices)
	matrix.ensure(DomainInfra, DimensionIdentities)
	matrix.ensure(DomainCTF, DimensionHypotheses)
	matrix.ensure(DomainCTF, DimensionArtifacts)
	matrix.ensure(DomainCTF, DimensionProofs)
	return matrix
}

func (m *CoverageMatrix) Update(domain Domain, dimension Dimension, delta CoverageCounters) {
	m.ensure(domain, dimension)
	current := m.counters[domain][dimension]
	current.Discovered += delta.Discovered
	current.Tested += delta.Tested
	current.Verified += delta.Verified
	current.Exhausted += delta.Exhausted
	m.counters[domain][dimension] = current
}

func (m *CoverageMatrix) Counters(domain Domain, dimension Dimension) (CoverageCounters, bool) {
	dimensions, ok := m.counters[domain]
	if !ok {
		return CoverageCounters{}, false
	}
	counters, ok := dimensions[dimension]
	return counters, ok
}

func (m *CoverageMatrix) Bucket(domain Domain, dimension Dimension) Bucket {
	counters, ok := m.Counters(domain, dimension)
	if !ok || counters == (CoverageCounters{}) {
		return BucketUnknown
	}
	if counters.Exhausted >= counters.Discovered && counters.Discovered > 0 {
		return BucketExhausted
	}
	if counters.Verified > 0 {
		return BucketVerified
	}
	if counters.Tested > 0 {
		return BucketActive
	}
	return BucketShallow
}

func (m *CoverageMatrix) ensure(domain Domain, dimension Dimension) {
	if m.counters[domain] == nil {
		m.counters[domain] = make(map[Dimension]CoverageCounters)
	}
	if _, ok := m.counters[domain][dimension]; !ok {
		m.counters[domain][dimension] = CoverageCounters{}
	}
}
