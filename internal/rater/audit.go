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

type SkillAudit struct {
	SkillID       string   `json:"skill_id"`
	Score         float64  `json:"score"`
	Strengths     []string `json:"strengths"`
	Risks         []string `json:"risks"`
	RiskHigh      int      `json:"risk_high"`
	RiskMedium    int      `json:"risk_medium"`
	RiskLow       int      `json:"risk_low"`
	ReviewedFiles []string `json:"reviewed_files"`
	ReportHash    string   `json:"report_hash"`
	Report        string   `json:"report"`
	Sampled       bool     `json:"sampled"`
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
	high, medium, low := 0, 0, 0
	score := 95.0

	foundDangerExec := false
	for _, f := range files {
		buf, err := os.ReadFile(filepath.Join(skillDir, f))
		if err != nil {
			continue
		}
		content := strings.ToLower(string(buf))
		if containsAny(content, "os.system(", "subprocess.", "exec(", "eval(", "shell=true", "bash -c") {
			high++
			score -= 20
			foundDangerExec = true
			risks = append(risks, fmt.Sprintf("发现命令执行风险（%s）", f))
		}
		if strings.Contains(content, "http://") {
			medium++
			score -= 10
			risks = append(risks, fmt.Sprintf("发现明文HTTP调用（%s）", f))
		}
		if containsAny(content, "open(", "read_text(", "write_text(") && containsAny(content, "argparse", "sys.argv", "click.option") {
			low++
			score -= 5
			risks = append(risks, fmt.Sprintf("发现路径参数读写，需限制目录（%s）", f))
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

	audit := SkillAudit{
		SkillID:       skillID,
		Score:         score,
		Strengths:     strengths,
		Risks:         risks,
		RiskHigh:      high,
		RiskMedium:    medium,
		RiskLow:       low,
		ReviewedFiles: files,
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

	reportOneLine := strings.ReplaceAll(a.Report, "\n", " ")
	reportOneLine = strings.Join(strings.Fields(reportOneLine), " ")
	comment := fmt.Sprintf("audit:v1 hash=%s sample=%d score=%.1f risk(h/m/l)=%d/%d/%d files=%d summary=%s",
		hash, sample, a.Score, a.RiskHigh, a.RiskMedium, a.RiskLow, len(a.ReviewedFiles), reportOneLine)
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

func buildAuditNarrative(a SkillAudit) string {
	skillName := a.SkillID
	if idx := strings.Index(skillName, "/"); idx >= 0 && idx+1 < len(skillName) {
		skillName = skillName[idx+1:]
	}
	files := strings.Join(a.ReviewedFiles, " + ")
	if files == "" {
		files = "SKILL.md"
	}
	strength := "暂无"
	if len(a.Strengths) > 0 {
		strength = strings.Join(a.Strengths, "；")
	}
	risk := "未发现明显风险"
	if len(a.Risks) > 0 {
		risk = strings.Join(a.Risks, "；")
	}
	return fmt.Sprintf("1) %s 安全审查结果 我审了 %s，结论：优点 - %s。风险点（中等，可控） - %s。建议优先修复高风险点并限制输入边界。最终分：%.1f。", skillName, files, strength, risk, a.Score)
}

func buildAuditHash(a SkillAudit) string {
	parts := []string{a.SkillID, fmt.Sprintf("%.2f", a.Score), strings.Join(a.Strengths, "|"), strings.Join(a.Risks, "|"), strings.Join(a.ReviewedFiles, "|")}
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
