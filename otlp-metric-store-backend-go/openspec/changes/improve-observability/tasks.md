## 1. OpenTelemetry setup

- [x] 1.1 Update OpenTelemetry dependencies and semantic convention imports to the latest stable version supported by the repository.
- [x] 1.2 Keep stdout exporters in `otel.go` while aligning the bootstrap code and emitted metadata with the selected semantic conventions.
- [x] 1.3 Update service resource initialization to use the selected semantic conventions consistently for standard service attributes.

## 2. Ingest instrumentation

- [x] 2.1 Add application metrics for export request counts, datapoint counts, ingest latency, and failure outcomes with low-cardinality attributes.
- [x] 2.2 Instrument the export flow in `metrics_service.go` with request-stage spans, structured logs, and aggregate row-count context.
- [x] 2.3 Instrument ClickHouse metadata and datapoint insert paths with latency and failure telemetry for gauge and sum pipelines.

## 3. Health and readiness

- [x] 3.1 Review startup and failure-path logs to ensure storage initialization problems are clearly observable without adding a readiness endpoint.

## 4. Validation and documentation

- [x] 4.1 Update `README.md` with emitted operational signals and stdout telemetry behavior.
- [x] 4.2 Document recommended follow-up adoption of Weaver and CI-based observability contract testing.
- [x] 4.3 Run `make lint` and any targeted checks needed after the observability changes.
