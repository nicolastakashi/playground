package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
)

type dash0MetricsServiceServer struct {
	addr  string
	store MetricsStore

	colmetricspb.UnimplementedMetricsServiceServer
}

func newServer(addr string, store MetricsStore) colmetricspb.MetricsServiceServer {
	return &dash0MetricsServiceServer{addr: addr, store: store}
}

func (m *dash0MetricsServiceServer) Export(ctx context.Context, request *colmetricspb.ExportMetricsServiceRequest) (*colmetricspb.ExportMetricsServiceResponse, error) {
	start := time.Now()
	resourceMetrics := request.GetResourceMetrics()
	ctx, span := tracer.Start(ctx, "metrics.export", trace.WithAttributes(exportSpanAttributes(m.addr, len(resourceMetrics))...))
	defer span.End()

	slog.InfoContext(ctx, "Received ExportMetricsServiceRequest", slog.Int("com.dash0.resource_metrics_count", len(resourceMetrics)))
	metricsReceivedCounter.Add(ctx, 1)

	gaugeRows := MapNormalizedGaugeRows(resourceMetrics)
	sumRows := MapNormalizedSumRows(resourceMetrics)
	totalMetadataRows := len(gaugeRows.Metadata) + len(sumRows.Metadata)
	totalDataPoints := len(gaugeRows.DataPoints) + len(sumRows.DataPoints)

	span.SetAttributes(
		attribute.Int("com.dash0.export.metadata_rows", totalMetadataRows),
		attribute.Int("com.dash0.export.datapoints", totalDataPoints),
	)
	span.AddEvent("mapping.completed", trace.WithAttributes(
		attribute.Int("com.dash0.gauge.metadata_rows", len(gaugeRows.Metadata)),
		attribute.Int("com.dash0.gauge.datapoints", len(gaugeRows.DataPoints)),
		attribute.Int("com.dash0.sum.metadata_rows", len(sumRows.Metadata)),
		attribute.Int("com.dash0.sum.datapoints", len(sumRows.DataPoints)),
	))

	slog.InfoContext(ctx,
		"Mapped export request",
		slog.Int("com.dash0.gauge_metadata_rows", len(gaugeRows.Metadata)),
		slog.Int("com.dash0.gauge_datapoints", len(gaugeRows.DataPoints)),
		slog.Int("com.dash0.sum_metadata_rows", len(sumRows.Metadata)),
		slog.Int("com.dash0.sum_datapoints", len(sumRows.DataPoints)),
	)

	outcome := "success"
	defer func() {
		recordExportTelemetry(ctx, time.Since(start), outcome, len(gaugeRows.DataPoints), len(sumRows.DataPoints))
		span.SetAttributes(outcomeAttr(outcome))
		if outcome == "error" {
			span.SetStatus(codes.Error, "export failed")
		} else {
			span.SetStatus(codes.Ok, "")
		}
	}()

	if m.store != nil {
		var wg sync.WaitGroup
		errCh := make(chan error, 2)

		if len(gaugeRows.DataPoints) > 0 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := m.store.InsertGaugeMetadata(ctx, gaugeRows.Metadata); err != nil {
					errCh <- err
					return
				}
				errCh <- m.store.InsertGaugeDataPoints(ctx, gaugeRows.DataPoints)
			}()
		}
		if len(sumRows.DataPoints) > 0 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := m.store.InsertSumMetadata(ctx, sumRows.Metadata); err != nil {
					errCh <- err
					return
				}
				errCh <- m.store.InsertSumDataPoints(ctx, sumRows.DataPoints)
			}()
		}

		wg.Wait()
		close(errCh)

		for err := range errCh {
			if err != nil {
				outcome = "error"
				span.RecordError(err)
				slog.ErrorContext(ctx,
					"ExportMetricsServiceRequest failed",
					slog.Int("com.dash0.gauge_metadata_rows", len(gaugeRows.Metadata)),
					slog.Int("com.dash0.gauge_datapoints", len(gaugeRows.DataPoints)),
					slog.Int("com.dash0.sum_metadata_rows", len(sumRows.Metadata)),
					slog.Int("com.dash0.sum_datapoints", len(sumRows.DataPoints)),
					slog.Any("com.dash0.error", err),
				)
				return nil, err
			}
		}
	}

	slog.InfoContext(ctx,
		"ExportMetricsServiceRequest completed",
		slog.String("com.dash0.outcome", outcome),
		slog.Int("com.dash0.metadata_rows", totalMetadataRows),
		slog.Int("com.dash0.datapoints", totalDataPoints),
	)

	return &colmetricspb.ExportMetricsServiceResponse{}, nil
}
