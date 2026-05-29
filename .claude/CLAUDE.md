# .claude/CLAUDE.md — script-agent 자동화 오케스트레이션 규칙

이 파일은 script-agent repo의 `.claude/` sub-agent 자동화가 따르는 운영 규칙이다.
script-agent 루트의 `CLAUDE.md` / `AGENTS.md`(Claude Code ↔ Codex 이중 에이전트 규칙)와 별개로, **sub-agent 파이프라인 동작**을 규정한다.

## 0. 언어 규칙
- 답변 / 문서 / 주석은 **한국어**.
- 변수 / 함수 / 타입 / 파일명 등 식별자는 **영어**(Go 표준 컨벤션).

## 1. 위상 구분 경고 (가장 중요)
- **envelope spec(`../monitoring-meta/docs/envelope.md`)이 monitoring-meta에 박혔지만, script-agent의 Go Kafka 코드는 여전히 Phase 0 데모 spec(v0.2.1) 위상에 있다.**
- "envelope.md가 정의됐다 ≠ Go 코드가 envelope을 따른다." 현재 코드는 데모 spec v0.2.1 §7.2의 envelope 헤더 규약(x-message-id/x-message-version/x-source/x-trace-id)을 따르는 Phase 0 위상이다. 이 자동화는 **이 위상 차이를 인지한 채로** 작동한다.
- envelope 헤더 발행은 Phase 0 spec v0.2.1에 이미 포함된 동작이므로 정상이다. 반면 envelope.md의 Phase 1 consumer측 동작(x-message-id 중복 검사, x-trace-id trace 복원 등)이 없는 것도 **Phase 0 위상에서는 정상**이며 위반이 아니다.

## 2. ground truth 우선순위
1. **코드** (현재 동작의 사실)
2. **데모 spec v0.2.1** (`docs/monitoring-demo-message-spec-v0.2.1.md`) — Phase 0 회귀 방지 기준
3. **통합본 v0.9 + envelope + kafka-payloads** (`../monitoring-meta/docs/`) — Phase 1+ 도달 목표

## 3. 작업 입력 형식
- 작업 spec은 **`../monitoring-meta/handoff/<work-id>-script-agent.md`** 한 곳에서만 받는다.
- 다른 위치(채팅 임의 지시, 다른 디렉터리 파일)에서 작업 spec을 받아 파이프라인을 시작하지 않는다.

## 4. 금지 사항
- **단계 점프 금지**: analyzer 산출물 없이 implementer로 가는 등 표준 호출 순서를 건너뛰지 않는다.
- **monitoring-meta / hub는 read-only**: `../monitoring-meta/`(HANDOFF.md, 통합본 v0.9, envelope.md, kafka-payloads.md)와 `../hub/`를 script-agent repo에서 직접 수정하지 않는다.
- **script-agent 루트 AGENTS.md 자동 갱신 금지**: 셋업 이후 사람이 수동 처리한다.
- **HANDOFF.md §5 체크박스 자동 갱신 금지**: 사람이 수동 처리한다.

## 5. 표준 호출 순서와 재시도 한도
```
analyzer → implementer → tester → (병렬) reviewer + spec-guardian → (필요시) refactorer → Stop 시 Codex
```
- analyzer 산출물에 **사람 결정이 필요한 미결정 사안**이 있으면 즉시 멈추고 **사람을 호출**한다. implementer를 호출하지 않는다.
- **implementer 재시도는 작업 spec id 단위로 최대 3회.** 초과 시 사람 escalation.
- **reviewer / spec-guardian은 병렬 호출**하며, **둘 다 통과(critical 0)해야** 다음 단계로 넘어간다.
- **refactorer는 reviewer가 구조 개선을 권고한 경우에만** 호출하고, 행위 보존을 reviewer + tester가 확인한다.

## 6. sub-agent 결과 보고 스키마 (공통)
모든 sub-agent는 본문 끝에 아래 JSON을 출력하고, 그 뒤에 **"외부 surface"** 섹션(script-agent 외부 — monitoring-meta / hub / infra(특히 heartbeats/OTel Collector) 파급 이슈 분류)을 둔다.
```json
{
  "status": "ok | blocked | failed",
  "outputs": ["생성/수정한 파일 경로"],
  "findings": ["발견 사항"],
  "blockers": ["사람 결정이 필요하거나 다음 단계를 막는 항목"],
  "next_action": "다음에 할 일 한 줄"
}
```

## 7. sub-agent 역할 / Write 권한 요약
| agent | Write 권한 | 핵심 |
|---|---|---|
| analyzer | `docs/`, `analysis/` | 종합 분석, 미결정 사안 발견 시 사람 호출 게이트 |
| implementer | `cmd/**`, `internal/**`, `go.mod`, `go.sum`, 리소스 | Go 코드 구현, 재시도 3회 한도 |
| tester | `*_test.go`만 | 회귀 1차 책임, 프로덕션 코드 수정 금지, Kafka 통합 테스트는 e2e 위임 |
| reviewer | 없음 | §6.2 Job 실행 정책 불변식 critical / internal 의존 방향 warning, 보고서만 |
| spec-guardian | 없음 | 위상 분류 + envelope 헤더 규약, 보고서만 |
| refactorer | `cmd/**`, `internal/**`, `*_test.go` | 행위 보존 리팩터링, reviewer 권고 시에만 |

## 8. script-agent 핵심 불변식 (모든 sub-agent 공유 회귀 금지선 — 데모 spec v0.2.1 §6.2)
1. **at-least-once**: 결과/감사 발행 완료 전 Kafka offset commit 금지(fetch → dispatch 완료 → commit).
2. **fail-fast**: `job-results`/`audit-events` publish 실패 시 `exit 1`(consumer self-terminate) 경로로만 진행.
3. **발행 순서**: `job-results` 먼저, `audit-events`(JOB_EXECUTED) 나중. results 실패 후 audit 시도 금지.
4. **valid_until**: 지난 명령은 silent skip(commit safe).
5. **직렬 처리**: 단일 consumer goroutine 직렬 처리(동일 schedule 재진입 가능 구조 금지).
6. **LOG_JOB rotation**: `file_id`(inode/file index) 변경·size shrink 시 재시작 누락 금지.

## 9. Stop hook (Codex 게이트)
- `hooks/codex-gate.sh`가 Stop 이벤트에서 작동한다.
- **실행 방식(Windows 주의)**: `settings.json`은 exec form으로 Git Bash 절대경로(`C:\Program Files\Git\bin\bash.exe`)를 직접 호출한다. shell form에서 `bash`를 쓰면 Windows PATH상 `C:\Windows\System32\bash.exe`(WSL bash)로 잡혀 실패하므로, **중첩 `bash` 호출 없이 Git Bash `.exe`를 exec form(`args`)으로 지정**한다. Git Bash 설치 경로가 다른 머신에서는 이 절대경로를 수정해야 한다(이식성 주의).
- **발화 대상**: `cmd/**`, `internal/**`, `go.mod`, `go.sum`, `*.go` 변경 시 Codex(`codex exec --sandbox read-only`) 검토 호출.
- **스킵 대상**: `.claude/**`, `docs/**`, `analysis/**` 등 비코드 산출물만 변경된 경우.
- **안전장치**: `stop_hook_active` 무한루프 가드, FAIL 3회 초과 시 강제 통과, 파싱 2회 연속 실패 시 강제 통과(모두 escalation 로그 기록).
- 상태 파일(`.codex-gate-*`, `.codex-last-message.json` 등)은 `.gitignore`에 등록되어 추적되지 않는다.
