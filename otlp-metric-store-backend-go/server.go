package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"log/slog"
	"net"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	listenAddr            = flag.String("listenAddr", "localhost:4317", "The listen address")
	maxReceiveMessageSize = flag.Int("maxReceiveMessageSize", 16777216, "The max message size in bytes the server can receive")
	clickhouseAddr        = flag.String("clickhouseAddr", "localhost:9000", "The ClickHouse address")
	clickhouseDatabase    = flag.String("clickhouseDatabase", "default", "The ClickHouse database")
	clickhouseUsername    = flag.String("clickhouseUsername", "default", "The ClickHouse username")
	clickhousePassword    = flag.String("clickhousePassword", "", "The ClickHouse password")
)

const name = "dash0.com/otlp-log-processor-backend"

var (
	meter  = otel.Meter(name)
	logger = otelslog.NewLogger(name)
)

func main() {
	if err := run(); err != nil {
		log.Fatalln(err)
	}
}

func run() (err error) {
	slog.SetDefault(logger)
	logger.Info("Starting application")

	// Set up OpenTelemetry.
	otelShutdown, err := setupOTelSDK(context.Background())
	if err != nil {
		slog.Error("Failed to initialize OpenTelemetry SDK", slog.Any("com.dash0.error", err))
		return
	}

	// Handle shutdown properly so nothing leaks.
	defer func() {
		err = errors.Join(err, otelShutdown(context.Background()))
	}()

	flag.Parse()
	slog.Info("Configuration loaded",
		slog.String("com.dash0.listen_addr", *listenAddr),
		slog.Int("com.dash0.max_receive_message_size", *maxReceiveMessageSize),
		slog.String("com.dash0.clickhouse_addr", *clickhouseAddr),
	)

	store, err := setupMetricsStore(context.Background())
	if err != nil {
		slog.Error("Failed to initialize metrics store", slog.Any("com.dash0.error", err))
		return err
	}
	if store != nil {
		slog.Info("Metrics store initialized", slog.String("com.dash0.clickhouse_addr", *clickhouseAddr), slog.String("com.dash0.clickhouse_database", *clickhouseDatabase))
		defer func() {
			err = errors.Join(err, store.Close())
		}()
	} else {
		slog.Info("Metrics store disabled")
	}

	slog.Debug("Starting listener", slog.String("com.dash0.listen_addr", *listenAddr))
	listener, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		slog.Error("Failed to start listener", slog.String("com.dash0.listen_addr", *listenAddr), slog.Any("com.dash0.error", err))
		return err
	}

	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.MaxRecvMsgSize(*maxReceiveMessageSize),
		grpc.Creds(insecure.NewCredentials()),
	)
	colmetricspb.RegisterMetricsServiceServer(grpcServer, newServer(*listenAddr, store))

	slog.Debug("Starting gRPC server")

	return grpcServer.Serve(listener)
}

func setupMetricsStore(ctx context.Context) (MetricsStore, error) {
	if *clickhouseAddr == "" {
		return nil, nil
	}

	slog.Info("Connecting to ClickHouse", slog.String("com.dash0.clickhouse_addr", *clickhouseAddr), slog.String("com.dash0.clickhouse_database", *clickhouseDatabase))
	store, err := NewClickHouseMetricsStore(ctx, *clickhouseAddr, *clickhouseDatabase, *clickhouseUsername, *clickhousePassword)
	if err != nil {
		return nil, err
	}

	if err := store.CreateTables(ctx); err != nil {
		_ = store.Close()
		return nil, err
	}

	return store, nil
}
