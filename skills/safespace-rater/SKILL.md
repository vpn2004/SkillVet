---
name: safespace-rater
description: Use when agent needs to audit local skills, generate trust/reputation scores, submit ratings to SafeSpace, or retry failed pending uploads.
license: MIT
compatibility:
  openclaw: ">=0.0.0"
  runtime: "cli"
---

# SafeSpace Rater

Use this skill to submit OpenClaw skill ratings into SafeSpace.

## When to Use

- 需要对本地 skill 做安全审查并生成量化评分
- 需要把评分上传到 SafeSpace 公共网络（`skill_id` 维度）
- 需要批量评分本地 skills，并控制提交节奏
- 需要重试历史失败上传（pending queue）
- 需要“runtime 模型判断 + CLI 规则融合 + 上传”这类解耦流程

## Do Not Use

- 仅闲聊或无评分/审计目标的普通对话
- 与 skill 安全、信誉、评分无关的任务
- 需要修改服务端评分协议的场景（本 skill 不改 server API）

## Inputs

- `skills-dir`：本地技能目录（默认 `~/.agents/skills`）
- `identity`：本地 DID 身份文件
- `server`：SafeSpace API 地址
- `llm-score-file`（可选，推荐）：runtime/tool 侧输出的 LLM 分数 JSON
- `sample-rate` / `max-submit` / `max-report-runes`：审计与提交控制参数

## Outputs

- 提交结果：成功/失败数量、跳过数量
- 审计摘要：`audit:v2`（source/rule/llm/final/model 元信息）
- 本地报告：`~/.safespace/audit-reports/*.md`
- 待重试队列：`~/.safespace/pending-uploads.json`

## Discovery Trigger Phrases

- "audit local skills"
- "rate local skills for security"
- "submit skill reputation score"
- "retry pending skill ratings"
- "给本地技能做安全审计并上传评分"
- "批量生成技能信誉分"

## Prerequisites

- SafeSpace API is reachable (default: `https://skillvet.cc.cd`, override with `--server` or env `SAFESPACE_SERVER`)
- OpenClaw can execute shell scripts
- One of the following is true:
  - `SAFESPACE_RATER_BIN` points to an existing `safespace-rater` binary
  - Repository root is available and can build binary (`go >= 1.21`, optional `make`)

## Dependency Check (recommended before first run)

```bash
# Run from skill directory (SKILL.md location)
${SKILL_DIR:-.}/scripts/safespace-rater.sh --check
```

If check fails:

1. Build binary in project root:
```bash
make build
```

2. Or point to existing binary:
```bash
export SAFESPACE_RATER_BIN=/absolute/path/to/safespace-rater
```

## Workflow

1. Register local DID once:

```bash
${SKILL_DIR:-.}/scripts/safespace-rater.sh register --agent-id <your-agent-id>
```

2. Submit a single rating:

```bash
${SKILL_DIR:-.}/scripts/safespace-rater.sh rate \
  --skill-id openclaw/weather@1.0.0 \
  --score 90 \
  --comment "reliable"
```

3. Discover + bulk rate local skills:

```bash
# Auto mode: infer source/version/name from SKILL.md frontmatter if present
${SKILL_DIR:-.}/scripts/safespace-rater.sh discover --skills-dir ~/.agents/skills --auto --source openclaw --version local

# Bulk mode: one score for many skills
${SKILL_DIR:-.}/scripts/safespace-rater.sh rate-local --score 85 --skills-dir ~/.agents/skills --auto

# Interactive template mode: score/comment per skill
${SKILL_DIR:-.}/scripts/safespace-rater.sh rate-local --interactive --skills-dir ~/.agents/skills --auto --max-submit 5
```

4. Generate concise security audit report (<=500 chars) and submit score:

> `audit-local` 在客户端本地计算 hybrid 分：`0.7 * rule + 0.3 * llm`。若 LLM 不可用，自动降级为 rule 分，不影响上传流程。
> 默认优先使用 runtime/tool 侧产出的 LLM 评分文件；OpenAI-compatible API 仅作为显式 fallback。

```bash
# Dry-run: generate local reports only
${SKILL_DIR:-.}/scripts/safespace-rater.sh audit-local \
  --skills-dir ~/.agents/skills \
  --auto \
  --sample-rate 5 \
  --max-report-runes 500 \
  --dry-run

# Runtime/tool 侧模型评分文件（推荐）
# 结构示例：{"default":{"score":70,"model":"runtime"},"skills":{"openclaw/weather@local":{"score":82,"model":"runtime"}}}
${SKILL_DIR:-.}/scripts/safespace-rater.sh audit-local \
  --skills-dir ~/.agents/skills \
  --auto \
  --llm-score-file ./runtime-llm-scores.json \
  --sample-rate 5 \
  --max-report-runes 500 \
  --max-submit 5

# Submit audited score summaries to server (dedupe by local report hash)
${SKILL_DIR:-.}/scripts/safespace-rater.sh audit-local \
  --skills-dir ~/.agents/skills \
  --auto \
  --sample-rate 5 \
  --max-report-runes 500 \
  --max-submit 5
```

5. Query public single-score result:

```bash
${SKILL_DIR:-.}/scripts/safespace-rater.sh summary --skill-id openclaw/weather@1.0.0
${SKILL_DIR:-.}/scripts/safespace-rater.sh top --limit 10 --min-count 1
```

## Notes

- skill_id format: `source/name@version`
- Same DID rating the same skill within 10 minutes returns `429`
- `rate-local` defaults to max 5 submissions per run to avoid per-DID rate limit
- Interactive mode input: `0..100` submit / `s` skip / `q` quit
- `audit-local` report/comment are capped at 500 chars and saved under `~/.safespace/audit-reports`
- `audit-local` uses local hash cache (`~/.safespace/audit-cache.json`) to avoid duplicate uploads
- 默认优先读取 runtime 评分文件：`--llm-score-file` 或 `SAFESPACE_LLM_SCORE_FILE`
- OpenAI-compatible fallback 需显式开启：`SAFESPACE_LLM_OPENAI_FALLBACK=1`
- fallback 生效时需设置：`SAFESPACE_LLM_MODEL` + `SAFESPACE_LLM_API_KEY`
- 可选：`SAFESPACE_LLM_BASE_URL`（默认 OpenAI API），`SAFESPACE_LLM_TIMEOUT_MS`
