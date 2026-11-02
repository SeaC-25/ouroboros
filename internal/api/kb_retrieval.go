package api

// kb_retrieval.go — recipe retrieval over the distilled skill index
// (kb/index.jsonl, built by benchmark/distill_skill_index.py from the
// Claude-Red MIT skill library).
//
// GateMesh L4 usage points:
//   - Progress gate: stuck-loop fingerprint (403/WAF/filter/bypass...) ->
//     matching recipe sections injected into the pivot prompt (<=1.5K, L3 line)
//   - SPAWN seeding: goal semantics -> matching skill workflow prepended to
//     the sub-loop's system prompt (alongside blackboard seeds)
//
// Retrieval is lexical (category/trigger/payload-title overlap) — the model,
// not the matcher, decides relevance; full files stay on disk for READ.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type kbSkillEntry struct {
	Category     string          `json:"category"`
	Skill        string          `json:"skill"`
	Name         string          `json:"name"`
	Path         string          `json:"path"`
	Triggers     []string        `json:"triggers"`
	Workflow     string          `json:"workflow"`
	PayloadHeads []kbPayloadHead `json:"payload_heads"`
}

type kbPayloadHead struct {
	T string `json:"t"`
	P string `json:"p"`
}

// kbIndexPath resolves the index relative to the workspace root.
func (s *Server) kbIndexPath() string {
	return filepath.Join(s.workspace, "kb", "index.jsonl")
}

// kbLoadIndex reads the distilled index (absent -> nil, callers degrade).
func (s *Server) kbLoadIndex() []kbSkillEntry {
	data, err := os.ReadFile(s.kbIndexPath())
	if err != nil {
		return nil
	}
	var entries []kbSkillEntry
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry kbSkillEntry
		if json.Unmarshal([]byte(line), &entry) == nil && entry.Skill != "" {
			entries = append(entries, entry)
		}
	}
	return entries
}

// kbQuery tokens: lowercase distinct words (CJK bigrams like the blackboard
// relevance matcher — goals and stuck-fingerprints are bilingual).
func kbQueryTokens(text string) map[string]bool {
	return tokenizeForRelevance(text)
}

// cjkTechAliases maps common CJK goal phrasing to English tech triggers —
// goals are written in Chinese, the index is English; without the bridge
// the matcher sees zero overlap.
var cjkTechAliases = map[string][]string{
	"注入":   {"inject", "sqli"},
	"绕过":   {"bypass"},
	"反向代理": {"proxy", "reverse"},
	"过滤":   {"filter", "waf"},
	"登录":   {"login", "auth"},
	"提权":   {"privesc", "privilege"},
	"越权":   {"auth", "api"},
	"上传":   {"upload"},
	"执行":   {"exec", "rce", "command"},
	"命令":   {"command", "shell", "exec"},
	"逃逸":   {"escape", "sandbox", "container"},
	"容器":   {"container", "docker"},
	"反序列化": {"deserial"},
	"凭据":   {"token", "auth", "session"},
	"会话":   {"session", "cookie"},
	"证书":   {"tls", "ssl"},
	"加密":   {"crypto"},
	"爆破":   {"enum", "recon"},
	"信息收集": {"recon", "enum", "osint"},
	"子域":   {"subdomain", "dns"},
	"漏洞":   {"cve", "exploit"},
	"利用":   {"exploit", "rce"},
	"内网":   {"network", "smb"},
	"域控":   {"active directory", "kerberos"},
	"横向":   {"lateral"},
	"持久":   {"persistence"},
	"免杀":   {"evasion", "edr"},
	"沙箱":   {"sandbox", "escape"},
	"移动":   {"mobile", "android", "ios"},
	"无线":   {"wireless", "wifi"},
}

// kbQueryTokensWithAliases layers the CJK->English bridge over the base
// tokenizer.
func kbQueryTokensWithAliases(text string) map[string]bool {
	tokens := kbQueryTokens(text)
	lower := strings.ToLower(text)
	for cjk, aliases := range cjkTechAliases {
		if strings.Contains(lower, cjk) {
			for _, alias := range aliases {
				tokens[alias] = true
			}
		}
	}
	return tokens
}

// kbScore entry relevance against query tokens.
func kbScore(entry kbSkillEntry, query map[string]bool) int {
	score := 0
	for _, trigger := range entry.Triggers {
		if query[strings.ToLower(strings.TrimSpace(trigger))] {
			score += 2 // trigger words weigh double
		}
	}
	if query[strings.ToLower(entry.Category)] {
		score += 2
	}
	headTokens := " " + strings.ToLower(entry.Workflow) + " "
	for _, h := range entry.PayloadHeads {
		headTokens += " " + strings.ToLower(h.T+" "+h.P)
	}
	for token := range query {
		if len(token) >= 4 && strings.Contains(headTokens, token) {
			score++
		}
	}
	return score
}

// kbRetrieveSkills returns up to limit best-matching entries.
func (s *Server) kbRetrieveSkills(queryText string, limit int) []kbSkillEntry {
	if limit <= 0 {
		limit = 3
	}
	entries := s.kbLoadIndex()
	if len(entries) == 0 {
		return nil
	}
	query := kbQueryTokensWithAliases(queryText)
	if len(query) == 0 {
		return nil
	}
	type scored struct {
		entry kbSkillEntry
		score int
	}
	var hits []scored
	for _, entry := range entries {
		if sc := kbScore(entry, query); sc >= 3 {
			hits = append(hits, scored{entry: entry, score: sc})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	if len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]kbSkillEntry, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.entry)
	}
	return out
}

// kbStuckRecipeBlock builds the Progress-gate injection: compact recipe
// hints for the stuck fingerprint (workflow + matching payload section
// titles + pointer to the full file). Capped to keep the L3 line.
func (s *Server) kbStuckRecipeBlock(fingerprint string) string {
	entries := s.kbRetrieveSkills(fingerprint, 2)
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n【知识库配方】与当前卡门特征匹配的先验手法：\n")
	for _, e := range entries {
		b.WriteString("## " + e.Name + "（" + e.Category + "）\n")
		wf := strings.TrimSpace(e.Workflow)
		if len(wf) > 700 {
			wf = wf[:700] + "…"
		}
		b.WriteString(wf + "\n")
		if len(e.PayloadHeads) > 0 {
			titles := make([]string, 0, len(e.PayloadHeads))
			for _, h := range e.PayloadHeads {
				if h.T != "" {
					titles = append(titles, h.T)
				}
			}
			if len(titles) > 6 {
				titles = titles[:6]
			}
			b.WriteString("可用手法节： " + strings.Join(titles, "、") + "\n")
		}
		b.WriteString("完整配方： ACTION:READ " + e.Path + "\n\n")
	}
	block := b.String()
	if len(block) > 1600 {
		block = block[:1600] + "\n…（截断，读文件深挖）\n"
	}
	return block
}

// kbSpawnSeedBlock builds the SPAWN goal-seed: top skill workflow summaries
// relevant to the sub-loop's goal (knowledge transfer alongside blackboard
// facts).
func (s *Server) kbSpawnSeedBlock(goal string) string {
	entries := s.kbRetrieveSkills(goal, 2)
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n相关技能方法论（来自知识库，按需 ACTION:READ 深挖）：\n")
	for _, e := range entries {
		wf := strings.TrimSpace(e.Workflow)
		if len(wf) > 500 {
			wf = wf[:500] + "…"
		}
		b.WriteString("- " + e.Name + "：" + wf + "（完整：" + e.Path + "）\n")
	}
	return b.String()
}
