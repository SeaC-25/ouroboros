package api

// hypergraph.go — conjugate hypergraph matching over the recipe node store
// (kb/hypergraph.jsonl, ~1.9k section-level recipe nodes distilled from the
// Claude-Red skill library).
//
// Plan alignment (Conjugate Hypergraph):
//   entity state    = bitmask of observed attack-surface components
//   recipe node     = payload + precondition bitmask (tail of its hyperedge)
//   matching        = entity ∩ precondition coverage >= 70% AND no excluded
//                     component set; the matched node's slice (<=1.5K) is
//                     what wakes the model with — never the whole graph.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Component dimensions (mirrors benchmark/distill_hypergraph.py DIM_BITS).
const (
	DimSurf   uint16 = 1 << 0
	DimWeb    uint16 = 1 << 1
	DimWAF    uint16 = 1 << 2
	DimInject uint16 = 1 << 3
	DimAuth   uint16 = 1 << 4
	DimFile   uint16 = 1 << 5
	DimRCE    uint16 = 1 << 6
	DimCloud  uint16 = 1 << 7
	DimNet    uint16 = 1 << 8
	DimCrypto uint16 = 1 << 9
	DimMobile uint16 = 1 << 10
	DimRecon  uint16 = 1 << 11
)

var dimNames = map[uint16]string{
	DimSurf: "网络面", DimWeb: "Web应用", DimWAF: "拦截过滤", DimInject: "注入点",
	DimAuth: "认证面", DimFile: "文件处理", DimRCE: "命令执行", DimCloud: "云/容器",
	DimNet: "内网/域", DimCrypto: "密码学", DimMobile: "移动/IoT", DimRecon: "侦察枚举",
}

func dimNamesFor(mask uint16) []string {
	var out []string
	for bit, name := range dimNames {
		if mask&bit != 0 {
			out = append(out, name)
		}
	}
	return out
}

type recipeNode struct {
	ID           string   `json:"id"`
	Category     string   `json:"category"`
	Skill        string   `json:"skill"`
	Path         string   `json:"path"`
	Section      string   `json:"section"`
	Precondition uint16   `json:"precondition"`
	Exclude      uint16   `json:"exclude"`
	Tags         []string `json:"tags"`
	Payload      string   `json:"payload"`
	Summary      string   `json:"summary"`
}

// hypergraphPath resolves the node store relative to the workspace.
func (s *Server) hypergraphPath() string {
	return filepath.Join(s.workspace, "kb", "hypergraph.jsonl")
}

// hypergraphLoad reads all recipe nodes (absent -> nil, callers degrade).
func (s *Server) hypergraphLoad() []recipeNode {
	data, err := os.ReadFile(s.hypergraphPath())
	if err != nil {
		return nil
	}
	var nodes []recipeNode
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var node recipeNode
		if json.Unmarshal([]byte(line), &node) == nil && node.ID != "" {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// entityStateFromText derives an entity component bitmask from free text
// (stuck fingerprint, goal, or blackboard facts). CJK alias bridge reused.
func entityStateFromText(text string) uint16 {
	lower := strings.ToLower(text)
	state := uint16(0)
	set := func(bits ...uint16) {
		for _, b := range bits {
			state |= b
		}
	}
	// English technique words.
	for _, w := range []string{"waf", "filter", "blocking", "intercept", "403", "proxy"} {
		if strings.Contains(lower, w) {
			set(DimWAF)
		}
	}
	for _, w := range []string{"inject", "sqli", "sql injection", "xss", "ssti", "xxe", "deserial"} {
		if strings.Contains(lower, w) {
			set(DimInject, DimWeb)
		}
	}
	for _, w := range []string{"login", "auth", "jwt", "oauth", "token", "session", "cookie", "credential", "password"} {
		if strings.Contains(lower, w) {
			set(DimAuth)
		}
	}
	for _, w := range []string{"command", "rce", "exec", "shell", "os system"} {
		if strings.Contains(lower, w) {
			set(DimRCE, DimWeb)
		}
	}
	for _, w := range []string{"upload", "traversal", "lfi", "rfi", "file inclusion", "path"} {
		if strings.Contains(lower, w) {
			set(DimFile, DimWeb)
		}
	}
	for _, w := range []string{"api", "rest", "graphql", "websocket", "endpoint", "php", "node", "java", "wordpress"} {
		if strings.Contains(lower, w) {
			set(DimWeb, DimSurf)
		}
	}
	for _, w := range []string{"cloud", "aws", "azure", "gcp", "s3", "container", "docker", "k8s", "kubernetes", "sandbox"} {
		if strings.Contains(lower, w) {
			set(DimCloud)
		}
	}
	for _, w := range []string{"active directory", "kerberos", "ntlm", "smb", "ldap", "domain", "lateral", "internal network"} {
		if strings.Contains(lower, w) {
			set(DimNet)
		}
	}
	for _, w := range []string{"tls", "ssl", "certificate", "crypto", "cipher"} {
		if strings.Contains(lower, w) {
			set(DimCrypto)
		}
	}
	for _, w := range []string{"mobile", "android", "ios", "apk", "iot", "firmware", "wireless", "wifi"} {
		if strings.Contains(lower, w) {
			set(DimMobile)
		}
	}
	for _, w := range []string{"recon", "enum", "subdomain", "fingerprint", "port", "scan"} {
		if strings.Contains(lower, w) {
			set(DimRecon, DimSurf)
		}
	}
	// CJK aliases.
	cjk := map[string][]uint16{
		"注入": {DimInject, DimWeb}, "绕过": {DimWAF}, "拦截": {DimWAF}, "过滤": {DimWAF},
		"反向代理": {DimWAF, DimWeb}, "登录": {DimAuth}, "认证": {DimAuth}, "凭据": {DimAuth},
		"会话": {DimAuth}, "命令": {DimRCE, DimWeb}, "执行": {DimRCE}, "上传": {DimFile, DimWeb},
		"文件": {DimFile}, "遍历": {DimFile, DimWeb}, "容器": {DimCloud}, "逃逸": {DimCloud},
		"反序列化": {DimInject, DimWeb}, "提权": {DimAuth}, "越权": {DimAuth},
		"内网": {DimNet}, "域控": {DimNet}, "横向": {DimNet}, "证书": {DimCrypto},
		"加密": {DimCrypto}, "子域": {DimRecon, DimSurf}, "爆破": {DimRecon}, "枚举": {DimRecon},
		"移动": {DimMobile}, "无线": {DimMobile},
	}
	for word, bits := range cjk {
		if strings.Contains(text, word) {
			set(bits...)
		}
	}
	return state
}

// hyperMatchCoverage computes precondition coverage (matched bits / required
// bits) and exclusion violation in one pass.
func hyperMatchCoverage(entity uint16, node recipeNode) (float64, bool) {
	if node.Exclude&entity != 0 {
		return 0, false
	}
	if node.Precondition == 0 {
		return 0, false
	}
	matched := entity & node.Precondition
	required := popcount16(node.Precondition)
	got := popcount16(matched)
	return float64(got) / float64(required), true
}

func popcount16(v uint16) int {
	count := 0
	for v != 0 {
		v &= v - 1
		count++
	}
	return count
}

// hyperRetrieve returns recipe nodes whose hyperedge preconditions are
// >= 70% satisfied by the entity state (plan: 70% match wakes the model).
// Text additionally filters by tag/section keywords to break ties.
func (s *Server) hyperRetrieve(stateText string, limit int) []recipeNode {
	if limit <= 0 {
		limit = 3
	}
	nodes := s.hypergraphLoad()
	if len(nodes) == 0 || stateText == "" {
		return nil
	}
	entity := entityStateFromText(stateText)
	if entity == 0 {
		return nil
	}
	textTokens := kbQueryTokensWithAliases(stateText)

	type scored struct {
		node  recipeNode
		score float64
	}
	var hits []scored
	for _, node := range nodes {
		cov, ok := hyperMatchCoverage(entity, node)
		if !ok || cov < 0.7 {
			continue
		}
		// Tag affinity bonus (lexical tiebreak).
		bonus := 0.0
		for _, tag := range node.Tags {
			if textTokens[tag] {
				bonus += 0.05
			}
		}
		hits = append(hits, scored{node: node, score: cov + bonus})
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	if len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]recipeNode, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.node)
	}
	return out
}

// hyperSliceBlock renders matched recipe nodes as the <=1.5K injection for
// the model (plan: 3-5 entity slices only, prompt capped).
func (s *Server) hyperSliceBlock(stateText string) string {
	nodes := s.hyperRetrieve(stateText, 3)
	if len(nodes) == 0 {
		return ""
	}
	entity := entityStateFromText(stateText)
	var b strings.Builder
	b.WriteString("\n【配方超图命中】当前状态（" + strings.Join(dimNamesFor(entity), "·") + "）匹配的手法：\n")
	for _, n := range nodes {
		b.WriteString("### " + n.Section + "（" + n.Skill + "）\n")
		if n.Payload != "" {
			payload := n.Payload
			if len(payload) > 500 {
				payload = payload[:500] + "…"
			}
			b.WriteString("```\n" + payload + "\n```\n")
		} else if n.Summary != "" {
			b.WriteString(n.Summary[:min(200, len(n.Summary))] + "\n")
		}
		b.WriteString("深挖： ACTION:READ " + n.Path + "\n\n")
	}
	block := b.String()
	if len(block) > 1500 {
		block = block[:1500] + "\n…（截断）\n"
	}
	return block
}
