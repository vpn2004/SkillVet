# SkillVet Launch Kit (2026-03-02)

Primary link (single source of truth):
- https://clawhub.ai/vpn2004/safespace-rater

## Positioning (3 points)

### What is it?
- SkillVet is an open client to audit AI skills and build trust signals.

### Why does it matter?
- It helps teams decide whether a skill is safe **before** adoption.
- It converts subjective trust into score + evidence + history.

### How to use?
```bash
npx -y clawhub install safespace-rater
cd skills/safespace-rater
./scripts/safespace-rater.sh --check
./scripts/safespace-rater.sh audit-local --skills-dir ~/.agents/skills --auto --dry-run
```

---

## Copy A (Short CN)

我们发布了 SafeSpace Rater：
一个给 AI skills 做“安全体检 + 评分”的工具。

它能帮你：
1) 审计本地 skills
2) 生成可解释的安全评分
3) 可选上传到社区形成信誉信号

链接： https://clawhub.ai/vpn2004/safespace-rater

## Copy B (Short EN)

We launched SafeSpace Rater —
a practical tool for AI skills security audit and trust scoring.

It helps you:
1) Audit local skills
2) Generate explainable trust scores
3) Optionally publish ratings for community reputation

Link: https://clawhub.ai/vpn2004/safespace-rater

## Copy C (Quickstart)

```bash
npx -y clawhub install safespace-rater
cd skills/safespace-rater
./scripts/safespace-rater.sh --check
./scripts/safespace-rater.sh audit-local --skills-dir ~/.agents/skills --auto --dry-run
```

Review locally first, publish later if needed.
