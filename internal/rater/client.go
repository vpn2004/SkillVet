package rater

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vpn2004/SkillVet/internal/did"
	"gopkg.in/yaml.v3"
)

type Identity struct {
	AgentID    string `json:"agent_id"`
	DID        string `json:"did"`
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
}

type DiscoveredSkill struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	SkillID string `json:"skill_id"`
}

type SkillScore struct {
	SkillID string  `json:"skill_id"`
	Score   float64 `json:"score"`
}

type ScoreSuggestion struct {
	Score    float64
	Category string
	Reason   string
}

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewClient(baseURL string) *Client {
	base := strings.TrimRight(baseURL, "/")
	return &Client{
		BaseURL:    base,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func BuildAuthHeaders(didValue, privateKey string, now time.Time, nonce string) (http.Header, error) {
	ts := now.UTC().Format(time.RFC3339)
	msg := fmt.Sprintf("%s|%s|%s", didValue, ts, nonce)
	sig, err := did.SignMessage(privateKey, msg)
	if err != nil {
		return nil, err
	}

	h := make(http.Header)
	h.Set("X-DID", didValue)
	h.Set("X-Timestamp", ts)
	h.Set("X-Nonce", nonce)
	h.Set("X-Signature", sig)
	return h, nil
}

func (c *Client) RegisterIdentity(agentID string) (Identity, error) {
	payload := map[string]string{"agent_id": agentID}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/api/did/register", bytes.NewReader(body))
	if err != nil {
		return Identity{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return Identity{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		var er map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&er)
		return Identity{}, fmt.Errorf("register failed: status=%d error=%v", resp.StatusCode, er["error"])
	}
	var out Identity
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Identity{}, err
	}
	out.AgentID = agentID
	return out, nil
}

func (c *Client) SubmitSkillRating(identity Identity, skillID string, score float64, comment string) error {
	payload := map[string]any{
		"did":      identity.DID,
		"skill_id": skillID,
		"score":    score,
		"comment":  comment,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/api/skills/ratings", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	nonce := fmt.Sprintf("rater-%d", time.Now().UnixNano())
	headers, err := BuildAuthHeaders(identity.DID, identity.PrivateKey, time.Now(), nonce)
	if err != nil {
		return err
	}
	for k, values := range headers {
		for _, v := range values {
			req.Header.Add(k, v)
		}
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		var er map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&er)
		return fmt.Errorf("submit skill rating failed: status=%d error=%v", resp.StatusCode, er["error"])
	}
	return nil
}

func (c *Client) GetSkillScore(skillID string) (SkillScore, error) {
	query := url.QueryEscape(skillID)
	endpoint := fmt.Sprintf("%s/api/skills/score?skill_id=%s", c.BaseURL, query)
	resp, err := c.HTTPClient.Get(endpoint)
	if err != nil {
		return SkillScore{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var er map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&er)
		return SkillScore{}, fmt.Errorf("get skill score failed: status=%d error=%v", resp.StatusCode, er["error"])
	}
	var out SkillScore
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return SkillScore{}, err
	}
	return out, nil
}

func (c *Client) GetTopSkills(limit, minCount int) ([]SkillScore, error) {
	if limit <= 0 {
		limit = 20
	}
	if minCount <= 0 {
		minCount = 1
	}
	endpoint := fmt.Sprintf("%s/api/skills/top?limit=%d&min_count=%d", c.BaseURL, limit, minCount)
	resp, err := c.HTTPClient.Get(endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var er map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&er)
		return nil, fmt.Errorf("get top skills failed: status=%d error=%v", resp.StatusCode, er["error"])
	}

	var raw struct {
		Items []SkillScore `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	return raw.Items, nil
}

func DiscoverSkills(rootDir, source, version string) ([]DiscoveredSkill, error) {
	if source == "" {
		source = "local"
	}
	if version == "" {
		version = "local"
	}

	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return nil, err
	}

	skills := make([]DiscoveredSkill, 0)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(rootDir, e.Name())
		if _, err := os.Stat(filepath.Join(p, "SKILL.md")); err != nil {
			continue
		}
		name := normalizeSkillName(e.Name())
		skillID := fmt.Sprintf("%s/%s@%s", source, name, version)
		skills = append(skills, DiscoveredSkill{Name: e.Name(), Path: p, SkillID: skillID})
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].SkillID < skills[j].SkillID })
	return skills, nil
}

func DiscoverSkillsAuto(rootDir, defaultSource, defaultVersion string) ([]DiscoveredSkill, error) {
	if defaultSource == "" {
		defaultSource = "local"
	}
	if defaultVersion == "" {
		defaultVersion = "local"
	}

	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return nil, err
	}

	skills := make([]DiscoveredSkill, 0)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(rootDir, e.Name())
		if _, err := os.Stat(filepath.Join(p, "SKILL.md")); err != nil {
			continue
		}
		meta := detectSkillMetadata(p)

		source := meta.Source
		if source == "" {
			source = defaultSource
		}
		version := meta.Version
		if version == "" {
			version = defaultVersion
		}
		name := normalizeSkillName(meta.Name)
		if name == "unknown" {
			name = normalizeSkillName(e.Name())
		}

		skillID := fmt.Sprintf("%s/%s@%s", source, name, version)
		skills = append(skills, DiscoveredSkill{Name: name, Path: p, SkillID: skillID})
	}

	sort.Slice(skills, func(i, j int) bool { return skills[i].SkillID < skills[j].SkillID })
	return skills, nil
}

type skillMetadata struct {
	Name    string
	Source  string
	Version string
}

func detectSkillMetadata(skillDir string) skillMetadata {
	mdPath := filepath.Join(skillDir, "SKILL.md")
	buf, err := os.ReadFile(mdPath)
	if err != nil {
		return skillMetadata{}
	}

	content := string(buf)
	if !strings.HasPrefix(content, "---\n") {
		return skillMetadata{}
	}
	parts := strings.SplitN(content, "\n---\n", 2)
	if len(parts) < 2 {
		return skillMetadata{}
	}
	frontmatter := strings.TrimPrefix(parts[0], "---\n")
	var fm struct {
		Name    string `yaml:"name"`
		Source  string `yaml:"source"`
		Version string `yaml:"version"`
	}
	if err := yaml.Unmarshal([]byte(frontmatter), &fm); err != nil {
		return skillMetadata{}
	}
	source := ""
	if strings.TrimSpace(fm.Source) != "" {
		source = normalizeSkillName(fm.Source)
	}
	return skillMetadata{Name: fm.Name, Source: source, Version: normalizeVersion(fm.Version)}
}

func ParseInteractiveScoreInput(input string) (float64, string, error) {
	v := strings.TrimSpace(strings.ToLower(input))
	switch v {
	case "", "s", "skip":
		return 0, "skip", nil
	case "q", "quit", "exit":
		return 0, "quit", nil
	}
	score, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, "", fmt.Errorf("invalid score input")
	}
	if score < 0 || score > 100 {
		return 0, "", fmt.Errorf("score must be between 0 and 100")
	}
	return score, "submit", nil
}

func SuggestScoreForSkillID(skillID string) ScoreSuggestion {
	id := strings.ToLower(strings.TrimSpace(skillID))
	if id == "" {
		return ScoreSuggestion{Score: 80, Category: "general", Reason: "fallback baseline"}
	}

	containsAny := func(tokens ...string) bool {
		for _, token := range tokens {
			if strings.Contains(id, token) {
				return true
			}
		}
		return false
	}

	suggestion := ScoreSuggestion{Score: 80, Category: "general", Reason: "default baseline for uncategorized skills"}
	switch {
	case containsAny("security", "scanner", "audit", "privacy", "safety"):
		suggestion = ScoreSuggestion{Score: 90, Category: "security", Reason: "security-critical skills should be rated with stricter reliability expectation"}
	case containsAny("infra", "deploy", "docker", "kubernetes", "ci", "build", "git", "release"):
		suggestion = ScoreSuggestion{Score: 85, Category: "infrastructure", Reason: "infra workflow skills should prioritize stability and reproducibility"}
	case containsAny("danger", "experimental", "beta", "prototype", "lab"):
		suggestion = ScoreSuggestion{Score: 68, Category: "experimental", Reason: "experimental skills carry higher uncertainty and need conservative defaults"}
	case containsAny("image", "comic", "cover", "video", "design", "creative", "xhs"):
		suggestion = ScoreSuggestion{Score: 78, Category: "creative", Reason: "creative output quality is subjective; start with a moderate baseline"}
	}
	return suggestion
}

var invalidSkillNameChars = regexp.MustCompile(`[^a-z0-9._-]`)

func normalizeSkillName(s string) string {
	v := strings.ToLower(strings.TrimSpace(s))
	v = strings.ReplaceAll(v, " ", "-")
	v = invalidSkillNameChars.ReplaceAllString(v, "-")
	v = strings.Trim(v, "-")
	if v == "" {
		return "unknown"
	}
	return v
}

func normalizeVersion(s string) string {
	v := strings.TrimSpace(s)
	v = strings.ReplaceAll(v, " ", "")
	if v == "" {
		return ""
	}
	return v
}

func SaveIdentity(path string, identity Identity) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	buf, err := json.MarshalIndent(identity, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, buf, 0o600)
}

func LoadIdentity(path string) (Identity, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return Identity{}, err
	}
	var out Identity
	if err := json.Unmarshal(buf, &out); err != nil {
		return Identity{}, err
	}
	if out.DID == "" || out.PrivateKey == "" {
		return Identity{}, fmt.Errorf("invalid identity file")
	}
	return out, nil
}
