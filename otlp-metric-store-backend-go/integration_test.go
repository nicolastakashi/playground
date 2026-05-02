//go:build integration

package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func setupClickHouse(t *testing.T) (*ClickHouseMetricsStore, func()) {
	t.Helper()
	ctx := context.Background()

	ctr, err := testcontainers.Run(ctx, "clickhouse/clickhouse-server:26.2",
		testcontainers.WithExposedPorts("9000/tcp"),
		testcontainers.WithEnv(map[string]string{
			"CLICKHOUSE_USER":     "default",
			"CLICKHOUSE_PASSWORD": "test",
		}),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("9000/tcp").WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("starting clickhouse container: %v", err)
	}

	host, err := ctr.Host(ctx)
	if err != nil {
		t.Fatalf("getting container host: %v", err)
	}
	mappedPort, err := ctr.MappedPort(ctx, "9000/tcp")
	if err != nil {
		t.Fatalf("getting mapped port: %v", err)
	}

	addr := fmt.Sprintf("%s:%s", host, mappedPort.Port())
	store, err := NewClickHouseMetricsStore(ctx, addr, "default", "default", "test")
	if err != nil {
		t.Fatalf("creating clickhouse metrics store: %v", err)
	}

	cleanup := func() {
		store.Close()
		if err := ctr.Terminate(ctx); err != nil {
			t.Logf("terminating clickhouse container: %v", err)
		}
	}

	return store, cleanup
}

func TestCreateTables(t *testing.T) {
	store, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	if err := store.CreateTables(ctx); err != nil {
		t.Fatalf("creating tables: %v", err)
	}

	expectedTables := []string{
		"otel_metrics_gauge_metadata",
		"otel_metrics_sum_metadata",
		"otel_metrics_gauge",
		"otel_metrics_sum",
		"otel_metrics_histogram",
		"otel_metrics_exponential_histogram",
		"otel_metrics_summary",
	}

	for _, table := range expectedTables {
		var count uint64
		err := store.conn.QueryRow(ctx,
			"SELECT count() FROM system.tables WHERE database = 'default' AND name = $1", table,
		).Scan(&count)
		if err != nil {
			t.Fatalf("querying system.tables for %s: %v", table, err)
		}
		if count != 1 {
			t.Errorf("expected table %s to exist, got count=%d", table, count)
		}
	}

	expectedMetadataEngines := map[string]string{
		"otel_metrics_gauge_metadata": "ReplacingMergeTree",
		"otel_metrics_sum_metadata":   "ReplacingMergeTree",
	}

	for table, expectedEngine := range expectedMetadataEngines {
		var engine string
		err := store.conn.QueryRow(ctx,
			"SELECT engine FROM system.tables WHERE database = 'default' AND name = $1", table,
		).Scan(&engine)
		if err != nil {
			t.Fatalf("querying engine for %s: %v", table, err)
		}
		if engine != expectedEngine {
			t.Errorf("expected table %s to use %s, got %s", table, expectedEngine, engine)
		}
	}
}

func TestInsertGauge(t *testing.T) {
	store, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	if err := store.CreateTables(ctx); err != nil {
		t.Fatalf("creating tables: %v", err)
	}

	now := uint64(time.Now().UnixNano())
	startTime := now - uint64(time.Minute)
	resourceMetrics := []*metricspb.ResourceMetrics{
		{
			Resource: &resourcepb.Resource{
				Attributes: []*commonpb.KeyValue{
					{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "test-service"}}},
					{Key: "host.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "test-host"}}},
				},
			},
			SchemaUrl: "https://opentelemetry.io/schemas/1.4.0",
			ScopeMetrics: []*metricspb.ScopeMetrics{
				{
					Scope: &commonpb.InstrumentationScope{
						Name:    "test-scope",
						Version: "1.0.0",
					},
					Metrics: []*metricspb.Metric{
						{
							Name:        "cpu.utilization",
							Description: "CPU utilization percentage",
							Unit:        "%",
							Data: &metricspb.Metric_Gauge{
								Gauge: &metricspb.Gauge{
									DataPoints: []*metricspb.NumberDataPoint{
										{
											Attributes:        []*commonpb.KeyValue{{Key: "cpu", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "0"}}}},
											StartTimeUnixNano: startTime,
											TimeUnixNano:      now,
											Value:             &metricspb.NumberDataPoint_AsDouble{AsDouble: 42.5},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	rows := MapNormalizedGaugeRows(resourceMetrics)
	if err := store.InsertGaugeMetadata(ctx, rows.Metadata); err != nil {
		t.Fatalf("inserting gauge metadata rows: %v", err)
	}
	if err := store.InsertGaugeDataPoints(ctx, rows.DataPoints); err != nil {
		t.Fatalf("inserting gauge datapoint rows: %v", err)
	}

	var (
		serviceName          string
		metricName           string
		datapointMetadataKey MetadataKey
		metadataKey          MetadataKey
		value                float64
	)
	err := store.conn.QueryRow(ctx,
		"SELECT m.ServiceName, m.MetricName, g.MetadataKey, m.MetadataKey, g.Value FROM otel_metrics_gauge g INNER JOIN otel_metrics_gauge_metadata FINAL m USING (MetadataKey) WHERE m.MetricName = 'cpu.utilization'",
	).Scan(&serviceName, &metricName, &datapointMetadataKey, &metadataKey, &value)
	if err != nil {
		t.Fatalf("querying gauge: %v", err)
	}

	if serviceName != "test-service" {
		t.Errorf("expected ServiceName=test-service, got %s", serviceName)
	}
	if metricName != "cpu.utilization" {
		t.Errorf("expected MetricName=cpu.utilization, got %s", metricName)
	}
	if datapointMetadataKey != metadataKey {
		t.Errorf("expected datapoint metadata key %s to reference metadata row %s", datapointMetadataKey, metadataKey)
	}
	if value != 42.5 {
		t.Errorf("expected Value=42.5, got %f", value)
	}
}

func TestInsertSum(t *testing.T) {
	store, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	if err := store.CreateTables(ctx); err != nil {
		t.Fatalf("creating tables: %v", err)
	}

	now := uint64(time.Now().UnixNano())
	startTime := now - uint64(time.Minute)
	resourceMetrics := []*metricspb.ResourceMetrics{
		{
			Resource: &resourcepb.Resource{
				Attributes: []*commonpb.KeyValue{
					{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "test-service"}}},
					{Key: "host.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "test-host"}}},
				},
			},
			SchemaUrl: "https://opentelemetry.io/schemas/1.4.0",
			ScopeMetrics: []*metricspb.ScopeMetrics{
				{
					Scope: &commonpb.InstrumentationScope{
						Name:    "test-scope",
						Version: "1.0.0",
					},
					Metrics: []*metricspb.Metric{
						{
							Name:        "http.requests.total",
							Description: "Total HTTP requests",
							Unit:        "{request}",
							Data: &metricspb.Metric_Sum{
								Sum: &metricspb.Sum{
									AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
									IsMonotonic:            true,
									DataPoints: []*metricspb.NumberDataPoint{
										{
											Attributes: []*commonpb.KeyValue{
												{Key: "method", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "GET"}}},
												{Key: "status", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "200"}}},
											},
											StartTimeUnixNano: startTime,
											TimeUnixNano:      now,
											Value:             &metricspb.NumberDataPoint_AsDouble{AsDouble: 1234},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	rows := MapNormalizedSumRows(resourceMetrics)
	if err := store.InsertSumMetadata(ctx, rows.Metadata); err != nil {
		t.Fatalf("inserting sum metadata rows: %v", err)
	}
	if err := store.InsertSumDataPoints(ctx, rows.DataPoints); err != nil {
		t.Fatalf("inserting sum datapoint rows: %v", err)
	}

	var (
		serviceName            string
		metricName             string
		datapointMetadataKey   MetadataKey
		metadataKey            MetadataKey
		value                  float64
		aggregationTemporality int32
		isMonotonic            bool
	)
	err := store.conn.QueryRow(ctx,
		"SELECT m.ServiceName, m.MetricName, s.MetadataKey, m.MetadataKey, s.Value, m.AggregationTemporality, m.IsMonotonic FROM otel_metrics_sum s INNER JOIN otel_metrics_sum_metadata FINAL m USING (MetadataKey) WHERE m.MetricName = 'http.requests.total'",
	).Scan(&serviceName, &metricName, &datapointMetadataKey, &metadataKey, &value, &aggregationTemporality, &isMonotonic)
	if err != nil {
		t.Fatalf("querying sum: %v", err)
	}

	if serviceName != "test-service" {
		t.Errorf("expected ServiceName=test-service, got %s", serviceName)
	}
	if metricName != "http.requests.total" {
		t.Errorf("expected MetricName=http.requests.total, got %s", metricName)
	}
	if datapointMetadataKey != metadataKey {
		t.Errorf("expected datapoint metadata key %s to reference metadata row %s", datapointMetadataKey, metadataKey)
	}
	if value != 1234 {
		t.Errorf("expected Value=1234, got %f", value)
	}
	if aggregationTemporality != 2 {
		t.Errorf("expected AggregationTemporality=2, got %d", aggregationTemporality)
	}
	if !isMonotonic {
		t.Errorf("expected IsMonotonic=true, got false")
	}
}

func TestGaugeMetadataReplacingMergeTreeSemantics(t *testing.T) {
	store, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	if err := store.CreateTables(ctx); err != nil {
		t.Fatalf("creating tables: %v", err)
	}

	first := makeTestGaugeMetadataRow(
		[]*commonpb.KeyValue{stringAttr("service.name", "test-service"), stringAttr("host.name", "test-host")},
		nil,
		[]*commonpb.KeyValue{stringAttr("cpu", "0")},
		"https://resource.schema/1",
		"https://scope.schema/1",
		"CPU utilization percentage",
		"%",
	)
	second := makeTestGaugeMetadataRow(
		[]*commonpb.KeyValue{stringAttr("service.name", "test-service"), stringAttr("host.name", "test-host")},
		nil,
		[]*commonpb.KeyValue{stringAttr("cpu", "0")},
		"https://resource.schema/2",
		"https://scope.schema/2",
		"CPU utilization percentage v2",
		"%",
	)

	if first.MetadataKey != second.MetadataKey {
		t.Fatalf("expected non-identifying metadata changes to reuse the same metadata key, got %s and %s", first.MetadataKey, second.MetadataKey)
	}

	if err := store.InsertGaugeMetadata(ctx, []MetricMetadataRow{first, second}); err != nil {
		t.Fatalf("inserting gauge metadata rows: %v", err)
	}

	var physicalRows uint64
	err := store.conn.QueryRow(ctx,
		"SELECT count() FROM otel_metrics_gauge_metadata WHERE MetadataKey = $1",
		first.MetadataKey,
	).Scan(&physicalRows)
	if err != nil {
		t.Fatalf("counting physical metadata rows: %v", err)
	}
	if physicalRows != 2 {
		t.Fatalf("expected 2 physical metadata rows before merge, got %d", physicalRows)
	}

	var logicalRows uint64
	err = store.conn.QueryRow(ctx,
		"SELECT count() FROM otel_metrics_gauge_metadata FINAL WHERE MetadataKey = $1",
		first.MetadataKey,
	).Scan(&logicalRows)
	if err != nil {
		t.Fatalf("counting logical metadata rows: %v", err)
	}
	if logicalRows != 1 {
		t.Fatalf("expected FINAL to return 1 logical metadata row, got %d", logicalRows)
	}

	winner := first
	if bytes.Compare(second.ReplacementRank[:], first.ReplacementRank[:]) > 0 {
		winner = second
	}

	var (
		metricDescription string
		resourceSchemaURL string
		scopeSchemaURL    string
	)
	err = store.conn.QueryRow(ctx,
		"SELECT MetricDescription, ResourceSchemaUrl, ScopeSchemaUrl FROM otel_metrics_gauge_metadata FINAL WHERE MetadataKey = $1",
		first.MetadataKey,
	).Scan(&metricDescription, &resourceSchemaURL, &scopeSchemaURL)
	if err != nil {
		t.Fatalf("querying logical metadata survivor: %v", err)
	}
	if metricDescription != winner.MetricDescription {
		t.Errorf("expected surviving description %q, got %q", winner.MetricDescription, metricDescription)
	}
	if resourceSchemaURL != winner.ResourceSchemaUrl {
		t.Errorf("expected surviving resource schema URL %q, got %q", winner.ResourceSchemaUrl, resourceSchemaURL)
	}
	if scopeSchemaURL != winner.ScopeSchemaUrl {
		t.Errorf("expected surviving scope schema URL %q, got %q", winner.ScopeSchemaUrl, scopeSchemaURL)
	}
}

func TestGaugeMetadataIdentifyingDriftCreatesNewIdentity(t *testing.T) {
	store, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	if err := store.CreateTables(ctx); err != nil {
		t.Fatalf("creating tables: %v", err)
	}

	now := uint64(time.Now().UnixNano())
	firstRows := MapNormalizedGaugeRows([]*metricspb.ResourceMetrics{newGaugeResourceMetrics(now, "test-service", "test-host-a", "cpu.utilization", "%", "CPU utilization percentage", 42.5)})
	secondRows := MapNormalizedGaugeRows([]*metricspb.ResourceMetrics{newGaugeResourceMetrics(now+1, "test-service", "test-host-b", "cpu.utilization", "%", "CPU utilization percentage", 43.5)})

	if err := store.InsertGaugeMetadata(ctx, append(firstRows.Metadata, secondRows.Metadata...)); err != nil {
		t.Fatalf("inserting gauge metadata rows: %v", err)
	}
	if err := store.InsertGaugeDataPoints(ctx, append(firstRows.DataPoints, secondRows.DataPoints...)); err != nil {
		t.Fatalf("inserting gauge datapoint rows: %v", err)
	}

	var distinctMetadataKeys uint64
	err := store.conn.QueryRow(ctx,
		"SELECT countDistinct(MetadataKey) FROM otel_metrics_gauge_metadata FINAL WHERE MetricName = 'cpu.utilization'",
	).Scan(&distinctMetadataKeys)
	if err != nil {
		t.Fatalf("counting distinct metadata keys: %v", err)
	}
	if distinctMetadataKeys != 2 {
		t.Fatalf("expected identifying drift to create 2 metadata identities, got %d", distinctMetadataKeys)
	}

	var datapointRows uint64
	err = store.conn.QueryRow(ctx,
		"SELECT count() FROM otel_metrics_gauge WHERE MetadataKey IN ($1, $2)",
		firstRows.Metadata[0].MetadataKey,
		secondRows.Metadata[0].MetadataKey,
	).Scan(&datapointRows)
	if err != nil {
		t.Fatalf("counting datapoint rows: %v", err)
	}
	if datapointRows != 2 {
		t.Fatalf("expected 2 datapoints referencing drifted metadata identities, got %d", datapointRows)
	}
}

func TestSumMetadataReplacingMergeTreeSemantics(t *testing.T) {
	store, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	if err := store.CreateTables(ctx); err != nil {
		t.Fatalf("creating tables: %v", err)
	}

	first := makeTestSumMetadataRow(
		[]*commonpb.KeyValue{stringAttr("service.name", "test-service"), stringAttr("host.name", "test-host")},
		nil,
		[]*commonpb.KeyValue{stringAttr("method", "GET")},
		"https://resource.schema/1",
		"https://scope.schema/1",
		"Total HTTP requests",
		"{request}",
		metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
		true,
	)
	second := makeTestSumMetadataRow(
		[]*commonpb.KeyValue{stringAttr("service.name", "test-service"), stringAttr("host.name", "test-host")},
		nil,
		[]*commonpb.KeyValue{stringAttr("method", "GET")},
		"https://resource.schema/2",
		"https://scope.schema/2",
		"Total HTTP requests v2",
		"{request}",
		metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
		true,
	)

	if first.MetadataKey != second.MetadataKey {
		t.Fatalf("expected non-identifying metadata changes to reuse the same metadata key, got %s and %s", first.MetadataKey, second.MetadataKey)
	}

	if err := store.InsertSumMetadata(ctx, []SumMetadataRow{first, second}); err != nil {
		t.Fatalf("inserting sum metadata rows: %v", err)
	}

	var physicalRows uint64
	err := store.conn.QueryRow(ctx,
		"SELECT count() FROM otel_metrics_sum_metadata WHERE MetadataKey = $1",
		first.MetadataKey,
	).Scan(&physicalRows)
	if err != nil {
		t.Fatalf("counting physical metadata rows: %v", err)
	}
	if physicalRows != 2 {
		t.Fatalf("expected 2 physical metadata rows before merge, got %d", physicalRows)
	}

	var logicalRows uint64
	err = store.conn.QueryRow(ctx,
		"SELECT count() FROM otel_metrics_sum_metadata FINAL WHERE MetadataKey = $1",
		first.MetadataKey,
	).Scan(&logicalRows)
	if err != nil {
		t.Fatalf("counting logical metadata rows: %v", err)
	}
	if logicalRows != 1 {
		t.Fatalf("expected FINAL to return 1 logical metadata row, got %d", logicalRows)
	}

	winner := first
	if bytes.Compare(second.ReplacementRank[:], first.ReplacementRank[:]) > 0 {
		winner = second
	}

	var (
		metricDescription string
		resourceSchemaURL string
		scopeSchemaURL    string
	)
	err = store.conn.QueryRow(ctx,
		"SELECT MetricDescription, ResourceSchemaUrl, ScopeSchemaUrl FROM otel_metrics_sum_metadata FINAL WHERE MetadataKey = $1",
		first.MetadataKey,
	).Scan(&metricDescription, &resourceSchemaURL, &scopeSchemaURL)
	if err != nil {
		t.Fatalf("querying logical metadata survivor: %v", err)
	}
	if metricDescription != winner.MetricDescription {
		t.Errorf("expected surviving description %q, got %q", winner.MetricDescription, metricDescription)
	}
	if resourceSchemaURL != winner.ResourceSchemaUrl {
		t.Errorf("expected surviving resource schema URL %q, got %q", winner.ResourceSchemaUrl, resourceSchemaURL)
	}
	if scopeSchemaURL != winner.ScopeSchemaUrl {
		t.Errorf("expected surviving scope schema URL %q, got %q", winner.ScopeSchemaUrl, scopeSchemaURL)
	}
}

func TestSumMetadataIdentifyingDriftCreatesNewIdentity(t *testing.T) {
	store, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	if err := store.CreateTables(ctx); err != nil {
		t.Fatalf("creating tables: %v", err)
	}

	now := uint64(time.Now().UnixNano())
	firstRows := MapNormalizedSumRows([]*metricspb.ResourceMetrics{newSumResourceMetrics(now, "test-service", "test-host", "http.requests.total", "{request}", "Total HTTP requests", metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, true, 1234)})
	secondRows := MapNormalizedSumRows([]*metricspb.ResourceMetrics{newSumResourceMetrics(now+1, "test-service", "test-host", "http.requests.total", "{request}", "Total HTTP requests", metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA, true, 1235)})

	if err := store.InsertSumMetadata(ctx, append(firstRows.Metadata, secondRows.Metadata...)); err != nil {
		t.Fatalf("inserting sum metadata rows: %v", err)
	}
	if err := store.InsertSumDataPoints(ctx, append(firstRows.DataPoints, secondRows.DataPoints...)); err != nil {
		t.Fatalf("inserting sum datapoint rows: %v", err)
	}

	var distinctMetadataKeys uint64
	err := store.conn.QueryRow(ctx,
		"SELECT countDistinct(MetadataKey) FROM otel_metrics_sum_metadata FINAL WHERE MetricName = 'http.requests.total'",
	).Scan(&distinctMetadataKeys)
	if err != nil {
		t.Fatalf("counting distinct metadata keys: %v", err)
	}
	if distinctMetadataKeys != 2 {
		t.Fatalf("expected identifying drift to create 2 metadata identities, got %d", distinctMetadataKeys)
	}

	var datapointRows uint64
	err = store.conn.QueryRow(ctx,
		"SELECT count() FROM otel_metrics_sum WHERE MetadataKey IN ($1, $2)",
		firstRows.Metadata[0].MetadataKey,
		secondRows.Metadata[0].MetadataKey,
	).Scan(&datapointRows)
	if err != nil {
		t.Fatalf("counting datapoint rows: %v", err)
	}
	if datapointRows != 2 {
		t.Fatalf("expected 2 datapoints referencing drifted metadata identities, got %d", datapointRows)
	}
}

func TestGRPCToClickHouse(t *testing.T) {
	store, cleanup := setupClickHouse(t)
	defer cleanup()

	ctx := context.Background()
	if err := store.CreateTables(ctx); err != nil {
		t.Fatalf("creating tables: %v", err)
	}

	// Start gRPC server wired to the ClickHouse store.
	lis := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	colmetricspb.RegisterMetricsServiceServer(grpcServer, newServer("bufconn", store))
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Printf("error serving server: %v", err)
		}
	}()
	defer grpcServer.Stop()

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("connecting to grpc server: %v", err)
	}
	defer conn.Close()

	client := colmetricspb.NewMetricsServiceClient(conn)

	// Send a gauge metric via gRPC.
	now := uint64(time.Now().UnixNano())
	_, err = client.Export(ctx, &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{
			{
				Resource: &resourcepb.Resource{
					Attributes: []*commonpb.KeyValue{
						{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "e2e-service"}}},
					},
				},
				ScopeMetrics: []*metricspb.ScopeMetrics{
					{
						Scope: &commonpb.InstrumentationScope{Name: "e2e-scope"},
						Metrics: []*metricspb.Metric{
							{
								Name: "e2e.gauge",
								Data: &metricspb.Metric_Gauge{
									Gauge: &metricspb.Gauge{
										DataPoints: []*metricspb.NumberDataPoint{
											{
												TimeUnixNano: now,
												Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: 99.9},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("exporting metrics via grpc: %v", err)
	}

	// Verify the metric landed in ClickHouse.
	var (
		svcName              string
		metricName           string
		datapointMetadataKey MetadataKey
		metadataKey          MetadataKey
		value                float64
	)
	err = store.conn.QueryRow(ctx,
		"SELECT m.ServiceName, m.MetricName, g.MetadataKey, m.MetadataKey, g.Value FROM otel_metrics_gauge g INNER JOIN otel_metrics_gauge_metadata FINAL m USING (MetadataKey) WHERE m.MetricName = 'e2e.gauge'",
	).Scan(&svcName, &metricName, &datapointMetadataKey, &metadataKey, &value)
	if err != nil {
		t.Fatalf("querying clickhouse: %v", err)
	}
	if svcName != "e2e-service" {
		t.Errorf("expected ServiceName=e2e-service, got %s", svcName)
	}
	if metricName != "e2e.gauge" {
		t.Errorf("expected MetricName=e2e.gauge, got %s", metricName)
	}
	if datapointMetadataKey != metadataKey {
		t.Errorf("expected datapoint metadata key %s to reference metadata row %s", datapointMetadataKey, metadataKey)
	}
	if value != 99.9 {
		t.Errorf("expected Value=99.9, got %f", value)
	}
}

func newGaugeResourceMetrics(now uint64, service string, host string, metricName string, unit string, description string, value float64) *metricspb.ResourceMetrics {
	return &metricspb.ResourceMetrics{
		Resource: &resourcepb.Resource{
			Attributes: []*commonpb.KeyValue{
				stringAttr("service.name", service),
				stringAttr("host.name", host),
			},
		},
		SchemaUrl: "https://opentelemetry.io/schemas/1.4.0",
		ScopeMetrics: []*metricspb.ScopeMetrics{
			{
				Scope: &commonpb.InstrumentationScope{
					Name:    "test-scope",
					Version: "1.0.0",
				},
				Metrics: []*metricspb.Metric{
					{
						Name:        metricName,
						Description: description,
						Unit:        unit,
						Data: &metricspb.Metric_Gauge{
							Gauge: &metricspb.Gauge{
								DataPoints: []*metricspb.NumberDataPoint{
									{
										Attributes: []*commonpb.KeyValue{
											stringAttr("cpu", "0"),
										},
										StartTimeUnixNano: now - uint64(time.Minute),
										TimeUnixNano:      now,
										Value:             &metricspb.NumberDataPoint_AsDouble{AsDouble: value},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func newSumResourceMetrics(now uint64, service string, host string, metricName string, unit string, description string, temporality metricspb.AggregationTemporality, monotonic bool, value float64) *metricspb.ResourceMetrics {
	return &metricspb.ResourceMetrics{
		Resource: &resourcepb.Resource{
			Attributes: []*commonpb.KeyValue{
				stringAttr("service.name", service),
				stringAttr("host.name", host),
			},
		},
		SchemaUrl: "https://opentelemetry.io/schemas/1.4.0",
		ScopeMetrics: []*metricspb.ScopeMetrics{
			{
				Scope: &commonpb.InstrumentationScope{
					Name:    "test-scope",
					Version: "1.0.0",
				},
				Metrics: []*metricspb.Metric{
					{
						Name:        metricName,
						Description: description,
						Unit:        unit,
						Data: &metricspb.Metric_Sum{
							Sum: &metricspb.Sum{
								AggregationTemporality: temporality,
								IsMonotonic:            monotonic,
								DataPoints: []*metricspb.NumberDataPoint{
									{
										Attributes: []*commonpb.KeyValue{
											stringAttr("method", "GET"),
										},
										StartTimeUnixNano: now - uint64(time.Minute),
										TimeUnixNano:      now,
										Value:             &metricspb.NumberDataPoint_AsDouble{AsDouble: value},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}
