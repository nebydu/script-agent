package config

import (
	"os"
	"testing"
	"time"
)

// unsetEnv는 테스트 종료 시 지정 env 키를 원래 상태로 복원한다.
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	prev, hadPrev := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("os.Unsetenv(%q) 실패: %v", key, err)
	}
	t.Cleanup(func() {
		if hadPrev {
			os.Setenv(key, prev)
		} else {
			os.Unsetenv(key)
		}
	})
}

// setEnv는 테스트 내에서 env 키를 임시로 설정하고, 종료 시 원래 상태로 복원한다.
func setEnv(t *testing.T, key, val string) {
	t.Helper()
	prev, hadPrev := os.LookupEnv(key)
	if err := os.Setenv(key, val); err != nil {
		t.Fatalf("os.Setenv(%q, %q) 실패: %v", key, val, err)
	}
	t.Cleanup(func() {
		if hadPrev {
			os.Setenv(key, prev)
		} else {
			os.Unsetenv(key)
		}
	})
}

// TestLoad_DefaultTopicNames_Phase1: env 미설정 시 토픽 default가 로드되는지
// 검증한다. T4-2 result-topic 분리로 결과 토픽은 result-topic-job /
// result-topic-log 2개로 갈라진다. 이 테스트는 토픽 default의 회귀 앵커다.
func TestLoad_DefaultTopicNames_Phase1(t *testing.T) {
	// 테스트 환경에서 관련 env가 주입돼 있을 수 있으므로 명시적으로 해제한다.
	unsetEnv(t, "KAFKA_TOPIC_COMMANDS")
	unsetEnv(t, "KAFKA_TOPIC_RESULT_JOB")
	unsetEnv(t, "KAFKA_TOPIC_RESULT_LOG")
	unsetEnv(t, "KAFKA_TOPIC_AUDIT_EVENTS")

	cfg := Load()

	// phase1-040 재명명: commands → command-topic (구독 토픽)
	if got, want := cfg.KafkaTopicCommands, "command-topic"; got != want {
		t.Errorf("KafkaTopicCommands default = %q, want %q (phase1-040 재명명 미반영)", got, want)
	}

	// T4-2 result-topic 분리: SCRIPT_JOB → result-topic-job
	if got, want := cfg.KafkaTopicResultJob, "result-topic-job"; got != want {
		t.Errorf("KafkaTopicResultJob default = %q, want %q (T4-2 분리 미반영)", got, want)
	}

	// T4-2 result-topic 분리: LOG_JOB → result-topic-log
	if got, want := cfg.KafkaTopicResultLog, "result-topic-log"; got != want {
		t.Errorf("KafkaTopicResultLog default = %q, want %q (T4-2 분리 미반영)", got, want)
	}

	// phase1-040 재명명: audit-events → audit-topic (발행 토픽)
	if got, want := cfg.KafkaTopicAuditEvents, "audit-topic"; got != want {
		t.Errorf("KafkaTopicAuditEvents default = %q, want %q (phase1-040 재명명 미반영)", got, want)
	}
}

// TestLoad_EnvKeyNamesPreserved: env 키 override가 정상 동작하는지 확인한다.
// T4-2로 결과 토픽 키는 KAFKA_TOPIC_RESULT_JOB / KAFKA_TOPIC_RESULT_LOG 2키로
// 교체됐다(구 단일 결과 토픽 키는 폐기).
func TestLoad_EnvKeyNamesPreserved(t *testing.T) {
	setEnv(t, "KAFKA_TOPIC_COMMANDS", "my-custom-command")
	setEnv(t, "KAFKA_TOPIC_RESULT_JOB", "my-custom-result-job")
	setEnv(t, "KAFKA_TOPIC_RESULT_LOG", "my-custom-result-log")
	setEnv(t, "KAFKA_TOPIC_AUDIT_EVENTS", "my-custom-audit")

	cfg := Load()

	if got, want := cfg.KafkaTopicCommands, "my-custom-command"; got != want {
		t.Errorf("KAFKA_TOPIC_COMMANDS override 무효: got %q, want %q", got, want)
	}
	if got, want := cfg.KafkaTopicResultJob, "my-custom-result-job"; got != want {
		t.Errorf("KAFKA_TOPIC_RESULT_JOB override 무효: got %q, want %q", got, want)
	}
	if got, want := cfg.KafkaTopicResultLog, "my-custom-result-log"; got != want {
		t.Errorf("KAFKA_TOPIC_RESULT_LOG override 무효: got %q, want %q", got, want)
	}
	if got, want := cfg.KafkaTopicAuditEvents, "my-custom-audit"; got != want {
		t.Errorf("KAFKA_TOPIC_AUDIT_EVENTS override 무효: got %q, want %q", got, want)
	}
}

// TestLoad_OtherDefaultsUnchanged: 토픽 재명명 외 기타 default 값이
// phase1-040 작업에 의해 변경되지 않았음을 확인한다(동결 spec 회귀 앵커).
func TestLoad_OtherDefaultsUnchanged(t *testing.T) {
	// 관련 env 전부 해제하여 순수 default 경로를 검증한다.
	keysToUnset := []string{
		"AGENT_ID_PATH", "AGENT_VERSION", "LOG_LEVEL",
		"KAFKA_BROKERS", "LOG_STATE_DIR", "OTLP_ENDPOINT",
		"HEARTBEAT_INTERVAL_SECONDS",
	}
	for _, k := range keysToUnset {
		unsetEnv(t, k)
	}

	cfg := Load()

	checks := []struct {
		name string
		got  string
		want string
	}{
		{"AgentIDPath", cfg.AgentIDPath, "./.agent_id"},
		{"AgentVersion", cfg.AgentVersion, "0.1.0"},
		{"LogLevel", cfg.LogLevel, "info"},
		{"KafkaBrokers", cfg.KafkaBrokers, "localhost:9092"},
		{"LogStateDir", cfg.LogStateDir, "./.agent_state"},
		{"OTLPEndpoint", cfg.OTLPEndpoint, "http://localhost:4318"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s default = %q, want %q", c.name, c.got, c.want)
		}
	}

	// HeartbeatInterval default = 10s
	if got, want := cfg.HeartbeatInterval, 10*time.Second; got != want {
		t.Errorf("HeartbeatInterval default = %v, want %v", got, want)
	}
}

// TestLoad_HeartbeatIntervalOverride: HEARTBEAT_INTERVAL_SECONDS env override
// 경로가 정상 동작하는지 확인한다 (getenvDurationSeconds 간접 검증).
func TestLoad_HeartbeatIntervalOverride(t *testing.T) {
	setEnv(t, "HEARTBEAT_INTERVAL_SECONDS", "30")

	cfg := Load()

	if got, want := cfg.HeartbeatInterval, 30*time.Second; got != want {
		t.Errorf("HeartbeatInterval override = %v, want %v", got, want)
	}
}

// TestLoad_HeartbeatIntervalInvalidFallback: HEARTBEAT_INTERVAL_SECONDS가
// 비정상 값이면 default(10s)로 fallback되는지 확인한다.
func TestLoad_HeartbeatIntervalInvalidFallback(t *testing.T) {
	cases := []struct {
		name string
		val  string
	}{
		{"빈 문자열", ""},
		{"비숫자", "abc"},
		{"0", "0"},
		{"음수", "-5"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			unsetEnv(t, "HEARTBEAT_INTERVAL_SECONDS")
			if tc.val != "" {
				setEnv(t, "HEARTBEAT_INTERVAL_SECONDS", tc.val)
			}

			cfg := Load()

			if got, want := cfg.HeartbeatInterval, 10*time.Second; got != want {
				t.Errorf("val=%q: HeartbeatInterval = %v, want %v (fallback 실패)", tc.val, got, want)
			}
		})
	}
}
