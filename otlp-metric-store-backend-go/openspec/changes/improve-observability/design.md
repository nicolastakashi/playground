## Context

The service currently exposes only minimal observability. gRPC server spans are enabled through `otelgrpc`, but application telemetry is limited to a single request counter and sparse debug logs. The OpenTelemetry SDK is configured with stdout exporters only, and this change keeps that model in place.

This change crosses startup, request handling, storage integration, tests, and documentation. It also touches OpenTelemetry resource configuration and semantic conventions, so decisions made here need to be consistent across the service.

## Goals / Non-Goals

**Goals:**
- Instrument the ingest path with actionable metrics, spans, and structured logs for request volume, datapoint volume, latency, and failures.
- Align resource attributes and emitted telemetry fields with the latest stable OpenTelemetry semantic conventions supported by the Go dependencies in the repository.
- Document the observability contract exposed by the service and explicitly capture follow-up adoption guidance for Weaver and CI-based contract testing.

**Non-Goals:**
- Redesign the ingest pipeline around asynchronous queues, retries, or background workers.
- Introduce a metrics query API, dashboards, or alert rules in this repository.
- Add observability support for histogram, exponential histogram, or summary ingestion beyond the existing gauge and sum scope.
- Change the service from stdout-based telemetry export to an external collector or backend integration.

## Decisions

### Keep stdout exporters and improve the emitted signals
The service will keep the current stdout exporters for traces, metrics, and logs. The change will focus on improving what the service emits through those pipelines rather than adding exporter configurability.

Rationale:
- Exporter changes are out of scope for this iteration.
- Keeping stdout mode avoids startup and configuration complexity while still allowing instrumentation work to proceed.

Alternatives considered:
- Add configurable OTLP exporters: rejected for this change because the requested scope is to retain today's stdout-only behavior.
- Add a vendor-specific exporter directly: rejected because exporter integration is out of scope.

### Instrument the request pipeline at application boundaries instead of every helper
Instrumentation will focus on stable boundaries: request receipt, mapping completion, metadata insert, datapoint insert, and request completion. The service will emit counters and histograms with low-cardinality attributes such as metric kind, outcome, and storage step.

Rationale:
- Boundary instrumentation captures throughput and latency with limited cardinality risk.
- It avoids scattering telemetry across mapper internals that are likely to change.
- The repository does not have an established external dashboard contract, so consistency with the existing application metric naming is the safest baseline.

Alternatives considered:
- Instrument every mapping helper: rejected because it adds noise and maintenance cost.
- Rely on gRPC middleware spans only: rejected because middleware does not expose domain-specific row counts or storage steps.

### Standardize on the latest stable semantic convention version supported by the repo
The implementation will update OpenTelemetry dependencies as needed and use the latest stable semantic convention package supported by those versions for resource attributes and span/log field naming.

Rationale:
- This keeps telemetry interoperable with current tooling and avoids drifting field names.
- The repository already hard-codes a semantic convention version, so making the upgrade explicit reduces ambiguity.

Alternatives considered:
- Leave the current version in place: rejected because the requested change explicitly requires latest-version alignment.
- Invent custom attribute names for new telemetry: rejected except where no stable semantic convention exists.

### Document the observability contract and recommend stronger governance follow-up
This change will document the emitted signals in the repository. It will also capture that Weaver and observability-by-design contract testing in CI are follow-up considerations only and are not being implemented in this change.

Rationale:
- The immediate gap is lack of clear signal documentation and an explicit observability contract.
- Weaver and contract-testing adoption is valuable, but it is a broader process and tooling decision than this change needs to implement.

Alternatives considered:
- Implement contract-testing and Weaver adoption now: rejected because it expands scope beyond the current instrumentation and readiness work.

## Risks / Trade-offs

- Higher telemetry volume from per-request instrumentation -> Mitigation: keep attributes low-cardinality and emit aggregate metrics instead of per-series dimensions.
- Semantic convention upgrades may require dependency and field-name changes -> Mitigation: scope the upgrade to the service resource and newly added telemetry, and validate build/test compatibility.
- Logs, traces, and metrics may duplicate some information -> Mitigation: define clear ownership for each signal type and avoid high-volume debug logs in steady state.
- Documentation without automated contract enforcement may drift over time -> Mitigation: document recommended follow-up adoption of Weaver and CI contract testing.

## Migration Plan

1. Update the OTel bootstrap path for semantic convention alignment while preserving stdout exporters.
2. Add ingest and storage instrumentation while preserving the existing synchronous request behavior.
3. Update README with signal names and follow-up recommendations for Weaver and CI contract testing.
4. Roll back by removing the new instrumentation if issues appear during rollout.

## Open Questions

- None for this change.
