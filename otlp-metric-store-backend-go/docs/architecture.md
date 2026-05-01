# Architecture

## Overview

OTLP Metric Store Backend is a Go gRPC service that receives OpenTelemetry metrics via the OTLP protocol and persists them to ClickHouse.

## Component Diagram

```
                    OTLP/gRPC (port 4317)
                           |
                    +------+------+
                    |  gRPC Server |  (server.go)
                    +------+------+
                           |
                    +------+------+
                    | MetricsSvc  |  (metrics_service.go)
                    |   .Export() |
                    +------+------+
                           |
              +------------+------------+
              |                         |
     +--------+--------+      +--------+--------+
     | MapGaugeRows()  |      |  MapSumRows()   |  (metrics_mapper.go)
     +--------+--------+      +--------+--------+
              |                         |
              +------------+------------+
                           |
                    +------+------+
                    | MetricsStore |  (clickhouse_client.go)
                    |  interface   |
                    +------+------+
                           |
                    +------+------+
                    |  ClickHouse  |
                    +-------------+
```

## Components

### server.go

Entry point. Configures and starts the gRPC server.

- Listens on `localhost:4317` (configurable via `-listenAddr` flag)
- Uses `grpc.MaxRecvMsgSize` of 16MB (configurable via `-maxReceiveMessageSize`)
- Instrumented with `otelgrpc` for distributed tracing
- Uses insecure credentials (no TLS)
- Registers an `ExportMetricsServiceRequest` counter (`com.dash0.homeexercise.metrics.received`)

### otel.go

Bootstraps the OpenTelemetry SDK with stdout exporters for traces, metrics, and logs.

- Resource: `service.name=otlp-metrics-processor-backend`, namespace `dash0-exercise`
- Trace sampler: `AlwaysSample`, batch timeout 1s
- Metric reader: periodic, 10s interval
- Log processor: batch

### metrics_service.go

Implements the `colmetricspb.MetricsServiceServer` interface.

- `dash0MetricsServiceServer` holds an address and a `MetricsStore`
- `Export()` receives the request, increments the received counter, and routes gauge/sum rows to the store
- The store is optional (nil-safe)

### metrics_mapper.go

Maps OTLP proto structures to domain row types.

- `serviceName()` — extracts `service.name` from resource attributes
- `kvToMap()` — converts `[]*commonpb.KeyValue` to `map[string]string`
- `anyValueToString()` — converts `AnyValue` to string
- `nanosToTime()` — converts nanoseconds to `time.Time`
- `numberDataPointValue()` — extracts float64 from `NumberDataPoint`
- `MapGaugeRows()` — extracts all gauge data points into `[]GaugeRow`
- `MapSumRows()` — extracts all sum data points into `[]SumRow`

### clickhouse_client.go

ClickHouse persistence layer.

- `MetricsStore` interface: `CreateTables`, `InsertGauge`, `InsertSum`, `Close`
- `ClickHouseMetricsStore` implements batch inserts using `clickhouse-go/v2`
- Connection options: 60s max execution time, 5s dial timeout

### clickhouse_schema.go

DDL constants for 5 ClickHouse tables:

| Table                              | Metric Type             | Insert Implemented |
| ---------------------------------- | ----------------------- | ------------------ |
| `otel_metrics_gauge`               | Gauge                   | Yes                |
| `otel_metrics_sum`                 | Sum                     | Yes                |
| `otel_metrics_histogram`           | Histogram               | No                 |
| `otel_metrics_exponential_histogram` | Exponential Histogram | No                 |
| `otel_metrics_summary`             | Summary                 | No                 |

All tables share the same base schema pattern:
- `MergeTree` engine
- Partitioned by `toDate(TimeUnix)`
- Ordered by `(ServiceName, MetricName, Attributes, toUnixTimestamp64Nano(TimeUnix))`
- ZSTD(1) compression on all columns
- Bloom filter indexes on all attribute map keys/values

## Data Types

### GaugeRow

Core data point representation shared across metric types:

```
ResourceAttributes, ResourceSchemaUrl, ScopeName, ScopeVersion,
ScopeAttributes, ScopeDroppedAttrCount, ScopeSchemaUrl, ServiceName,
MetricName, MetricDescription, MetricUnit, Attributes,
StartTimeUnix, TimeUnix, Value, Flags
```

### SumRow

Embeds `GaugeRow` and adds:

```
AggregationTemporality (Int32), IsMonotonic (Bool)
```
