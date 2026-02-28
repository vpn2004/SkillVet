#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SKILL_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT_CANDIDATE="$(cd "${SKILL_DIR}/../.." 2>/dev/null && pwd || true)"

DEFAULT_BIN="${REPO_ROOT_CANDIDATE}/bin/safespace-rater"
BIN_PATH="${SAFESPACE_RATER_BIN:-${DEFAULT_BIN}}"

has_command() {
  command -v "$1" >/dev/null 2>&1
}

print_dep_status() {
  echo "[safespace-rater] dependency check"
  echo "- skill_dir: ${SKILL_DIR}"
  echo "- server(default): ${SAFESPACE_SERVER:-https://skillvet.cc.cd}"

  if has_command go; then
    echo "- go: $(go version)"
  else
    echo "- go: not found (optional if binary already exists)"
  fi

  if [[ -x "${BIN_PATH}" ]]; then
    echo "- binary: ok (${BIN_PATH})"
    return 0
  fi

  echo "- binary: missing (${BIN_PATH})"
  echo "  hint: set SAFESPACE_RATER_BIN=/absolute/path/to/safespace-rater"
  echo "  hint: or build from repo root: make build"
  return 1
}

try_auto_build() {
  if [[ -x "${BIN_PATH}" ]]; then
    return 0
  fi

  if [[ -n "${REPO_ROOT_CANDIDATE}" && -f "${REPO_ROOT_CANDIDATE}/go.mod" && -d "${REPO_ROOT_CANDIDATE}/cmd/safespace-rater" ]] && has_command go; then
    echo "[safespace-rater] binary not found, trying auto-build..."
    mkdir -p "${REPO_ROOT_CANDIDATE}/bin"
    if [[ -f "${REPO_ROOT_CANDIDATE}/Makefile" ]] && has_command make; then
      (cd "${REPO_ROOT_CANDIDATE}" && make build)
    else
      (cd "${REPO_ROOT_CANDIDATE}" && go build -o "${REPO_ROOT_CANDIDATE}/bin/safespace-rater" ./cmd/safespace-rater)
    fi
  fi

  [[ -x "${BIN_PATH}" ]]
}

if [[ "${1:-}" == "--check" ]]; then
  print_dep_status
  exit $?
fi

if ! try_auto_build; then
  print_dep_status >/dev/stderr || true
  cat >/dev/stderr <<'EOF'

[safespace-rater] cannot locate executable.
Use one of the following:
1) Build in repository root:
   make build
2) Point to an existing binary:
   export SAFESPACE_RATER_BIN=/absolute/path/to/safespace-rater
EOF
  exit 1
fi

exec "${BIN_PATH}" "$@"
