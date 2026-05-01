## 1. Schema and row model

- [ ] 1.1 Add a ClickHouse metadata lookup table schema for normalized metric metadata using `ReplacingMergeTree`, including a compact 128-bit metadata key column, replacement/version fields, and separate non-identifying fields for description and schema URLs.
- [ ] 1.2 Replace denormalized gauge and sum table schemas with datapoint-focused schemas that store the 128-bit metadata reference plus measurement fields.
- [ ] 1.3 Introduce Go row types for metadata records and normalized gauge and sum datapoint records.

## 2. Metadata identity and mapping

- [ ] 2.1 Implement deterministic metadata normalization and 128-bit identity generation over identifying fields only, stable across attribute ordering.
- [ ] 2.2 Refactor metric mapping so gauge and sum datapoints produce normalized metadata plus datapoint rows instead of fully denormalized rows.
- [ ] 2.3 Ensure identifying metadata changes produce a new identity while description and schema URL changes reuse the same identity and rely on replacement semantics for eventual convergence.

## 3. Store write path

- [ ] 3.1 Extend the `MetricsStore` interface and ClickHouse store implementation to insert metadata records.
- [ ] 3.2 Update gauge and sum insert paths to append metadata rows without synchronous uniqueness checks before sending datapoint rows with metadata references.
- [ ] 3.3 Refactor the gRPC export flow to use the normalized write path for both gauge and sum metrics and apply a merge policy for non-identifying metadata conflicts.

## 4. Validation

- [ ] 4.1 Add unit tests for metadata normalization and 128-bit identity generation, including reordered-attribute, unit-change, description-change, and schema-URL-change cases.
- [ ] 4.2 Update integration tests to verify metadata table creation, `ReplacingMergeTree` semantics, identifying metadata drift, non-identifying metadata handling, and normalized gauge and sum inserts.
- [ ] 4.3 Update the gRPC-to-ClickHouse integration test to assert datapoint rows reference persisted metadata correctly.
