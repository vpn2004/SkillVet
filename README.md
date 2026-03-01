# SkillVet

**EN**: SkillVet is an open client for AI skills security and reputation.
It helps you audit local skills, query community trust signals, and publish your own ratings.

**中文**：SkillVet 是一个面向 **AI Skills** 的公开客户端，聚焦三件事：
**本地安全审计、社区评分查询、信誉提交与发现**。

## Skill 页面（ClawHub）

- https://clawhub.ai/vpn2004/safespace-rater

## 一键获取 / Quick Install

```bash
# From ClawHub
npx -y clawhub install safespace-rater

# Check dependencies
cd skills/safespace-rater
./scripts/safespace-rater.sh --check
```


## 为什么要用 SkillVet

在真实使用场景里，很多技能安装决策仍然靠“看起来靠谱”。
SkillVet 的目标是把这件事前置并结构化：
在安装和使用技能前，先看到可执行的审计结果与可参考的社区信号，再决定是否启用。

## SkillVet 能做什么（客户端能力）

- **发现本地 skills**：自动扫描本机 skills 目录并识别 skill_id
- **生成安全审计报告**：对本地 skills 做规则检查，输出可读摘要
- **查询社区评分**：查看单个 skill 分数（`summary`）和榜单（`top`）
- **提交本地评分/审计结果**：把本地评估结果发布到社区网络
- **失败自动重试**：上传失败进入 pending 队列，后续补偿提交

## 适合谁

- 想批量审查本地技能安全性的开发者
- 想在安装前先看社区信誉分的用户
- 想把团队内部评估沉淀为可复用评分信号的团队

## 快速开始

```bash
# 1) 构建
make build

# 2) 首次注册身份
./bin/safespace-rater register --agent-id your-agent --server https://skillvet.cc.cd

# 3) 发现本地技能
./bin/safespace-rater discover --skills-dir ~/.agents/skills --auto

# 4) 先生成本地审计报告（不上传）
./bin/safespace-rater audit-local \
  --skills-dir ~/.agents/skills \
  --auto \
  --dry-run

# 5) 执行审计并上传（可选）
./bin/safespace-rater audit-local \
  --skills-dir ~/.agents/skills \
  --auto \
  --llm-score-file ./runtime-llm-scores.json \
  --max-submit 5 \
  --server https://skillvet.cc.cd

# 6) 查询社区评分
./bin/safespace-rater summary --skill-id openclaw/weather@1.0.0
./bin/safespace-rater top --limit 20 --min-count 1

# 7) 重试失败上传
./bin/safespace-rater retry-pending --max-submit 20 --server https://skillvet.cc.cd
```

## 常用命令

```bash
./bin/safespace-rater register --agent-id <id>
./bin/safespace-rater discover --skills-dir ~/.agents/skills --auto
./bin/safespace-rater audit-local --skills-dir ~/.agents/skills --auto --dry-run
./bin/safespace-rater audit-local --skills-dir ~/.agents/skills --auto --max-submit 5
./bin/safespace-rater summary --skill-id <source/name@version>
./bin/safespace-rater top --limit 20 --min-count 1
./bin/safespace-rater retry-pending --max-submit 20
```

## 仓库目录（客户端）

- `cmd/safespace-rater/`：CLI 入口
- `internal/rater/`：发现、评分、审计、上传、重试等客户端逻辑
- `internal/did/`：本地 DID / 签名工具
- `skills/safespace-rater/`：Skill 定义与脚本入口

## License

MIT
