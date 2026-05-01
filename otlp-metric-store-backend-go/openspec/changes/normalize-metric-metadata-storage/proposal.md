## Why

The current ingest path stores full metric metadata on every gauge and sum datapoint, which increases write amplification and duplicates low-cardinality metadata across time-series rows. The assignment now requires a metadata lookup model, so this change is needed to separate stable metric identity from high-volume datapoint storage while preserving efficient time-bounded queries.

## What Changes

- Introduce a metadata lookup table that stores normalized metric identity separately from non-identifying descriptive metadata and datapoint rows.
- Change gauge and sum datapoint storage to reference metadata rows instead of embedding full resource, scope, metric, and attribute metadata inline.
- Add deterministic metadata identity generation using canonicalized identifying metadata hashed to a compact 128-bit key so the ingest path can reuse metadata rows with negligible collision risk while keeping OpenTelemetry non-identifying fields outside the identity boundary.
- Use eventual-consistency deduplication for metadata records so ingestion can stay append-oriented and high-throughput without synchronous uniqueness checks.
- Update ClickHouse schema, ingest logic, and test coverage to validate metadata reuse and correct behavior when metric metadata changes over time.

## Capabilities

### New Capabilities
- `metric-metadata-lookup`: Store reusable metric metadata separately from datapoints and reference it from gauge and sum records.

### Modified Capabilities

## Impact

- Affected code: `clickhouse_schema.go`, `clickhouse_client.go`, `metrics_mapper.go`, `metrics_service.go`, and integration tests.
- Affected systems: ClickHouse schema and write path for gauge and sum metrics.
- API impact: no gRPC API contract changes, but persisted storage layout and query shape change.
- Operational impact: ingest adds metadata resolution and tests must validate lookup reuse, deduplication behavior, and time-range query alignment.
