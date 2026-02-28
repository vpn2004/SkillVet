package rater

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type stubLLMScorer struct {
	result LLMScoreResult
	err    error
}

func (s stubLLMScorer) Score(audit SkillAudit) (LLMScoreResult, error) {
	if s.err != nil {
		return LLMScoreResult{}, s.err
	}
	return s.result, nil
}

func TestBuildHybridSkillAuditUsesBlendedScoreWhenLLMSucceeds(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# demo\n"), 0o644); err != nil {
		t.Fatalf("write SKILL.md failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scripts", "demo.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatalf("write script failed: %v", err)
	}

	audit, err := BuildHybridSkillAudit("acme/demo@1.0.0", dir, 5, 500, stubLLMScorer{result: LLMScoreResult{Score: 50, Model: "stub-llm", Reason: "test"}}, HybridWeights{Rule: 0.7, LLM: 0.3})
	if err != nil {
		t.Fatalf("BuildHybridSkillAudit failed: %v", err)
	}

	if audit.ScoreSource != "hybrid" {
		t.Fatalf("expected hybrid score source, got %q", audit.ScoreSource)
	}
	if audit.RuleScore <= 0 {
		t.Fatalf("expected rule score > 0, got %.2f", audit.RuleScore)
	}
	if audit.LLMScore == nil {
		t.Fatalf("expected llm score populated")
	}
	want := 0.7*audit.RuleScore + 0.3*50
	if math.Abs(audit.Score-want) > 0.001 {
		t.Fatalf("unexpected blended score: got %.4f want %.4f", audit.Score, want)
	}
	if audit.LLMModel != "stub-llm" {
		t.Fatalf("expected llm model stored")
	}
}

func TestBuildHybridSkillAuditFallsBackToRuleScoreOnLLMFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# demo\n"), 0o644); err != nil {
		t.Fatalf("write SKILL.md failed: %v", err)
	}

	audit, err := BuildHybridSkillAudit("acme/demo@1.0.0", dir, 5, 500, stubLLMScorer{err: errors.New("llm unavailable")}, HybridWeights{Rule: 0.7, LLM: 0.3})
	if err != nil {
		t.Fatalf("BuildHybridSkillAudit failed: %v", err)
	}

	if audit.ScoreSource != "rule" {
		t.Fatalf("expected rule score source, got %q", audit.ScoreSource)
	}
	if audit.LLMScore != nil {
		t.Fatalf("expected nil llm score on fallback")
	}
	if strings.TrimSpace(audit.LLMError) == "" {
		t.Fatalf("expected llm error to be captured")
	}
	if math.Abs(audit.Score-audit.RuleScore) > 0.001 {
		t.Fatalf("expected fallback score == rule score, got score=%.2f rule=%.2f", audit.Score, audit.RuleScore)
	}
}

func TestComposeAuditCommentIncludesHybridMetadata(t *testing.T) {
	llm := 62.0
	audit := SkillAudit{
		SkillID:       "acme/demo@1.0.0",
		Score:         81.6,
		RuleScore:     90,
		LLMScore:      &llm,
		ScoreSource:   "hybrid",
		LLMModel:      "stub-llm",
		ReportHash:    strings.Repeat("b", 64),
		Sampled:       false,
		RiskHigh:      0,
		RiskMedium:    1,
		RiskLow:       2,
		ReviewedFiles: []string{"SKILL.md"},
		Report:        "ok",
	}

	comment := ComposeAuditComment(audit, 500)
	if !strings.Contains(comment, "audit:v2") {
		t.Fatalf("expected comment to mark v2 format: %s", comment)
	}
	if !strings.Contains(comment, "source=hybrid") {
		t.Fatalf("expected comment to include source: %s", comment)
	}
	if !strings.Contains(comment, "rule=90.0") || !strings.Contains(comment, "llm=62.0") || !strings.Contains(comment, "final=81.6") {
		t.Fatalf("expected comment to include score breakdown: %s", comment)
	}
}
