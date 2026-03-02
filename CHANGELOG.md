## 1.3.4 - 2026-03-02

### Fixes
- Correct default Go install source from private `agent-safespace` to public `SkillVet`.
- Update fallback install hint to:
  - `go install github.com/vpn2004/SkillVet/cmd/safespace-rater@latest`

## 1.3.3 - 2026-03-02

### Fixes
- Add MIT license file for `safespace-rater` to make license metadata explicit.
- Improve `scripts/safespace-rater.sh` with auto-bootstrap flow:
  - local auto-build when source is available
  - `go install` fallback when binary is missing
- Update bilingual quickstart notes to clarify out-of-box bootstrap behavior.
