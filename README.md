# Script Agent

모니터링 솔루션 데모의 Script Agent. 호스트에서 스크립트 실행 / 로그
스캔 작업을 수행하고 결과를 Kafka로 보고하는 경량 Go 에이전트다.

> **위상.** 본 코드는 데모 단계의 walking skeleton이자 본개발 구조의
> 모태가 되는 reference implementation이다. 인증/영속 저장소/SQL_JOB
> 등은 데모 범위 밖.

## 요구 사항

- Go 1.21 이상
- 인프라(부모 워크스페이스 `infra/docker-compose.yml`): Kafka 1대, OTel Collector 1대

## 실행 절차

### 1) 인프라 기동

```sh
docker compose -f ../infra/docker-compose.yml up -d
```

- Kafka: `localhost:9092` (호스트), `kafka:29092` (compose network 내부)
- OTel Collector OTLP HTTP: `localhost:14318` (Windows의 Hyper-V 예약
  포트 회피 — 컨테이너 내부는 표준 4318 그대로)
- `commands` / `job-results` / `audit-events` / `heartbeats` 토픽은
  `KAFKA_AUTO_CREATE_TOPICS_ENABLE=true`로 자동 생성된다.

### 2) Agent 실행

```sh
go run ./cmd/agent
```

첫 실행 시 작업 디렉토리에 `.agent_id` 파일이 생성된다 — Agent의 영구
식별자(UUIDv4)이며 이후 실행에서도 동일한 값이 사용된다 (spec §3.1).

`Ctrl+C` (또는 SIGTERM)로 정상 종료. 종료 직전 `AGENT_STOPPED` 이벤트가
발행된다.

### 종료 코드 / supervisor 정책

| exit code | 의미 | supervisor 권장 동작 |
|---|---|---|
| `0` | 정상 signal 종료 | 재기동 안 함 |
| `1` | 부팅 실패 또는 `job-results` / `audit-events` publish 실패 (fail-fast) | **재기동 필수** — last committed offset부터 redeliver되어 at-least-once 보장 |

운영 배포 시 supervisor 설정 예:
- systemd: `Restart=on-failure` (exit 0이면 재기동 안 함, exit 1이면 재기동)
- Kubernetes: `restartPolicy: OnFailure` 또는 Deployment(`Always`도 동작)
- supervisord: `autorestart=unexpected` + `exitcodes=0`

supervisor 없이 데모로만 돌릴 때는 publish 실패 시 사용자가 직접 `go run`을 다시 실행해야 미처리 명령이 재배달된다.

## 동작 개요

| 흐름 | 토픽 | 발행/소비 |
|---|---|---|
| 시작 알림 + 등록 | `audit-events` (`AGENT_STARTED`) | 발행 |
| 명령 수신 | `commands` | consume (`script-agent-<agent_id>` 그룹) |
| Job 실행 결과 | `job-results` | 발행 |
| 감사 이벤트 | `audit-events` (`JOB_EXECUTED`) | 발행 |
| 종료 알림 | `audit-events` (`AGENT_STOPPED`) | 발행 |
| Liveness | `heartbeats` | OTel Collector가 OTLP→Kafka 재발행 |

세부 메시지 스키마는
[`monitoring-demo-message-spec-v0.2.1.md`](../monitoring-meta/docs/phase0-snapshot/monitoring-demo-message-spec-v0.2.1.md)
참조 (ground truth).

### Job 실행 정책 (사전 결정)

- **실행 모델**: 단일 consumer goroutine에서 명령을 순차 처리 (Nagios/
  Zabbix 표준 — agent worker 단위 serial). 동일 schedule 재진입은 구조적
  으로 불가능.
- **at-least-once 보장**: Dispatch가 결과/감사 발행을 완료한 뒤에만
  Kafka offset commit. publish 실패 시 즉시 exit 1로 종료 → supervisor
  재기동 → last committed offset부터 redeliver.
- **발행 순서**: `job-results` 먼저, 성공 시 `audit-events` (JOB_EXECUTED).
  results 실패 시 audit은 시도하지 않음 — "audit엔 JOB_EXECUTED 있는데
  결과 데이터 없음" 비대칭 차단. 반대 케이스(results 성공 후 audit 실패
  → 재기동 시 results 중복)는 가능하므로 BE는 `execution_id`로 dedup
  해야 한다.
- **만료된 명령**: `valid_until` 지난 명령은 silent skip (spec §5.1).
- **SCRIPT_JOB**: `timeout_seconds`로 강제 중단, `output_cap_bytes` 초과
  분은 truncate + `truncated=true`.
- **LOG_JOB 첫 실행**: 파일 끝부터 매칭 (tail -f 스타일). 이후 offset 추적.
- **로그 rotation**: `file_id`(POSIX inode / Windows file index) 변경 또는
  `size shrink` 감지 시 새 파일 처음부터 재시작.

## 환경 변수

| 이름 | 기본값 | 설명 |
|---|---|---|
| `AGENT_ID_PATH` | `./.agent_id` | 영구 식별자(agent_id)를 저장할 파일 경로 (spec §3.1) |
| `AGENT_VERSION` | `0.1.0` | audit / heartbeat 페이로드에 포함될 Agent 버전 |
| `LOG_LEVEL` | `info` | slog 출력 최소 레벨. `debug`/`info`/`warn`/`error` |
| `KAFKA_BROKERS` | `localhost:9092` | 콤마로 구분된 Kafka broker 주소 |
| `KAFKA_TOPIC_COMMANDS` | `commands` | BE→Agent 명령 토픽 |
| `KAFKA_TOPIC_JOB_RESULTS` | `job-results` | Agent→BE Job 결과 토픽 |
| `KAFKA_TOPIC_AUDIT_EVENTS` | `audit-events` | Agent→BE 감사 이벤트 토픽 |
| `LOG_STATE_DIR` | `./.agent_state` | LOG_JOB file_state JSON 저장 디렉토리 |
| `OTLP_ENDPOINT` | `http://localhost:4318` | OTel Collector OTLP HTTP 엔드포인트 (Windows에서 docker compose로 띄울 때는 `http://localhost:14318`) |
| `HEARTBEAT_INTERVAL_SECONDS` | `10` | `agent.heartbeat` 메트릭 송신 주기 (spec §5.4.1) |

## 빌드 / 테스트

```sh
go build ./...
go test ./...
```

Kafka 통합 테스트는 포함되어 있지 않다 — 모델/유틸 단위 테스트만 자동화.
브로커가 떠있는 상태에서의 종단 검증은 수동 (아래 참조).

## 수동 검증 절차

인프라가 떠있는 상태에서:

1. Agent 기동: `go run ./cmd/agent`. 로그에 `agent started` + agent_id 확인.
2. `audit-events` 토픽 consume:
   ```sh
   docker compose -f ../infra/docker-compose.yml exec kafka \
     kafka-console-consumer --bootstrap-server localhost:9092 \
     --topic audit-events --from-beginning
   ```
   → `AGENT_STARTED` 이벤트 확인.
3. `commands` 토픽에 SCRIPT_JOB 또는 LOG_JOB JSON 1건 produce
   (`target_agent_id`는 위에서 확인한 agent_id).
4. `job-results` 토픽 consume → 결과 페이로드 확인.
5. `audit-events` 다시 → `JOB_EXECUTED` 확인.
6. `heartbeats` 토픽 consume → 10초 주기로 OTLP heartbeat 메시지 확인
   (Kafka wire 인코딩은 OTel Collector 소관 — spec §5.4 / ADR-0002).
7. Ctrl+C로 종료 → `AGENT_STOPPED` 이벤트 확인.

## 디렉토리 구조

```
script-agent/
├── cmd/agent/             # 엔트리포인트
├── internal/
│   ├── audit/             # AGENT_STARTED/STOPPED/JOB_EXECUTED 발행
│   ├── config/            # 환경 변수 로드
│   ├── heartbeat/         # OTel agent.heartbeat (OTLP HTTP)
│   ├── identity/          # agent_id 생성 / 로드
│   ├── job/               # Dispatcher + SCRIPT_JOB / LOG_JOB executor
│   ├── jobresult/         # JobResult 발행
│   ├── kafka/             # kafka-go 래퍼 (Writer/Reader + envelope 헤더)
│   └── model/             # 메시지 스키마 (spec §5)
├── .agent_id              # 런타임 생성 (gitignore)
└── .agent_state/          # LOG_JOB file_state (gitignore)
```

## 메시지 명세

데모 단계의 Kafka 토픽 / 페이로드 ground truth는
[`monitoring-demo-message-spec-v0.2.1.md`](../monitoring-meta/docs/phase0-snapshot/monitoring-demo-message-spec-v0.2.1.md).
코드 변경은 이 문서와 정합되어야 한다.
