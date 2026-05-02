package main

import (
	"context"
	"log/slog"

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
	slog.DebugContext(ctx, "Received ExportMetricsServiceRequest")
	metricsReceivedCounter.Add(ctx, 1)

	if m.store != nil {
		rm := request.GetResourceMetrics()

		if gaugeRows := MapNormalizedGaugeRows(rm); len(gaugeRows.DataPoints) > 0 {
			if err := m.store.InsertGaugeMetadata(ctx, gaugeRows.Metadata); err != nil {
				return nil, err
			}
			if err := m.store.InsertGaugeDataPoints(ctx, gaugeRows.DataPoints); err != nil {
				return nil, err
			}
		}
		if sumRows := MapNormalizedSumRows(rm); len(sumRows.DataPoints) > 0 {
			if err := m.store.InsertSumMetadata(ctx, sumRows.Metadata); err != nil {
				return nil, err
			}
			if err := m.store.InsertSumDataPoints(ctx, sumRows.DataPoints); err != nil {
				return nil, err
			}
		}
	}

	return &colmetricspb.ExportMetricsServiceResponse{}, nil
}
