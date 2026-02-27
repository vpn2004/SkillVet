package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vpn2004/SkillVet/internal/rater"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "register":
		runRegister(os.Args[2:])
	case "rate":
		runRate(os.Args[2:])
	case "discover":
		runDiscover(os.Args[2:])
	case "rate-local":
		runRateLocal(os.Args[2:])
	case "summary":
		runSummary(os.Args[2:])
	case "top":
		runTop(os.Args[2:])
	case "audit-local":
		runAuditLocal(os.Args[2:])
	default:
		printUsage()
		os.Exit(2)
	}
}

func runRegister(args []string) {
	fs := flag.NewFlagSet("register", flag.ExitOnError)
	server := fs.String("server", defaultServerURL(), "SafeSpace API base URL")
	agentID := fs.String("agent-id", "", "agent ID (required)")
	identityPath := fs.String("identity", defaultIdentityPath(), "path to identity json")
	_ = fs.Parse(args)

	if strings.TrimSpace(*agentID) == "" {
		die("--agent-id is required")
	}
	client := rater.NewClient(*server)
	identity, err := client.RegisterIdentity(*agentID)
	if err != nil {
		die(err.Error())
	}
	if err := rater.SaveIdentity(*identityPath, identity); err != nil {
		die(err.Error())
	}
	fmt.Printf("registered did=%s saved=%s\n", identity.DID, *identityPath)
}

func runRate(args []string) {
	fs := flag.NewFlagSet("rate", flag.ExitOnError)
	server := fs.String("server", defaultServerURL(), "SafeSpace API base URL")
	identityPath := fs.String("identity", defaultIdentityPath(), "path to identity json")
	skillID := fs.String("skill-id", "", "skill id, e.g. openclaw/weather@1.0.0")
	score := fs.Float64("score", -1, "score 0..100")
	comment := fs.String("comment", "", "optional comment")
	_ = fs.Parse(args)

	if strings.TrimSpace(*skillID) == "" {
		die("--skill-id is required")
	}
	if *score < 0 || *score > 100 {
		die("--score must be between 0 and 100")
	}
	identity, err := rater.LoadIdentity(*identityPath)
	if err != nil {
		die(err.Error())
	}
	client := rater.NewClient(*server)
	if err := client.SubmitSkillRating(identity, *skillID, *score, *comment); err != nil {
		die(err.Error())
	}
	fmt.Printf("rated skill=%s score=%.1f\n", *skillID, *score)
}

func runDiscover(args []string) {
	fs := flag.NewFlagSet("discover", flag.ExitOnError)
	skillsDir := fs.String("skills-dir", defaultSkillsDir(), "skills directory")
	source := fs.String("source", "openclaw", "fallback skill source")
	version := fs.String("version", "local", "fallback skill version")
	auto := fs.Bool("auto", true, "auto detect source/version/name from SKILL.md frontmatter")
	_ = fs.Parse(args)

	var (
		skills []rater.DiscoveredSkill
		err    error
	)
	if *auto {
		skills, err = rater.DiscoverSkillsAuto(*skillsDir, *source, *version)
	} else {
		skills, err = rater.DiscoverSkills(*skillsDir, *source, *version)
	}
	if err != nil {
		die(err.Error())
	}
	for _, s := range skills {
		fmt.Println(s.SkillID)
	}
	fmt.Printf("total=%d\n", len(skills))
}

func runRateLocal(args []string) {
	fs := flag.NewFlagSet("rate-local", flag.ExitOnError)
	server := fs.String("server", defaultServerURL(), "SafeSpace API base URL")
	identityPath := fs.String("identity", defaultIdentityPath(), "path to identity json")
	skillsDir := fs.String("skills-dir", defaultSkillsDir(), "skills directory")
	source := fs.String("source", "openclaw", "fallback skill source")
	version := fs.String("version", "local", "fallback skill version")
	auto := fs.Bool("auto", true, "auto detect source/version/name from SKILL.md frontmatter")
	interactive := fs.Bool("interactive", false, "interactive rating template mode")
	score := fs.Float64("score", -1, "score 0..100 (required when --interactive=false)")
	comment := fs.String("comment", "bulk-local-rating", "default comment")
	maxSubmit := fs.Int("max-submit", 5, "max skills to submit in one batch (rate-limit guard)")
	_ = fs.Parse(args)

	if *maxSubmit <= 0 {
		die("--max-submit must be > 0")
	}
	if !*interactive && (*score < 0 || *score > 100) {
		die("--score must be between 0 and 100")
	}

	identity, err := rater.LoadIdentity(*identityPath)
	if err != nil {
		die(err.Error())
	}

	var skills []rater.DiscoveredSkill
	if *auto {
		skills, err = rater.DiscoverSkillsAuto(*skillsDir, *source, *version)
	} else {
		skills, err = rater.DiscoverSkills(*skillsDir, *source, *version)
	}
	if err != nil {
		die(err.Error())
	}
	client := rater.NewClient(*server)

	submitted := 0
	if *interactive {
		submitted = submitInteractiveRatings(client, identity, skills, *comment, *maxSubmit)
	} else {
		submitted = submitBulkRatings(client, identity, skills, *score, *comment, *maxSubmit)
	}

	fmt.Printf("submitted=%d discovered=%d\n", submitted, len(skills))
}

func submitBulkRatings(client *rater.Client, identity rater.Identity, skills []rater.DiscoveredSkill, score float64, comment string, maxSubmit int) int {
	submitted := 0
	for _, s := range skills {
		if submitted >= maxSubmit {
			break
		}
		if err := client.SubmitSkillRating(identity, s.SkillID, score, comment); err != nil {
			fmt.Printf("skip skill=%s err=%v\n", s.SkillID, err)
			continue
		}
		submitted++
		fmt.Printf("rated skill=%s score=%.1f\n", s.SkillID, score)
	}
	return submitted
}

func submitInteractiveRatings(client *rater.Client, identity rater.Identity, skills []rater.DiscoveredSkill, defaultComment string, maxSubmit int) int {
	reader := bufio.NewReader(os.Stdin)
	submitted := 0

	fmt.Println("interactive rating template: enter score 0..100, 's' to skip, 'q' to quit")
	for i, s := range skills {
		if submitted >= maxSubmit {
			fmt.Printf("reached max-submit=%d\n", maxSubmit)
			break
		}

		suggestion := rater.SuggestScoreForSkillID(s.SkillID)
		for {
			fmt.Printf("[%d/%d] %s suggested=%.0f (%s) score> ", i+1, len(skills), s.SkillID, suggestion.Score, suggestion.Category)
			line, _ := reader.ReadString('\n')
			score, decision, err := rater.ParseInteractiveScoreInput(line)
			if err != nil {
				fmt.Printf("invalid input: %v\n", err)
				continue
			}
			if decision == "quit" {
				return submitted
			}
			if decision == "skip" {
				break
			}

			fmt.Printf("comment (enter for default=%q)> ", defaultComment)
			commentLine, _ := reader.ReadString('\n')
			comment := strings.TrimSpace(commentLine)
			if comment == "" {
				comment = defaultComment
			}

			if err := client.SubmitSkillRating(identity, s.SkillID, score, comment); err != nil {
				fmt.Printf("skip skill=%s err=%v\n", s.SkillID, err)
				break
			}
			submitted++
			fmt.Printf("rated skill=%s score=%.1f\n", s.SkillID, score)
			break
		}
	}
	return submitted
}

func runSummary(args []string) {
	fs := flag.NewFlagSet("summary", flag.ExitOnError)
	server := fs.String("server", defaultServerURL(), "SafeSpace API base URL")
	skillID := fs.String("skill-id", "", "skill id")
	_ = fs.Parse(args)
	if strings.TrimSpace(*skillID) == "" {
		die("--skill-id is required")
	}
	client := rater.NewClient(*server)
	s, err := client.GetSkillScore(*skillID)
	if err != nil {
		die(err.Error())
	}
	fmt.Printf("skill=%s score=%.2f\n", s.SkillID, s.Score)
}

func runTop(args []string) {
	fs := flag.NewFlagSet("top", flag.ExitOnError)
	server := fs.String("server", defaultServerURL(), "SafeSpace API base URL")
	limit := fs.Int("limit", 20, "top limit")
	minCount := fs.Int("min-count", 1, "minimum rating count")
	_ = fs.Parse(args)

	client := rater.NewClient(*server)
	items, err := client.GetTopSkills(*limit, *minCount)
	if err != nil {
		die(err.Error())
	}
	for i, item := range items {
		fmt.Printf("%d) %s score=%.2f\n", i+1, item.SkillID, item.Score)
	}
	fmt.Printf("total=%d\n", len(items))
}

func runAuditLocal(args []string) {
	fs := flag.NewFlagSet("audit-local", flag.ExitOnError)
	server := fs.String("server", defaultServerURL(), "SafeSpace API base URL")
	identityPath := fs.String("identity", defaultIdentityPath(), "path to identity json")
	skillsDir := fs.String("skills-dir", defaultSkillsDir(), "skills directory")
	source := fs.String("source", "openclaw", "fallback skill source")
	version := fs.String("version", "local", "fallback skill version")
	auto := fs.Bool("auto", true, "auto detect source/version/name from SKILL.md frontmatter")
	sampleRate := fs.Int("sample-rate", 5, "random sample percentage (0..100)")
	maxSubmit := fs.Int("max-submit", 5, "max submissions in one run")
	maxReportRunes := fs.Int("max-report-runes", 500, "max report/comment runes")
	dryRun := fs.Bool("dry-run", false, "generate report only without upload")
	cachePath := fs.String("cache", defaultAuditCachePath(), "audit dedupe cache path")
	reportDir := fs.String("report-dir", defaultAuditReportDir(), "output directory for audit reports")
	_ = fs.Parse(args)

	if *sampleRate < 0 || *sampleRate > 100 {
		die("--sample-rate must be between 0 and 100")
	}
	if *maxSubmit <= 0 {
		die("--max-submit must be > 0")
	}
	if *maxReportRunes <= 0 || *maxReportRunes > 500 {
		die("--max-report-runes must be between 1 and 500")
	}

	var (
		skills []rater.DiscoveredSkill
		err    error
	)
	if *auto {
		skills, err = rater.DiscoverSkillsAuto(*skillsDir, *source, *version)
	} else {
		skills, err = rater.DiscoverSkills(*skillsDir, *source, *version)
	}
	if err != nil {
		die(err.Error())
	}

	cache, err := rater.LoadAuditCache(*cachePath)
	if err != nil {
		die(err.Error())
	}

	var identity rater.Identity
	if !*dryRun {
		identity, err = rater.LoadIdentity(*identityPath)
		if err != nil {
			die(err.Error())
		}
	}
	client := rater.NewClient(*server)

	if err := os.MkdirAll(*reportDir, 0o700); err != nil {
		die(err.Error())
	}

	uploaded := 0
	skipped := 0
	for _, s := range skills {
		audit, err := rater.BuildSkillAudit(s.SkillID, s.Path, *sampleRate, *maxReportRunes)
		if err != nil {
			fmt.Printf("skip skill=%s err=%v\n", s.SkillID, err)
			skipped++
			continue
		}

		reportPath := filepath.Join(*reportDir, sanitizeFilename(s.SkillID)+".md")
		_ = os.WriteFile(reportPath, []byte(audit.Report+"\n"), 0o600)

		if !cache.ShouldUpload(s.SkillID, audit.ReportHash) {
			fmt.Printf("dedupe skill=%s hash=%s\n", s.SkillID, audit.ReportHash[:12])
			skipped++
			continue
		}

		comment := rater.ComposeAuditComment(audit, *maxReportRunes)
		if *dryRun {
			fmt.Printf("dry-run skill=%s score=%.1f sampled=%v report=%s\n", s.SkillID, audit.Score, audit.Sampled, reportPath)
			cache.MarkUploaded(s.SkillID, audit.ReportHash)
			uploaded++
			if uploaded >= *maxSubmit {
				break
			}
			continue
		}

		if err := client.SubmitSkillRating(identity, s.SkillID, audit.Score, comment); err != nil {
			fmt.Printf("skip skill=%s err=%v\n", s.SkillID, err)
			skipped++
			continue
		}
		cache.MarkUploaded(s.SkillID, audit.ReportHash)
		uploaded++
		fmt.Printf("audited skill=%s score=%.1f sampled=%v report=%s\n", s.SkillID, audit.Score, audit.Sampled, reportPath)
		if uploaded >= *maxSubmit {
			break
		}
	}

	if err := rater.SaveAuditCache(*cachePath, cache); err != nil {
		die(err.Error())
	}
	fmt.Printf("audit uploaded=%d skipped=%d discovered=%d\n", uploaded, skipped, len(skills))
}

func defaultServerURL() string {
	if v := strings.TrimSpace(os.Getenv("SAFESPACE_SERVER")); v != "" {
		return v
	}
	return "http://skillvet.cc.cd"
}

func defaultIdentityPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "data/rater-identity.json"
	}
	return filepath.Join(home, ".safespace", "rater-identity.json")
}

func defaultAuditCachePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "data/audit-cache.json"
	}
	return filepath.Join(home, ".safespace", "audit-cache.json")
}

func defaultAuditReportDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "data/audit-reports"
	}
	return filepath.Join(home, ".safespace", "audit-reports")
}

func sanitizeFilename(v string) string {
	replacer := strings.NewReplacer("/", "_", "@", "_", " ", "_", "..", "_", "\\", "_")
	s := replacer.Replace(strings.TrimSpace(v))
	if s == "" {
		return "skill"
	}
	return s
}

func defaultSkillsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".agents/skills"
	}
	return filepath.Join(home, ".agents", "skills")
}

func die(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}

func printUsage() {
	fmt.Println("safespace-rater commands:")
	fmt.Println("  register   --agent-id <id> [--server] [--identity]")
	fmt.Println("  rate       --skill-id <source/name@version> --score <0..100> [--comment] [--identity] [--server]")
	fmt.Println("  discover   [--skills-dir] [--auto] [--source] [--version]")
	fmt.Println("  rate-local [--interactive] [--score <0..100>] [--skills-dir] [--auto] [--source] [--version] [--max-submit] [--comment] [--identity] [--server]")
	fmt.Println("  summary    --skill-id <source/name@version> [--server]")
	fmt.Println("  top        [--limit] [--min-count] [--server]")
	fmt.Println("  audit-local [--skills-dir] [--sample-rate 5] [--max-report-runes 500] [--max-submit] [--dry-run] [--cache] [--report-dir] [--identity] [--server]")
}
