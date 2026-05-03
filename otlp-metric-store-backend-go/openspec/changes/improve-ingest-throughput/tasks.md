## 1. Restructure the export write path

- [x] 1.1 Update `metrics_service.go` to prepare normalized gauge and sum batches once per export request and skip storage work for empty kinds.
- [x] 1.2 Introduce the smallest helper structure needed to run one metric kind as a metadata-then-datapoint persistence pipeline.
- [x] 1.3 Execute populated gauge and sum pipelines independently within one export call while still returning a storage error when any pipeline fails.

## 2. Verify batching, ordering, and failure behavior

- [x] 2.1 Add unit-test fake store coverage for gauge-only and sum-only requests to verify one metadata batch and one datapoint batch per populated kind.
- [x] 2.2 Add unit tests for mixed requests that verify empty kinds are skipped, metadata remains ordered before datapoints within a kind, and gauge and sum pipelines can start independently.
- [x] 2.3 Add unit or integration coverage that verifies storage errors fail the export without retries and that partial persistence is treated as an accepted non-atomic outcome.

## 3. Document and validate the scoped change

- [x] 3.1 Update `README.md` to describe the synchronous request-scoped ingestion model, the per-kind concurrency improvement, and the explicit non-goals around buffering, retries, idempotency, and transactional guarantees.
- [x] 3.2 Run `make test` and `make test-integration` to validate the updated write-path behavior.
