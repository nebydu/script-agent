// Command agent는 모니터링 솔루션 데모의 Script Agent 엔트리포인트다.
//
// 라이프사이클:
//  1. config 로드, slog 초기화
//  2. agent_id 영속 (spec §3.1)
//  3. Kafka writer / commands reader 초기화
//  4. Dispatcher + Runners 구성 (Runners는 단계 D/E에서 등록)
//  5. AGENT_STARTED 발행 (best-effort)
//  6. commands consumer goroutine 시작
//  7. signal 대기 (SIGINT/SIGTERM)
//  8. consumer cancel + dispatcher drain (5s budget)
//  9. AGENT_STOPPED 발행 (5s budget) + writer close
package main

import (
	"context"
	"encoding/json"
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

func main() {
	cfg := config.Load()

	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	agentID, err := identity.GetOrCreate(cfg.AgentIDPath)
	if err != nil {
		logger.Error("failed to resolve agent_id", "error", err, "path", cfg.AgentIDPath)
		os.Exit(1)
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

	// commands consumer.
	consumerDone := make(chan struct{})
	go func() {
		defer close(consumerDone)
		consumeCommands(consumerCtx, reader, dispatcher, logger)
	}()

	// signal 대기. metadata.reason에 signal 이름을 그대로 넣기 위해
	// 명시적 채널 유지.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	sig := <-sigCh
	signal.Stop(sigCh)

	logger.Info("shutdown signal received", "signal", sig.String())

	// consumer 정리: 새 명령 fetch 중단, 진행 중 Job goroutine은 ctx 통해
	// 종료 시도 (예: exec.CommandContext가 자식 프로세스 kill).
	cancelConsumer()
	<-consumerDone

	// inflight Job drain (best-effort, 5s).
	drainDone := make(chan struct{})
	go func() {
		dispatcher.Wait()
		close(drainDone)
	}()
	select {
	case <-drainDone:
	case <-time.After(5 * time.Second):
		logger.Warn("inflight jobs did not drain in 5s — proceeding to shutdown")
	}

	// AGENT_STOPPED + 5s 발행 timeout.
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	if err := auditor.AgentStopped(shutdownCtx, sig.String()); err != nil {
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
		"reason", sig.String(),
	)
}

// consumeCommands는 commands 토픽을 끝없이 fetch + dispatch한다.
// ctx cancel 시 정상 반환. bad message(json unmarshal 실패)는 commit해
// 무한 재시도를 피한다.
func consumeCommands(ctx context.Context, reader *kafka.Reader, dispatcher *job.Dispatcher, logger *slog.Logger) {
	r := reader.Underlying()
	for {
		msg, err := r.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.Warn("failed to fetch command", "error", err)
			continue
		}

		var cmd model.Command
		if err := json.Unmarshal(msg.Value, &cmd); err != nil {
			logger.Warn("failed to unmarshal command — skipped",
				"error", err,
				"offset", msg.Offset,
			)
			if commitErr := r.CommitMessages(ctx, msg); commitErr != nil && ctx.Err() == nil {
				logger.Warn("failed to commit bad message", "error", commitErr)
			}
			continue
		}

		dispatcher.Dispatch(ctx, cmd)

		if err := r.CommitMessages(ctx, msg); err != nil && ctx.Err() == nil {
			logger.Warn("failed to commit offset", "error", err)
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
