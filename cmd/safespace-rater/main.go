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
	case "retry-pending":
		runRetryPending(os.Args[2:])
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
	llmScoreFile := fs.String("llm-score-file", "", "runtime llm score json path (preferred over API fallback)")
	cachePath := fs.String("cache", defaultAuditCachePath(), "audit dedupe cache path")
	reportDir := fs.String("report-dir", defaultAuditReportDir(), "output directory for audit reports")
	pendingPath := fs.String("pending-path", defaultPendingQueuePath(), "pending upload queue path")
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

	pendingQueue := rater.PendingUploadQueue{}
	if !*dryRun {
		pendingQueue, err = rater.LoadPendingUploadQueue(*pendingPath)
		if err != nil {
			die(err.Error())
		}
	}

	if err := os.MkdirAll(*reportDir, 0o700); err != nil {
		die(err.Error())
	}

	scorer, err := rater.NewRuntimePreferredLLMScorer(*llmScoreFile)
	if err != nil {
		die(err.Error())
	}
	weights := rater.HybridWeights{Rule: 0.7, LLM: 0.3}

	uploaded := 0
	skipped := 0
	for _, s := range skills {
		audit, err := rater.BuildHybridSkillAudit(s.SkillID, s.Path, *sampleRate, *maxReportRunes, scorer, weights)
		if err != nil {
			fmt.Printf("skip skill=%s err=%v\n", s.SkillID, err)
			skipped++
			continue
		}

		reportPath := filepath.Join(*reportDir, sanitizeFilename(s.SkillID)+".md")
		reportLines := []string{audit.Report}

		if !cache.ShouldUpload(s.SkillID, audit.ReportHash) {
			reportLines = append(reportLines, "", "上传状态：已跳过（重复内容）")
			_ = os.WriteFile(reportPath, []byte(strings.Join(reportLines, "\n")+"\n"), 0o600)
			fmt.Printf("dedupe skill=%s hash=%s\n", s.SkillID, audit.ReportHash[:12])
			skipped++
			continue
		}

		comment := rater.ComposeAuditComment(audit, *maxReportRunes)
		if *dryRun {
			reportLines = append(reportLines, "", "上传状态：dry-run（未上传）")
			_ = os.WriteFile(reportPath, []byte(strings.Join(reportLines, "\n")+"\n"), 0o600)
			fmt.Printf("dry-run skill=%s score=%.1f sampled=%v report=%s\n", s.SkillID, audit.Score, audit.Sampled, reportPath)
			cache.MarkUploaded(s.SkillID, audit.ReportHash)
			uploaded++
			if uploaded >= *maxSubmit {
				break
			}
			continue
		}

		beforeScore, beforeErr := client.GetSkillScoreOrZero(s.SkillID)
		if beforeErr != nil {
			fmt.Printf("warn skill=%s cannot load pre-score: %v\n", s.SkillID, beforeErr)
			beforeScore = 0
		}

		if err := client.SubmitSkillRating(identity, s.SkillID, audit.Score, comment); err != nil {
			pendingQueue.Enqueue(rater.PendingUpload{
				SkillID:    s.SkillID,
				Score:      audit.Score,
				Comment:    comment,
				ReportHash: audit.ReportHash,
			}, 1000)
			reportLines = append(reportLines, "", fmt.Sprintf("上传状态：失败（%v）", err), "补偿机制：已加入待重试队列")
			_ = os.WriteFile(reportPath, []byte(strings.Join(reportLines, "\n")+"\n"), 0o600)
			fmt.Printf("skip skill=%s err=%v queued_for_retry=true\n", s.SkillID, err)
			skipped++
			continue
		}

		afterScore, afterErr := client.GetSkillScoreOrZero(s.SkillID)
		if afterErr != nil {
			fmt.Printf("warn skill=%s cannot load post-score: %v\n", s.SkillID, afterErr)
			afterScore = beforeScore
		}
		delta := rater.BuildScoreDelta(beforeScore, afterScore)
		reportLines = append(reportLines, "", fmt.Sprintf("服务端评分回读：before=%.2f after=%.2f delta=%+.2f", beforeScore, afterScore, delta), "上传状态：成功")
		_ = os.WriteFile(reportPath, []byte(strings.Join(reportLines, "\n")+"\n"), 0o600)

		cache.MarkUploaded(s.SkillID, audit.ReportHash)
		uploaded++
		fmt.Printf("audited skill=%s score=%.1f sampled=%v server_delta=%+.2f report=%s\n", s.SkillID, audit.Score, audit.Sampled, delta, reportPath)
		if uploaded >= *maxSubmit {
			break
		}
	}

	if err := rater.SaveAuditCache(*cachePath, cache); err != nil {
		die(err.Error())
	}
	if !*dryRun {
		if err := rater.SavePendingUploadQueue(*pendingPath, pendingQueue); err != nil {
			die(err.Error())
		}
	}
	fmt.Printf("audit uploaded=%d skipped=%d discovered=%d pending=%d\n", uploaded, skipped, len(skills), len(pendingQueue.Items))
}

func runRetryPending(args []string) {
	fs := flag.NewFlagSet("retry-pending", flag.ExitOnError)
	server := fs.String("server", defaultServerURL(), "SafeSpace API base URL")
	identityPath := fs.String("identity", defaultIdentityPath(), "path to identity json")
	pendingPath := fs.String("pending-path", defaultPendingQueuePath(), "pending upload queue path")
	maxSubmit := fs.Int("max-submit", 20, "max pending uploads to retry per run")
	_ = fs.Parse(args)

	if *maxSubmit <= 0 {
		die("--max-submit must be > 0")
	}

	identity, err := rater.LoadIdentity(*identityPath)
	if err != nil {
		die(err.Error())
	}

	queue, err := rater.LoadPendingUploadQueue(*pendingPath)
	if err != nil {
		die(err.Error())
	}
	if len(queue.Items) == 0 {
		fmt.Println("retry submitted=0 failed=0 remaining=0")
		return
	}

	client := rater.NewClient(*server)
	submitted, failed, remaining := retryPendingUploads(queue, *maxSubmit, func(item rater.PendingUpload) error {
		err := client.SubmitSkillRating(identity, item.SkillID, item.Score, item.Comment)
		if err != nil {
			fmt.Printf("retry failed skill=%s err=%v\n", item.SkillID, err)
			return err
		}
		fmt.Printf("retry ok skill=%s score=%.1f\n", item.SkillID, item.Score)
		return nil
	})

	if err := rater.SavePendingUploadQueue(*pendingPath, remaining); err != nil {
		die(err.Error())
	}
	fmt.Printf("retry submitted=%d failed=%d remaining=%d\n", submitted, failed, len(remaining.Items))
}

func retryPendingUploads(queue rater.PendingUploadQueue, maxSubmit int, submitFn func(item rater.PendingUpload) error) (submitted int, failed int, remaining rater.PendingUploadQueue) {
	remaining = rater.PendingUploadQueue{Items: make([]rater.PendingUpload, 0, len(queue.Items))}
	if maxSubmit <= 0 {
		maxSubmit = len(queue.Items)
	}

	processed := 0
	for _, item := range queue.Items {
		if processed >= maxSubmit {
			remaining.Items = append(remaining.Items, item)
			continue
		}
		processed++
		if err := submitFn(item); err != nil {
			failed++
			remaining.Items = append(remaining.Items, item)
			continue
		}
		submitted++
	}
	return submitted, failed, remaining
}

func defaultServerURL() string {
	if v := strings.TrimSpace(os.Getenv("SAFESPACE_SERVER")); v != "" {
		return v
	}
	return "https://skillvet.cc.cd"
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

func defaultPendingQueuePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "data/pending-uploads.json"
	}
	return filepath.Join(home, ".safespace", "pending-uploads.json")
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
	fmt.Println("  audit-local [--skills-dir] [--sample-rate 5] [--max-report-runes 500] [--max-submit] [--dry-run] [--llm-score-file] [--cache] [--report-dir] [--pending-path] [--identity] [--server]")
	fmt.Println("  retry-pending [--pending-path] [--max-submit] [--identity] [--server]")
}
