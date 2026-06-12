# .claude/CLAUDE.md — script-agent 자동화 오케스트레이션 규칙

이 파일은 script-agent repo의 `.claude/` sub-agent 자동화가 따르는 운영 규칙이다.
script-agent 루트의 `CLAUDE.md` / `AGENTS.md`(Claude Code ↔ Codex 이중 에이전트 규칙)와 별개로, **sub-agent 파이프라인 동작**을 규정한다.

## 0. 언어 규칙
- 답변 / 문서 / 주석은 **한국어**.
- 변수 / 함수 / 타입 / 파일명 등 식별자는 **영어**(Go 표준 컨벤션).

## 1. 상태 구분 경고 (가장 중요)
- **envelope spec(`../monitoring-meta/docs/envelope.md`)이 monitoring-meta에 박혔지만, script-agent의 Go Kafka 코드는 여전히 Phase 0 데모 spec(v0.2.1) 상태에 있다.**
- "envelope.md가 정의됐다 ≠ Go 코드가 envelope을 따른다." 현재 코드는 데모 spec v0.2.1 §7.2의 envelope 헤더 규약(x-message-id/x-message-version/x-source/x-trace-id)을 따르는 Phase 0 상태다. 이 자동화는 **이 상태 차이를 인지한 채로** 작동한다.
- envelope 헤더 발행은 Phase 0 spec v0.2.1에 이미 포함된 동작이므로 정상이다. 반면 envelope.md의 Phase 1 consumer측 동작(x-message-id 중복 검사, x-trace-id trace 복원 등)이 없는 것도 **Phase 0 상태에서는 정상**이며 위반이 아니다.
- **운영 원칙**: "통합본 우선 + Phase 분류 + 데모 회귀 방지". 통합본(`../monitoring-meta/docs/master-design.md`)을 방향 판단의 최상위 기준으로 두되, 통합본의 Phase 1+ 목표를 현재 Phase 0 코드에 무조건 강제하지 않는다(불필요한 fail 방지). 작업마다 Phase 0 유지인지 Phase 1+ 선반영인지 먼저 분류한다.

## 2. ground truth 우선순위
> 방향 판단의 최상위 기준은 통합본이고, 데모 spec은 Phase 0 회귀 방지 가드로 역할을 축소한다.
1. **통합본** (`../monitoring-meta/docs/master-design.md`) — 전체 제품 요구·아키텍처·모듈 경계·Phase 방향의 최상위 판단 기준
2. **작업 spec** (`../monitoring-meta/handoff/<work-id>/<work-id>-script-agent.md`) — 이번 작업에서 script-agent가 구현할 구체 입력
3. **코드** (현재 script-agent의 실제 동작·제약의 사실)
4. **데모 spec v0.2.1** (`../monitoring-meta/docs/phase0-snapshot/monitoring-demo-message-spec-v0.2.1.md`) — Phase 0 회귀 방지 가드. 통합본과 충돌 시 현재 Phase에서 어떻게 적용할지 판단
5. **envelope + kafka-payloads** (`../monitoring-meta/docs/`) — 메시징 세부 규약(Phase 1+ 도달 목표)

> **근거(provenance)**: 셋업 원 브리프의 초기 우선순위는 "코드 → 데모 spec v0.2.1 → 통합본"이었으나, 이전 하네스 정렬 과정에서 통합본 중심 재조정이 반영됐고 형제 repo `hub`도 현재 통합본-우선 정렬 상태다. 폴리레포 오케스트레이션 일관성을 위해 위 순서로 정렬한 것은 **사용자 명시 승인(2026-05-29)** 사항이다. 원 브리프 acceptance 기준을 의도적으로 supersede한 것이며, 데모 spec v0.2.1은 버린 것이 아니라 #4 Phase 0 회귀 가드로 유지된다.

## 3. 작업 입력 형식
- 작업 spec은 **`../monitoring-meta/handoff/<work-id>/<work-id>-script-agent.md`** 한 곳에서만 받는다.
- 다른 위치(채팅 임의 지시, 다른 디렉터리 파일)에서 작업 spec을 받아 파이프라인을 시작하지 않는다.
- **work-id 바인딩 계약**: 파이프라인 시작 시 `<work-id>`를 **명시적으로 확정**하고, analyzer → implementer → tester → reviewer/spec-guardian 모든 호출에 **동일한 work-id를 전달**한다. work-id가 불명확하면 대화 맥락으로 추론하지 말고 멈춰 사람에게 확인한다. (Codex 게이트(plugin Stop hook)는 git diff 기반 경량 게이트라 work-id를 받지 않으며 handoff 일관성을 검사하지 않는다 — handoff 검사는 analyzer/spec-guardian 책임.)
- **monitoring-meta 버전 핀**: 작업 spec(`../monitoring-meta/handoff/<work-id>/<work-id>-script-agent.md`) 헤더에 `기준 monitoring-meta commit: <hash>`(전체 또는 단축 SHA)가 **반드시 명시**돼야 한다. analyzer가 파이프라인 첫 행동으로 `git -C ../monitoring-meta rev-parse HEAD` 결과와 대조하며, **불일치하면 분석을 진행하지 않고 멈춰 사람에게 보고**한다(`blockers`에 drift 사실 명시, `status: blocked`). 기준 문서(monitoring-meta)가 spec 작성 시점 이후 변동된 상태에서 그 spec을 기준으로 분석·구현하지 않기 위한 안전장치다.

## 4. 금지 사항
- **단계 점프 금지**: analyzer 산출물 없이 implementer로 가는 등 표준 호출 순서를 건너뛰지 않는다.
- **monitoring-meta / hub는 read-only**: `../monitoring-meta/`(HANDOFF.md, 통합본, envelope.md, kafka-payloads.md)와 `../hub/`를 script-agent repo에서 직접 수정하지 않는다.
- **script-agent 루트 AGENTS.md / CLAUDE.md 파이프라인 자동 갱신 금지**: sub-agent 파이프라인이 임의로 건드리지 않는다. 단, **사람이 명시적으로 지시한 수동 갱신은 예외**이며 근거를 커밋 메시지에 남긴다(예: 2026-05-29 통합본 중심 정렬 — 사용자 승인).

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
| analyzer | `analysis/`만 | 종합 분석, 미결정 사안 발견 시 사람 호출 게이트. `docs/`는 기준 문서 사본 보호로 쓰기 금지(settings.json deny와 일치) |
| implementer | `cmd/**`, `internal/**`, `go.mod`, `go.sum`, 리소스 | Go 코드 구현, 재시도 3회 한도 |
| tester | `*_test.go`만 | 회귀 1차 책임, 프로덕션 코드 수정 금지, Kafka 통합 테스트는 e2e 위임 |
| reviewer | 없음 | §6.2 Job 실행 정책 불변식 critical / internal 의존 방향 warning, 보고서만 |
| spec-guardian | 없음 | 상태 분류 + envelope 헤더 규약, 보고서만 |
| refactorer | `cmd/**`, `internal/**`, `*_test.go` | 행위 보존 리팩터링, reviewer 권고 시에만 |

## 8. script-agent 핵심 불변식 (모든 sub-agent 공유 회귀 금지선 — 데모 spec v0.2.1 §6.2)
1. **at-least-once**: 결과/감사 발행 완료 전 Kafka offset commit 금지(fetch → dispatch 완료 → commit).
2. **fail-fast**: `job-results`/`audit-topic` publish 실패 시 `exit 1`(consumer self-terminate) 경로로만 진행.
3. **발행 순서**: `job-results` 먼저, `audit-topic`(JOB_EXECUTED) 나중. results 실패 후 audit 시도 금지.
4. **valid_until**: 지난 명령은 silent skip(commit safe).
5. **직렬 처리**: 단일 consumer goroutine 직렬 처리(동일 schedule 재진입 가능 구조 금지).
6. **LOG_JOB rotation**: `file_id`(inode/file index) 변경·size shrink 시 재시작 누락 금지.

## 9. Stop hook (Codex 게이트) — monitoring-harness plugin
- Codex 게이트는 **`harness@monitoring` plugin**(project scope, `enabledPlugins`)이 제공하는 Stop hook으로 작동한다. 이전의 native `.claude/hooks/codex-gate.sh` + `settings.json`의 Stop 블록은 **2026-06-02 plugin 전환**으로 제거됐다(런타임 동등성은 plugin `shared/hooks/h2b-validation.md`에서 검증).
- **실행 방식**: plugin `hooks/hooks.json`의 Stop hook이 `${CLAUDE_PLUGIN_ROOT}/hooks/git-bash.cmd` shim을 통해 `codex-gate-entry.sh`를 호출한다. shim은 `%ProgramFiles%\Git\bin\bash.exe`를 동적 expand한다(표준 Git for Windows; **PATH 의존 없음** — WSL `bash`가 먼저 잡혀도 무영향). entry는 **convention 경로**(`${CLAUDE_PROJECT_DIR}/.claude/codex-gate.profile`)의 consumer delta를 로드한 뒤 plugin 공통 골격(`shared/hooks/codex-gate-core.sh`)을 실행한다. profile은 convention 위치에서 자동 발견되므로 per-user config가 필요 없다(협업자 공유 안전).
- **consumer delta**: 트리거/스킵 glob·Codex 프롬프트·escalation 임계 등 script-agent 도메인 값은 `.claude/codex-gate.profile`에만 둔다(공통 골격은 plugin이 보유하며 복제하지 않는다).
- **발화 대상**: `cmd/**`, `internal/**`, `go.mod`, `go.sum`, `*.go` 변경 시 Codex(`codex exec --sandbox read-only`) 검토 호출.
- **스킵 대상**: `.claude/**`, `docs/**`, `analysis/**` 등 비코드 산출물만 변경된 경우.
- **안전장치**: `stop_hook_active` 무한루프 가드, FAIL 3회 초과 시 강제 통과, 파싱 2회 연속 실패 시 강제 통과(모두 escalation 로그 기록).
- **상태 파일**: escalation 카운터 등 보존 상태는 plugin 업데이트를 넘어 사는 `${CLAUDE_PLUGIN_DATA}`에 둔다. 구 native 상태 파일(`.codex-gate-*`, `.codex-last-message.json` 등) 패턴은 `.gitignore`에 유지된다.
