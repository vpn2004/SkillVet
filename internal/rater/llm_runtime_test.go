package rater

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewRuntimePreferredLLMScorerUsesScoreFileFirst(t *testing.T) {
	tmp := t.TempDir()
	scoreFile := filepath.Join(tmp, "scores.json")
	payload := map[string]any{
		"default": map[string]any{"score": 61.5, "model": "runtime-default", "reason": "default"},
		"skills": map[string]any{
			"openclaw/weather@1.0.0": map[string]any{"score": 80, "model": "runtime-skill", "reason": "skill-specific"},
		},
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if err := os.WriteFile(scoreFile, buf, 0o600); err != nil {
		t.Fatalf("write score file failed: %v", err)
	}

	scorer, err := NewRuntimePreferredLLMScorer(scoreFile)
	if err != nil {
		t.Fatalf("NewRuntimePreferredLLMScorer failed: %v", err)
	}
	if scorer == nil {
		t.Fatalf("expected scorer from score file")
	}

	res, err := scorer.Score(SkillAudit{SkillID: "openclaw/weather@1.0.0", RuleScore: 90})
	if err != nil {
		t.Fatalf("Score failed: %v", err)
	}
	if res.Score != 80 {
		t.Fatalf("expected score 80, got %.2f", res.Score)
	}
	if res.Model != "runtime-skill" {
		t.Fatalf("expected model runtime-skill, got %q", res.Model)
	}
}

func TestNewRuntimePreferredLLMScorerReturnsNilWhenNoRuntimeOrFallback(t *testing.T) {
	t.Setenv("SAFESPACE_LLM_SCORE_FILE", "")
	t.Setenv("SAFESPACE_LLM_OPENAI_FALLBACK", "")
	t.Setenv("SAFESPACE_LLM_MODEL", "test-model")
	t.Setenv("SAFESPACE_LLM_API_KEY", "test-key")

	scorer, err := NewRuntimePreferredLLMScorer("")
	if err != nil {
		t.Fatalf("NewRuntimePreferredLLMScorer failed: %v", err)
	}
	if scorer != nil {
		t.Fatalf("expected nil scorer when fallback switch is off")
	}
}

func TestNewRuntimePreferredLLMScorerAllowsOpenAIFallbackWhenEnabled(t *testing.T) {
	t.Setenv("SAFESPACE_LLM_SCORE_FILE", "")
	t.Setenv("SAFESPACE_LLM_OPENAI_FALLBACK", "1")
	t.Setenv("SAFESPACE_LLM_MODEL", "test-model")
	t.Setenv("SAFESPACE_LLM_API_KEY", "test-key")

	scorer, err := NewRuntimePreferredLLMScorer("")
	if err != nil {
		t.Fatalf("NewRuntimePreferredLLMScorer failed: %v", err)
	}
	if scorer == nil {
		t.Fatalf("expected scorer when fallback enabled")
	}
	if _, ok := scorer.(*OpenAICompatibleLLMScorer); !ok {
		t.Fatalf("expected OpenAICompatibleLLMScorer, got %T", scorer)
	}
}
