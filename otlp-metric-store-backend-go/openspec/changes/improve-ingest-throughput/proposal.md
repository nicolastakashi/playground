## Why

The current write path already does per-request batch inserts and deduplicates metadata within a request, which is a solid baseline, but it still processes gauge and sum persistence synchronously and leaves its throughput and failure semantics mostly implicit. This change is needed to improve ingestion throughput in a small, deliberate way without introducing premature complexity such as async buffering, retries, or cross-request write coalescing.

## What Changes

- Add an explicit normalized ingestion contract for gauge and sum exports that preserves request-scoped batching and allows independent metric kinds to be persisted concurrently within one export request.
- Define the scoped write guarantees for the first throughput-focused implementation: metadata is written before datapoints within a kind, storage errors fail the export, and the service does not promise transactional atomicity, background retries, or idempotent replay handling.
- Add focused tests for mixed gauge and sum export requests so the repository verifies the intended batching, per-kind sequencing, and non-goals of the write path.

## Capabilities

### New Capabilities
- `normalized-metric-ingestion`: Covers request-scoped batching, per-kind persistence flow, and the bounded throughput guarantees for gauge and sum export ingestion.

### Modified Capabilities

## Impact

- Affected code: `metrics_service.go`, `clickhouse_client.go` only if small helper extraction is warranted, and relevant tests such as `server_test.go` and `integration_test.go`.
- Affected systems: ClickHouse write behavior for normalized gauge and sum ingestion.
- API impact: no OTLP/gRPC contract changes; the change affects how one export request is persisted internally.
- Operational impact: throughput improves only within the existing synchronous request lifecycle; async buffering, retry/idempotency strategy, transactional guarantees, and cross-request write coalescing remain explicit non-goals.
