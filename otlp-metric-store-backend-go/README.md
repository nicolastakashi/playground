# OTLP Metric Storage

This service accepts OTLP metrics over gRPC and stores Gauge and Sum datapoints in ClickHouse using separate lookup tables for metric metadata.

## Requirements

- Go 1.26
- ClickHouse reachable by the application

## Build

```shell
make build
```

## Run

The server listens on `localhost:4317` by default and connects to ClickHouse at `localhost:9000`.

```shell
go run .
```

You can override the storage connection with flags:

```shell
go run . \
  -listenAddr localhost:4317 \
  -clickhouseAddr localhost:9000 \
  -clickhouseDatabase default \
  -clickhouseUsername default \
  -clickhousePassword ''
```

Passing `-clickhouseAddr ''` starts the gRPC server without ClickHouse wiring.

On startup, the service creates the required ClickHouse tables if they do not already exist.

## Test

```shell
make test
make test-integration
```

## Storage Model

- `otel_metrics_gauge_metadata` and `otel_metrics_sum_metadata` store metric identity and descriptive metadata.
- `otel_metrics_gauge` and `otel_metrics_sum` store only datapoint values, timestamps, flags, and a `MetadataKey` reference.
- Datapoint tables are partitioned by `toDate(TimeUnix)` and ordered by timestamp plus `MetadataKey` to support time-range queries without full table scans.
