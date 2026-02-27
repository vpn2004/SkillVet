package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/vpn2004/SkillVet/internal/did"
	"github.com/vpn2004/SkillVet/internal/rater"
)

func TestAuditFlowEndToEnd(t *testing.T) {
	pub, priv, err := did.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	type serverState struct {
		mu          sync.Mutex
		scores      map[string]float64
		attemptByID map[string]int
		firstFailID string
	}
	state := &serverState{
		scores:      map[string]float64{},
		attemptByID: map[string]int{},
		firstFailID: "openclaw/skill-fail@local",
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
				_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
				return
			}
			skillID, _ := req["skill_id"].(string)
			score, _ := req["score"].(float64)
			state.mu.Lock()
			state.attemptByID[skillID]++
			attempt := state.attemptByID[skillID]
			shouldFail := skillID == state.firstFailID && attempt == 1
			if !shouldFail {
				state.scores[skillID] = score
			}
			state.mu.Unlock()
			if shouldFail {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "temporary failure"})
				return
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "created"})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/api/skills/score":
			skillID := r.URL.Query().Get("skill_id")
			state.mu.Lock()
			score, ok := state.scores[skillID]
			state.mu.Unlock()
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "not found"})
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"skill_id": skillID, "score": score})
			return
		default:
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "not found"})
		}
	}))
	defer ts.Close()

	tmp := t.TempDir()
	identity := filepath.Join(tmp, "identity.json")
	cachePath := filepath.Join(tmp, "audit-cache.json")
	reportDir := filepath.Join(tmp, "reports")
	pendingPath := filepath.Join(tmp, "pending.json")
	skillsDir := filepath.Join(tmp, "skills")

	mustWriteFile(t, filepath.Join(skillsDir, "skill-ok", "SKILL.md"), "# ok\n")
	mustWriteFile(t, filepath.Join(skillsDir, "skill-ok", "scripts", "demo.py"), "print('ok')\n")
	mustWriteFile(t, filepath.Join(skillsDir, "skill-fail", "SKILL.md"), "# fail\n")
	mustWriteFile(t, filepath.Join(skillsDir, "skill-fail", "scripts", "demo.py"), "import subprocess,sys\nsubprocess.run(sys.argv[1], shell=True)\n")

	repoRoot := filepath.Join("..", "..")
	env := append(os.Environ(), "HOME="+tmp)

	out := runCLI(t, repoRoot, env,
		"register", "--server", ts.URL, "--agent-id", "e2e-agent", "--identity", identity,
	)
	if !strings.Contains(out, "registered did=") {
		t.Fatalf("unexpected register output: %s", out)
	}

	out = runCLI(t, repoRoot, env,
		"audit-local",
		"--server", ts.URL,
		"--identity", identity,
		"--skills-dir", skillsDir,
		"--cache", cachePath,
		"--report-dir", reportDir,
		"--pending-path", pendingPath,
		"--max-submit", "5",
	)
	if !strings.Contains(out, "audit uploaded=") {
		t.Fatalf("unexpected audit-local output: %s", out)
	}

	okReport := filepath.Join(reportDir, "openclaw_skill-ok_local.md")
	buf, err := os.ReadFile(okReport)
	if err != nil {
		t.Fatalf("read ok report failed: %v", err)
	}
	if !strings.Contains(string(buf), "服务端评分回读：before=") {
		t.Fatalf("expected score delta in report, got: %s", string(buf))
	}

	queue, err := rater.LoadPendingUploadQueue(pendingPath)
	if err != nil {
		t.Fatalf("LoadPendingUploadQueue failed: %v", err)
	}
	if len(queue.Items) != 1 || queue.Items[0].SkillID != "openclaw/skill-fail@local" {
		t.Fatalf("expected one pending failed upload, got %+v", queue.Items)
	}

	out = runCLI(t, repoRoot, env,
		"retry-pending",
		"--server", ts.URL,
		"--identity", identity,
		"--pending-path", pendingPath,
	)
	if !strings.Contains(out, "retry submitted=1") {
		t.Fatalf("unexpected retry output: %s", out)
	}

	queue, err = rater.LoadPendingUploadQueue(pendingPath)
	if err != nil {
		t.Fatalf("reload pending queue failed: %v", err)
	}
	if len(queue.Items) != 0 {
		t.Fatalf("expected empty pending queue after retry, got %+v", queue.Items)
	}

	out = runCLI(t, repoRoot, env,
		"summary", "--server", ts.URL, "--skill-id", "openclaw/skill-fail@local",
	)
	if !strings.Contains(out, "skill=openclaw/skill-fail@local score=") {
		t.Fatalf("unexpected summary output: %s", out)
	}
}

func runCLI(t *testing.T, repoRoot string, env []string, args ...string) string {
	t.Helper()
	allArgs := append([]string{"run", "./cmd/safespace-rater"}, args...)
	cmd := exec.Command("go", allArgs...)
	cmd.Dir = repoRoot
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("command failed: go %s\nerr=%v\nout=%s", strings.Join(allArgs, " "), err, out.String())
	}
	return out.String()
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
}
