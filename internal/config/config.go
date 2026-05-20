// Package config는 Agent 실행 설정을 환경변수에서 로드한다.
//
// 데모 단계에서는 외부 라이브러리 없이 표준 os.Getenv만 사용한다.
// Kafka / OTel 관련 설정은 후속 단계에서 추가된다.
package config

import (
	"os"
	"strconv"
	"time"
)

// Config는 Agent 프로세스의 실행 설정을 담는다.
type Config struct {
	// AgentIDPath는 영구 식별자 파일(.agent_id)의 경로다. spec §3.1.
	AgentIDPath string

	// AgentVersion은 audit/heartbeat 등에 포함되는 Agent 버전 문자열이다.
	AgentVersion string

	// LogLevel은 slog 핸들러의 최소 출력 레벨이다.
	// 허용 값: "debug" | "info" | "warn" | "error".
	LogLevel string

	// KafkaBrokers는 콤마로 구분된 broker 주소 목록이다 (예: "host1:9092,host2:9092").
	KafkaBrokers string

	// KafkaTopicCommands는 BE→Agent 명령 토픽 (spec §1).
	KafkaTopicCommands string

	// KafkaTopicJobResults는 Agent→BE Job 결과 토픽 (spec §1).
	KafkaTopicJobResults string

	// KafkaTopicAuditEvents는 Agent→BE 감사 이벤트 토픽 (spec §1).
	KafkaTopicAuditEvents string

	// LogStateDir은 LOG_JOB이 file_state(offset/size/file_id) JSON을
	// 저장할 디렉토리다. spec §5.2.3 노트 — Agent local only.
	LogStateDir string

	// OTLPEndpoint는 OTel Collector OTLP HTTP receiver 주소.
	// 예: "http://localhost:4318". heartbeat 메트릭 전송 대상.
	OTLPEndpoint string

	// HeartbeatInterval은 agent.heartbeat 메트릭 송신 주기. spec §5.4.1 기본 10s.
	HeartbeatInterval time.Duration
}

// Load는 환경변수에서 Config를 읽는다. 미설정 항목은 기본값을 사용한다.
func Load() Config {
	return Config{
		AgentIDPath:           getenv("AGENT_ID_PATH", "./.agent_id"),
		AgentVersion:          getenv("AGENT_VERSION", "0.1.0"),
		LogLevel:              getenv("LOG_LEVEL", "info"),
		KafkaBrokers:          getenv("KAFKA_BROKERS", "localhost:9092"),
		KafkaTopicCommands:    getenv("KAFKA_TOPIC_COMMANDS", "commands"),
		KafkaTopicJobResults:  getenv("KAFKA_TOPIC_JOB_RESULTS", "job-results"),
		KafkaTopicAuditEvents: getenv("KAFKA_TOPIC_AUDIT_EVENTS", "audit-events"),
		LogStateDir:           getenv("LOG_STATE_DIR", "./.agent_state"),
		OTLPEndpoint:          getenv("OTLP_ENDPOINT", "http://localhost:4318"),
		HeartbeatInterval:     getenvDurationSeconds("HEARTBEAT_INTERVAL_SECONDS", 10*time.Second),
	}
}

// getenvDurationSeconds는 정수 초로 표현된 env를 time.Duration으로 파싱한다.
// 미설정/파싱 실패 시 fallback.
func getenvDurationSeconds(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return time.Duration(n) * time.Second
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
