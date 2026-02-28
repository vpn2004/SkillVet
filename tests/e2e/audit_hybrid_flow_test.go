package e2e

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/vpn2004/SkillVet/internal/did"
)

func TestAuditLocalUploadsHybridScoreWhenLLMConfigured(t *testing.T) {
	pub, priv, err := did.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	var (
		mu            sync.Mutex
		uploadedScore float64
		uploadedSkill string
		uploadedCmt   string
	)

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/did/register":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"did":         "did:safespace:e2e-agent",
				"public_key":  pub,
				"private_key": priv,
			})
			return
		case r.Method == http.MethodPost && r.URL.Path == "/api/skills/ratings":
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			uploadedSkill, _ = req["skill_id"].(string)
			uploadedScore, _ = req["score"].(float64)
			uploadedCmt, _ = req["comment"].(string)
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "created"})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/api/skills/score":
			skillID := r.URL.Query().Get("skill_id")
			mu.Lock()
			score := uploadedScore
			mu.Unlock()
			if skillID == "" || score == 0 {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "not found"})
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"skill_id": skillID, "score": score})
			return
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer api.Close()

	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "mock-llm",
			"choices": []map[string]any{
				{"message": map[string]any{"content": `{"score":40,"reason":"mock review"}`}},
			},
		})
	}))
	defer llm.Close()

	tmp := t.TempDir()
	identity := filepath.Join(tmp, "identity.json")
	cachePath := filepath.Join(tmp, "audit-cache.json")
	reportDir := filepath.Join(tmp, "reports")
	pendingPath := filepath.Join(tmp, "pending.json")
	skillsDir := filepath.Join(tmp, "skills")

	mustWriteFile(t, filepath.Join(skillsDir, "skill-ok", "SKILL.md"), "# ok\n")
	mustWriteFile(t, filepath.Join(skillsDir, "skill-ok", "scripts", "demo.py"), "print('ok')\n")

	repoRoot := filepath.Join("..", "..")
	env := append(os.Environ(),
		"HOME="+tmp,
		"SAFESPACE_LLM_OPENAI_FALLBACK=1",
		"SAFESPACE_LLM_MODEL=test-model",
		"SAFESPACE_LLM_API_KEY=test-key",
		"SAFESPACE_LLM_BASE_URL="+llm.URL+"/v1",
	)

	_ = runCLI(t, repoRoot, env,
		"register", "--server", api.URL, "--agent-id", "e2e-agent", "--identity", identity,
	)

	out := runCLI(t, repoRoot, env,
		"audit-local",
		"--server", api.URL,
		"--identity", identity,
		"--skills-dir", skillsDir,
		"--cache", cachePath,
		"--report-dir", reportDir,
		"--pending-path", pendingPath,
		"--max-submit", "1",
	)
	if !strings.Contains(out, "audit uploaded=1") {
		t.Fatalf("unexpected audit output: %s", out)
	}

	mu.Lock()
	skillID := uploadedSkill
	score := uploadedScore
	comment := uploadedCmt
	mu.Unlock()

	if skillID != "openclaw/skill-ok@local" {
		t.Fatalf("unexpected skill uploaded: %s", skillID)
	}
	if math.Abs(score-78.5) > 0.01 {
		t.Fatalf("expected hybrid score 78.5, got %.2f", score)
	}
	if !strings.Contains(comment, "source=hybrid") || !strings.Contains(comment, "llm=40.0") {
		t.Fatalf("expected hybrid comment metadata, got: %s", comment)
	}
}

func TestAuditLocalPrefersRuntimeScoreFileOverOpenAIFallback(t *testing.T) {
	pub, priv, err := did.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	var (
		mu            sync.Mutex
		uploadedScore float64
		uploadedSkill string
		uploadedCmt   string
		llmCalls      int
	)

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/did/register":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"did":         "did:safespace:e2e-agent",
				"public_key":  pub,
				"private_key": priv,
			})
			return
		case r.Method == http.MethodPost && r.URL.Path == "/api/skills/ratings":
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			uploadedSkill, _ = req["skill_id"].(string)
			uploadedScore, _ = req["score"].(float64)
			uploadedCmt, _ = req["comment"].(string)
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "created"})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/api/skills/score":
			skillID := r.URL.Query().Get("skill_id")
			mu.Lock()
			score := uploadedScore
			mu.Unlock()
			if skillID == "" || score == 0 {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "not found"})
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"skill_id": skillID, "score": score})
			return
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer api.Close()

	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		llmCalls++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "mock-llm",
			"choices": []map[string]any{
				{"message": map[string]any{"content": `{"score":10,"reason":"should not be used"}`}},
			},
		})
	}))
	defer llm.Close()

	tmp := t.TempDir()
	identity := filepath.Join(tmp, "identity.json")
	cachePath := filepath.Join(tmp, "audit-cache.json")
	reportDir := filepath.Join(tmp, "reports")
	pendingPath := filepath.Join(tmp, "pending.json")
	skillsDir := filepath.Join(tmp, "skills")
	scoreFile := filepath.Join(tmp, "runtime-llm-scores.json")

	mustWriteFile(t, filepath.Join(skillsDir, "skill-ok", "SKILL.md"), "# ok\n")
	mustWriteFile(t, filepath.Join(skillsDir, "skill-ok", "scripts", "demo.py"), "print('ok')\n")
	mustWriteFile(t, scoreFile, `{"skills":{"openclaw/skill-ok@local":{"score":30,"model":"runtime-model","reason":"runtime review"}}}`)

	repoRoot := filepath.Join("..", "..")
	env := append(os.Environ(),
		"HOME="+tmp,
		"SAFESPACE_LLM_OPENAI_FALLBACK=1",
		"SAFESPACE_LLM_MODEL=test-model",
		"SAFESPACE_LLM_API_KEY=test-key",
		"SAFESPACE_LLM_BASE_URL="+llm.URL+"/v1",
	)

	_ = runCLI(t, repoRoot, env,
		"register", "--server", api.URL, "--agent-id", "e2e-agent", "--identity", identity,
	)

	out := runCLI(t, repoRoot, env,
		"audit-local",
		"--server", api.URL,
		"--identity", identity,
		"--skills-dir", skillsDir,
		"--cache", cachePath,
		"--report-dir", reportDir,
		"--pending-path", pendingPath,
		"--llm-score-file", scoreFile,
		"--max-submit", "1",
	)
	if !strings.Contains(out, "audit uploaded=1") {
		t.Fatalf("unexpected audit output: %s", out)
	}

	mu.Lock()
	skillID := uploadedSkill
	score := uploadedScore
	comment := uploadedCmt
	calls := llmCalls
	mu.Unlock()

	if skillID != "openclaw/skill-ok@local" {
		t.Fatalf("unexpected skill uploaded: %s", skillID)
	}
	if math.Abs(score-75.5) > 0.01 {
		t.Fatalf("expected runtime hybrid score 75.5, got %.2f", score)
	}
	if !strings.Contains(comment, "model=runtime-model") || !strings.Contains(comment, "llm=30.0") {
		t.Fatalf("expected runtime score metadata, got: %s", comment)
	}
	if calls != 0 {
		t.Fatalf("expected openai fallback not used, got llm calls=%d", calls)
	}
}
