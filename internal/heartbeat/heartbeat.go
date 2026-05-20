// Package heartbeat은 OTel 메트릭 SDK를 구성해 `agent.heartbeat` Gauge를
// 주기적으로 OTLP HTTP exporter로 발행한다. OTel Collector가 이를 받아
// Kafka `heartbeats` 토픽으로 otlp_json 인코딩으로 재발행한다 (spec §5.4).
//
// Agent 코드는 Kafka heartbeats 토픽을 직접 다루지 않는다 — OTel 표준
// metric 파이프라인을 따른다 (spec 5.4 / ADR #2/13).
package heartbeat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

// Start는 OTLP HTTP exporter + PeriodicReader 기반 MeterProvider를 만들고
// `agent.heartbeat` observable gauge(=1, attribute agent_id) 를 등록한다.
//
// 호출 측은 종료 시 provider.Shutdown(ctx)를 호출해야 한다.
//
// endpoint는 "http://host:4318" 형태. http:// 또는 https:// scheme 자동 파싱.
func Start(ctx context.Context, endpoint, agentID string, interval time.Duration) (*sdkmetric.MeterProvider, error) {
	host := stripScheme(endpoint)
	insecure := !strings.HasPrefix(strings.ToLower(endpoint), "https://")

	opts := []otlpmetrichttp.Option{
		otlpmetrichttp.WithEndpoint(host),
	}
	if insecure {
		opts = append(opts, otlpmetrichttp.WithInsecure())
	}

	exporter, err := otlpmetrichttp.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("otlp http exporter: %w", err)
	}

	reader := sdkmetric.NewPeriodicReader(exporter,
		sdkmetric.WithInterval(interval),
	)

	// service.name 명시 — Default()의 fallback("unknown_service:<bin>")을
	// 덮어쓴다. Default()의 telemetry.sdk.* / process.* 등은 유지.
	res, err := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(attribute.String("service.name", "script-agent")),
	)
	if err != nil {
		return nil, fmt.Errorf("build otel resource: %w", err)
	}

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(reader),
	)

	meter := provider.Meter("script-agent")
	_, err = meter.Float64ObservableGauge(
		"agent.heartbeat",
		metric.WithFloat64Callback(func(_ context.Context, o metric.Float64Observer) error {
			o.Observe(1, metric.WithAttributes(attribute.String("agent_id", agentID)))
			return nil
		}),
	)
	if err != nil {
		_ = provider.Shutdown(context.Background())
		return nil, fmt.Errorf("register agent.heartbeat gauge: %w", err)
	}

	return provider, nil
}

func stripScheme(endpoint string) string {
	if i := strings.Index(endpoint, "://"); i >= 0 {
		return endpoint[i+3:]
	}
	return endpoint
}
