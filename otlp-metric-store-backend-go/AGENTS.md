# AGENTS.md

## Build & Run

```bash
make build          # go build ./...
make run            # go run . (listens on localhost:4317 by default)
```

## Test

```bash
make test                     # unit tests only (no external deps)
make test-integration         # requires Docker (testcontainers — ClickHouse container)
go test -run TestFoo -v .     # single test
```

Integration tests use the `//go:build integration` tag. Run them with `-tags integration`.

## Lint

```bash
make lint   # runs go vet + staticcheck (if installed)
make vet    # go vet only
make fmt    # go fmt
```

Always run `make lint` and `make test` before committing.

## Project Structure

Single `main` package — all `.go` files live at repo root.

| File                   | Role                                                                 |
| ---------------------- | -------------------------------------------------------------------- |
| `server.go`            | Entry point, gRPC server setup, OTel meter + counter                |
| `otel.go`              | OTel SDK bootstrap (stdout exporters for traces/metrics/logs)        |
| `metrics_service.go`   | gRPC `Export()` handler — routes gauge/sum rows to `MetricsStore`    |
| `metrics_mapper.go`    | OTLP proto → `GaugeRow`/`SumRow` conversion                         |
| `clickhouse_client.go` | `MetricsStore` interface + ClickHouse implementation, batch inserts  |
| `clickhouse_schema.go` | DDL constants for 5 ClickHouse tables                                |
| `server_test.go`       | Unit tests (bufconn gRPC, no ClickHouse)                             |
| `integration_test.go`  | Integration tests (testcontainers ClickHouse `clickhouse/clickhouse-server:26.2`) |

## Key Facts

- **Module name** is `dash0.com/otlp-log-processor-backend` (historical — despite the name this is a metrics backend).
- **Only Gauge and Sum** have mapper + insert implementations. Histogram, ExponentialHistogram, and Summary have table schemas in `clickhouse_schema.go` but no mapper or insert logic yet.
- The `MetricsStore` is passed as `nil` in `main()` — ClickHouse is not wired up in the production startup path.
- `SumRow` embeds `GaugeRow` and adds `AggregationTemporality` + `IsMonotonic`.
- All ClickHouse tables: `MergeTree` engine, partitioned by `toDate(TimeUnix)`, ZSTD(1) compression, bloom filter indexes on attribute maps.
- Unit tests use `bufconn` (in-memory gRPC) with no store. Integration tests spin up a real ClickHouse via testcontainers.

## Style

- No comments unless explicitly requested.
- Standard library `log/slog` for logging, `otelslog` bridge for OTel log correlation.
- Flags only for config (no config files).
