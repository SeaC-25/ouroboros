package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
)

func TestParserEventsForToolExecutionPromotesNmapOpenServiceForCIDRScope(t *testing.T) {
	events := parserEventsForToolExecution("run-nmap-cidr-scope", nmapParserRequest([]ScopeTarget{{Kind: "cidr", Value: "10.0.0.0/24"}}), ToolExecutionResult{
		ExecutionID: "native-nmap",
		Status:      "completed",
		Transcript: `<?xml version="1.0"?>
<nmaprun scanner="nmap" args="nmap -oX - 10.0.0.0/24 --script secret-token" startstr="2026-06-03" version="7.95">
  <host>
    <address addr="10.0.0.5" addrtype="ipv4"/>
    <hostnames><hostname name="ptr.evil.example" type="PTR"/></hostnames>
    <ports>
      <port protocol="tcp" portid="443">
        <state state="open" reason="syn-ack" reason_ttl="0"/>
        <service name="https" tunnel="ssl" conf="10" product="nginx" version="1.27" extrainfo="secret-token" hostname="ptr.evil.example" ostype="Linux" devicetype="server">
          <cpe>cpe:/a:nginx:nginx:1.27</cpe>
        </service>
      </port>
    </ports>
  </host>
  <host>
    <address addr="10.0.1.5" addrtype="ipv4"/>
    <ports>
      <port protocol="tcp" portid="22">
        <state state="open"/>
        <service name="ssh" product="OpenSSH"/>
      </port>
    </ports>
  </host>
</nmaprun>`,
	})

	if len(events) != 2 {
		t.Fatalf("expected parser event and one in-scope surface fact, got %#v", events)
	}
	if events[0].Type != eventlog.EventParser || events[1].Type != eventlog.EventFact {
		t.Fatalf("expected parser event followed by fact event, got %#v", events)
	}
	var parserPayload struct {
		Parser       string `json:"parser"`
		Status       string `json:"status"`
		FindingCount int    `json:"finding_count"`
		ToolID       string `json:"tool_id"`
	}
	if err := json.Unmarshal(events[0].Payload, &parserPayload); err != nil {
		t.Fatalf("decode parser event: %v", err)
	}
	if parserPayload.Parser != "nmap_xml" || parserPayload.Status != "parsed" || parserPayload.FindingCount != 1 || parserPayload.ToolID != "nmap" {
		t.Fatalf("unexpected nmap parser payload: %#v", parserPayload)
	}
	assertNmapFactWhitelist(t, events[1].Payload)
	var payload struct {
		Source        string  `json:"source"`
		Kind          string  `json:"kind"`
		Statement     string  `json:"statement"`
		Confidence    float64 `json:"confidence"`
		ParserID      string  `json:"parser_id"`
		ScopeKind     string  `json:"scope_kind"`
		ScopeTarget   string  `json:"scope_target"`
		Host          string  `json:"host"`
		Address       string  `json:"address"`
		AddressType   string  `json:"address_type"`
		Protocol      string  `json:"protocol"`
		Port          int     `json:"port"`
		PortState     string  `json:"port_state"`
		ServiceName   string  `json:"service_name"`
		ServiceTunnel string  `json:"service_tunnel"`
		ServiceConf   int     `json:"service_conf"`
	}
	if err := json.Unmarshal(events[1].Payload, &payload); err != nil {
		t.Fatalf("decode fact event: %v", err)
	}
	if payload.Source != "parser.nmap_xml" || payload.Kind != "surface" || payload.ParserID != "nmap_xml" {
		t.Fatalf("unexpected nmap surface identity: %#v", payload)
	}
	if payload.ScopeKind != "cidr" || payload.ScopeTarget != "10.0.0.0/24" || payload.Host != "10.0.0.5" || payload.Address != "10.0.0.5" || payload.AddressType != "ipv4" {
		t.Fatalf("unexpected CIDR-bound host metadata: %#v", payload)
	}
	if payload.Protocol != "tcp" || payload.Port != 443 || payload.PortState != "open" || payload.ServiceName != "https" || payload.ServiceTunnel != "ssl" || payload.ServiceConf != 10 {
		t.Fatalf("unexpected nmap service metadata: %#v", payload)
	}
	if payload.Statement != "nmap | 10.0.0.5 | tcp/443 | open | service=https" {
		t.Fatalf("unexpected compact nmap statement: %#v", payload.Statement)
	}
	if payload.Confidence < 0.7 {
		t.Fatalf("expected nmap surface to have useful confidence, got %#v", payload)
	}
	for _, leaked := range []string{"10.0.1.5", "ptr.evil.example", "secret-token", "OpenSSH", "nginx", "1.27", "reason", "cpe:/"} {
		if strings.Contains(string(events[1].Payload), leaked) || strings.Contains(payload.Statement, leaked) {
			t.Fatalf("nmap surface payload leaked untrusted output %q: %s", leaked, events[1].Payload)
		}
	}
}

func TestParserEventsForToolExecutionPromotesNmapOpenServiceForHostScope(t *testing.T) {
	events := parserEventsForToolExecution("run-nmap-host-scope", nmapParserRequest([]ScopeTarget{{Kind: "host", Value: "api.example.com"}}), ToolExecutionResult{
		ExecutionID: "native-nmap",
		Status:      "completed",
		Transcript: `<?xml version="1.0"?>
<nmaprun>
  <host>
    <address addr="203.0.113.10" addrtype="ipv4"/>
    <hostnames><hostname name="ptr.evil.example" type="PTR"/></hostnames>
    <ports>
      <port protocol="tcp" portid="22">
        <state state="open"/>
        <service name="ssh" conf="10" product="OpenSSH" version="9.6"/>
      </port>
    </ports>
  </host>
</nmaprun>`,
	})

	if len(events) != 2 {
		t.Fatalf("expected parser event and one host-bound surface fact, got %#v", events)
	}
	var payload struct {
		Statement   string `json:"statement"`
		ScopeKind   string `json:"scope_kind"`
		ScopeTarget string `json:"scope_target"`
		Host        string `json:"host"`
		Address     string `json:"address"`
		Protocol    string `json:"protocol"`
		Port        int    `json:"port"`
		ServiceName string `json:"service_name"`
	}
	if err := json.Unmarshal(events[1].Payload, &payload); err != nil {
		t.Fatalf("decode fact event: %v", err)
	}
	if payload.ScopeKind != "host" || payload.ScopeTarget != "api.example.com" || payload.Host != "api.example.com" || payload.Address != "203.0.113.10" {
		t.Fatalf("expected host scope to bind fact to scoped host target, got %#v", payload)
	}
	if payload.Protocol != "tcp" || payload.Port != 22 || payload.ServiceName != "ssh" {
		t.Fatalf("unexpected host-scope service metadata: %#v", payload)
	}
	if payload.Statement != "nmap | api.example.com | tcp/22 | open | service=ssh" {
		t.Fatalf("unexpected host-scope statement: %#v", payload.Statement)
	}
	for _, leaked := range []string{"ptr.evil.example", "OpenSSH", "9.6"} {
		if strings.Contains(string(events[1].Payload), leaked) || strings.Contains(payload.Statement, leaked) {
			t.Fatalf("host-scope fact leaked untrusted XML metadata %q: %s", leaked, events[1].Payload)
		}
	}
}

func TestParserEventsForToolExecutionSanitizesNmapXMLAndFiltersUnsafeFields(t *testing.T) {
	events := parserEventsForToolExecution("run-nmap-sanitize", nmapParserRequest([]ScopeTarget{{Kind: "cidr", Value: "10.0.0.0/24"}}), ToolExecutionResult{
		ExecutionID: "native-nmap",
		Status:      "completed",
		Transcript: `<?xml version="1.0"?>
<nmaprun scanner="nmap" args="nmap -A --script secret-token 10.0.0.0/24" start="1780000000" startstr="Wed Jun 03 12:00:00 2026" version="7.95" xmloutputversion="1.05">
  <scaninfo type="syn" protocol="tcp" numservices="1" services="8443"/>
  <host starttime="1780000000" endtime="1780000001">
    <status state="up" reason="echo-reply" reason_ttl="63"/>
    <address addr="10.0.0.7" addrtype="ipv4"/>
    <address addr="00:11:22:33:44:55" addrtype="mac" vendor="EvilCorp"/>
    <hostnames><hostname name="ptr.evil.example" type="PTR"/></hostnames>
    <ports>
      <extraports state="closed" count="999"><extrareasons reason="resets" count="999"/></extraports>
      <port protocol="tcp" portid="8443">
        <state state="open" reason="syn-ack" reason_ttl="61"/>
        <service name="https" tunnel="ssl" conf="8" method="probed" product="EvilProduct" version="1.2.3" extrainfo="secret-token" hostname="service.evil.example" ostype="Linux" devicetype="router" servicefp="SF-Port8443-TCP:secret-token">
          <cpe>cpe:/a:evil:product:1.2.3</cpe>
        </service>
        <script id="ssl-cert" output="secret-token certificate"/>
      </port>
    </ports>
    <os><osmatch name="Linux 5.X" accuracy="98"><osclass vendor="Linux" osfamily="Linux"/></osmatch></os>
    <uptime seconds="3600" lastboot="secret-token"/>
    <distance value="1"/>
    <tcpsequence index="1" difficulty="secret-token"/>
    <ipidsequence class="secret-token"/>
    <tcptssequence class="secret-token"/>
    <trace><hop ttl="1" ipaddr="10.0.0.1" host="router.evil.example"/></trace>
    <hostscript><script id="host-script" output="secret-token"/></hostscript>
  </host>
  <runstats><finished time="1780000001" timestr="Wed Jun 03 12:00:01 2026" elapsed="1.0"/><hosts up="1" down="0" total="1"/></runstats>
</nmaprun>`,
	})

	if len(events) != 2 {
		t.Fatalf("expected parser event and sanitized surface fact, got %#v", events)
	}
	assertNmapFactWhitelist(t, events[1].Payload)
	payloadText := string(events[1].Payload)
	for _, leaked := range []string{
		"<nmaprun", "scanner", "args", "Wed Jun 03", "7.95", "xmloutputversion", "scaninfo",
		"ptr.evil.example", "00:11:22:33:44:55", "EvilCorp", "Linux 5.X", "uptime", "distance",
		"tcpsequence", "trace", "runstats", "extraports", "reason", "reason_ttl", "ssl-cert",
		"host-script", "EvilProduct", "1.2.3", "service.evil.example", "router.evil.example",
		"cpe:/", "SF-Port8443", "secret-token",
	} {
		if strings.Contains(payloadText, leaked) {
			t.Fatalf("nmap fact payload leaked forbidden XML field %q: %s", leaked, payloadText)
		}
	}
}

func TestParserEventsForToolExecutionDropsOutOfScopeNmapCIDRHost(t *testing.T) {
	events := parserEventsForToolExecution("run-nmap-cidr-out-of-scope", nmapParserRequest([]ScopeTarget{{Kind: "cidr", Value: "10.0.0.0/24"}}), ToolExecutionResult{
		ExecutionID: "native-nmap",
		Status:      "completed",
		Transcript: `<?xml version="1.0"?>
<nmaprun>
  <host>
    <address addr="10.0.1.20" addrtype="ipv4"/>
    <ports><port protocol="tcp" portid="22"><state state="open"/><service name="ssh"/></port></ports>
  </host>
  <host>
    <hostnames><hostname name="unverifiable.evil.example" type="PTR"/></hostnames>
    <ports><port protocol="tcp" portid="80"><state state="open"/><service name="http"/></port></ports>
  </host>
</nmaprun>`,
	})

	if len(events) != 1 || events[0].Type != eventlog.EventParser {
		t.Fatalf("expected parser event only for out-of-scope/unverifiable hosts, got %#v", events)
	}
	var payload struct {
		Parser       string `json:"parser"`
		Status       string `json:"status"`
		FindingCount int    `json:"finding_count"`
	}
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("decode parser event: %v", err)
	}
	if payload.Parser != "nmap_xml" || payload.Status != "parsed" || payload.FindingCount != 0 {
		t.Fatalf("expected parsed event with zero promoted surfaces, got %#v", payload)
	}
}

func TestParserEventsForToolExecutionIgnoresClosedFilteredAndExtraports(t *testing.T) {
	events := parserEventsForToolExecution("run-nmap-open-only", nmapParserRequest([]ScopeTarget{{Kind: "host", Value: "api.example.com"}}), ToolExecutionResult{
		ExecutionID: "native-nmap",
		Status:      "completed",
		Transcript: `<?xml version="1.0"?>
<nmaprun>
  <host>
    <address addr="203.0.113.10" addrtype="ipv4"/>
    <ports>
      <extraports state="closed" count="997"/>
      <port protocol="tcp" portid="80"><state state="closed"/><service name="http"/></port>
      <port protocol="tcp" portid="81"><state state="filtered"/><service name="http"/></port>
      <port protocol="udp" portid="53"><state state="open|filtered"/><service name="domain"/></port>
      <port protocol="tcp" portid="443"><state state="open"/><service name="https"/></port>
    </ports>
  </host>
</nmaprun>`,
	})

	if len(events) != 2 {
		t.Fatalf("expected parser event and only the open port fact, got %#v", events)
	}
	var payload struct {
		Statement string `json:"statement"`
		Protocol  string `json:"protocol"`
		Port      int    `json:"port"`
		PortState string `json:"port_state"`
	}
	if err := json.Unmarshal(events[1].Payload, &payload); err != nil {
		t.Fatalf("decode fact event: %v", err)
	}
	if payload.Protocol != "tcp" || payload.Port != 443 || payload.PortState != "open" {
		t.Fatalf("expected only tcp/443 open to be promoted, got %#v", payload)
	}
	for _, leaked := range []string{"tcp/80", "tcp/81", "udp/53", "closed", "filtered", "997"} {
		if strings.Contains(payload.Statement, leaked) || strings.Contains(string(events[1].Payload), leaked) {
			t.Fatalf("nmap fact leaked ignored port/extraports data %q: %s", leaked, events[1].Payload)
		}
	}
}

func TestParserEventsForToolExecutionRequiresNmapHostOrCIDRScope(t *testing.T) {
	for _, scopeTargets := range [][]ScopeTarget{
		nil,
		{{Kind: "domain", Value: "example.com"}},
		{{Kind: "url", Value: "https://app.example"}},
	} {
		events := parserEventsForToolExecution("run-nmap-invalid-scope", nmapParserRequest(scopeTargets), ToolExecutionResult{
			ExecutionID: "native-nmap",
			Status:      "completed",
			Transcript:  `<?xml version="1.0"?><nmaprun><host><address addr="10.0.0.5" addrtype="ipv4"/><ports><port protocol="tcp" portid="22"><state state="open"/><service name="ssh"/></port></ports></host></nmaprun>`,
		})

		if len(events) != 1 || events[0].Type != eventlog.EventParser {
			t.Fatalf("expected one parse-failure event for scope %#v, got %#v", scopeTargets, events)
		}
		var payload struct {
			Parser       string `json:"parser"`
			Status       string `json:"status"`
			FindingCount int    `json:"finding_count"`
			Message      string `json:"message"`
		}
		if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
			t.Fatalf("decode parser event: %v", err)
		}
		if payload.Parser != "nmap_xml" || payload.Status != "parse_failed" || payload.FindingCount != 0 || payload.Message == "" {
			t.Fatalf("unexpected no-scope parse-failure payload: %#v", payload)
		}
	}
}

func TestParserEventsForToolExecutionRecordsNmapParseFailureWithoutSurface(t *testing.T) {
	events := parserEventsForToolExecution("run-nmap-parse-failure", nmapParserRequest([]ScopeTarget{{Kind: "cidr", Value: "10.0.0.0/24"}}), ToolExecutionResult{
		ExecutionID: "native-nmap",
		Status:      "completed",
		Transcript:  "not-xml",
	})

	if len(events) != 1 || events[0].Type != eventlog.EventParser {
		t.Fatalf("expected one parse-failure event, got %#v", events)
	}
	var payload struct {
		Parser       string `json:"parser"`
		ParserID     string `json:"parser_id"`
		Status       string `json:"status"`
		ToolID       string `json:"tool_id"`
		ExecutionID  string `json:"execution_id"`
		FindingCount int    `json:"finding_count"`
		Message      string `json:"message"`
	}
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("decode parser event: %v", err)
	}
	if payload.Parser != "nmap_xml" || payload.ParserID != "nmap_xml" || payload.Status != "parse_failed" || payload.ToolID != "nmap" || payload.ExecutionID != "native-nmap" || payload.FindingCount != 0 || payload.Message == "" {
		t.Fatalf("unexpected parse-failure payload: %#v", payload)
	}
}

func nmapParserRequest(scopeTargets []ScopeTarget) ToolExecutionRequest {
	return ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "nmap-service-fingerprint:01",
			Capability:   "infra.service_enumeration",
			ManifestName: "infra-pack",
			ToolID:       "nmap",
			Operation:    "service_fingerprint",
			ParserID:     "nmap_xml",
		},
		ScopeTargets: scopeTargets,
	}
}

func assertNmapFactWhitelist(t *testing.T, payload []byte) {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil {
		t.Fatalf("decode raw nmap fact event: %v", err)
	}
	allowed := map[string]bool{
		"fact_id":        true,
		"kind":           true,
		"statement":      true,
		"confidence":     true,
		"source":         true,
		"parser":         true,
		"parser_id":      true,
		"plan_node_id":   true,
		"tool":           true,
		"tool_id":        true,
		"operation":      true,
		"execution_id":   true,
		"scope_kind":     true,
		"scope_target":   true,
		"host":           true,
		"address":        true,
		"address_type":   true,
		"protocol":       true,
		"port":           true,
		"port_state":     true,
		"service_name":   true,
		"service_tunnel": true,
		"service_conf":   true,
	}
	for key := range raw {
		if !allowed[key] {
			t.Fatalf("nmap fact persisted non-whitelisted key %q: %s", key, string(payload))
		}
	}
}
