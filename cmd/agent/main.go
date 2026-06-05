// Command agent는 모니터링 솔루션 데모의 Script Agent 엔트리포인트다.
//
// 라이프사이클:
//  1. config 로드, slog 초기화
//  2. agent_id 영속 (spec §3.1)
//  3. Kafka writer / commands reader 초기화
//  4. Dispatcher + Runners 구성 (Runners는 단계 D/E에서 등록)
//  5. AGENT_STARTED 발행 (best-effort)
//  6. commands consumer goroutine 시작
//  7. signal 또는 consumer 자기 종료(publish 실패 등) 대기
//  8. consumer cancel — 진행 중인 Dispatch도 ctx cancel로 조기 종료
//  9. AGENT_STOPPED 발행 (5s budget) + writer close
//
// fail-fast + exit code 정책: publish 실패 시 consumer가 error 반환 →
// run()이 exit code 1로 종료 → supervisor(systemd/k8s 등)가 재기동.
// exit code 0(정상 signal)에서는 supervisor가 재기동하지 않으므로 두
// 경로를 반드시 구분해야 한다. main은 os.Exit(run())만 호출하고 정리는
// run() 안의 defer에 맡긴다 — os.Exit는 defer를 실행하지 않기 때문.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"monitoring/script-agent/internal/audit"
	"monitoring/script-agent/internal/config"
	"monitoring/script-agent/internal/heartbeat"
	"monitoring/script-agent/internal/identity"
	"monitoring/script-agent/internal/job"
	"monitoring/script-agent/internal/jobresult"
	"monitoring/script-agent/internal/kafka"
	"monitoring/script-agent/internal/model"
)

// 종료 코드 (supervisor 재기동 정책과 직결):
//   exitOK     = 0 — 정상 signal 종료
//   exitFatal  = 1 — 부팅 실패 (agent_id 등) 또는 consumer self-terminate.
//                    supervisor가 재기동 → last committed offset부터 redeliver.
const (
	exitOK    = 0
	exitFatal = 1
)

func main() {
	os.Exit(run())
}

func run() int {
	cfg := config.Load()

	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	agentID, err := identity.GetOrCreate(cfg.AgentIDPath)
	if err != nil {
		logger.Error("failed to resolve agent_id", "error", err, "path", cfg.AgentIDPath)
		return exitFatal
	}

	// Kafka writer. Async=false + RequireAll로 매 WriteMessage가 broker
	// ack까지 동기 대기 — ctx 만료가 그대로 발행 timeout이 된다.
	writer := kafka.NewWriter(cfg.KafkaBrokers)
	defer func() {
		if err := writer.Close(); err != nil {
			logger.Warn("failed to close kafka writer", "error", err)
		}
	}()

	auditor := audit.NewPublisher(writer, cfg.KafkaTopicAuditEvents, agentID, cfg.AgentVersion)
	results := jobresult.NewPublisher(writer, cfg.KafkaTopicJobResults)

	runners := map[model.JobType]job.Runner{
		model.JobTypeScript: job.NewScriptRunner(),
		model.JobTypeLog:    job.NewLogRunner(cfg.LogStateDir),
	}
	dispatcher := job.NewDispatcher(agentID, runners, results, auditor, logger)

	consumerCtx, cancelConsumer := context.WithCancel(context.Background())
	defer cancelConsumer()

	reader := kafka.NewReader(cfg.KafkaBrokers, cfg.KafkaTopicCommands, agentID)
	defer func() {
		if err := reader.Close(); err != nil {
			logger.Warn("failed to close kafka reader", "error", err)
		}
	}()

	// Heartbeat (OTel) — best-effort. Collector 부재 시 OTLP HTTP exporter
	// 자체는 만들어지지만 push 시점에 실패할 뿐 main 흐름은 막지 않는다.
	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 5*time.Second)
	hbProvider, hbErr := heartbeat.Start(startupCtx, cfg.OTLPEndpoint, agentID, cfg.HeartbeatInterval)
	if hbErr != nil {
		logger.Warn("failed to start heartbeat — proceeding without it", "error", hbErr)
	}

	// AGENT_STARTED — best-effort (사전 결정: warn + 계속).
	if err := auditor.AgentStarted(startupCtx); err != nil {
		logger.Warn("failed to publish AGENT_STARTED", "error", err)
	}
	cancelStartup()

	logger.Info("agent started",
		"agent_id", agentID,
		"agent_version", cfg.AgentVersion,
	)

	// commands consumer. consumer가 publish 실패 등으로 자기 종료할 수
	// 있으므로 error도 보관 — main이 signal 또는 consumer 종료 중 먼저
	// 발생한 쪽을 reason으로 삼는다.
	consumerDone := make(chan struct{})
	var consumerErr error
	go func() {
		defer close(consumerDone)
		consumerErr = consumeCommands(consumerCtx, reader, dispatcher, logger)
	}()

	// signal 대기. metadata.reason에 signal 이름을 그대로 넣기 위해
	// 명시적 채널 유지.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	var (
		stopReason string
		exitCode   int
	)
	select {
	case sig := <-sigCh:
		stopReason = sig.String()
		exitCode = exitOK
		logger.Info("shutdown signal received", "signal", stopReason)
		// 새 명령 fetch 중단, 진행 중인 Dispatch도 ctx cancel로 빠르게
		// 반환. consumerDone 닫힘이 곧 inflight drain.
		cancelConsumer()
		<-consumerDone
	case <-consumerDone:
		// consumer가 자기 종료 (publish 실패 등). last committed offset부터
		// 재기동 시 redeliver되도록 fail-fast + exit 1.
		if consumerErr != nil {
			stopReason = "consumer-error"
			exitCode = exitFatal
			logger.Error("consumer terminated — initiating shutdown to preserve at-least-once",
				"error", consumerErr,
			)
		} else {
			// 정상 종료 경로가 신호 없이 일어나는 경우는 없지만 방어적 처리.
			// exit 0 — supervisor 재기동 불필요 (이 경로 자체가 비정상이지만
			// 데이터 손실은 없으므로).
			stopReason = "consumer-exit"
			exitCode = exitOK
			logger.Warn("consumer exited without error or signal")
		}
		cancelConsumer() // 이미 종료됐지만 ctx 정리.
	}
	signal.Stop(sigCh)

	// AGENT_STOPPED + 5s 발행 timeout.
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	if err := auditor.AgentStopped(shutdownCtx, stopReason); err != nil {
		logger.Warn("failed to publish AGENT_STOPPED", "error", err)
	}

	// Heartbeat provider 정리 — 마지막 metric push 시도.
	if hbProvider != nil {
		if err := hbProvider.Shutdown(shutdownCtx); err != nil {
			logger.Warn("failed to shutdown heartbeat provider", "error", err)
		}
	}

	logger.Info("agent stopped",
		"agent_id", agentID,
		"reason", stopReason,
		"exit_code", exitCode,
	)
	return exitCode
}

// consumeCommands는 commands 토픽을 끝없이 fetch + dispatch한다.
//
// 반환값:
//   - nil + ctx cancelled: 정상 shutdown 경로
//   - non-nil error: fail-fast 경로. publish 실패나 commit 실패는 더 이상
//     처리해선 안 된다 (out-of-order commit 시 손실). 호출 측이 Agent를
//     종료해 재기동 시 last committed offset부터 redeliver되게 한다.
//
// bad message(json unmarshal 실패)는 commit해 poison-pill 회피.
func consumeCommands(ctx context.Context, reader *kafka.Reader, dispatcher *job.Dispatcher, logger *slog.Logger) error {
	r := reader.Underlying()
	for {
		msg, err := r.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			// fetch transient 오류는 재시도 (kafka-go 내부 reconnect).
			logger.Warn("failed to fetch command", "error", err)
			continue
		}

		// envelope §2.3 — x-source는 폐쇄 enum이 아니며 미지값에도 dispatch가
		// 깨지지 않음을 의도된 가드로 명시한다. 여기서는 값을 관찰(debug 로깅)만
		// 하고, 알려진 값 여부를 판정하거나 처리·commit 흐름을 분기시키지 않는다.
		// x-source가 미지값이거나 부재여도 명령 처리는 그대로 진행된다.
		if source, present := kafka.SourceFromHeaders(msg.Headers); present {
			logger.Debug("command envelope x-source observed",
				"x_source", source,
				"offset", msg.Offset,
			)
		}

		var cmd model.Command
		if err := json.Unmarshal(msg.Value, &cmd); err != nil {
			logger.Warn("failed to unmarshal command — skipped",
				"error", err,
				"offset", msg.Offset,
			)
			if commitErr := r.CommitMessages(ctx, msg); commitErr != nil {
				if ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("commit bad message at offset %d: %w", msg.Offset, commitErr)
			}
			continue
		}

		// Dispatch는 동기 — Runner 실행 + 결과/감사 발행 시도까지 완료 후
		// 반환한다. error는 publish 실패 등 commit 금지 신호.
		if err := dispatcher.Dispatch(ctx, cmd); err != nil {
			if ctx.Err() != nil {
				// shutdown 진행 중이면 정상 종료 경로. commit 건너뛰고 반환 —
				// 재기동 시 같은 명령 redeliver.
				return nil
			}
			// publish 실패. commit하면 손실 → fail-fast로 종료.
			return fmt.Errorf("dispatch failed at offset %d (execution_id=%s): %w",
				msg.Offset, cmd.ExecutionID, err)
		}

		if ctx.Err() != nil {
			return nil
		}

		if err := r.CommitMessages(ctx, msg); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("commit offset %d: %w", msg.Offset, err)
		}
	}
}

// newLogger는 cfg.LogLevel을 slog 레벨로 파싱해 JSON 핸들러를 만든다.
// 알 수 없는 값은 info로 fallback한다.
func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	return slog.New(h)
}
