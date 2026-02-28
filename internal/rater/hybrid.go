package rater

import (
	"fmt"
	"math"
	"strings"
)

type HybridWeights struct {
	Rule float64
	LLM  float64
}

type LLMScoreResult struct {
	Score  float64
	Model  string
	Reason string
}

type LLMScorer interface {
	Score(audit SkillAudit) (LLMScoreResult, error)
}

func BuildHybridSkillAudit(skillID, skillDir string, sampleRate, maxRunes int, scorer LLMScorer, weights HybridWeights) (SkillAudit, error) {
	audit, err := BuildSkillAudit(skillID, skillDir, sampleRate, maxRunes)
	if err != nil {
		return SkillAudit{}, err
	}
	audit.RuleScore = audit.Score
	audit.ScoreSource = "rule"

	if scorer == nil {
		audit.ReportHash = buildAuditHash(audit)
		audit.Sampled = shouldSample(audit.SkillID, audit.ReportHash, sampleRate)
		audit.Report = truncateRunes(buildAuditNarrative(audit), maxRunes)
		return audit, nil
	}

	result, err := scorer.Score(audit)
	if err != nil {
		audit.LLMError = strings.TrimSpace(err.Error())
		audit.ReportHash = buildAuditHash(audit)
		audit.Sampled = shouldSample(audit.SkillID, audit.ReportHash, sampleRate)
		audit.Report = truncateRunes(buildAuditNarrative(audit), maxRunes)
		return audit, nil
	}

	llmScore := clampScore(result.Score)
	audit.LLMScore = &llmScore
	audit.LLMModel = strings.TrimSpace(result.Model)
	audit.LLMReason = strings.TrimSpace(result.Reason)
	audit.Score = blendScores(audit.RuleScore, llmScore, weights)
	audit.ScoreSource = "hybrid"
	audit.ReportHash = buildAuditHash(audit)
	audit.Sampled = shouldSample(audit.SkillID, audit.ReportHash, sampleRate)
	audit.Report = truncateRunes(buildAuditNarrative(audit), maxRunes)
	return audit, nil
}

func blendScores(ruleScore, llmScore float64, weights HybridWeights) float64 {
	r := weights.Rule
	l := weights.LLM
	if r < 0 {
		r = 0
	}
	if l < 0 {
		l = 0
	}
	if r == 0 && l == 0 {
		r = 0.7
		l = 0.3
	}
	total := r + l
	if total == 0 {
		return clampScore(ruleScore)
	}
	blended := (ruleScore*r + llmScore*l) / total
	return clampScore(blended)
}

func clampScore(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
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
	scorePart := fmt.Sprintf("最终分：%.1f（规则分：%.1f", a.Score, a.RuleScore)
	if a.LLMScore != nil {
		model := a.LLMModel
		if model == "" {
			model = "llm"
		}
		scorePart += fmt.Sprintf("，LLM分：%.1f，模型：%s", *a.LLMScore, model)
	}
	scorePart += "）。"

	return fmt.Sprintf("1) %s 安全审查结果 我审了 %s，结论：优点 - %s。风险点（中等，可控） - %s。证据链：%d 条。建议优先修复高风险点并限制输入边界。%s", skillName, files, strength, risk, len(a.Evidence), scorePart)
}
