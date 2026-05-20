# Monitoring Demo — Message Spec (v0.2.1, 데모 단계)

## 0. 위상

이 문서는 데모(개발 v0) 단계의 Kafka 메시지 스키마와 토픽 규약을 정의한다.
Schema Registry 미도입 상태에서 이 문서가 ground truth이며, 변경 시 문서
버전 bump + 양쪽(Agent/BE) 코드 동시 갱신을 원칙으로 한다.

본개발 Phase 1 진입 시 Schema Registry 도입 여부, Heartbeat 마샬링
(`otlp_json` → protobuf), 인증, 영속 저장소, SQL_JOB 등이 추가 검토된다.
모든 데모/본개발 차이는 본 문서 7장 ADR 후보 리스트에 정리한다.

---

## 1. 토픽 목록

| 토픽 | 방향 | 발행자 | 소비자 | 내용 |
|---|---|---|---|---|
| `commands` | BE → Agent | BE Quartz | Script Agent | Job 실행 명령 |
| `job-results` | Agent → BE | Script Agent | BE Consumer | Job 실행 결과 |
| `audit-events` | Agent → BE | Script Agent | BE Consumer | 감사 이벤트 (Agent 시작/종료, Job 실행) |
| `heartbeats` | OTel Collector → BE | OTel Collector | BE Consumer | Agent heartbeat (OTLP JSON) |

토픽 명명 규칙: `kebab-case`, 환경 prefix 없음, 도메인 prefix 없음.

### 1.1 메시지 흐름

```
Agent 시작
  → audit-events: AGENT_STARTED   ─┐
  → heartbeats: agent.heartbeat   ─┤
                                   │
BE Quartz 트리거                   │
  → commands                       │  ────► BE Ring Buffer
        ▼                          │        + Agent 목록 (in-memory)
      Agent 실행                   │
        ▼                          │
  → job-results                    │
  → audit-events: JOB_EXECUTED    ─┘

Agent 종료
  → audit-events: AGENT_STOPPED
```

---

## 2. 공통 규약

### 2.1 직렬화

JSON (UTF-8). Heartbeat 토픽만 OTLP JSON (OTel Collector exporter 표준).

### 2.2 Kafka 메시지 헤더 (envelope)

도메인 데이터가 아닌 메타데이터는 Kafka 헤더로 분리한다. payload에는 도메인
데이터만 남는다.

| 헤더 | 타입 | 필수 | 설명 |
|---|---|---|---|
| `x-message-id` | UUID string | ● | 메시지 자체의 식별자. 중복 감지용 (데모는 발행만, 검사 없음) |
| `x-message-version` | string | ● | payload 스키마 버전. 데모는 `1` 고정 |
| `x-source` | string | ● | 발행자: `script-agent` \| `monitoring-be` \| `otel-collector` |
| `x-trace-id` | string | ○ | OTel trace propagation 대비. 데모는 발행만, 검사 없음 |

`heartbeats` 토픽은 OTel Collector가 발행하므로 위 헤더 규약이 적용되지
않는다 (OTLP 표준 헤더 그대로).

### 2.3 메시지 키

| 토픽 | 키 |
|---|---|
| `commands` | `target_agent_id` |
| `job-results` | `agent_id` |
| `audit-events` | `agent_id` |
| `heartbeats` | OTel Collector 기본 (Agent 단위 분배 불보장) |

키 정책: Agent 단위 ordering 보장. 본개발에서도 동일 유지.

### 2.4 ID 컨벤션

| ID | 발급 시점 | 발급자 | 비고 |
|---|---|---|---|
| `agent_id` | Agent 첫 실행 시 | Agent | UUIDv4. 로컬 파일에 저장 (3.1) |
| `job_id` | Job 정의 등록 시 | BE | UUIDv4. Job = "무엇을 실행할지" 정의 |
| `schedule_id` | Schedule 등록 시 | BE | UUIDv4. Schedule = "어떤 Job을 언제, 어떤 Agent에서" |
| `execution_id` | Schedule 트리거 시 | BE Quartz | UUIDv4. 1회 실행 식별. commands/result/audit 상관 키 |

세 ID의 관계는 4장에서 상세 설명.

### 2.5 Timestamp

모든 도메인 timestamp는 RFC3339 (예: `2026-05-19T14:00:00Z`). Heartbeat 영역만
OTLP 표준 (UnixNano).

각 timestamp의 의미는 토픽별로 명시 (5장).

---

## 3. Agent 등록 메커니즘

### 3.1 agent_id 생성

Agent 첫 실행 시 다음 위치를 확인:
- 데모: 작업 디렉토리의 `.agent_id`
- 본개발: `/var/lib/monitoring-agent/agent_id` (검토 예정)

파일이 없으면 UUIDv4 생성 후 저장. 있으면 그 값 사용. **agent_id는
Agent의 영구 식별자**이며, hostname/OS 변경과 무관하게 유지된다.

### 3.2 등록 흐름

데모 단계에선 별도 등록 endpoint를 두지 않고 `audit-events` 토픽의
`AGENT_STARTED` 이벤트가 등록 역할을 겸한다.

Agent 시작 시점에 발행되는 `AGENT_STARTED` 이벤트의 payload에 등록 정보
(hostname, os, agent_version, started_at)가 포함되며, BE는 이 이벤트를 받아
in-memory의 **Agent 목록 맵**(`Map<AgentId, AgentInfo>`)에 등록 또는
갱신한다. 동일 agent_id가 이미 있으면 last_seen만 갱신.

이후 heartbeat은 등록 정보를 다시 보내지 않는다. agent_id attribute만 박혀
있으면 BE가 매칭해 last_seen 갱신.

`AGENT_STOPPED` 이벤트가 들어오면 BE는 해당 agent의 상태를 OFFLINE으로
갱신 (목록에서 제거하지 않음).

본개발 진입 시 이 메커니즘은 별도 `/agents/register` endpoint + 사전
발급 토큰 + 관리자 승인 게이트로 진화 예정 (ADR).

---

## 4. Job 도메인 모델

### 4.1 세 객체의 분리

데모/본개발 모두 Job, Schedule, Execution을 세 객체로 분리한다.

**Job (정의).** "무엇을 어떻게 실행할지"의 정의. 시간/대상과 무관.
- SCRIPT_JOB: 스크립트 경로, 인자, timeout, 출력 cap
- LOG_JOB: 로그 파일 경로, 패턴, 인코딩

**Schedule.** "어떤 Job을 언제, 어떤 Agent에서" 실행할지의 결합.
- (job_id, target_agent_id, cron_expression, enabled)

**Execution.** Schedule이 한 번 트리거되어 실제로 실행된 사건.
- (execution_id, schedule_id, started_at, finished_at, status, output)

### 4.2 데모 UI에서의 단순화

데모 UI는 schedule 중심으로만 노출한다. "스케줄 등록" 폼 하나에서 job_type,
실행 대상(script_path 또는 log_path), 옵션, cron, target_agent_id를 모두
받고, BE는 내부적으로 Job + Schedule 두 객체를 생성한다. job_id는 화면에
노출되지 않음. 본개발에서 Job 등록 화면과 Schedule 등록 화면을 분리.

### 4.3 BE in-memory 상태

데모 BE는 다음 상태를 in-memory로 보유:

| 구조 | 형태 | 크기 |
|---|---|---|
| commands | Ring buffer | 최근 50개 |
| job-results | Ring buffer | 최근 100개 |
| audit-events | Ring buffer | 최근 200개 |
| heartbeats | Latest map `Map<AgentId, HeartbeatState>` | Agent당 최신 1개 |
| Agent 목록 | Map `Map<AgentId, AgentInfo>` | 등록된 Agent 수 |
| Job 정의 | Map `Map<JobId, JobDefinition>` | 등록된 Job 수 |
| Schedule 정의 | Map `Map<ScheduleId, ScheduleDefinition>` | 등록된 Schedule 수 |

Heartbeat은 ring buffer가 아닌 latest map 구조다 — Agent별로 "마지막
살아있음" 시각만 필요하기 때문. Ring buffer로 둘 경우 동일 Agent의 heartbeat이
ring을 채워 다른 Agent의 정보를 밀어내는 문제가 있음.

BE 재시작 시 모두 휘발. 본개발에서 PG/OpenSearch로 영속화.

동시성: 여러 Consumer 스레드가 동시에 ring buffer에 쓰고 Thymeleaf 요청이
동시에 읽으므로, `Collections.synchronizedList` 또는 Apache Commons의
`CircularFifoQueue` (synchronized wrap) 등 thread-safe 구조 필수.

---

## 5. 토픽별 페이로드 명세

### 5.1 `commands`

BE Quartz가 스케줄 트리거 시 발행. Agent별 unique consumer group으로
consume하며, payload의 `target_agent_id`가 자기 것이 아니면 무시.

**오프라인 Agent 처리 (표준 패턴).** 명령 발행 시점에 `valid_until`을 함께
포함한다. Agent가 명령을 consume했을 때 현재 시각이 `valid_until`을 지났으면
즉시 skip(silent). 다음 정상 트리거까지 대기. 이는 누적 실행을 회피하는
모니터링 솔루션의 표준 패턴.

BE Quartz의 misfire는 `MISFIRE_INSTRUCTION_DO_NOTHING`으로 설정 — BE 자체가
다운됐다가 복구된 경우의 누적도 동일 정책으로 처리.

#### 5.1.1 SCRIPT_JOB

```json
{
  "execution_id": "8f4b1c9e-...",
  "schedule_id": "3a7d2b5f-...",
  "job_id": "9c1e8a4d-...",
  "target_agent_id": "agent-001",
  "job_type": "SCRIPT_JOB",
  "issued_at": "2026-05-19T14:00:00Z",
  "valid_until": "2026-05-19T14:04:30Z",
  "spec": {
    "script_path": "/opt/scripts/check_disk.sh",
    "args": ["--threshold", "80"],
    "timeout_seconds": 30,
    "output_cap_bytes": 65536
  }
}
```

#### 5.1.2 LOG_JOB

```json
{
  "execution_id": "8f4b1c9e-...",
  "schedule_id": "3a7d2b5f-...",
  "job_id": "9c1e8a4d-...",
  "target_agent_id": "agent-001",
  "job_type": "LOG_JOB",
  "issued_at": "2026-05-19T14:00:00Z",
  "valid_until": "2026-05-19T14:04:30Z",
  "spec": {
    "log_path": "/var/log/app/error.log",
    "pattern": "ERROR|FATAL",
    "encoding": "UTF-8"
  }
}
```

#### 5.1.3 Field 정의

| 필드 | 타입 | 필수 | 설명 |
|---|---|---|---|
| `execution_id` | UUID | ● | 이 한 번 실행의 식별자. result/audit가 이걸로 상관 |
| `schedule_id` | UUID | ● | 트리거된 Schedule 식별자 |
| `job_id` | UUID | ● | Job 정의 식별자 |
| `target_agent_id` | string | ● | 매치 안 되면 Agent 무시 |
| `job_type` | enum | ● | `SCRIPT_JOB` \| `LOG_JOB` |
| `issued_at` | RFC3339 | ● | BE가 명령을 발행한 시각 |
| `valid_until` | RFC3339 | ● | 이 시각 이후 받은 Agent는 명령을 skip. BE Quartz가 다음 트리거 90% 지점으로 계산 |
| `spec` | object | ● | job_type별 구조 상이 |

**`valid_until` 계산 규칙.** 5분 주기면 `issued_at + 4분 30초`. 일반화하면
"다음 트리거 예정 시각의 90% 지점". 일회성 트리거는 본개발 영역.

### 5.2 `job-results`

Agent가 Job 실행 후 발행.

#### 5.2.1 SCRIPT_JOB 결과

```json
{
  "execution_id": "8f4b1c9e-...",
  "schedule_id": "3a7d2b5f-...",
  "job_id": "9c1e8a4d-...",
  "agent_id": "agent-001",
  "job_type": "SCRIPT_JOB",
  "status": "SUCCESS",
  "started_at": "2026-05-19T14:00:01Z",
  "finished_at": "2026-05-19T14:00:03Z",
  "script": {
    "exit_code": 0,
    "stdout_cap": "Disk usage: 42%",
    "stderr_cap": "",
    "truncated": false
  },
  "log": null
}
```

#### 5.2.2 LOG_JOB 결과

```json
{
  "execution_id": "8f4b1c9e-...",
  "schedule_id": "3a7d2b5f-...",
  "job_id": "9c1e8a4d-...",
  "agent_id": "agent-001",
  "job_type": "LOG_JOB",
  "status": "SUCCESS",
  "started_at": "2026-05-19T14:00:01Z",
  "finished_at": "2026-05-19T14:00:02Z",
  "script": null,
  "log": {
    "matched_lines_count": 3,
    "sample_lines": [
      "[2026-05-19 13:59:42] ERROR Failed to connect to DB",
      "[2026-05-19 13:59:55] ERROR Retry exceeded"
    ]
  }
}
```

#### 5.2.3 Field 정의

| 필드 | 타입 | 필수 | 설명 |
|---|---|---|---|
| `execution_id` | UUID | ● | command와 매치 |
| `agent_id` | string | ● | 발행 Agent |
| `job_type` | enum | ● | `SCRIPT_JOB` \| `LOG_JOB` |
| `status` | enum | ● | `SUCCESS` \| `FAIL` \| `TIMEOUT` |
| `started_at` | RFC3339 | ● | Agent가 작업을 시작한 시각 |
| `finished_at` | RFC3339 | ● | Agent가 작업을 종료한 시각 |
| `script` | object \| null | ○ | SCRIPT_JOB일 때 채움 |
| `log` | object \| null | ○ | LOG_JOB일 때 채움 |

**Timestamp 주의.** `started_at`, `finished_at`은 **Agent의 작업 시간**이다.
LOG_JOB의 경우 로그 라인 자체의 발생 시각과는 별개이며, 데모 단계에서는 로그
발생 시각을 추출하지 않는다 (sample_lines의 원문에 포함되어 있을 수 있지만
별도 필드로 파싱하지 않음). 본개발에서 추가 예정 (ADR).

**LOG_JOB의 file_state**(offset, inode, size)는 **Agent local 상태**이며
BE에 전송하지 않는다. Agent 내부의 로컬 JSON 파일에 보관.

본개발에서 envelope + job_type별 분기 메시지 구조로 전환 검토 (ADR).

### 5.3 `audit-events`

Agent가 감사 이벤트를 발행. 데모 단계 audit 액션은 다음 세 가지로 한정.

| action | 발생 시점 | result 값 |
|---|---|---|
| `AGENT_STARTED` | Agent 프로세스 시작 직후 | `SUCCESS` |
| `AGENT_STOPPED` | Agent 정상 종료 직전 | `SUCCESS` |
| `JOB_EXECUTED` | Job 실행 종료 시 | `SUCCESS` \| `FAIL` \| `TIMEOUT` |

**`JOB_EXECUTED`의 `occurred_at`은 Job 실행 종료 시각**(= `job-results`의
`finished_at`)으로 한다.

#### 5.3.1 AGENT_STARTED

```json
{
  "event_id": "uuid",
  "actor": {
    "type": "AGENT",
    "id": "agent-001"
  },
  "action": "AGENT_STARTED",
  "target": {
    "type": "AGENT",
    "id": "agent-001"
  },
  "result": "SUCCESS",
  "occurred_at": "2026-05-19T13:55:00Z",
  "metadata": {
    "hostname": "demo-host-01",
    "os": "linux/amd64",
    "agent_version": "0.1.0",
    "started_at": "2026-05-19T13:55:00Z"
  }
}
```

BE는 이 이벤트를 받아 Agent 목록 맵에 등록 (3.2 참조).

#### 5.3.2 AGENT_STOPPED

```json
{
  "event_id": "uuid",
  "actor": {
    "type": "AGENT",
    "id": "agent-001"
  },
  "action": "AGENT_STOPPED",
  "target": {
    "type": "AGENT",
    "id": "agent-001"
  },
  "result": "SUCCESS",
  "occurred_at": "2026-05-19T18:30:00Z",
  "metadata": {
    "reason": "SIGTERM"
  }
}
```

#### 5.3.3 JOB_EXECUTED

```json
{
  "event_id": "uuid",
  "actor": {
    "type": "AGENT",
    "id": "agent-001"
  },
  "action": "JOB_EXECUTED",
  "target": {
    "type": "SCRIPT",
    "id": "/opt/scripts/check_disk.sh"
  },
  "result": "SUCCESS",
  "occurred_at": "2026-05-19T14:00:03Z",
  "metadata": {
    "execution_id": "8f4b1c9e-...",
    "schedule_id": "3a7d2b5f-...",
    "job_id": "9c1e8a4d-...",
    "job_type": "SCRIPT_JOB",
    "exit_code": 0
  }
}
```

`target.type`은 SCRIPT_JOB이면 `SCRIPT`, LOG_JOB이면 `LOG_FILE`. `target.id`는
실행 대상의 경로.

#### 5.3.4 Field 정의

| 필드 | 타입 | 필수 | 설명 |
|---|---|---|---|
| `event_id` | UUID | ● | 감사 이벤트 식별자 |
| `actor.type` | enum | ● | 데모는 `AGENT` 고정. 본개발에서 `USER`, `SYSTEM` 추가 |
| `actor.id` | string | ● | agent_id |
| `action` | enum | ● | `AGENT_STARTED` \| `AGENT_STOPPED` \| `JOB_EXECUTED` |
| `target.type` | enum | ● | `AGENT` \| `SCRIPT` \| `LOG_FILE` |
| `target.id` | string | ● | 대상 식별자 (agent_id 또는 경로) |
| `result` | enum | ● | `SUCCESS` \| `FAIL` \| `TIMEOUT` |
| `occurred_at` | RFC3339 | ● | 사건 발생 시각 (JOB_EXECUTED는 종료 시각) |
| `metadata` | object | ○ | action별 자유 형식 |

### 5.4 `heartbeats`

OTel Collector가 `otlp_json` exporter로 발행. payload 구조는 OTLP JSON 표준을
따르며 본 문서가 정의하지 않는다.

#### 5.4.1 Agent 측 송신 규약

| 항목 | 값 |
|---|---|
| metric name | `agent.heartbeat` |
| metric type | Gauge (value: 1) |
| 송신 주기 | 10초 (데모 기본) |
| attribute `agent_id` | Agent UUID |

본개발에서 `agent_version` 등 추가 attribute, 송신 주기 조정.

#### 5.4.2 BE 측 추출 규약

BE는 `resourceMetrics[].scopeMetrics[].metrics[]` 경로에서 metric name이
`agent.heartbeat`인 항목을 찾고, dataPoints의 attribute에서 `agent_id`를
추출, dataPoint의 `timeUnixNano`를 해당 Agent의 last_seen으로 갱신.

OTLP 표준 구조 참조: https://opentelemetry.io/docs/specs/otlp/

본개발에서 마샬링 `otlp_json` → protobuf 전환 예정 (ADR).

---

## 6. 누락 영역 (데모 범위 외)

다음은 데모 범위에 들어가지 않으며, Phase 1 본개발 또는 Phase 2에서 추가된다.

| 영역 | 단계 | 비고 |
|---|---|---|
| 인증/인가 (JWT, Knox 어댑터) | Phase 1 본개발 | Spring Security 도입 |
| 영속 저장소 (PG, OpenSearch) | Phase 1 본개발 | 기술 스택 선정 + 구축 |
| SQL_JOB | Phase 1 본개발 | Job 유형 추가 |
| LOG_JOB 로그 발생 시각 추출 | Phase 1 본개발 | sample_lines에 occurred_at 필드 추가 |
| Alert / Incident 도메인 | Phase 1 본개발 | v0.6 5.4 참조 |
| 시계열 메트릭 (OS metric) | Phase 2 | Infra Agent + VictoriaMetrics |
| 화면 LEGO + WebSocket 전환 | Phase 1 본개발 | 데모는 Thymeleaf 한 페이지 |

---

## 7. ADR 후보 리스트

데모 단계에서 의식적으로 단순화/지연한 결정들. 본개발 진입 시 각각 별도
ADR 문서로 격상.

| # | 주제 | 데모 결정 | 본개발 전환 |
|---|---|---|---|
| 1 | 메시지 스키마 관리 | 마크다운 + 수동 작성 | v0.6 8장 Schema Registry 결정 따름 |
| 2 | Heartbeat 마샬링 | `otlp_json` | protobuf |
| 3 | Audit 채널 | Kafka 직행 (OTel 미경유) | 동일 유지 |
| 4 | Consumer group | Agent별 unique group.id | 동일 유지 (Phase 1 규모 검토) |
| 5 | 토픽 명명 | 환경 prefix 없이 단순 | 환경/리전 prefix 검토 |
| 6 | 메시지 키 | `agent_id` | 동일 유지 |
| 7 | 인증/인가 | 없음 | JWT + Knox 어댑터 도입 |
| 8 | 시각화 | Thymeleaf 한 페이지 | LEGO + WebSocket |
| 9 | SQL_JOB | 데모 범위 외 | Phase 1에 포함 |
| 10 | LOG_JOB 로그 발생 시각 | 추출하지 않음 | sample_lines에 occurred_at 추가 |
| 11 | Agent 자가 등록 | audit `AGENT_STARTED`가 등록 겸함 | 별도 register endpoint + 사전 토큰 |
| 12 | 영속 저장소 | 없음 (in-memory ring buffer + latest map) | PG + OpenSearch |
| 13 | OTel Collector 라우팅 | heartbeat 단일 경로 | metric/heartbeat 분리 (Phase 2) |
| 14 | LOG_JOB file_state | Agent local만 | 검토 (BE 보고 필요 여부) |
| 15 | `x-message-id` 중복 검사 | 발행만, 검사 없음 | 검사 도입 (at-least-once 대비) |
| 16 | 명령 만료 정책 | `valid_until` payload 필드 + 다음 트리거 90% 지점 | 정책 유지. 만료 시 audit 발행 추가 |
| 17 | Quartz misfire | `MISFIRE_INSTRUCTION_DO_NOTHING` | 동일 |
| 18 | 오프라인 Agent 발행 게이팅 | 없음 (발행 후 `valid_until`로 자체 정리) | heartbeat 기반 발행 게이팅 + Alert 도입 검토 |

---

## 8. 변경 이력

| 버전 | 일자 | 변경 |
|---|---|---|
| v0.1 | 2026-05-19 | 초안 |
| v0.2 | 2026-05-19 | 헤더 분리, Agent 자가 등록, audit 액션 3개 명확화, schedule/job/execution 분리 설명, timestamp 의미 정리, JWT 데모 제거 |
| v0.2.1 | 2026-05-19 | `valid_until` 필드 + Quartz misfire 정책 추가 (오프라인 Agent 표준 처리), heartbeat을 ring buffer가 아닌 latest map으로 정정, BE in-memory 동시성 메모 추가, ADR 16~18 추가 |
