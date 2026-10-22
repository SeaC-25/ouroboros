package evidencegate

import (
	"context"
	"fmt"
	"testing"
)

type fakeRepro struct {
	outputs map[string]string // cmd -> output
	errs    map[string]error
}

func (f fakeRepro) Reproduce(_ context.Context, cmd string) (string, error) {
	if err, ok := f.errs[cmd]; ok {
		return f.outputs[cmd], err
	}
	return f.outputs[cmd], nil
}

func TestSignalMatching(t *testing.T) {
	cases := []struct {
		name   string
		output string
		signal string
		want   bool
	}{
		{"literal hit", "FLAG_LAB_OK found", "FLAG_LAB_OK", true},
		{"literal case-insensitive", "flag_lab_ok", "FLAG_LAB_OK", true},
		{"literal miss", "nothing here", "FLAG_LAB_OK", false},
		{"regex hit", "error at line 42: boom", `re:error at line \d+`, true},
		{"regex miss", "warning at line x", `re:error at line \d+`, false},
		{"invalid regex falls back to literal", "has re:[weird", "re:[weird", true},
		{"status hit", "HTTP/1.1 200 OK", "status:200", true},
		{"status miss", "HTTP/1.1 404 Not Found", "status:200", false},
		{"empty signal never matches", "anything", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Signal(tc.output, tc.signal); got != tc.want {
				t.Fatalf("Signal(%q, %q) = %v, want %v", tc.output, tc.signal, got, tc.want)
			}
		})
	}
}

func TestPromoteDifferentialFullLadder(t *testing.T) {
	ctx := context.Background()
	pos := "curl http://target/?id=1' AND 1=1--"
	neg := "curl http://target/?id=1' AND 1=2--"

	t.Run("positive hit negative clean promotes to E3", func(t *testing.T) {
		f := Finding{ID: "f1", ReproduceCmd: pos, ExpectedSignal: "user=admin", Level: E2Tentative}
		r := fakeRepro{outputs: map[string]string{pos: "user=admin row returned", neg: "no rows"}}
		res := PromoteDifferential(ctx, f, r, neg)
		if res.To != E3Verified {
			t.Fatalf("want E3Verified, got %s (%s)", res.To, res.Note)
		}
		if !res.PositiveHit || res.NegativeHit {
			t.Fatalf("pos=%v neg=%v mismatch", res.PositiveHit, res.NegativeHit)
		}
	})

	t.Run("signal fires on baseline too demotes to FP", func(t *testing.T) {
		f := Finding{ID: "f2", ReproduceCmd: pos, ExpectedSignal: "200", Level: E2Tentative}
		r := fakeRepro{outputs: map[string]string{pos: "HTTP 200", neg: "HTTP 200"}}
		res := PromoteDifferential(ctx, f, r, neg)
		if res.To != EFalsePositive {
			t.Fatalf("want FALSE_POSITIVE, got %s (%s)", res.To, res.Note)
		}
	})

	t.Run("positive miss demotes", func(t *testing.T) {
		f := Finding{ID: "f3", ReproduceCmd: pos, ExpectedSignal: "SECRET", Level: E2Tentative}
		r := fakeRepro{outputs: map[string]string{pos: "nothing"}}
		res := PromoteDifferential(ctx, f, r, neg)
		if res.To != EFalsePositive {
			t.Fatalf("want FALSE_POSITIVE, got %s", res.To)
		}
	})

	t.Run("no negative degrades to positive-only", func(t *testing.T) {
		f := Finding{ID: "f4", ReproduceCmd: pos, ExpectedSignal: "user=admin", Level: E2Tentative}
		r := fakeRepro{outputs: map[string]string{pos: "user=admin"}}
		res := PromoteDifferential(ctx, f, r, "")
		if res.To != E3Verified {
			t.Fatalf("want E3Verified (positive-only), got %s (%s)", res.To, res.Note)
		}
	})

	t.Run("missing reproduce stays tentative", func(t *testing.T) {
		f := Finding{ID: "f5", Level: E2Tentative}
		res := PromoteDifferential(ctx, f, fakeRepro{}, neg)
		if res.To != E2Tentative {
			t.Fatalf("want E2Tentative, got %s", res.To)
		}
	})
}

func TestLevelString(t *testing.T) {
	if E3Verified.String() != "E3_VERIFIED" {
		t.Fatalf("unexpected level string: %s", E3Verified)
	}
	_ = fmt.Sprintf("%s", E0Raw) // log-friendly
}
