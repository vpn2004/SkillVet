package rater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type AuditEvidence struct {
	RuleID      string `json:"rule_id"`
	Severity    string `json:"severity"`
	Path        string `json:"path"`
	SnippetHash string `json:"snippet_hash"`
	Excerpt     string `json:"excerpt,omitempty"`
}

type SkillAudit struct {
	SkillID       string          `json:"skill_id"`
	Score         float64         `json:"score"`
	RuleScore     float64         `json:"rule_score,omitempty"`
	LLMScore      *float64        `json:"llm_score,omitempty"`
	ScoreSource   string          `json:"score_source,omitempty"`
	LLMModel      string          `json:"llm_model,omitempty"`
	LLMReason     string          `json:"llm_reason,omitempty"`
	LLMError      string          `json:"llm_error,omitempty"`
	Strengths     []string        `json:"strengths"`
	Risks         []string        `json:"risks"`
	RiskHigh      int             `json:"risk_high"`
	RiskMedium    int             `json:"risk_medium"`
	RiskLow       int             `json:"risk_low"`
	ReviewedFiles []string        `json:"reviewed_files"`
	Evidence      []AuditEvidence `json:"evidence"`
	ReportHash    string          `json:"report_hash"`
	Report        string          `json:"report"`
	Sampled       bool            `json:"sampled"`
}

type AuditCacheEntry struct {
	ReportHash string `json:"report_hash"`
	UpdatedAt  string `json:"updated_at"`
}

type AuditCache struct {
	Items map[string]AuditCacheEntry `json:"items"`
}

func BuildSkillAudit(skillID, skillDir string, sampleRate int, maxRunes int) (SkillAudit, error) {
	if strings.TrimSpace(skillID) == "" {
		return SkillAudit{}, fmt.Errorf("skill_id is required")
	}
	if maxRunes <= 0 {
		maxRunes = 500
	}

	files := collectAuditFiles(skillDir)
	strengths := make([]string, 0)
	risks := make([]string, 0)
	evidences := make([]AuditEvidence, 0)
	high, medium, low := 0, 0, 0
	score := 95.0

	foundDangerExec := false
	for _, f := range files {
		buf, err := os.ReadFile(filepath.Join(skillDir, f))
		if err != nil {
			continue
		}
		origin := string(buf)
		content := strings.ToLower(origin)

		hasDangerExec := containsAny(content, "os.system(", "subprocess.", "exec(", "eval(", "shell=true", "bash -c")
		hasUntrustedInput := containsAny(content, "input(", "sys.argv", "argparse", "request.args", "request.form", "ctx.query", "params[", "stdin", "click.option")
		hasFileIO := containsAny(content, "open(", "read_text(", "write_text(", "filepath.join", "path.join")

		if hasDangerExec {
			foundDangerExec = true
			if hasUntrustedInput {
				high++
				score -= 20
				risks = append(risks, fmt.Sprintf("发现可控输入触发的命令执行风险（%s）", f))
				evidences = append(evidences, buildEvidence("R_EXEC_UNTRUSTED", "high", f, origin, "subprocess"))
			} else {
				medium++
				score -= 8
				risks = append(risks, fmt.Sprintf("发现命令执行调用，暂未识别可控输入（%s）", f))
				evidences = append(evidences, buildEvidence("R_EXEC_STATIC", "medium", f, origin, "subprocess"))
			}
		}

		if strings.Contains(content, "http://") {
			medium++
			score -= 10
			risks = append(risks, fmt.Sprintf("发现明文HTTP调用（%s）", f))
			evidences = append(evidences, buildEvidence("R_HTTP_PLAINTEXT", "medium", f, origin, "http://"))
		}

		if hasFileIO {
			if hasUntrustedInput {
				medium++
				score -= 10
				risks = append(risks, fmt.Sprintf("发现可控输入参与文件路径读写（%s）", f))
				evidences = append(evidences, buildEvidence("R_PATH_UNTRUSTED", "medium", f, origin, "open("))
			} else {
				low++
				score -= 3
				risks = append(risks, fmt.Sprintf("发现文件读写操作，建议限制路径边界（%s）", f))
				evidences = append(evidences, buildEvidence("R_PATH_IO", "low", f, origin, "open("))
			}
		}
	}

	if !foundDangerExec {
		strengths = append(strengths, "未发现 os.system/subprocess/eval/exec 高危执行点")
	}
	if len(risks) == 0 {
		strengths = append(strengths, "未发现高风险规则命中，整体实现较克制")
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	if len(files) == 0 {
		files = []string{"SKILL.md"}
	}

	sort.Slice(evidences, func(i, j int) bool {
		if evidences[i].Path == evidences[j].Path {
			return evidences[i].RuleID < evidences[j].RuleID
		}
		return evidences[i].Path < evidences[j].Path
	})

	audit := SkillAudit{
		SkillID:       skillID,
		Score:         score,
		RuleScore:     score,
		ScoreSource:   "rule",
		Strengths:     strengths,
		Risks:         risks,
		RiskHigh:      high,
		RiskMedium:    medium,
		RiskLow:       low,
		ReviewedFiles: files,
		Evidence:      evidences,
	}

	audit.ReportHash = buildAuditHash(audit)
	audit.Sampled = shouldSample(audit.SkillID, audit.ReportHash, sampleRate)
	audit.Report = truncateRunes(buildAuditNarrative(audit), maxRunes)
	return audit, nil
}

func ComposeAuditComment(a SkillAudit, maxRunes int) string {
	if maxRunes <= 0 {
		maxRunes = 500
	}
	hash := a.ReportHash
	if len(hash) > 12 {
		hash = hash[:12]
	}
	sample := 0
	if a.Sampled {
		sample = 1
	}

	source := strings.TrimSpace(a.ScoreSource)
	if source == "" {
		source = "rule"
	}
	ruleScore := a.RuleScore
	if ruleScore <= 0 {
		ruleScore = a.Score
	}
	llmScore := "na"
	if a.LLMScore != nil {
		llmScore = fmt.Sprintf("%.1f", *a.LLMScore)
	}
	model := strings.TrimSpace(a.LLMModel)
	if model == "" {
		model = "na"
	}

	reportOneLine := strings.ReplaceAll(a.Report, "\n", " ")
	reportOneLine = strings.Join(strings.Fields(reportOneLine), " ")
	comment := fmt.Sprintf("audit:v2 hash=%s sample=%d source=%s rule=%.1f llm=%s final=%.1f model=%s risk(h/m/l)=%d/%d/%d files=%d evidence=%d summary=%s",
		hash, sample, source, ruleScore, llmScore, a.Score, model, a.RiskHigh, a.RiskMedium, a.RiskLow, len(a.ReviewedFiles), len(a.Evidence), reportOneLine)
	return truncateRunes(comment, maxRunes)
}

func LoadAuditCache(path string) (AuditCache, error) {
	cache := AuditCache{Items: map[string]AuditCacheEntry{}}
	buf, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cache, nil
		}
		return AuditCache{}, err
	}
	if len(buf) == 0 {
		return cache, nil
	}
	if err := json.Unmarshal(buf, &cache); err != nil {
		return AuditCache{}, err
	}
	if cache.Items == nil {
		cache.Items = map[string]AuditCacheEntry{}
	}
	return cache, nil
}

func SaveAuditCache(path string, cache AuditCache) error {
	if cache.Items == nil {
		cache.Items = map[string]AuditCacheEntry{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	buf, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, buf, 0o600)
}

func (c *AuditCache) ShouldUpload(skillID, reportHash string) bool {
	if c.Items == nil {
		c.Items = map[string]AuditCacheEntry{}
	}
	entry, ok := c.Items[skillID]
	if !ok {
		return true
	}
	return entry.ReportHash != reportHash
}

func (c *AuditCache) MarkUploaded(skillID, reportHash string) {
	if c.Items == nil {
		c.Items = map[string]AuditCacheEntry{}
	}
	c.Items[skillID] = AuditCacheEntry{ReportHash: reportHash, UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
}

func collectAuditFiles(skillDir string) []string {
	out := make([]string, 0)
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if _, err := os.Stat(skillPath); err == nil {
		out = append(out, "SKILL.md")
	}

	scriptsDir := filepath.Join(skillDir, "scripts")
	_ = filepath.WalkDir(scriptsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(skillDir, path)
		if rerr != nil {
			return nil
		}
		if strings.HasSuffix(rel, ".py") || strings.HasSuffix(rel, ".js") || strings.HasSuffix(rel, ".ts") || strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, ".sh") || strings.HasSuffix(rel, ".md") {
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})

	sort.Strings(out)
	if len(out) > 12 {
		out = out[:12]
	}
	return out
}

func containsAny(text string, tokens ...string) bool {
	for _, token := range tokens {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

func buildEvidence(ruleID, severity, path, content, token string) AuditEvidence {
	excerpt := snippetAround(content, strings.ToLower(token), 80)
	if excerpt == "" {
		excerpt = truncateRunes(strings.TrimSpace(content), 120)
	}
	h := sha256.Sum256([]byte(excerpt))
	return AuditEvidence{
		RuleID:      ruleID,
		Severity:    severity,
		Path:        path,
		SnippetHash: hex.EncodeToString(h[:]),
		Excerpt:     truncateRunes(strings.Join(strings.Fields(excerpt), " "), 120),
	}
}

func snippetAround(content, token string, radius int) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	lower := strings.ToLower(content)
	idx := strings.Index(lower, strings.ToLower(token))
	if idx < 0 {
		return ""
	}
	start := idx - radius
	if start < 0 {
		start = 0
	}
	end := idx + len(token) + radius
	if end > len(content) {
		end = len(content)
	}
	return content[start:end]
}

func buildAuditHash(a SkillAudit) string {
	evidenceParts := make([]string, 0, len(a.Evidence))
	for _, e := range a.Evidence {
		evidenceParts = append(evidenceParts, e.RuleID+"|"+e.Severity+"|"+e.Path+"|"+e.SnippetHash)
	}
	parts := []string{a.SkillID, fmt.Sprintf("%.2f", a.Score), strings.Join(a.Strengths, "|"), strings.Join(a.Risks, "|"), strings.Join(a.ReviewedFiles, "|"), strings.Join(evidenceParts, "|")}
	s := strings.Join(parts, "\n")
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func shouldSample(skillID, reportHash string, sampleRate int) bool {
	if sampleRate <= 0 {
		return false
	}
	if sampleRate >= 100 {
		return true
	}
	sum := sha256.Sum256([]byte(skillID + "|" + reportHash))
	v := int(sum[0])
	return v%100 < sampleRate
}

func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	r := []rune(strings.TrimSpace(s))
	if len(r) <= maxRunes {
		return string(r)
	}
	if maxRunes <= 1 {
		return string(r[:maxRunes])
	}
	return string(r[:maxRunes-1]) + "…"
}
