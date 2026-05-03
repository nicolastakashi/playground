package main

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type MetricsStore interface {
	CreateTables(ctx context.Context) error
	InsertGaugeMetadata(ctx context.Context, rows []MetricMetadataRow) error
	InsertGaugeDataPoints(ctx context.Context, rows []GaugeDataPointRow) error
	InsertSumMetadata(ctx context.Context, rows []SumMetadataRow) error
	InsertSumDataPoints(ctx context.Context, rows []SumDataPointRow) error
	Close() error
}

type ClickHouseMetricsStore struct {
	conn driver.Conn
}

func NewClickHouseMetricsStore(ctx context.Context, addr string, database string, username string, password string) (*ClickHouseMetricsStore, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{addr},
		Auth: clickhouse.Auth{
			Database: database,
			Username: username,
			Password: password,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("opening clickhouse connection: %w", err)
	}
	if err := conn.Ping(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("pinging clickhouse: %w", err)
	}
	return &ClickHouseMetricsStore{conn: conn}, nil
}

func (s *ClickHouseMetricsStore) CreateTables(ctx context.Context) error {
	ddls := []string{
		createGaugeMetadataTableSQL,
		createSumMetadataTableSQL,
		createGaugeTableSQL,
		createSumTableSQL,
		createHistogramTableSQL,
		createExponentialHistogramTableSQL,
		createSummaryTableSQL,
	}
	for _, ddl := range ddls {
		if err := s.conn.Exec(ctx, ddl); err != nil {
			return fmt.Errorf("creating table: %w", err)
		}
	}
	return nil
}

func (s *ClickHouseMetricsStore) InsertGaugeMetadata(ctx context.Context, rows []MetricMetadataRow) error {
	return observeStoreOperation(ctx, "gauge", "metadata", len(rows), func(ctx context.Context) error {
		batch, err := s.conn.PrepareBatch(ctx, "INSERT INTO otel_metrics_gauge_metadata")
		if err != nil {
			return fmt.Errorf("preparing gauge metadata batch: %w", err)
		}
		for _, r := range rows {
			if err := batch.Append(
				r.MetadataKey,
				replacementRankBigInt(r.ReplacementRank),
				r.ResourceAttributes,
				r.ResourceSchemaUrl,
				r.ScopeName,
				r.ScopeVersion,
				r.ScopeAttributes,
				r.ScopeDroppedAttrCount,
				r.ScopeSchemaUrl,
				r.ServiceName,
				r.MetricName,
				r.MetricDescription,
				r.MetricUnit,
				r.Attributes,
			); err != nil {
				return fmt.Errorf("appending gauge metadata row: %w", err)
			}
		}
		if err := batch.Send(); err != nil {
			return fmt.Errorf("sending gauge metadata batch: %w", err)
		}
		return nil
	})
}

func (s *ClickHouseMetricsStore) InsertGaugeDataPoints(ctx context.Context, rows []GaugeDataPointRow) error {
	return observeStoreOperation(ctx, "gauge", "datapoints", len(rows), func(ctx context.Context) error {
		batch, err := s.conn.PrepareBatch(ctx, "INSERT INTO otel_metrics_gauge")
		if err != nil {
			return fmt.Errorf("preparing gauge datapoint batch: %w", err)
		}
		for _, r := range rows {
			if err := batch.Append(
				r.MetadataKey,
				r.StartTimeUnix,
				r.TimeUnix,
				r.Value,
				r.Flags,
			); err != nil {
				return fmt.Errorf("appending gauge datapoint row: %w", err)
			}
		}
		if err := batch.Send(); err != nil {
			return fmt.Errorf("sending gauge datapoint batch: %w", err)
		}
		return nil
	})
}

func (s *ClickHouseMetricsStore) InsertSumMetadata(ctx context.Context, rows []SumMetadataRow) error {
	return observeStoreOperation(ctx, "sum", "metadata", len(rows), func(ctx context.Context) error {
		batch, err := s.conn.PrepareBatch(ctx, "INSERT INTO otel_metrics_sum_metadata")
		if err != nil {
			return fmt.Errorf("preparing sum metadata batch: %w", err)
		}
		for _, r := range rows {
			if err := batch.Append(
				r.MetadataKey,
				replacementRankBigInt(r.ReplacementRank),
				r.ResourceAttributes,
				r.ResourceSchemaUrl,
				r.ScopeName,
				r.ScopeVersion,
				r.ScopeAttributes,
				r.ScopeDroppedAttrCount,
				r.ScopeSchemaUrl,
				r.ServiceName,
				r.MetricName,
				r.MetricDescription,
				r.MetricUnit,
				r.Attributes,
				r.AggregationTemporality,
				r.IsMonotonic,
			); err != nil {
				return fmt.Errorf("appending sum metadata row: %w", err)
			}
		}
		if err := batch.Send(); err != nil {
			return fmt.Errorf("sending sum metadata batch: %w", err)
		}
		return nil
	})
}

func (s *ClickHouseMetricsStore) InsertSumDataPoints(ctx context.Context, rows []SumDataPointRow) error {
	return observeStoreOperation(ctx, "sum", "datapoints", len(rows), func(ctx context.Context) error {
		batch, err := s.conn.PrepareBatch(ctx, "INSERT INTO otel_metrics_sum")
		if err != nil {
			return fmt.Errorf("preparing sum datapoint batch: %w", err)
		}
		for _, r := range rows {
			if err := batch.Append(
				r.MetadataKey,
				r.StartTimeUnix,
				r.TimeUnix,
				r.Value,
				r.Flags,
			); err != nil {
				return fmt.Errorf("appending sum datapoint row: %w", err)
			}
		}
		if err := batch.Send(); err != nil {
			return fmt.Errorf("sending sum datapoint batch: %w", err)
		}
		return nil
	})
}

func (s *ClickHouseMetricsStore) Close() error {
	return s.conn.Close()
}

func replacementRankBigInt(rank ReplacementRank) *big.Int {
	return new(big.Int).SetBytes(rank[:])
}
