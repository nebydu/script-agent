# AGENTS.md

이 파일은 Claude Code와 Codex가 공통으로 따르는 단일 진실 원천(SoT)이다.
Claude Code는 `CLAUDE.md`에서 이 파일을 import하여 동일한 규칙을 적용한다.

## 언어
- 답변은 한국어로 한다.
- 코드 주석과 문서는 한국어로 작성한다.
- 변수명과 함수명은 영어로 작성한다(Go 표준 컨벤션 준수).

## 역할 분담
- 기본 구현과 큰 기능 개발은 Claude Code가 주도한다.
- Codex는 Claude Code가 만든 변경사항을 검토하고, 필요한 수정과 테스트를 병행한다.
- Codex는 리뷰 시 버그, 회귀 가능성, 명세 불일치, 테스트 누락을 우선 확인한다.
- Codex가 직접 수정한 변경사항은 Claude Code의 리뷰 대상이 된다.

## 작업 규칙
- 기존 코드 스타일을 우선한다.
- 변경 전 관련 파일을 먼저 읽는다.
- 사용자가 만든 변경사항은 되돌리지 않는다.
- 다른 에이전트가 만든 변경사항도 임의로 되돌리지 않으며, 필요한 경우 근거를 남기고 최소 범위로 수정한다.
- 변경 후 가능한 경우 테스트와 lint를 실행한다.
  - Go: `go test ./...`, `go vet ./...`, `gofmt -l .`
- 큰 수정 후에는 `git diff` 기준으로 변경 요약을 제공한다.

## 커밋 규약 (역할 추적용)
- Claude Code가 만든 커밋의 메시지 끝에는 `[claude]` 태그를 붙인다.
- Codex가 만든 커밋의 메시지 끝에는 `[codex]` 태그를 붙인다.
- 사용자가 직접 만든 커밋은 태그를 붙이지 않는다.
- 리뷰 대상 판별 시 `git log --grep` 또는 git diff로 자신이 만들지 않은 변경을 우선 검토한다.

## 검증 규칙
- Claude Code가 만든 변경사항은 Codex가 리뷰한다.
- Codex가 만든 변경사항은 Claude Code가 리뷰한다.
- 리뷰 트리거가 모호할 경우 미커밋 변경(`git status` / `git diff HEAD`)을 우선 대상으로 삼는다.
- 리뷰 결과는 심각도 높은 항목부터 짧고 명확하게 작성한다.
- 테스트를 실행하지 못한 경우 이유와 남은 위험을 명시한다.

## 프로젝트 컨텍스트 (script-agent)
- 언어/런타임: Go
- 역할: monitoring 데모의 Script Agent — Kafka로 명령 수신, 결과/감사/하트비트 발행
- 인접 모듈: `../hub` (Spring Boot/Java, Maven), `../infra` (docker-compose: Kafka 9092 / OTel 14318)
- 메시지 명세: `docs/monitoring-demo-message-spec-v0.2.1.md`
