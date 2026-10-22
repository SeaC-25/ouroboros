package conveyor

import (
	"math"
	"testing"
)

func caps(cs ...Capability) map[Capability]bool {
	m := map[Capability]bool{}
	for _, c := range cs {
		m[c] = true
	}
	return m
}

func TestPredicateMatching(t *testing.T) {
	reg := DefaultRegistry()

	t.Run("http_probe on bare web endpoint", func(t *testing.T) {
		e := Entity{ID: "e1", Confidence: 0.8, Capabilities: caps(CapWebEndpoint)}
		got := reg.Matched(e, false)
		names := map[string]bool{}
		for _, c := range got {
			names[c.Spec.Action] = true
		}
		// Bare endpoint: probe/body/cms/dir/verify are candidates…
		for _, want := range []string{"http_probe", "body_fetch", "shallow_dir", "cms_scan"} {
			if !names[want] {
				t.Fatalf("expected %s in matches, got %v", want, names)
			}
		}
		// …but not fingerprint (excluded by absence of fingerprinted? no —
		// fingerprint REQUIRES only web_endpoint and EXCLUDES fingerprinted;
		// bare endpoint qualifies). sqli/xss/nuclei require fingerprinted.
		if names["sqli_probe"] || names["xss_probe"] || names["nuclei_subset"] {
			t.Fatalf("verify-family must require fingerprinted, got %v", names)
		}
	})

	t.Run("auth_login requires credentials and session", func(t *testing.T) {
		e := Entity{ID: "e2", Confidence: 0.9, Capabilities: caps(CapWebEndpoint, CapAuthSession)}
		if got := reg.Matched(e, false); hasAction(got, "auth_login") {
			t.Fatal("auth_login must be skipped without credentials")
		}
		if got := reg.Matched(e, true); !hasAction(got, "auth_login") {
			t.Fatal("auth_login must match with credentials + session")
		}
	})

	t.Run("excludes mask blocks", func(t *testing.T) {
		e := Entity{ID: "e3", Confidence: 0.5, Capabilities: caps(CapWebEndpoint, CapFingerprinted)}
		for _, c := range reg.Matched(e, false) {
			if c.Spec.Action == "fingerprint" {
				t.Fatal("fingerprint must be excluded when already fingerprinted")
			}
		}
	})

	t.Run("tested_auth blocks repeat auth_login", func(t *testing.T) {
		e := Entity{ID: "e4", Confidence: 0.9, Capabilities: caps(CapWebEndpoint, CapAuthSession, CapTestedAuth)}
		if hasAction(reg.Matched(e, true), "auth_login") {
			t.Fatal("auth_login must be excluded after tested_auth")
		}
	})
}

func TestRuleRouterScoring(t *testing.T) {
	reg := DefaultRegistry()

	t.Run("equal confidence phase order wins", func(t *testing.T) {
		e := Entity{ID: "e5", Confidence: 0.7, Capabilities: caps(CapWebEndpoint, CapFingerprinted)}
		ordered := Select(reg.Matched(e, false))
		if len(ordered) < 2 {
			t.Fatal("expected multiple candidates")
		}
		if ordered[0].Spec.Action != "http_probe" {
			t.Fatalf("http_probe (phase order 0) must rank first, got %s", ordered[0].Spec.Action)
		}
		// scores strictly non-increasing
		for i := 1; i < len(ordered); i++ {
			if ordered[i].Score > ordered[i-1].Score+1e-9 {
				t.Fatalf("scores must be non-increasing: %v before %v", ordered[i-1].Score, ordered[i].Score)
			}
		}
	})

	t.Run("confidence dominates phase order", func(t *testing.T) {
		high := Entity{ID: "hi", Confidence: 0.99, Capabilities: caps(CapWebEndpoint)}
		low := Entity{ID: "lo", Confidence: 0.10, Capabilities: caps(CapWebEndpoint, CapDomain, CapHost)}
		hiTop := Select(reg.Matched(high, false))[0]
		loCands := reg.Matched(low, false)
		// any low-entity candidate scores below the high-entity's best
		for _, c := range loCands {
			if c.Score > hiTop.Score {
				t.Fatalf("confidence 0.99 must dominate: %v > %v", c.Score, hiTop.Score)
			}
		}
	})

	t.Run("insight penalized below default", func(t *testing.T) {
		def := Entity{ID: "d", Confidence: 0.5, Source: SourceDefault, Capabilities: caps(CapWebEndpoint)}
		ins := Entity{ID: "i", Confidence: 0.5, Source: SourceInsight, Capabilities: caps(CapWebEndpoint)}
		dScore := Select(reg.Matched(def, false))[0].Score
		iScore := Select(reg.Matched(ins, false))[0].Score
		if math.Abs((dScore-iScore)-(0.15-0.05)) > 1e-9 {
			t.Fatalf("insight penalty delta wrong: d=%v i=%v", dScore, iScore)
		}
	})
}

func hasAction(cs []Candidate, name string) bool {
	for _, c := range cs {
		if c.Spec.Action == name {
			return true
		}
	}
	return false
}

func TestRegistryDefensiveCopy(t *testing.T) {
	reg := DefaultRegistry()
	a := reg.Actions()
	a[0].Action = "mutated"
	if reg.Actions()[0].Action == "mutated" {
		t.Fatal("Actions() must return a defensive copy")
	}
}

func TestFastSlowHint(t *testing.T) {
	// The conveyor's tool binding tells the executor which track to use:
	// curl/fastsec families map to fast-first nativetools equivalents with
	// fastsec deep passes scheduled behind (S5 integration point).
	reg := DefaultRegistry()
	for _, a := range reg.Actions() {
		if a.Tool == "" {
			t.Fatalf("action %s has no tool binding", a.Action)
		}
	}
}
