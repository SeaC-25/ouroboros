package frontier

import "testing"

func TestCoverageMatrixDefinesDefaultDomainDimensions(t *testing.T) {
	matrix := NewCoverageMatrix()

	for _, check := range []struct {
		domain    Domain
		dimension Dimension
	}{
		{DomainWebAPI, DimensionRoutes},
		{DomainWebAPI, DimensionParams},
		{DomainWebAPI, DimensionAuthStates},
		{DomainInfra, DimensionHosts},
		{DomainInfra, DimensionServices},
		{DomainInfra, DimensionIdentities},
		{DomainCTF, DimensionHypotheses},
		{DomainCTF, DimensionArtifacts},
		{DomainCTF, DimensionProofs},
	} {
		if _, ok := matrix.Counters(check.domain, check.dimension); !ok {
			t.Fatalf("expected coverage dimension %s/%s", check.domain, check.dimension)
		}
	}
}

func TestCoverageMatrixUpdatesCounters(t *testing.T) {
	matrix := NewCoverageMatrix()

	matrix.Update(DomainWebAPI, DimensionRoutes, CoverageCounters{
		Discovered: 10,
		Tested:     4,
		Verified:   1,
		Exhausted:  2,
	})
	matrix.Update(DomainWebAPI, DimensionRoutes, CoverageCounters{
		Discovered: 3,
		Tested:     2,
	})

	counters, ok := matrix.Counters(DomainWebAPI, DimensionRoutes)
	if !ok {
		t.Fatalf("expected route counters")
	}
	if counters.Discovered != 13 || counters.Tested != 6 || counters.Verified != 1 || counters.Exhausted != 2 {
		t.Fatalf("unexpected counters: %#v", counters)
	}
}

func TestCoverageMatrixHeatmapBuckets(t *testing.T) {
	matrix := NewCoverageMatrix()
	if bucket := matrix.Bucket(DomainWebAPI, DimensionRoutes); bucket != BucketUnknown {
		t.Fatalf("expected unknown bucket, got %s", bucket)
	}

	matrix.Update(DomainWebAPI, DimensionRoutes, CoverageCounters{Discovered: 10})
	if bucket := matrix.Bucket(DomainWebAPI, DimensionRoutes); bucket != BucketShallow {
		t.Fatalf("expected shallow bucket, got %s", bucket)
	}

	matrix.Update(DomainWebAPI, DimensionRoutes, CoverageCounters{Tested: 3})
	if bucket := matrix.Bucket(DomainWebAPI, DimensionRoutes); bucket != BucketActive {
		t.Fatalf("expected active bucket, got %s", bucket)
	}

	matrix.Update(DomainWebAPI, DimensionRoutes, CoverageCounters{Verified: 1})
	if bucket := matrix.Bucket(DomainWebAPI, DimensionRoutes); bucket != BucketVerified {
		t.Fatalf("expected verified bucket, got %s", bucket)
	}

	matrix.Update(DomainWebAPI, DimensionRoutes, CoverageCounters{Exhausted: 10})
	if bucket := matrix.Bucket(DomainWebAPI, DimensionRoutes); bucket != BucketExhausted {
		t.Fatalf("expected exhausted bucket, got %s", bucket)
	}
}
