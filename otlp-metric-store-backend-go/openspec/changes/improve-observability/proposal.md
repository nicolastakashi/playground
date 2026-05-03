## Why

The service has basic OpenTelemetry wiring, but it does not yet provide sufficient observability for the ingest path. We need actionable metrics, traces, logs, and health signals now so operators can understand throughput, failures, storage latency, and service readiness while keeping the current stdout-based export model.

## What Changes

- Add first-class observability for OTLP metric ingestion, including request, mapping, and ClickHouse write telemetry.
- Adopt the latest stable OpenTelemetry semantic conventions used by the Go SDK and apply them consistently to service resource attributes, spans, and log fields.
- Document the emitted observability signals and capture follow-up guidance to evaluate Weaver and observability-by-design contract testing in CI.

## Capabilities

### New Capabilities
- `ingest-observability`: Improved telemetry coverage for metric ingestion, including structured logs, traces, metrics, and semantic convention alignment.

### Modified Capabilities

## Impact

- Affected code: `server.go`, `otel.go`, `metrics_service.go`, `clickhouse_client.go`, tests, and README/run instructions.
- Affected systems: OpenTelemetry SDK configuration and operational dashboards/alerts built on emitted signals.
- Dependencies: OpenTelemetry Go SDK/exporter packages may need updates to align with the latest stable semantic convention version.
