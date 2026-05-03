# OTLP Metric Storage

This service accepts OTLP metrics over gRPC and stores Gauge and Sum datapoints in ClickHouse using separate lookup tables for metric metadata.

See `docs/storage-design.md` for the detailed schema rationale, query model, write semantics, and current limitations of the normalized storage layout.

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

## Observability

The service keeps the existing stdout-based OpenTelemetry exporters for traces, metrics, and logs. This repository does not configure an external OTLP collector or backend.

### Application metrics

The service emits these application metrics in addition to the gRPC middleware telemetry:

- `com.dash0.homeexercise.metrics.received`: count of OTLP export requests received
- `com.dash0.homeexercise.metrics.exports`: count of completed export requests with a `com.dash0.outcome` attribute
- `com.dash0.homeexercise.metrics.datapoints`: count of processed datapoints with a `com.dash0.metric.kind` attribute
- `com.dash0.homeexercise.metrics.export.duration`: export request latency with a `com.dash0.outcome` attribute
- `com.dash0.homeexercise.metrics.store.rows`: count of rows written to ClickHouse with `com.dash0.metric.kind` and `com.dash0.storage.operation` attributes
- `com.dash0.homeexercise.metrics.store.failures`: count of ClickHouse write failures with `com.dash0.metric.kind` and `com.dash0.storage.operation` attributes
- `com.dash0.homeexercise.metrics.store.duration`: ClickHouse write latency with `com.dash0.metric.kind`, `com.dash0.storage.operation`, and `com.dash0.outcome` attributes

### Traces and logs

- Export requests create an application span named `metrics.export`
- ClickHouse writes create child spans named `clickhouse.<metric-kind>.<operation>`
- Export request logs include aggregate metadata row and datapoint counts
- ClickHouse failure logs identify the metric kind, storage operation, row count, and error
- Startup logs make OpenTelemetry initialization, ClickHouse connection attempts, and listener failures explicit

### Follow-up guidance

Weaver adoption, observability by design, and CI-based observability contract testing are not implemented in this repository change. They are documented as recommended follow-up work once the service's emitted signals stabilize.

## Storage Model

- `otel_metrics_gauge_metadata` and `otel_metrics_sum_metadata` store metric identity and descriptive metadata.
- `otel_metrics_gauge` and `otel_metrics_sum` store only datapoint values, timestamps, flags, and a `MetadataKey` reference.
- Datapoint tables are partitioned by `toDate(TimeUnix)` and ordered by timestamp plus `MetadataKey` to support the repository's time-bounded normalized query flow.

`docs/storage-design.md` is the authoritative explanation of why the metadata tables use `ReplacingMergeTree`, why datapoint tables order by time first, and where the current scan-avoidance guarantees stop.

## Write Flow

Normalized writes stay synchronous and request-scoped:

1. The server maps one export request into normalized gauge and sum batches.
2. Each populated metric kind writes one metadata batch and one datapoint batch for that request.
3. Gauge and sum pipelines may run independently when both kinds are present, but each kind still writes metadata before datapoints.

This is a scoped throughput improvement, not a broader ingestion redesign. The service does not currently provide background buffering, cross-request batching, retries, idempotent replay handling, or transactional all-or-nothing guarantees across tables or metric kinds. If a storage write fails, the export call fails, and rows written earlier in the same request may already be persisted.

## Query Flow

Normalized reads follow a two-step contract:

1. Query the metadata table to discover which series match filters such as service name, metric name, or datapoint attributes, and capture the returned `MetadataKey` values.
2. Query the datapoint table for a time range using those `MetadataKey` values.

This is a behavioral contract, not a single required SQL shape. Metadata discovery may scan metadata tables when needed; the bounded read guarantee applies to the datapoint tables once the relevant `MetadataKey` values are known.

Illustrative example:

```sql
SELECT MetadataKey, ServiceName, MetricName
FROM otel_metrics_gauge_metadata FINAL
WHERE ServiceName = 'checkout' AND MetricName = 'http.server.duration';

SELECT TimeUnix, Value, Flags
FROM otel_metrics_gauge
WHERE MetadataKey IN (<selected keys>)
  AND TimeUnix >= toDateTime64('2026-05-03 10:00:00', 9)
  AND TimeUnix <= toDateTime64('2026-05-03 11:00:00', 9)
ORDER BY TimeUnix;
```
