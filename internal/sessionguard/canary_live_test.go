package sessionguard

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
)

// TestCanaryAgainstLiveLab is an integration test (skipped unless the lab is
// up): replicates the agent's exact sessioned-client + canary wiring against
// the real lab login flow. Run with:
//
//	OURO_LAB_URL=http://127.0.0.1:18083 go test ./internal/sessionguard/ -run TestCanaryAgainstLiveLab -v
func TestCanaryAgainstLiveLab(t *testing.T) {
	lab := "http://127.0.0.1:18083"
	form := url.Values{"username": {"admin"}, "password": {"admin123"}}
	jar, _ := cookiejar.New(nil)
	base := &http.Client{Jar: jar}
	client := &http.Client{
		Transport: base.Transport,
		Jar:       jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if IsDestructiveAuthPath(req.URL) {
				return fmt.Errorf("blocked logout path")
			}
			return nil
		},
	}
	resp, err := client.Post(lab+"/login", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Skipf("lab not reachable: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("login status %d", resp.StatusCode)
	}

	c, err := NewCanary("primary", CanaryConfig{
		ProbeURL:       lab + "/admin/secret",
		ExpectedStatus: http.StatusOK,
		Client:         client,
		ProbeMethod:    http.MethodGet,
		EveryN:         1,
	})
	if err != nil {
		t.Fatalf("canary: %v", err)
	}
	c.Tick()
	hit, perr := c.ProbeNow(nil)
	if perr != nil {
		t.Fatalf("probe error: %v", perr)
	}
	if hit {
		frozen, reason := c.Frozen()
		t.Fatalf("probe MISSED (hit=%v frozen=%v reason=%s) — session lost or probe wrong", hit, frozen, reason)
	}
	if frozen, _ := c.Frozen(); frozen {
		t.Fatal("canary froze on a healthy session")
	}
}
