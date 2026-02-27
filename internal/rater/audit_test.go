package rater

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildSkillAuditGeneratesBoundedReportAndDeterministicSample(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# fund-assistant\n"), 0o644); err != nil {
		t.Fatalf("write SKILL.md failed: %v", err)
	}

	code := `import requests, subprocess
FUND_GZ_URL = "http://fund.example.com"
def run_cmd(x):
    return subprocess.check_output(x, shell=True)
`
	if err := os.WriteFile(filepath.Join(dir, "scripts", "fund_api.py"), []byte(code), 0o644); err != nil {
		t.Fatalf("write fund_api.py failed: %v", err)
	}

	audit, err := BuildSkillAudit("acme/fund-assistant@1.0.0", dir, 5, 500)
	if err != nil {
		t.Fatalf("BuildSkillAudit failed: %v", err)
	}

	if audit.ReportHash == "" {
		t.Fatalf("expected report hash")
	}
	if len([]rune(audit.Report)) > 500 {
		t.Fatalf("report should be <= 500 runes, got %d", len([]rune(audit.Report)))
	}
	if !strings.Contains(audit.Report, "fund-assistant") {
		t.Fatalf("report should mention skill name: %s", audit.Report)
	}
	if audit.Score >= 90 {
		t.Fatalf("expected score deduction from risky patterns, got %.2f", audit.Score)
	}

	audit2, err := BuildSkillAudit("acme/fund-assistant@1.0.0", dir, 5, 500)
	if err != nil {
		t.Fatalf("BuildSkillAudit second call failed: %v", err)
	}
	if audit.Sampled != audit2.Sampled {
		t.Fatalf("sample decision must be deterministic")
	}
}

func TestComposeAuditCommentBounded(t *testing.T) {
	audit := SkillAudit{
		SkillID:       "acme/fund-assistant@1.0.0",
		Score:         72,
		RiskHigh:      1,
		RiskMedium:    2,
		RiskLow:       0,
		ReportHash:    strings.Repeat("a", 64),
		Sampled:       true,
		ReviewedFiles: []string{"SKILL.md", "scripts/fund_api.py", "scripts/portfolio.py"},
		Report:        strings.Repeat("测", 800),
	}
	comment := ComposeAuditComment(audit, 500)
	if len([]rune(comment)) > 500 {
		t.Fatalf("comment should be <= 500 runes, got %d", len([]rune(comment)))
	}
	if !strings.Contains(comment, "hash=") {
		t.Fatalf("comment should include hash")
	}
	if strings.Contains(comment, "\n") {
		t.Fatalf("comment should be single-line")
	}
}

func TestAuditCacheDedup(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "audit-cache.json")
	cache, err := LoadAuditCache(cachePath)
	if err != nil {
		t.Fatalf("LoadAuditCache failed: %v", err)
	}
	if !cache.ShouldUpload("a/b@1", "h1") {
		t.Fatalf("first upload should be allowed")
	}
	cache.MarkUploaded("a/b@1", "h1")
	if err := SaveAuditCache(cachePath, cache); err != nil {
		t.Fatalf("SaveAuditCache failed: %v", err)
	}

	cache2, err := LoadAuditCache(cachePath)
	if err != nil {
		t.Fatalf("LoadAuditCache reload failed: %v", err)
	}
	if cache2.ShouldUpload("a/b@1", "h1") {
		t.Fatalf("same hash should be deduped")
	}
	if !cache2.ShouldUpload("a/b@1", "h2") {
		t.Fatalf("changed hash should be uploaded")
	}
}

func TestBuildSkillAuditIncludesEvidence(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# demo\n"), 0o644); err != nil {
		t.Fatalf("write SKILL.md failed: %v", err)
	}
	code := `import subprocess, sys
cmd = "bash -c " + sys.argv[1]
subprocess.check_output(cmd, shell=True)
url = "http://example.com"
`
	if err := os.WriteFile(filepath.Join(dir, "scripts", "demo.py"), []byte(code), 0o644); err != nil {
		t.Fatalf("write script failed: %v", err)
	}

	audit, err := BuildSkillAudit("acme/demo@1.0.0", dir, 5, 500)
	if err != nil {
		t.Fatalf("BuildSkillAudit failed: %v", err)
	}
	if len(audit.Evidence) == 0 {
		t.Fatalf("expected non-empty evidence")
	}
	for _, e := range audit.Evidence {
		if e.RuleID == "" || e.Path == "" || e.SnippetHash == "" {
			t.Fatalf("invalid evidence: %+v", e)
		}
	}
}

func TestContextAwareCommandExecutionRule(t *testing.T) {
	untrustedDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(untrustedDir, "scripts"), 0o755)
	_ = os.WriteFile(filepath.Join(untrustedDir, "SKILL.md"), []byte("# demo\n"), 0o644)
	_ = os.WriteFile(filepath.Join(untrustedDir, "scripts", "a.py"), []byte("import subprocess,sys\nsubprocess.run(sys.argv[1], shell=True)\n"), 0o644)

	trustedDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(trustedDir, "scripts"), 0o755)
	_ = os.WriteFile(filepath.Join(trustedDir, "SKILL.md"), []byte("# demo\n"), 0o644)
	_ = os.WriteFile(filepath.Join(trustedDir, "scripts", "b.py"), []byte("import subprocess\nsubprocess.run('ls -la', shell=True)\n"), 0o644)

	a1, err := BuildSkillAudit("acme/untrusted@1", untrustedDir, 5, 500)
	if err != nil {
		t.Fatalf("audit untrusted failed: %v", err)
	}
	a2, err := BuildSkillAudit("acme/trusted@1", trustedDir, 5, 500)
	if err != nil {
		t.Fatalf("audit trusted failed: %v", err)
	}

	if a1.Score >= a2.Score {
		t.Fatalf("expected untrusted command execution to score lower, got untrusted=%.1f trusted=%.1f", a1.Score, a2.Score)
	}
}
