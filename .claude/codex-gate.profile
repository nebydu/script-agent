# codex-gate.profile — script-agent 도메인 delta (monitoring-harness plugin 주입값)
#
# 이 파일은 script-agent가 monitoring-harness 플러그인의 공통 codex-gate 골격에 주입하는
# 도메인 delta다. 실행 로직(골격)은 플러그인이 보유하며 여기에는 복제하지 않는다.
# 플러그인은 이 파일을 convention 경로(${CLAUDE_PROJECT_DIR}/.claude/codex-gate.profile)에서
# 자동 발견하여 로드한다(별도 설정 불필요 — userConfig/per-user config 의존 없음).
#
# 동등성 기준: 이 값들은 기존 .claude/hooks/codex-gate.sh 동작을 그대로 재현한다.

# ── 트리거 경로 (script-agent Go 코드) ────────────────────────────────────
CODEX_GATE_TRIGGER_GLOBS=( "cmd/*" "internal/*" "go.mod" "go.sum" "*.go" )

# ── 스킵 경로 (트리거보다 우선; 비코드 산출물) ────────────────────────────
CODEX_GATE_SKIP_GLOBS=( ".claude/*" "docs/*" "analysis/*" )

# ── 코드 변경 없음일 때 안내 메시지 ───────────────────────────────────────
CODEX_GATE_SKIP_MSG="[codex-gate] SKIP: cmd/**, internal/**, go.mod, go.sum, *.go 변경이 없어 Codex 검증을 건너뜁니다."

# ── Codex 리뷰 프롬프트 (script-agent 도메인 전체) ────────────────────────
CODEX_GATE_PROMPT="script-agent(Go) 코드 변경 리뷰. 통합본(../monitoring-meta/docs/master-design.md)이 전체 제품/아키텍처 최상위 기준이다. 다음을 read-only로만 검토하고 codex-schema.json 형식의 JSON으로만 응답하라: (1) 통합본 기준 전체 제품/아키텍처·command-topic routing 방향 위반 (2) 현재 작업 상태 분류 오류(Phase 0 유지 vs Phase 1+ 선반영) (3) Phase 0 데모 spec ../monitoring-meta/docs/phase0-snapshot/monitoring-demo-message-spec-v0.2.1.md 회귀 (4) envelope/kafka-payloads 메시징 계약(헤더 4종) 위반 (5) 데모 spec v0.2.1 §6.2 Job 실행 정책 불변식 6종 위반 — ① at-least-once(결과/감사 발행 완료 전 Kafka offset commit 금지) ② fail-fast(job-results/audit-topic publish 실패 시 exit 1 경로) ③ 발행 순서(job-results 먼저, audit-topic JOB_EXECUTED 나중; results 실패 후 audit 시도 금지) ④ valid_until 지난 명령 silent skip ⑤ 단일 consumer goroutine 직렬 처리(동일 schedule 재진입 불가) ⑥ LOG_JOB file_id 변경·size shrink 시 재시작 (6) 버그·회귀 가능성 (7) 테스트 누락. 참고: handoff 작업 spec 일관성은 이 gate가 아니라 analyzer/spec-guardian이 담당하므로 여기서 검사하지 않는다(이 gate 입력에는 handoff가 포함되지 않음). 빌드 검증 기준은 'go build ./...', 'go test ./...', 'go vet ./...'이고, 로컬 실행 명령은 'go run ./cmd/agent'다(이 read-only 게이트의 검증 대상은 아님). [기준 문서 접근] 위 기준 문서(통합본·Phase 0 데모 spec·envelope/kafka-payloads)는 현재 작업 디렉터리에서 상대경로(../monitoring-meta/...)로 read-only 읽기가 보장된다. 판정에 필요하면 추측 말고 직접 읽어라. 파일 접근 불가를 사유로 critical/fail을 내지 마라(접근 가능). [Phase 분류 한계] 이 gate 입력에는 handoff 작업 spec이 없어 'Phase 0 유지'인지 'ADR/통합본으로 승인된 Phase 1 forward 컷오버'인지 단독 판별 불가하며, 그 분류는 analyzer/spec-guardian 책임이다(이 gate 범위 밖). 따라서 critical/fail은 diff에서 §6.2 동작 불변식(at-least-once·fail-fast 경로·발행순서·valid_until·단일 consumer 직렬·LOG_JOB rotation)을 실제로 깨거나 envelope/payload 계약을 위반하는 경우로 한정하라. 토픽명·상수·기본값이 Phase 0 snapshot 리터럴과 다르다는 사실만으로 '회귀'를 critical로 올리지 마라 — 방향이 통합본/ADR과 정합하고 §6.2 동작이 보존되면 forward 변경이며 승인 판정은 handoff를 가진 analyzer/spec-guardian 몫이다. 그런 우려는 spec_violations 비차단 note로만 남겨라. 상태 주의: 통합본의 Phase 1+ 목표를 Phase 0 코드에 무조건 강제하지 말 것. envelope.md는 Phase 1 목표이고 script-agent 코드는 Phase 0 상태이므로, envelope consumer측 동작(중복검사/trace복원) 미구현은 위반이 아니다. 데모 단계는 Kafka 통합 테스트가 없으므로(모델/유틸 단위 테스트만), Kafka 실연동 테스트 부재 자체를 위반으로 보고하지 마라."

# ── escalation 임계: script-agent 현행과 동일 ─────────────────────────────
CODEX_GATE_FAIL_LIMIT=3
CODEX_GATE_PARSE_FAIL_LIMIT=2
