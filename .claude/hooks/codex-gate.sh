#!/usr/bin/env bash
# codex-gate.sh — script-agent Stop hook (Git Bash 전용)
# Codex 호출 경로 = fallback(codex exec). 이유: codex-cli 0.134.0의 `codex review`가 --output-schema/--json 둘 다 미지원.
# JSON 파싱 = python. 이유: 이 환경에 jq 미설치(설치본 없음, Python 가용).
# 발화 대상 = script-agent Go 코드(cmd/**, internal/**, go.mod, go.sum, *.go). 스킵 = .claude/, docs/, analysis/ 등 비코드 산출물.
set -euo pipefail

# ── 경로 ────────────────────────────────────────────────────────────────
REPO_ROOT="$(git rev-parse --show-toplevel)"
CLAUDE_DIR="$REPO_ROOT/.claude"
STATE_FILE="$CLAUDE_DIR/.codex-gate-state"
LOG_FILE="$CLAUDE_DIR/codex-gate.log"
ESC_LOG="$CLAUDE_DIR/codex-gate-escalation.log"
SCHEMA="$CLAUDE_DIR/codex-schema.json"
LAST_MSG="$CLAUDE_DIR/.codex-last-message.json"
ISSUES_FILE="$CLAUDE_DIR/.codex-gate-issues.txt"
CODEX_ERR="$CLAUDE_DIR/.codex-gate-stderr.txt"

# empty tree object hash — 아직 커밋이 없을 때(HEAD 부재) diff 비교 기준
EMPTY_TREE="4b825dc642cb6eb9a060e54bf8d69288fbee4904"

# ── 유틸 ────────────────────────────────────────────────────────────────
log_line() { # verdict | crit_count | viol_count | triggered_files
  printf '%s | %s | %s | %s | %s\n' "$(date -Is)" "$1" "$2" "$3" "$4" >> "$LOG_FILE"
}
emit_system_message() { # message
  # PYTHONIOENCODING=utf-8: Windows 기본 콘솔 인코딩(cp949)에서 em dash 등 non-CP949 문자를
  # stdout에 쓸 때 UnicodeEncodeError가 나는 것을 막는다.
  PYTHONIOENCODING=utf-8 python -c 'import json, sys; print(json.dumps({"systemMessage": sys.argv[1]}, ensure_ascii=False))' "$1"
}
escalate() { # message
  printf '%s | %s\n' "$(date -Is)" "$1" >> "$ESC_LOG"
  emit_system_message "[codex-gate] 게이트 강제 통과 — 사람 확인 필요: $1"
}
read_state() {
  FAIL_COUNT=0; PARSE_FAIL_COUNT=0
  if [ -f "$STATE_FILE" ]; then
    read -r FAIL_COUNT PARSE_FAIL_COUNT < "$STATE_FILE" || true
  fi
  FAIL_COUNT=${FAIL_COUNT:-0}
  PARSE_FAIL_COUNT=${PARSE_FAIL_COUNT:-0}
}
write_state() { printf '%s %s\n' "$FAIL_COUNT" "$PARSE_FAIL_COUNT" > "$STATE_FILE"; }
reset_state() { FAIL_COUNT=0; PARSE_FAIL_COUNT=0; write_state; }

# ── 1) 무한 Stop 루프 가드 ───────────────────────────────────────────────
INPUT="$(cat)"
STOP_ACTIVE="$(printf '%s' "$INPUT" | python -c '
import sys, json
try:
    d = json.loads(sys.stdin.read())
    print("1" if d.get("stop_hook_active") else "0")
except Exception:
    print("0")
' 2>/dev/null || echo "0")"
[ "$STOP_ACTIVE" = "1" ] && exit 0

read_state

# ── 2) 트리거 가드 — script-agent Go 코드 변경이 있을 때만 Codex 호출 ──────
if git rev-parse --verify -q HEAD >/dev/null 2>&1; then
  BASE="HEAD"
else
  BASE="$EMPTY_TREE"
fi

CHANGED="$( { git -c core.quotepath=false diff --name-only "$BASE"; \
              git -c core.quotepath=false ls-files --others --exclude-standard; } | sort -u )"

TRIGGERED=""
while IFS= read -r f; do
  [ -z "$f" ] && continue
  case "$f" in
    .claude/*)      ;;                                  # 자동화 산출물 → 트리거 제외
    docs/*)         ;;                                  # 문서 → 트리거 제외
    analysis/*)     ;;                                  # analyzer 임시 산출물 → 트리거 제외
    cmd/*)          TRIGGERED="${TRIGGERED}${f}"$'\n' ;;
    internal/*)     TRIGGERED="${TRIGGERED}${f}"$'\n' ;;
    go.mod)         TRIGGERED="${TRIGGERED}${f}"$'\n' ;;
    go.sum)         TRIGGERED="${TRIGGERED}${f}"$'\n' ;;
    *.go)           TRIGGERED="${TRIGGERED}${f}"$'\n' ;;  # 루트 등 기타 위치의 Go 파일
  esac
done <<EOF
$CHANGED
EOF

# pipefail+set -e 환경: TRIGGERED가 비면 grep이 exit 1을 내므로 || true로 방어
TRIG_CSV="$(printf '%s' "$TRIGGERED" | grep -v '^$' | tr '\n' ',' | sed 's/,$//' || true)"

if [ -z "$TRIG_CSV" ]; then
  # .claude/, docs/, analysis/ 등 비코드 산출물만 변경 → 매번 검토 비용 크므로 스킵
  log_line "skipped" 0 0 "(no code change)"
  emit_system_message "[codex-gate] SKIP: cmd/**, internal/**, go.mod, go.sum, *.go 변경이 없어 Codex 검증을 건너뜁니다."
  exit 0
fi

# ── 3) 검토 입력 구성 (추적 변경 diff + 미추적 신규 코드 파일 내용) ────────
REVIEW_INPUT="$(git -c core.quotepath=false diff "$BASE")"
while IFS= read -r f; do
  [ -z "$f" ] && continue
  if ! git ls-files --error-unmatch "$f" >/dev/null 2>&1; then
    # 미추적 파일은 diff에 안 잡히므로 내용을 직접 합류
    REVIEW_INPUT="${REVIEW_INPUT}
--- NEW FILE: ${f} ---
$(cat "$REPO_ROOT/$f" 2>/dev/null)"
  fi
done <<EOF
$TRIGGERED
EOF

# ── 4) Codex 호출 (fallback: codex exec, read-only) ──────────────────────
PROMPT="script-agent(Go) 코드 변경 리뷰. 통합본 v0.9(../monitoring-meta/docs/통합본_v0_9.md)가 전체 제품/아키텍처 최상위 기준이다. 다음을 read-only로만 검토하고 codex-schema.json 형식의 JSON으로만 응답하라: (1) 통합본 v0.9 기준 전체 제품/아키텍처·command-topic routing 방향 위반 (2) 현재 작업 위상 분류 오류(Phase 0 유지 vs Phase 1+ 선반영) (3) Phase 0 데모 spec docs/monitoring-demo-message-spec-v0.2.1.md 회귀 (4) envelope/kafka-payloads 메시징 계약(헤더 4종) 위반 (5) 데모 spec v0.2.1 §6.2 Job 실행 정책 불변식 6종 위반 — ① at-least-once(결과/감사 발행 완료 전 Kafka offset commit 금지) ② fail-fast(job-results/audit-events publish 실패 시 exit 1 경로) ③ 발행 순서(job-results 먼저, audit-events JOB_EXECUTED 나중; results 실패 후 audit 시도 금지) ④ valid_until 지난 명령 silent skip ⑤ 단일 consumer goroutine 직렬 처리(동일 schedule 재진입 불가) ⑥ LOG_JOB file_id 변경·size shrink 시 재시작 (6) 버그·회귀 가능성 (7) 테스트 누락. 참고: handoff 작업 spec 정합성은 이 gate가 아니라 analyzer/spec-guardian이 담당하므로 여기서 검사하지 않는다(이 gate 입력에는 handoff가 포함되지 않음). 빌드 검증 기준은 'go build ./...', 'go test ./...', 'go vet ./...'이고, 로컬 실행 명령은 'go run ./cmd/agent'다(이 read-only 게이트의 검증 대상은 아님). 위상 주의: 통합본의 Phase 1+ 목표를 Phase 0 코드에 무조건 강제하지 말 것. envelope.md는 Phase 1 목표이고 script-agent 코드는 Phase 0 위상이므로, envelope consumer측 동작(중복검사/trace복원) 미구현은 위반이 아니다. 데모 단계는 Kafka 통합 테스트가 없으므로(모델/유틸 단위 테스트만), Kafka 실연동 테스트 부재 자체를 위반으로 보고하지 마라."

rm -f "$LAST_MSG" "$ISSUES_FILE"
set +e
printf '%s' "$REVIEW_INPUT" | codex exec --sandbox read-only \
  --output-schema "$SCHEMA" \
  -o "$LAST_MSG" \
  "$PROMPT" >/dev/null 2>"$CODEX_ERR"
set -e

# ── 5) 결과 파싱 (python) ────────────────────────────────────────────────
set +e
PARSE_OUT="$(python -c '
import sys, json
last_msg, issues_path = sys.argv[1], sys.argv[2]
try:
    with open(last_msg, "r", encoding="utf-8") as fp:
        d = json.load(fp)
    verdict = str(d.get("verdict", "")).strip()
    crit = d.get("critical_issues") or []
    viol = d.get("spec_violations") or []
    if verdict not in ("pass", "fail"):
        raise ValueError("invalid verdict: %r" % verdict)
    with open(issues_path, "w", encoding="utf-8") as g:
        for c in crit:
            g.write("[critical] " + str(c) + "\n")
        for v in viol:
            g.write("[spec] " + str(v) + "\n")
    sys.stdout.write("%s\t%d\t%d" % (verdict, len(crit), len(viol)))
except Exception as e:
    sys.stderr.write(str(e))
    sys.exit(3)
' "$LAST_MSG" "$ISSUES_FILE" 2>/dev/null)"
PARSE_RC=$?
set -e

# ── 5a) 파싱 실패 ────────────────────────────────────────────────────────
if [ "$PARSE_RC" -ne 0 ] || [ -z "$PARSE_OUT" ]; then
  PARSE_FAIL_COUNT=$((PARSE_FAIL_COUNT + 1))
  if [ "$PARSE_FAIL_COUNT" -ge 2 ]; then
    escalate "Codex 응답 파싱 2회 연속 실패 — 사람 확인 필요 (triggered: $TRIG_CSV)"
    log_line "parse_error(escalated)" 0 0 "$TRIG_CSV"
    reset_state
    exit 0
  fi
  write_state
  log_line "parse_error" 0 0 "$TRIG_CSV"
  {
    echo "[codex-gate] Codex 응답 파싱 실패. 원본 출력 앞 200자:"
    head -c 200 "$LAST_MSG" 2>/dev/null || true
    head -c 200 "$CODEX_ERR" 2>/dev/null || true
    echo ""
  } >&2
  exit 2
fi

VERDICT="$(printf '%s' "$PARSE_OUT" | cut -f1)"
CRIT_COUNT="$(printf '%s' "$PARSE_OUT" | cut -f2)"
VIOL_COUNT="$(printf '%s' "$PARSE_OUT" | cut -f3)"

# ── 5b) verdict == pass ──────────────────────────────────────────────────
if [ "$VERDICT" = "pass" ]; then
  log_line "pass" "$CRIT_COUNT" "$VIOL_COUNT" "$TRIG_CSV"
  reset_state
  emit_system_message "[codex-gate] PASS: Codex 검증 완료. blocking issue 없음, 수정사항 없음. 대상: $TRIG_CSV"
  exit 0
fi

# ── 5c) verdict == fail ──────────────────────────────────────────────────
PARSE_FAIL_COUNT=0   # 파싱은 성공했으므로 연속 파싱 실패 카운터 리셋
FAIL_COUNT=$((FAIL_COUNT + 1))
if [ "$FAIL_COUNT" -gt 3 ]; then
  escalate "Codex 검증 fail 3회 초과 — 사람 확인 필요 (triggered: $TRIG_CSV)"
  log_line "fail(escalated)" "$CRIT_COUNT" "$VIOL_COUNT" "$TRIG_CSV"
  reset_state
  exit 0
fi
write_state
log_line "fail" "$CRIT_COUNT" "$VIOL_COUNT" "$TRIG_CSV"
{
  echo "[codex-gate] Codex 검증 FAIL — 종료 보류. 아래 항목을 해소한 뒤 다시 종료하십시오:"
  cat "$ISSUES_FILE" 2>/dev/null
} >&2
exit 2
