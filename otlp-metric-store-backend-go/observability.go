package main

import (
	"context"
	"log/slog"
	"net"
	"strconv"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.39.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	metricKindKey           = attribute.Key("com.dash0.metric.kind")
	storageOperationKey     = attribute.Key("com.dash0.storage.operation")
	outcomeKey              = attribute.Key("com.dash0.outcome")
	resourceMetricsCountKey = attribute.Key("com.dash0.resource_metrics.count")
	dbBatchSizeKey          = attribute.Key("com.dash0.db.batch.size")
)

var (
	tracer = otel.Tracer(name)

	metricsReceivedCounter     metric.Int64Counter
	exportsCompletedCounter    metric.Int64Counter
	datapointsProcessedCounter metric.Int64Counter
	exportDurationHistogram    metric.Float64Histogram
	storageRowsCounter         metric.Int64Counter
	storageFailuresCounter     metric.Int64Counter
	storageDurationHistogram   metric.Float64Histogram
)

func init() {
	var err error

	metricsReceivedCounter, err = meter.Int64Counter(
		"com.dash0.homeexercise.metrics.received",
		metric.WithDescription("The number of OTLP export requests received by otlp-metrics-processor-backend"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		panic(err)
	}

	exportsCompletedCounter, err = meter.Int64Counter(
		"com.dash0.homeexercise.metrics.exports",
		metric.WithDescription("The number of OTLP export requests completed by otlp-metrics-processor-backend"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		panic(err)
	}

	datapointsProcessedCounter, err = meter.Int64Counter(
		"com.dash0.homeexercise.metrics.datapoints",
		metric.WithDescription("The number of OTLP datapoints processed by otlp-metrics-processor-backend"),
		metric.WithUnit("{datapoint}"),
	)
	if err != nil {
		panic(err)
	}

	exportDurationHistogram, err = meter.Float64Histogram(
		"com.dash0.homeexercise.metrics.export.duration",
		metric.WithDescription("The time spent processing an OTLP export request"),
		metric.WithUnit("s"),
	)
	if err != nil {
		panic(err)
	}

	storageRowsCounter, err = meter.Int64Counter(
		"com.dash0.homeexercise.metrics.store.rows",
		metric.WithDescription("The number of rows written to ClickHouse by otlp-metrics-processor-backend"),
		metric.WithUnit("{row}"),
	)
	if err != nil {
		panic(err)
	}

	storageFailuresCounter, err = meter.Int64Counter(
		"com.dash0.homeexercise.metrics.store.failures",
		metric.WithDescription("The number of ClickHouse write failures in otlp-metrics-processor-backend"),
		metric.WithUnit("{operation}"),
	)
	if err != nil {
		panic(err)
	}

	storageDurationHistogram, err = meter.Float64Histogram(
		"com.dash0.homeexercise.metrics.store.duration",
		metric.WithDescription("The time spent writing ClickHouse batches in otlp-metrics-processor-backend"),
		metric.WithUnit("s"),
	)
	if err != nil {
		panic(err)
	}
}

func metricKindAttr(kind string) attribute.KeyValue {
	return metricKindKey.String(kind)
}

func storageOperationAttr(operation string) attribute.KeyValue {
	return storageOperationKey.String(operation)
}

func outcomeAttr(outcome string) attribute.KeyValue {
	return outcomeKey.String(outcome)
}

func exportSpanAttributes(addr string, resourceMetricsCount int) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		resourceMetricsCountKey.Int(resourceMetricsCount),
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return append(attrs, semconv.ServerAddressKey.String(addr))
	}

	attrs = append(attrs, semconv.ServerAddressKey.String(host))
	if parsedPort, err := strconv.Atoi(port); err == nil {
		attrs = append(attrs, semconv.ServerPortKey.Int(parsedPort))
	}

	return attrs
}

func recordExportTelemetry(ctx context.Context, duration time.Duration, outcome string, gaugeDataPoints int, sumDataPoints int) {
	attrs := metric.WithAttributes(outcomeAttr(outcome))
	exportsCompletedCounter.Add(ctx, 1, attrs)
	exportDurationHistogram.Record(ctx, duration.Seconds(), attrs)

	if gaugeDataPoints > 0 {
		datapointsProcessedCounter.Add(ctx, int64(gaugeDataPoints), metric.WithAttributes(metricKindAttr("gauge")))
	}
	if sumDataPoints > 0 {
		datapointsProcessedCounter.Add(ctx, int64(sumDataPoints), metric.WithAttributes(metricKindAttr("sum")))
	}
}

func observeStoreOperation(ctx context.Context, metricKind string, operation string, rowCount int, run func(context.Context) error) error {
	ctx, span := tracer.Start(ctx,
		"clickhouse."+metricKind+"."+operation,
		trace.WithAttributes(
			metricKindAttr(metricKind),
			storageOperationAttr(operation),
			dbBatchSizeKey.Int(rowCount),
		),
	)
	defer span.End()

	start := time.Now()
	err := run(ctx)
	outcome := "success"
	if err != nil {
		outcome = "error"
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		storageFailuresCounter.Add(ctx, 1, metric.WithAttributes(metricKindAttr(metricKind), storageOperationAttr(operation)))
		slog.ErrorContext(ctx,
			"ClickHouse write failed",
			slog.String("com.dash0.metric.kind", metricKind),
			slog.String("com.dash0.storage.operation", operation),
			slog.Int("com.dash0.row_count", rowCount),
			slog.Any("com.dash0.error", err),
		)
	} else {
		storageRowsCounter.Add(ctx, int64(rowCount), metric.WithAttributes(metricKindAttr(metricKind), storageOperationAttr(operation)))
		span.SetStatus(codes.Ok, "")
	}

	storageDurationHistogram.Record(
		ctx,
		time.Since(start).Seconds(),
		metric.WithAttributes(metricKindAttr(metricKind), storageOperationAttr(operation), outcomeAttr(outcome)),
	)

	return err
}
