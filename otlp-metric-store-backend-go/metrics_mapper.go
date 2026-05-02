package main

import (
	"fmt"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

// serviceName extracts the service.name from resource attributes, returning "" if not found.
func serviceName(resource *resourcepb.Resource) string {
	if resource == nil {
		return ""
	}
	for _, attr := range resource.GetAttributes() {
		if attr.GetKey() == "service.name" {
			return attr.GetValue().GetStringValue()
		}
	}
	return ""
}

// kvToMap converts a slice of OTLP KeyValue pairs to a Go map.
func kvToMap(attrs []*commonpb.KeyValue) map[string]string {
	m := make(map[string]string, len(attrs))
	for _, kv := range attrs {
		m[kv.GetKey()] = anyValueToString(kv.GetValue())
	}
	return m
}

// anyValueToString converts an OTLP AnyValue to its string representation.
func anyValueToString(v *commonpb.AnyValue) string {
	if v == nil {
		return ""
	}
	switch v.Value.(type) {
	case *commonpb.AnyValue_StringValue:
		return v.GetStringValue()
	case *commonpb.AnyValue_IntValue:
		return fmt.Sprintf("%d", v.GetIntValue())
	case *commonpb.AnyValue_DoubleValue:
		return fmt.Sprintf("%g", v.GetDoubleValue())
	case *commonpb.AnyValue_BoolValue:
		return fmt.Sprintf("%t", v.GetBoolValue())
	default:
		return fmt.Sprintf("%v", v)
	}
}

// nanosToTime converts a uint64 nanoseconds-since-epoch to time.Time.
func nanosToTime(nanos uint64) time.Time {
	return time.Unix(0, int64(nanos))
}

// numberDataPointValue extracts the float64 value from a NumberDataPoint.
func numberDataPointValue(dp *metricspb.NumberDataPoint) float64 {
	switch v := dp.GetValue().(type) {
	case *metricspb.NumberDataPoint_AsDouble:
		return v.AsDouble
	case *metricspb.NumberDataPoint_AsInt:
		return float64(v.AsInt)
	default:
		return 0
	}
}

func newMetricMetadataRow(
	serviceName string,
	resourceAttributes map[string]string,
	resourceSchemaURL string,
	scope *commonpb.InstrumentationScope,
	scopeAttributes map[string]string,
	scopeSchemaURL string,
	metric *metricspb.Metric,
	attributes map[string]string,
) MetricMetadataRow {
	row := MetricMetadataRow{
		ResourceAttributes:    resourceAttributes,
		ResourceSchemaUrl:     resourceSchemaURL,
		ScopeName:             scope.GetName(),
		ScopeVersion:          scope.GetVersion(),
		ScopeAttributes:       scopeAttributes,
		ScopeDroppedAttrCount: scope.GetDroppedAttributesCount(),
		ScopeSchemaUrl:        scopeSchemaURL,
		ServiceName:           serviceName,
		MetricName:            metric.GetName(),
		MetricDescription:     metric.GetDescription(),
		MetricUnit:            metric.GetUnit(),
		Attributes:            attributes,
	}
	row.MetadataKey = gaugeMetadataKey(row)
	row.ReplacementRank = metadataReplacementRank(row.ResourceSchemaUrl, row.ScopeSchemaUrl, row.MetricDescription)
	return row
}

func newSumMetadataRow(
	serviceName string,
	resourceAttributes map[string]string,
	resourceSchemaURL string,
	scope *commonpb.InstrumentationScope,
	scopeAttributes map[string]string,
	scopeSchemaURL string,
	metric *metricspb.Metric,
	sum *metricspb.Sum,
	attributes map[string]string,
) SumMetadataRow {
	row := SumMetadataRow{
		MetricMetadataRow: newMetricMetadataRow(
			serviceName,
			resourceAttributes,
			resourceSchemaURL,
			scope,
			scopeAttributes,
			scopeSchemaURL,
			metric,
			attributes,
		),
		AggregationTemporality: int32(sum.GetAggregationTemporality()),
		IsMonotonic:            sum.GetIsMonotonic(),
	}
	row.MetadataKey = sumMetadataKey(row)
	return row
}

func MapNormalizedGaugeRows(resourceMetrics []*metricspb.ResourceMetrics) NormalizedGaugeRows {
	rows := NormalizedGaugeRows{}
	for _, rm := range resourceMetrics {
		svcName := serviceName(rm.GetResource())
		resAttrs := kvToMap(rm.GetResource().GetAttributes())
		resSchemaURL := rm.GetSchemaUrl()

		for _, sm := range rm.GetScopeMetrics() {
			scope := sm.GetScope()
			scopeAttrs := kvToMap(scope.GetAttributes())

			for _, metric := range sm.GetMetrics() {
				gauge := metric.GetGauge()
				if gauge == nil {
					continue
				}
				for _, dp := range gauge.GetDataPoints() {
					metadata := newMetricMetadataRow(
						svcName,
						resAttrs,
						resSchemaURL,
						scope,
						scopeAttrs,
						sm.GetSchemaUrl(),
						metric,
						kvToMap(dp.GetAttributes()),
					)
					// We intentionally append duplicate metadata rows within a batch for now; exact intra-batch dedupe can be added later if write volume justifies it.
					rows.Metadata = append(rows.Metadata, metadata)
					rows.DataPoints = append(rows.DataPoints, GaugeDataPointRow{
						MetadataKey:   metadata.MetadataKey,
						StartTimeUnix: nanosToTime(dp.GetStartTimeUnixNano()),
						TimeUnix:      nanosToTime(dp.GetTimeUnixNano()),
						Value:         numberDataPointValue(dp),
						Flags:         dp.GetFlags(),
					})
				}
			}
		}
	}
	return rows
}

func MapNormalizedSumRows(resourceMetrics []*metricspb.ResourceMetrics) NormalizedSumRows {
	rows := NormalizedSumRows{}
	for _, rm := range resourceMetrics {
		svcName := serviceName(rm.GetResource())
		resAttrs := kvToMap(rm.GetResource().GetAttributes())
		resSchemaURL := rm.GetSchemaUrl()

		for _, sm := range rm.GetScopeMetrics() {
			scope := sm.GetScope()
			scopeAttrs := kvToMap(scope.GetAttributes())

			for _, metric := range sm.GetMetrics() {
				sum := metric.GetSum()
				if sum == nil {
					continue
				}
				for _, dp := range sum.GetDataPoints() {
					metadata := newSumMetadataRow(
						svcName,
						resAttrs,
						resSchemaURL,
						scope,
						scopeAttrs,
						sm.GetSchemaUrl(),
						metric,
						sum,
						kvToMap(dp.GetAttributes()),
					)
					// We intentionally append duplicate metadata rows within a batch for now; exact intra-batch dedupe can be added later if write volume justifies it.
					rows.Metadata = append(rows.Metadata, metadata)
					rows.DataPoints = append(rows.DataPoints, SumDataPointRow{
						GaugeDataPointRow: GaugeDataPointRow{
							MetadataKey:   metadata.MetadataKey,
							StartTimeUnix: nanosToTime(dp.GetStartTimeUnixNano()),
							TimeUnix:      nanosToTime(dp.GetTimeUnixNano()),
							Value:         numberDataPointValue(dp),
							Flags:         dp.GetFlags(),
						},
					})
				}
			}
		}
	}
	return rows
}
