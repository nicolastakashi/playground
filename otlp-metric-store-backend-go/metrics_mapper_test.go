package main

import (
	"testing"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

func TestMapNormalizedGaugeRowsDeduplicatesMetadataWithinBatch(t *testing.T) {
	now := uint64(time.Now().UnixNano())
	rows := MapNormalizedGaugeRows([]*metricspb.ResourceMetrics{newGaugeBatchWithRepeatedMetadata(now)})

	if len(rows.Metadata) != 1 {
		t.Fatalf("expected 1 metadata row, got %d", len(rows.Metadata))
	}
	if len(rows.DataPoints) != 2 {
		t.Fatalf("expected 2 datapoint rows, got %d", len(rows.DataPoints))
	}
	if rows.DataPoints[0].MetadataKey != rows.Metadata[0].MetadataKey || rows.DataPoints[1].MetadataKey != rows.Metadata[0].MetadataKey {
		t.Fatal("expected datapoints to reference the deduplicated metadata row")
	}
}

func TestMapNormalizedSumRowsDeduplicatesMetadataWithinBatch(t *testing.T) {
	now := uint64(time.Now().UnixNano())
	rows := MapNormalizedSumRows([]*metricspb.ResourceMetrics{newSumBatchWithRepeatedMetadata(now)})

	if len(rows.Metadata) != 1 {
		t.Fatalf("expected 1 metadata row, got %d", len(rows.Metadata))
	}
	if len(rows.DataPoints) != 2 {
		t.Fatalf("expected 2 datapoint rows, got %d", len(rows.DataPoints))
	}
	if rows.DataPoints[0].MetadataKey != rows.Metadata[0].MetadataKey || rows.DataPoints[1].MetadataKey != rows.Metadata[0].MetadataKey {
		t.Fatal("expected datapoints to reference the deduplicated metadata row")
	}
}

func newGaugeBatchWithRepeatedMetadata(now uint64) *metricspb.ResourceMetrics {
	return &metricspb.ResourceMetrics{
		Resource: &resourcepb.Resource{
			Attributes: []*commonpb.KeyValue{
				stringAttr("service.name", "test-service"),
				stringAttr("host.name", "test-host"),
			},
		},
		SchemaUrl: "https://opentelemetry.io/schemas/1.4.0",
		ScopeMetrics: []*metricspb.ScopeMetrics{{
			Scope: &commonpb.InstrumentationScope{Name: "test-scope", Version: "1.0.0"},
			Metrics: []*metricspb.Metric{{
				Name:        "cpu.utilization",
				Description: "CPU utilization percentage",
				Unit:        "%",
				Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: []*metricspb.NumberDataPoint{
					{
						Attributes:        []*commonpb.KeyValue{stringAttr("cpu", "0")},
						StartTimeUnixNano: now - uint64(time.Minute),
						TimeUnixNano:      now,
						Value:             &metricspb.NumberDataPoint_AsDouble{AsDouble: 42.5},
					},
					{
						Attributes:        []*commonpb.KeyValue{stringAttr("cpu", "0")},
						StartTimeUnixNano: now - uint64(time.Minute),
						TimeUnixNano:      now + 1,
						Value:             &metricspb.NumberDataPoint_AsDouble{AsDouble: 43.5},
					},
				}}},
			}},
		}},
	}
}

func newSumBatchWithRepeatedMetadata(now uint64) *metricspb.ResourceMetrics {
	return &metricspb.ResourceMetrics{
		Resource: &resourcepb.Resource{
			Attributes: []*commonpb.KeyValue{
				stringAttr("service.name", "test-service"),
				stringAttr("host.name", "test-host"),
			},
		},
		SchemaUrl: "https://opentelemetry.io/schemas/1.4.0",
		ScopeMetrics: []*metricspb.ScopeMetrics{{
			Scope: &commonpb.InstrumentationScope{Name: "test-scope", Version: "1.0.0"},
			Metrics: []*metricspb.Metric{{
				Name:        "http.requests.total",
				Description: "Total HTTP requests",
				Unit:        "{request}",
				Data: &metricspb.Metric_Sum{Sum: &metricspb.Sum{
					AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
					IsMonotonic:            true,
					DataPoints: []*metricspb.NumberDataPoint{
						{
							Attributes:        []*commonpb.KeyValue{stringAttr("method", "GET")},
							StartTimeUnixNano: now - uint64(time.Minute),
							TimeUnixNano:      now,
							Value:             &metricspb.NumberDataPoint_AsDouble{AsDouble: 1234},
						},
						{
							Attributes:        []*commonpb.KeyValue{stringAttr("method", "GET")},
							StartTimeUnixNano: now - uint64(time.Minute),
							TimeUnixNano:      now + 1,
							Value:             &metricspb.NumberDataPoint_AsDouble{AsDouble: 1235},
						},
					},
				}},
			}},
		}},
	}
}
