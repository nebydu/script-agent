#!/usr/bin/env bash
# deny-write-paths.sh — PreToolUse 가드 (Git Bash 전용)
# Edit/Write/NotebookEdit 호출 직전에 file_path를 검사해
# 금지 경로(docs/, ../monitoring-meta, ../hub)에 대한 쓰기를 차단한다.
#
# 정책:
#   - settings.json permissions.deny가 일부 환경(자동모드 등)에서 무력화되는 것에 대비한 보조 가드.
#   - `.claude/`는 settings.json deny에서 사용자가 명시 제거 → 메인 Claude의 하니스 자기 수정 허용.
#     sub-agent들은 frontmatter tools + 강제 룰로 별도 차단되므로 이 hook은 .claude/를 막지 않는다.
#
# 경로 정규화는 python `os.path.realpath` + 슬래시 통일 + lowercase로 일원화한다.
# bash parameter expansion(${var//\\//})은 MSYS `/c/...` 형식과 결합해 잘못 동작하는 함정이 있어
# 의도적으로 사용하지 않는다.
set -euo pipefail

INPUT="$(cat)"

# tool_input.file_path 추출 + 정규화 (realpath → 슬래시 → lowercase)
TARGET="$(printf '%s' "$INPUT" | PYTHONIOENCODING=utf-8 python -c '
import json, sys, os
try:
    d = json.loads(sys.stdin.read())
    ti = d.get("tool_input", {})
    # NotebookEdit은 notebook_path, 일부 도구는 path를 쓴다 — hub pre-write-guard.sh와 동일한 3중 폴백
    p = ti.get("file_path") or ti.get("notebook_path") or ti.get("path") or ""
    if not isinstance(p, str) or not p:
        sys.exit(0)
    norm = os.path.realpath(p).replace("\\", "/").lower()
    print(norm)
except SystemExit:
    raise
except Exception:
    sys.exit(0)
' 2>/dev/null || echo "")"

# 빈 file_path는 통과(다른 매개변수 형태나 비파일 도구 안전 기본값).
[ -z "$TARGET" ] && exit 0

# REPO_ROOT / PARENT도 동일한 정규화 형식으로 (lowercase mixed-style)
NORM_PY='import os, sys; print(os.path.realpath(sys.argv[1]).replace("\\", "/").lower())'
REPO_ROOT="$(PYTHONIOENCODING=utf-8 python -c "$NORM_PY" "$(git rev-parse --show-toplevel)")"
PARENT="$(dirname "$REPO_ROOT")"

DENY=""
case "$TARGET" in
  "$REPO_ROOT"/docs/*)             DENY="docs/ (정본 문서 사본)" ;;
  "$PARENT"/monitoring-meta/*)     DENY="../monitoring-meta/ (정본 repo)" ;;
  "$PARENT"/hub/*)                 DENY="../hub/ (형제 repo)" ;;
esac

if [ -n "$DENY" ]; then
  echo "[deny-write-paths] 금지 경로 쓰기 차단: $TARGET ($DENY)" >&2
  exit 2
fi

exit 0
