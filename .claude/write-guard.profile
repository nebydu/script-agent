# write-guard.profile — script-agent 도메인 delta (monitoring-harness plugin 주입값)
#
# 이 파일은 plugin 공통 write-guard 골격(shared/hooks/write-guard-core.sh)에 주입하는
# script-agent 도메인 delta다. 실행 로직(골격)은 plugin이 보유하며 여기에 복제하지 않는다.
# plugin은 이 파일을 convention 경로(${CLAUDE_PROJECT_DIR}/.claude/write-guard.profile)에서
# 자동 발견해 로드한다(codex-gate.profile과 동일 방식).
#
# ── opt-in 스위치 (중요) ──────────────────────────────────────────────────
# 이 파일의 "존재" 자체가 write-guard 활성 스위치다. 파일이 없으면 write-guard-entry.sh가
# 비소비자로 간주해 조용히 통과(exit 0)하여 가드가 아예 돌지 않는다. 따라서 추가 차단 경로가
# 없어도(내용이 비어도) 이 파일은 반드시 존재해야 가드가 켜진다.
#
# ── 차단 규칙 출처 ────────────────────────────────────────────────────────
# 자기 docs/ 와 형제 repo 전체(../hub·../monitoring-meta + 형제 .claude)는 골격의 "유도 규칙"이
# 자동 차단한다(parent 하위이며 repo 밖). 자기 .claude/ 와 자기 repo 코드는 허용한다. 따라서
# script-agent는 추가 차단 경로가 필요 없어 WRITE_GUARD_BLOCK_PATHS를 비워 둔다.
# (동등성: 구 .claude/hooks/deny-write-paths.sh와 exit 매트릭스 일치 실측 완료.)
#
# ── 주입점 (선택) ─────────────────────────────────────────────────────────
#   WRITE_GUARD_BLOCK_PATHS=( ... )  # parent 밖 등 유도 규칙으로 못 잡는 추가 차단 루트(add-only)
# script-agent는 추가 경로 없음 — 의도적으로 미설정.
