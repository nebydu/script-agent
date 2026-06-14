// Package model은 메시지 spec v0.2.1의 Kafka 메시지 구조체와 enum을
// 정의한다. Agent가 직접 다루는 토픽(command-topic / result-topic-job /
// result-topic-log / audit-topic)만 포함하며, heartbeats-topic은 OTel
// Collector가 발행하므로 (spec §5.4) Agent model에서 다루지 않는다.
//
// 모든 도메인 timestamp는 RFC3339 (spec §2.5). time.Time을 그대로
// 사용하며, 발행 측에서 .UTC().Truncate(time.Second)로 정규화한다.
package model

// JobType은 command-topic / 결과 토픽 페이로드의 job_type 필드값이다.
// T4-2 result-topic 분리 후 결과 토픽 선택 기준이기도 하다
// (SCRIPT_JOB→result-topic-job, LOG_JOB→result-topic-log). spec §5.1, §5.2.
type JobType string

const (
	JobTypeScript JobType = "SCRIPT_JOB"
	JobTypeLog    JobType = "LOG_JOB"
)

// JobStatus는 결과 토픽 페이로드의 status 필드값이다. spec §5.2.3.
type JobStatus string

const (
	JobStatusSuccess JobStatus = "SUCCESS"
	JobStatusFail    JobStatus = "FAIL"
	JobStatusTimeout JobStatus = "TIMEOUT"
)

// AuditAction은 audit-topic 페이로드의 action 필드값이다.
// 데모 단계는 다음 세 가지로 한정 (spec §5.3).
type AuditAction string

const (
	AuditActionAgentStarted AuditAction = "AGENT_STARTED"
	AuditActionAgentStopped AuditAction = "AGENT_STOPPED"
	AuditActionJobExecuted  AuditAction = "JOB_EXECUTED"
)

// ActorType은 audit-topic actor.type 필드값이다.
// 데모 단계는 AGENT 고정 (spec §5.3.4).
type ActorType string

const (
	ActorTypeAgent ActorType = "AGENT"
)

// TargetType은 audit-topic target.type 필드값이다. spec §5.3.4.
type TargetType string

const (
	TargetTypeAgent   TargetType = "AGENT"
	TargetTypeScript  TargetType = "SCRIPT"
	TargetTypeLogFile TargetType = "LOG_FILE"
)

// EventResult는 audit-topic result 필드값이다. spec §5.3.4.
//
// JobStatus와 값은 같지만 의미가 다르므로 별도 타입으로 둔다
// (audit 이벤트의 결과 vs job 자체의 결과).
type EventResult string

const (
	EventResultSuccess EventResult = "SUCCESS"
	EventResultFail    EventResult = "FAIL"
	EventResultTimeout EventResult = "TIMEOUT"
)
