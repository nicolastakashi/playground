package main

import (
	"testing"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
)

func TestMetadataIdentity(t *testing.T) {
	t.Run("reordered attributes resolve to the same gauge metadata key", func(t *testing.T) {
		first := makeTestGaugeMetadataRow(
			[]*commonpb.KeyValue{
				stringAttr("service.name", "svc"),
				stringAttr("host.name", "host-a"),
			},
			[]*commonpb.KeyValue{
				stringAttr("scope.attr.a", "a"),
				stringAttr("scope.attr.b", "b"),
			},
			[]*commonpb.KeyValue{
				stringAttr("cpu", "0"),
				stringAttr("state", "idle"),
			},
			"https://resource.schema/1",
			"https://scope.schema/1",
			"CPU utilization",
			"%",
		)

		second := makeTestGaugeMetadataRow(
			[]*commonpb.KeyValue{
				stringAttr("host.name", "host-a"),
				stringAttr("service.name", "svc"),
			},
			[]*commonpb.KeyValue{
				stringAttr("scope.attr.b", "b"),
				stringAttr("scope.attr.a", "a"),
			},
			[]*commonpb.KeyValue{
				stringAttr("state", "idle"),
				stringAttr("cpu", "0"),
			},
			"https://resource.schema/1",
			"https://scope.schema/1",
			"CPU utilization",
			"%",
		)

		if first.MetadataKey != second.MetadataKey {
			t.Fatalf("expected reordered attributes to keep metadata key stable, got %s and %s", first.MetadataKey, second.MetadataKey)
		}
	})

	t.Run("gauge and sum rows do not share metadata keys", func(t *testing.T) {
		gauge := makeTestGaugeMetadataRow(nil, nil, []*commonpb.KeyValue{stringAttr("method", "GET")}, "", "", "Request count", "1")
		sum := makeTestSumMetadataRow(nil, nil, []*commonpb.KeyValue{stringAttr("method", "GET")}, "", "", "Request count", "1", metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, true)

		if gauge.MetadataKey == sum.MetadataKey {
			t.Fatalf("expected gauge and sum metadata keys to differ, got %s", gauge.MetadataKey)
		}
	})

	t.Run("sum semantics changes create a new identity", func(t *testing.T) {
		base := makeTestSumMetadataRow(nil, nil, []*commonpb.KeyValue{stringAttr("method", "GET")}, "", "", "Request count", "1", metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, true)
		changedTemporality := makeTestSumMetadataRow(nil, nil, []*commonpb.KeyValue{stringAttr("method", "GET")}, "", "", "Request count", "1", metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA, true)
		changedMonotonic := makeTestSumMetadataRow(nil, nil, []*commonpb.KeyValue{stringAttr("method", "GET")}, "", "", "Request count", "1", metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, false)

		if base.MetadataKey == changedTemporality.MetadataKey {
			t.Fatal("expected aggregation temporality change to create a new metadata identity")
		}
		if base.MetadataKey == changedMonotonic.MetadataKey {
			t.Fatal("expected monotonicity change to create a new metadata identity")
		}
	})

	t.Run("metric unit changes create a new gauge identity", func(t *testing.T) {
		first := makeTestGaugeMetadataRow(nil, nil, []*commonpb.KeyValue{stringAttr("cpu", "0")}, "", "", "CPU utilization", "%")
		second := makeTestGaugeMetadataRow(nil, nil, []*commonpb.KeyValue{stringAttr("cpu", "0")}, "", "", "CPU utilization", "ratio")

		if first.MetadataKey == second.MetadataKey {
			t.Fatal("expected unit change to create a new metadata identity")
		}
	})

	t.Run("description changes keep identity but change replacement rank", func(t *testing.T) {
		first := makeTestGaugeMetadataRow(nil, nil, []*commonpb.KeyValue{stringAttr("cpu", "0")}, "", "", "CPU utilization", "%")
		second := makeTestGaugeMetadataRow(nil, nil, []*commonpb.KeyValue{stringAttr("cpu", "0")}, "", "", "CPU busy percentage", "%")

		if first.MetadataKey != second.MetadataKey {
			t.Fatal("expected description change to reuse the same metadata identity")
		}
		if first.ReplacementRank == second.ReplacementRank {
			t.Fatal("expected description change to alter replacement rank")
		}
	})

	t.Run("schema url changes keep identity but change replacement rank", func(t *testing.T) {
		first := makeTestGaugeMetadataRow(nil, nil, []*commonpb.KeyValue{stringAttr("cpu", "0")}, "https://resource.schema/1", "https://scope.schema/1", "CPU utilization", "%")
		second := makeTestGaugeMetadataRow(nil, nil, []*commonpb.KeyValue{stringAttr("cpu", "0")}, "https://resource.schema/2", "https://scope.schema/2", "CPU utilization", "%")

		if first.MetadataKey != second.MetadataKey {
			t.Fatal("expected schema URL changes to reuse the same metadata identity")
		}
		if first.ReplacementRank == second.ReplacementRank {
			t.Fatal("expected schema URL changes to alter replacement rank")
		}
	})
}

func makeTestGaugeMetadataRow(resourceAttrs []*commonpb.KeyValue, scopeAttrs []*commonpb.KeyValue, attrs []*commonpb.KeyValue, resourceSchemaURL string, scopeSchemaURL string, description string, unit string) MetricMetadataRow {
	if resourceAttrs == nil {
		resourceAttrs = []*commonpb.KeyValue{stringAttr("service.name", "svc")}
	}

	scope := &commonpb.InstrumentationScope{
		Name:                   "test-scope",
		Version:                "1.0.0",
		DroppedAttributesCount: 3,
		Attributes:             scopeAttrs,
	}

	metric := &metricspb.Metric{
		Name:        "test.metric",
		Description: description,
		Unit:        unit,
	}

	return newMetricMetadataRow(
		serviceNameFromAttrs(resourceAttrs),
		kvToMap(resourceAttrs),
		resourceSchemaURL,
		scope,
		kvToMap(scopeAttrs),
		scopeSchemaURL,
		metric,
		kvToMap(attrs),
	)
}

func makeTestSumMetadataRow(resourceAttrs []*commonpb.KeyValue, scopeAttrs []*commonpb.KeyValue, attrs []*commonpb.KeyValue, resourceSchemaURL string, scopeSchemaURL string, description string, unit string, temporality metricspb.AggregationTemporality, monotonic bool) SumMetadataRow {
	if resourceAttrs == nil {
		resourceAttrs = []*commonpb.KeyValue{stringAttr("service.name", "svc")}
	}

	scope := &commonpb.InstrumentationScope{
		Name:                   "test-scope",
		Version:                "1.0.0",
		DroppedAttributesCount: 3,
		Attributes:             scopeAttrs,
	}

	metric := &metricspb.Metric{
		Name:        "test.metric",
		Description: description,
		Unit:        unit,
	}

	return newSumMetadataRow(
		serviceNameFromAttrs(resourceAttrs),
		kvToMap(resourceAttrs),
		resourceSchemaURL,
		scope,
		kvToMap(scopeAttrs),
		scopeSchemaURL,
		metric,
		&metricspb.Sum{AggregationTemporality: temporality, IsMonotonic: monotonic},
		kvToMap(attrs),
	)
}

func serviceNameFromAttrs(attrs []*commonpb.KeyValue) string {
	for _, attr := range attrs {
		if attr.GetKey() == "service.name" {
			return attr.GetValue().GetStringValue()
		}
	}
	return ""
}

func stringAttr(key string, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{
		Key: key,
		Value: &commonpb.AnyValue{
			Value: &commonpb.AnyValue_StringValue{StringValue: value},
		},
	}
}
