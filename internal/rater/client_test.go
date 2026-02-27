package rater

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vpn2004/SkillVet/internal/did"
)

func TestBuildAuthHeadersSignsExpectedMessage(t *testing.T) {
	pub, priv, err := did.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate keypair failed: %v", err)
	}
	didValue := did.BuildDID("rater-node")
	now := time.Date(2026, 2, 25, 9, 0, 0, 0, time.UTC)
	nonce := "nonce-1"

	headers, err := BuildAuthHeaders(didValue, priv, now, nonce)
	if err != nil {
		t.Fatalf("BuildAuthHeaders failed: %v", err)
	}

	ts := headers.Get("X-Timestamp")
	sig := headers.Get("X-Signature")
	if ts == "" || sig == "" {
		t.Fatalf("missing auth headers: %+v", headers)
	}

	msg := didValue + "|" + ts + "|" + nonce
	ok, err := did.VerifyMessage(pub, msg, sig)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if !ok {
		t.Fatalf("signature not valid for message")
	}
}

func TestDiscoverSkillsFromDir(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "weather"))
	mustMkdirAll(t, filepath.Join(root, "search"))
	mustMkdirAll(t, filepath.Join(root, "ignore-me"))
	mustWriteFile(t, filepath.Join(root, "weather", "SKILL.md"), "# weather")
	mustWriteFile(t, filepath.Join(root, "search", "SKILL.md"), "# search")

	skills, err := DiscoverSkills(root, "openclaw", "1.0.0")
	if err != nil {
		t.Fatalf("DiscoverSkills failed: %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}
	got := map[string]bool{}
	for _, s := range skills {
		got[s.SkillID] = true
	}
	if !got["openclaw/weather@1.0.0"] || !got["openclaw/search@1.0.0"] {
		t.Fatalf("unexpected discovered skills: %+v", skills)
	}
}

func TestClientSubmitSkillRating(t *testing.T) {
	_, priv, err := did.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate keypair failed: %v", err)
	}
	identity := Identity{DID: did.BuildDID("node-a"), PrivateKey: priv}
	captured := map[string]any{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/skills/ratings" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.Header.Get("X-DID") == "" || r.Header.Get("X-Signature") == "" {
			t.Fatalf("missing auth headers")
		}
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode body failed: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"status":"created"}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	err = client.SubmitSkillRating(identity, "openclaw/weather@1.0.0", 91, "useful")
	if err != nil {
		t.Fatalf("SubmitSkillRating failed: %v", err)
	}

	if captured["skill_id"] != "openclaw/weather@1.0.0" {
		t.Fatalf("skill_id mismatch: %+v", captured)
	}
	if captured["score"].(float64) != 91 {
		t.Fatalf("score mismatch: %+v", captured)
	}
}

func TestGetSkillScoreParsesSinglePublicScoreResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/skills/score" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"skill_id":"openclaw/weather@1.0.0","score":86.4}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	score, err := client.GetSkillScore("openclaw/weather@1.0.0")
	if err != nil {
		t.Fatalf("GetSkillScore failed: %v", err)
	}
	if score.SkillID != "openclaw/weather@1.0.0" {
		t.Fatalf("unexpected skill id: %s", score.SkillID)
	}
	if score.Score != 86.4 {
		t.Fatalf("unexpected score: %v", score.Score)
	}
}

func TestGetTopSkillsParsesSingleScoreItems(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/skills/top" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"items":[{"skill_id":"openclaw/weather@1.0.0","score":90.1}]}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	items, err := client.GetTopSkills(10, 1)
	if err != nil {
		t.Fatalf("GetTopSkills failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].SkillID != "openclaw/weather@1.0.0" || items[0].Score != 90.1 {
		t.Fatalf("unexpected item: %+v", items[0])
	}
}

func TestIdentitySaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.json")
	in := Identity{AgentID: "agent-a", DID: "did:safespace:agent-a", PublicKey: "pub", PrivateKey: "priv"}
	if err := SaveIdentity(path, in); err != nil {
		t.Fatalf("SaveIdentity failed: %v", err)
	}
	out, err := LoadIdentity(path)
	if err != nil {
		t.Fatalf("LoadIdentity failed: %v", err)
	}
	if out != in {
		t.Fatalf("identity mismatch: got=%+v want=%+v", out, in)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
}

func TestDiscoverSkillsAutoUsesMetadata(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "weather-pro"))
	mustWriteFile(t, filepath.Join(root, "weather-pro", "SKILL.md"), "---\nname: weather-pro\nversion: 2.1.0\nsource: acme\n---\n# Weather\n")

	skills, err := DiscoverSkillsAuto(root, "openclaw", "local")
	if err != nil {
		t.Fatalf("DiscoverSkillsAuto failed: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].SkillID != "acme/weather-pro@2.1.0" {
		t.Fatalf("unexpected skill id: %s", skills[0].SkillID)
	}
}

func TestParseInteractiveScoreInput(t *testing.T) {
	_, decision, err := ParseInteractiveScoreInput("s")
	if err != nil || decision != "skip" {
		t.Fatalf("expected skip, got decision=%s err=%v", decision, err)
	}

	_, decision, err = ParseInteractiveScoreInput("q")
	if err != nil || decision != "quit" {
		t.Fatalf("expected quit, got decision=%s err=%v", decision, err)
	}

	score, decision, err := ParseInteractiveScoreInput("87.5")
	if err != nil || decision != "submit" {
		t.Fatalf("expected submit, got decision=%s err=%v", decision, err)
	}
	if score != 87.5 {
		t.Fatalf("unexpected score: %v", score)
	}

	if _, _, err := ParseInteractiveScoreInput("999"); err == nil {
		t.Fatalf("expected invalid score error")
	}
}

func TestSuggestScoreForSkillID(t *testing.T) {
	cases := []struct {
		skillID        string
		wantScore      float64
		wantCategory   string
		reasonContains string
	}{
		{skillID: "openclaw/security-scanner@1.0.0", wantScore: 90, wantCategory: "security", reasonContains: "security"},
		{skillID: "openclaw/release-helper@1.0.0", wantScore: 85, wantCategory: "infrastructure", reasonContains: "infra"},
		{skillID: "openclaw/danger-gemini-web@1.0.0", wantScore: 68, wantCategory: "experimental", reasonContains: "experimental"},
		{skillID: "openclaw/cover-image@1.0.0", wantScore: 78, wantCategory: "creative", reasonContains: "creative"},
		{skillID: "openclaw/weather@1.0.0", wantScore: 80, wantCategory: "general", reasonContains: "default"},
	}

	for _, tc := range cases {
		t.Run(tc.skillID, func(t *testing.T) {
			got := SuggestScoreForSkillID(tc.skillID)
			if got.Score != tc.wantScore {
				t.Fatalf("score mismatch: got=%v want=%v", got.Score, tc.wantScore)
			}
			if got.Category != tc.wantCategory {
				t.Fatalf("category mismatch: got=%s want=%s", got.Category, tc.wantCategory)
			}
			if !strings.Contains(strings.ToLower(got.Reason), strings.ToLower(tc.reasonContains)) {
				t.Fatalf("reason mismatch: got=%q need contains=%q", got.Reason, tc.reasonContains)
			}
		})
	}
}

func TestNormalizeSkillName(t *testing.T) {
	got := normalizeSkillName("My Skill_v2")
	if strings.Contains(got, " ") {
		t.Fatalf("expected no spaces: %q", got)
	}
	if got != "my-skill_v2" {
		t.Fatalf("unexpected normalized value: %q", got)
	}
}

func TestGetSkillScoreOrZeroReturnsZeroOnNotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"skill score not found"}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	score, err := client.GetSkillScoreOrZero("openclaw/missing@1.0.0")
	if err != nil {
		t.Fatalf("GetSkillScoreOrZero failed: %v", err)
	}
	if score != 0 {
		t.Fatalf("expected 0 for not found, got %v", score)
	}
}

func TestBuildScoreDelta(t *testing.T) {
	delta := BuildScoreDelta(80, 86.5)
	if delta != 6.5 {
		t.Fatalf("expected delta 6.5, got %v", delta)
	}
}
