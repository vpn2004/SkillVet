# TESTLIST.md

> 目的：把 SafeSpace 的测试从“临时跑一下”升级为“可维护清单”。
> 规则：**任何功能更新/修复后，必须同步更新本清单并跑对应测试。**

## 1) 单元测试（快速）

### rater 客户端与审计
- `go test ./internal/rater -run TestGetSkillScoreOrZeroReturnsZeroOnNotFound -v`
- `go test ./internal/rater -run TestBuildScoreDelta -v`
- `go test ./internal/rater -run TestBuildSkillAuditIncludesEvidence -v`
- `go test ./internal/rater -run TestContextAwareCommandExecutionRule -v`
- `go test ./internal/rater -run TestBuildHybridSkillAuditUsesBlendedScoreWhenLLMSucceeds -v`
- `go test ./internal/rater -run TestBuildHybridSkillAuditFallsBackToRuleScoreOnLLMFailure -v`
- `go test ./internal/rater -run TestComposeAuditCommentIncludesHybridMetadata -v`
- `go test ./internal/rater -run TestNewRuntimePreferredLLMScorerUsesScoreFileFirst -v`
- `go test ./internal/rater -run TestNewRuntimePreferredLLMScorerReturnsNilWhenNoRuntimeOrFallback -v`
- `go test ./internal/rater -run TestNewRuntimePreferredLLMScorerAllowsOpenAIFallbackWhenEnabled -v`
- `go test ./internal/rater -run TestPendingUploadQueueDedupAndPersistence -v`
- `go test ./internal/rater -run TestPendingUploadQueueDrain -v`

### CLI 辅助逻辑
- `go test ./cmd/safespace-rater -run TestDefaultPendingQueuePath -v`
- `go test ./cmd/safespace-rater -run TestRetryPendingUploads -v`

## 2) 端到端测试（核心流程）

### audit + 回读收益 + 重试补偿
- `go test ./tests/e2e -run TestAuditFlowEndToEnd -v`
- 覆盖路径：`register -> audit-local -> server score delta -> retry-pending -> summary`

### hybrid 评分联调（客户端规则+LLM）
- `go test ./tests/e2e -run TestAuditLocalUploadsHybridScoreWhenLLMConfigured -v`
- `go test ./tests/e2e -run TestAuditLocalPrefersRuntimeScoreFileOverOpenAIFallback -v`
- 覆盖路径：`register -> audit-local(LLM enabled) -> blended score upload -> comment metadata`
- 覆盖路径：`register -> audit-local(runtime score file) -> fallback bypass -> blended score upload`

## 3) 全量回归（提交前必跑）

- `go test ./...`

## 4) 人工验收 Checklist（发布前）

- [ ] `audit-local` 失败上传会写入 pending 队列
- [ ] `retry-pending` 可成功重试并清空已成功项
- [ ] 报告中含“服务端评分回读：before/after/delta”
- [ ] `ComposeAuditComment` 输出不超过 500 字约束
- [ ] 证据链字段完整：`rule_id/path/snippet_hash`

## 5) 维护规则

1. 新增命令/规则/队列行为时：
   - 先加失败测试（RED）
   - 再写最小实现（GREEN）
   - 通过后重构（REFACTOR）
2. 每次新增测试用例，必须把命令补到本文件。
3. 如果替换测试命令或目录结构，必须同步更新本文件，避免文档过期。
