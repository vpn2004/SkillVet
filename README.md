# SkillVet

公开客户端仓库（仅客户端，不含服务端）。

- ✅ 包含：`safespace-rater` CLI 源码、客户端审查打分逻辑、skill 定义

## 目录

- `cmd/safespace-rater/` CLI 入口
- `internal/rater/` 客户端能力（注册、评分、发现、audit-local、retry 队列、hybrid 评分）
- `internal/did/` 本地签名工具（Ed25519）
- `skills/safespace-rater/SKILL.md` Skill 定义
- `skills/safespace-rater/scripts/safespace-rater.sh` skill 内可执行入口（依赖检查/自动构建）
- `tests/e2e/` 端到端集成测试
- `TESTLIST.md` 回归测试清单（更新功能时必须同步）

## 快速开始

```bash
# 0) 可选：先跑 skill 依赖检查（推荐）
./skills/safespace-rater/scripts/safespace-rater.sh --check

# 1) 构建
make build

# 2) 注册 DID（首次）
./bin/safespace-rater register --agent-id your-agent --server https://skillvet.cc.cd

# 3) 发现本地 skills
./bin/safespace-rater discover --skills-dir ~/.agents/skills --auto

# 4) 生成 500 字内审查摘要并上传（默认 5% 抽检标记）
# 支持客户端 hybrid 分: final = 0.7*rule + 0.3*llm（LLM失败自动回退rule）
# 推荐 runtime/tool 侧提供 llm-score-file，CLI 只做融合与上传
./bin/safespace-rater audit-local \
  --skills-dir ~/.agents/skills \
  --auto \
  --llm-score-file ./runtime-llm-scores.json \
  --sample-rate 5 \
  --max-report-runes 500 \
  --max-submit 5 \
  --server https://skillvet.cc.cd

# 5) 上传失败可重试（离线补偿队列）
./bin/safespace-rater retry-pending --max-submit 20 --server https://skillvet.cc.cd
```

## 常用命令

```bash
./bin/safespace-rater register --agent-id <id>
./bin/safespace-rater rate --skill-id openclaw/weather@1.0.0 --score 88 --comment "good"
./bin/safespace-rater rate-local --skills-dir ~/.agents/skills --auto --score 80 --max-submit 3
./bin/safespace-rater summary --skill-id openclaw/weather@1.0.0
./bin/safespace-rater top --limit 20 --min-count 1
./bin/safespace-rater audit-local --skills-dir ~/.agents/skills --auto --dry-run
./bin/safespace-rater retry-pending --max-submit 20
```

## 测试（发布前必跑）

```bash
# 全量
make test

# 端到端集成测试（register -> audit-local -> retry-pending -> summary）
go test ./tests/e2e -run TestAuditFlowEndToEnd -v

# hybrid 联调（LLM enabled + runtime score file 优先）
go test ./tests/e2e -run 'TestAuditLocalUploadsHybridScoreWhenLLMConfigured|TestAuditLocalPrefersRuntimeScoreFileOverOpenAIFallback' -v
```

## 安全说明

- `audit-local` 评论摘要最大 500 字，便于轻量上传
- 建议仅上传必要摘要，不上传本地敏感原文
- 本仓库不托管服务端与密钥

## License

MIT
