package main

import (
	"context"
	"log/slog"
	"sync"

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
		gaugeRows := MapNormalizedGaugeRows(rm)
		sumRows := MapNormalizedSumRows(rm)

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
				return nil, err
			}
		}
	}

	return &colmetricspb.ExportMetricsServiceResponse{}, nil
}
