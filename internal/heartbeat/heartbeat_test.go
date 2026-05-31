// Package heartbeat의 Phase 0 회귀 방지 단위 테스트.
// 데모 spec v0.2.1 §5.4 heartbeat 불변식을 잠근다:
//   - metric name: "agent.heartbeat"
//   - metric type: Gauge, value = 1
//   - attribute: agent_id
//   - resource: service.name = "script-agent"
//   - 송신 주기: Start()의 interval 파라미터가 PeriodicReader로 주입됨
//
// 이 테스트들은 production startWithExporter()를 직접 호출한다. Start()에서
// 네트워크 의존(otlpmetrichttp.New)만 분리됐을 뿐 gauge/resource/interval 등록은
// startWithExporter()가 전담하므로, stub exporter를 주입하면 OTLP endpoint 없이도
// production 등록 로직 자체를 검증한다(Start 내부가 회귀하면 테스트가 실패한다).
// OTLP Collector 실연동 검증은 e2e(monitoring-meta/e2e-tester)로 위임.
package heartbeat

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// capturingExporter는 PeriodicReader가 호출하는 sdkmetric.Exporter를 구현해
// 마지막으로 export된 ResourceMetrics와 export 횟수를 기록하는 테스트용 stub이다.
// 네트워크 없이 production startWithExporter 경로를 그대로 거치게 한다.
type capturingExporter struct {
	mu    sync.Mutex
	last  *metricdata.ResourceMetrics
	count int
}

func (e *capturingExporter) Temporality(k sdkmetric.InstrumentKind) metricdata.Temporality {
	return sdkmetric.DefaultTemporalitySelector(k)
}

func (e *capturingExporter) Aggregation(k sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return sdkmetric.DefaultAggregationSelector(k)
}

func (e *capturingExporter) Export(_ context.Context, rm *metricdata.ResourceMetrics) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	cp := *rm
	e.last = &cp
	e.count++
	return nil
}

func (e *capturingExporter) ForceFlush(context.Context) error { return nil }
func (e *capturingExporter) Shutdown(context.Context) error   { return nil }

func (e *capturingExporter) snapshot() (*metricdata.ResourceMetrics, int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.last, e.count
}

// TestStripScheme: stripScheme이 scheme 부분을 제거하고 host:port만 반환한다.
func TestStripScheme(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"http://localhost:4318", "localhost:4318"},
		{"https://otel.example.com:4317", "otel.example.com:4317"},
		{"localhost:4318", "localhost:4318"}, // scheme 없으면 그대로
		{"http://", ""},                      // host 없는 edge case
	}
	for _, tc := range cases {
		got := stripScheme(tc.input)
		if got != tc.want {
			t.Errorf("stripScheme(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// TestStartWithExporter_GaugeInvariants: spec §5.4 불변식 검증.
// production startWithExporter()로 MeterProvider를 만든 뒤 ForceFlush로 결정적으로
// 수집해, Start()가 등록하는 실제 metric/resource를 stub exporter에서 읽어 단정한다.
//   - metric name = "agent.heartbeat", Gauge, value = 1, attr agent_id
//   - resource service.name = "script-agent"
func TestStartWithExporter_GaugeInvariants(t *testing.T) {
	const agentID = "test-agent-uuid"
	exp := &capturingExporter{}

	// 긴 interval — 주기 firing 대신 명시적 ForceFlush로 결정적 수집.
	provider, err := startWithExporter(exp, agentID, time.Hour)
	if err != nil {
		t.Fatalf("startWithExporter 실패: %v", err)
	}
	defer provider.Shutdown(context.Background()) //nolint:errcheck

	if err := provider.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush 실패: %v", err)
	}

	rm, _ := exp.snapshot()
	if rm == nil {
		t.Fatal("export된 ResourceMetrics 없음")
	}

	// resource: service.name = "script-agent"
	if v, ok := rm.Resource.Set().Value(attribute.Key("service.name")); !ok {
		t.Error("resource에 service.name 없음")
	} else if v.AsString() != "script-agent" {
		t.Errorf("service.name = %q, want \"script-agent\"", v.AsString())
	}

	// metric: name "agent.heartbeat" / Gauge / value 1 / attr agent_id
	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "agent.heartbeat" {
				continue
			}
			found = true

			g, ok := m.Data.(metricdata.Gauge[float64])
			if !ok {
				t.Fatalf("metric type = %T, want metricdata.Gauge[float64]", m.Data)
			}
			if len(g.DataPoints) == 0 {
				t.Fatal("DataPoints가 비어 있음 — gauge 콜백이 호출되지 않음")
			}

			dp := g.DataPoints[0]
			if dp.Value != 1.0 {
				t.Errorf("gauge value = %v, want 1.0", dp.Value)
			}
			if v, ok := dp.Attributes.Value(attribute.Key("agent_id")); !ok {
				t.Error("attribute agent_id 없음")
			} else if v.AsString() != agentID {
				t.Errorf("agent_id = %q, want %q", v.AsString(), agentID)
			}
		}
	}
	if !found {
		t.Error("metric name \"agent.heartbeat\" 을 찾지 못함")
	}
}

// TestStartWithExporter_IntervalDrivesPeriodicExport: spec §5.4 송신 주기 불변식.
// 짧은 interval을 주입하면 PeriodicReader가 주기적으로 Export를 호출해야 한다.
// interval이 PeriodicReader에 전달되지 않으면(기본 60초) 아래 대기 시간 내 export는 0이다.
// 즉 이 테스트는 "interval 파라미터가 실제로 PeriodicReader 주기를 구동함"을 검증한다.
func TestStartWithExporter_IntervalDrivesPeriodicExport(t *testing.T) {
	const agentID = "test-agent-uuid"
	exp := &capturingExporter{}

	provider, err := startWithExporter(exp, agentID, 20*time.Millisecond)
	if err != nil {
		t.Fatalf("startWithExporter 실패: %v", err)
	}
	defer provider.Shutdown(context.Background()) //nolint:errcheck

	// 주기 export가 발생할 때까지 폴링(넉넉한 데드라인으로 flakiness 완화).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, n := exp.snapshot(); n > 0 {
			return // 주기 export 발생 — interval 주입이 PeriodicReader를 구동함
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("2초 내 주기 export가 발생하지 않음 — interval이 PeriodicReader에 주입되지 않았을 수 있음")
}
