package main

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

type fakeMetricsStore struct {
	mu sync.Mutex

	calls []string

	gaugeMetadataBatchSizes  []int
	gaugeDataPointBatchSizes []int
	sumMetadataBatchSizes    []int
	sumDataPointBatchSizes   []int

	insertGaugeMetadata   func(context.Context, []MetricMetadataRow) error
	insertGaugeDataPoints func(context.Context, []GaugeDataPointRow) error
	insertSumMetadata     func(context.Context, []SumMetadataRow) error
	insertSumDataPoints   func(context.Context, []SumDataPointRow) error
}

func (s *fakeMetricsStore) CreateTables(context.Context) error {
	return nil
}

func (s *fakeMetricsStore) InsertGaugeMetadata(ctx context.Context, rows []MetricMetadataRow) error {
	s.mu.Lock()
	s.calls = append(s.calls, "gauge_metadata")
	s.gaugeMetadataBatchSizes = append(s.gaugeMetadataBatchSizes, len(rows))
	hook := s.insertGaugeMetadata
	s.mu.Unlock()

	if hook != nil {
		return hook(ctx, rows)
	}

	return nil
}

func (s *fakeMetricsStore) InsertGaugeDataPoints(ctx context.Context, rows []GaugeDataPointRow) error {
	s.mu.Lock()
	s.calls = append(s.calls, "gauge_datapoints")
	s.gaugeDataPointBatchSizes = append(s.gaugeDataPointBatchSizes, len(rows))
	hook := s.insertGaugeDataPoints
	s.mu.Unlock()

	if hook != nil {
		return hook(ctx, rows)
	}

	return nil
}

func (s *fakeMetricsStore) InsertSumMetadata(ctx context.Context, rows []SumMetadataRow) error {
	s.mu.Lock()
	s.calls = append(s.calls, "sum_metadata")
	s.sumMetadataBatchSizes = append(s.sumMetadataBatchSizes, len(rows))
	hook := s.insertSumMetadata
	s.mu.Unlock()

	if hook != nil {
		return hook(ctx, rows)
	}

	return nil
}

func (s *fakeMetricsStore) InsertSumDataPoints(ctx context.Context, rows []SumDataPointRow) error {
	s.mu.Lock()
	s.calls = append(s.calls, "sum_datapoints")
	s.sumDataPointBatchSizes = append(s.sumDataPointBatchSizes, len(rows))
	hook := s.insertSumDataPoints
	s.mu.Unlock()

	if hook != nil {
		return hook(ctx, rows)
	}

	return nil
}

func (s *fakeMetricsStore) Close() error {
	return nil
}

func TestMetricsServiceServer_ExportPersistsGaugeInOneBatch(t *testing.T) {
	t.Parallel()

	store := &fakeMetricsStore{}
	server := &dash0MetricsServiceServer{store: store}

	_, err := server.Export(context.Background(), testExportRequest(true, false))
	if err != nil {
		t.Fatalf("exporting gauge request: %v", err)
	}

	assertBatchSizes(t, store.gaugeMetadataBatchSizes, []int{1}, "gauge metadata")
	assertBatchSizes(t, store.gaugeDataPointBatchSizes, []int{1}, "gauge datapoints")
	assertBatchSizes(t, store.sumMetadataBatchSizes, nil, "sum metadata")
	assertBatchSizes(t, store.sumDataPointBatchSizes, nil, "sum datapoints")
}

func TestMetricsServiceServer_ExportPersistsSumInOneBatch(t *testing.T) {
	t.Parallel()

	store := &fakeMetricsStore{}
	server := &dash0MetricsServiceServer{store: store}

	_, err := server.Export(context.Background(), testExportRequest(false, true))
	if err != nil {
		t.Fatalf("exporting sum request: %v", err)
	}

	assertBatchSizes(t, store.gaugeMetadataBatchSizes, nil, "gauge metadata")
	assertBatchSizes(t, store.gaugeDataPointBatchSizes, nil, "gauge datapoints")
	assertBatchSizes(t, store.sumMetadataBatchSizes, []int{1}, "sum metadata")
	assertBatchSizes(t, store.sumDataPointBatchSizes, []int{1}, "sum datapoints")
}

func TestMetricsServiceServer_ExportSkipsEmptyKinds(t *testing.T) {
	t.Parallel()

	store := &fakeMetricsStore{}
	server := &dash0MetricsServiceServer{store: store}

	_, err := server.Export(context.Background(), testExportRequest(false, false))
	if err != nil {
		t.Fatalf("exporting empty request: %v", err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	if len(store.calls) != 0 {
		t.Fatalf("expected no store calls for empty request, got %v", store.calls)
	}
}

func TestMetricsServiceServer_ExportRunsKindsIndependently(t *testing.T) {
	t.Parallel()

	gaugeStarted := make(chan struct{})
	sumStarted := make(chan struct{})
	releaseWrites := make(chan struct{})

	store := &fakeMetricsStore{
		insertGaugeMetadata: func(context.Context, []MetricMetadataRow) error {
			close(gaugeStarted)
			<-releaseWrites
			return nil
		},
		insertSumMetadata: func(context.Context, []SumMetadataRow) error {
			close(sumStarted)
			<-releaseWrites
			return nil
		},
	}
	server := &dash0MetricsServiceServer{store: store}

	done := make(chan error, 1)
	go func() {
		_, err := server.Export(context.Background(), testExportRequest(true, true))
		done <- err
	}()

	waitForSignal(t, gaugeStarted, "gauge metadata start")
	waitForSignal(t, sumStarted, "sum metadata start")
	close(releaseWrites)

	if err := waitForExport(t, done); err != nil {
		t.Fatalf("exporting mixed request: %v", err)
	}

	assertCallBefore(t, store.calls, "gauge_metadata", "gauge_datapoints")
	assertCallBefore(t, store.calls, "sum_metadata", "sum_datapoints")
}

func TestMetricsServiceServer_ExportReturnsMetadataErrorWithoutRetry(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("gauge metadata failed")
	store := &fakeMetricsStore{
		insertGaugeMetadata: func(context.Context, []MetricMetadataRow) error {
			return wantErr
		},
	}
	server := &dash0MetricsServiceServer{store: store}

	_, err := server.Export(context.Background(), testExportRequest(true, false))
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error %v, got %v", wantErr, err)
	}

	assertBatchSizes(t, store.gaugeMetadataBatchSizes, []int{1}, "gauge metadata")
	assertBatchSizes(t, store.gaugeDataPointBatchSizes, nil, "gauge datapoints")
}

func TestMetricsServiceServer_ExportAllowsPartialPersistenceAcrossKinds(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("sum datapoints failed")
	gaugeDataPointsWritten := make(chan struct{})

	store := &fakeMetricsStore{
		insertGaugeDataPoints: func(context.Context, []GaugeDataPointRow) error {
			close(gaugeDataPointsWritten)
			return nil
		},
		insertSumMetadata: func(context.Context, []SumMetadataRow) error {
			<-gaugeDataPointsWritten
			return nil
		},
		insertSumDataPoints: func(context.Context, []SumDataPointRow) error {
			<-gaugeDataPointsWritten
			return wantErr
		},
	}
	server := &dash0MetricsServiceServer{store: store}

	_, err := server.Export(context.Background(), testExportRequest(true, true))
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error %v, got %v", wantErr, err)
	}

	assertBatchSizes(t, store.gaugeMetadataBatchSizes, []int{1}, "gauge metadata")
	assertBatchSizes(t, store.gaugeDataPointBatchSizes, []int{1}, "gauge datapoints")
	assertBatchSizes(t, store.sumMetadataBatchSizes, []int{1}, "sum metadata")
	assertBatchSizes(t, store.sumDataPointBatchSizes, []int{1}, "sum datapoints")
	assertCallBefore(t, store.calls, "gauge_datapoints", "sum_datapoints")
}

func testExportRequest(includeGauge bool, includeSum bool) *colmetricspb.ExportMetricsServiceRequest {
	const (
		start = uint64(1_000_000_000)
		now   = uint64(2_000_000_000)
	)

	metrics := make([]*metricspb.Metric, 0, 2)
	if includeGauge {
		metrics = append(metrics, &metricspb.Metric{
			Name:        "cpu.utilization",
			Description: "CPU utilization percentage",
			Unit:        "%",
			Data: &metricspb.Metric_Gauge{
				Gauge: &metricspb.Gauge{
					DataPoints: []*metricspb.NumberDataPoint{{
						Attributes:        []*commonpb.KeyValue{testStringAttr("cpu", "0")},
						StartTimeUnixNano: start,
						TimeUnixNano:      now,
						Value:             &metricspb.NumberDataPoint_AsDouble{AsDouble: 42.5},
					}},
				},
			},
		})
	}
	if includeSum {
		metrics = append(metrics, &metricspb.Metric{
			Name:        "http.requests.total",
			Description: "Total HTTP requests",
			Unit:        "{request}",
			Data: &metricspb.Metric_Sum{
				Sum: &metricspb.Sum{
					AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
					IsMonotonic:            true,
					DataPoints: []*metricspb.NumberDataPoint{{
						Attributes: []*commonpb.KeyValue{
							testStringAttr("method", "GET"),
							testStringAttr("status", "200"),
						},
						StartTimeUnixNano: start,
						TimeUnixNano:      now,
						Value:             &metricspb.NumberDataPoint_AsDouble{AsDouble: 1234},
					}},
				},
			},
		})
	}

	return &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{
			Resource: &resourcepb.Resource{
				Attributes: []*commonpb.KeyValue{
					testStringAttr("service.name", "test-service"),
					testStringAttr("host.name", "test-host"),
				},
			},
			SchemaUrl: "https://opentelemetry.io/schemas/1.4.0",
			ScopeMetrics: []*metricspb.ScopeMetrics{{
				Scope: &commonpb.InstrumentationScope{
					Name:    "test-scope",
					Version: "1.0.0",
				},
				Metrics: metrics,
			}},
		}},
	}
}

func testStringAttr(key string, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: key, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}}}
}

func assertBatchSizes(t *testing.T, got []int, want []int, name string) {
	t.Helper()

	if !slices.Equal(got, want) {
		t.Fatalf("unexpected %s batches: want %v, got %v", name, want, got)
	}
}

func assertCallBefore(t *testing.T, calls []string, first string, second string) {
	t.Helper()

	firstIndex := slices.Index(calls, first)
	secondIndex := slices.Index(calls, second)
	if firstIndex == -1 || secondIndex == -1 {
		t.Fatalf("missing calls in sequence %v: %s index=%d, %s index=%d", calls, first, firstIndex, second, secondIndex)
	}
	if firstIndex > secondIndex {
		t.Fatalf("expected %s before %s, got %v", first, second, calls)
	}
}

func waitForSignal(t *testing.T, ch <-chan struct{}, name string) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func waitForExport(t *testing.T, done <-chan error) error {
	t.Helper()

	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for export")
		return nil
	}
}
