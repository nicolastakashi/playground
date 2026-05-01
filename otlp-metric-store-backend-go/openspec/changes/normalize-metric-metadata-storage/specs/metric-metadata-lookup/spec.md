## ADDED Requirements

### Requirement: Persist reusable metric metadata separately from datapoints
The system SHALL persist gauge and sum metric metadata in a dedicated lookup table instead of embedding full metadata inline in every datapoint row. The stored metadata SHALL include the resource, scope, metric, and datapoint attribute fields needed to reconstruct the current denormalized context for a datapoint.

#### Scenario: Gauge datapoint writes metadata and datapoint rows separately
- **WHEN** the service receives a gauge datapoint with valid resource, scope, metric, and datapoint attributes
- **THEN** the system MUST persist the metadata in the lookup table and persist the datapoint in the gauge table with a reference to that metadata record

#### Scenario: Sum datapoint writes metadata and datapoint rows separately
- **WHEN** the service receives a sum datapoint with valid resource, scope, metric, and datapoint attributes
- **THEN** the system MUST persist the metadata in the lookup table and persist the datapoint in the sum table with a reference to that metadata record

#### Scenario: Metadata writes do not require synchronous uniqueness checks
- **WHEN** the service receives datapoints whose metadata identity has been seen before
- **THEN** the system MUST be allowed to append metadata records without a synchronous existence check before writing datapoint references

### Requirement: Reuse metadata identity for identical metric series
The system SHALL derive a deterministic 128-bit metadata identity from the normalized identifying metadata fields for a metric series. Logically identical metadata MUST resolve to the same identity even when OTLP attribute ordering differs.

#### Scenario: Reordered attributes resolve to the same metadata identity
- **WHEN** two datapoints carry the same metadata values but present resource, scope, or datapoint attributes in different orders
- **THEN** the system MUST resolve both datapoints to the same metadata identity

#### Scenario: Repeated series reuse an existing metadata record
- **WHEN** multiple datapoints in one or more export requests describe the same metric metadata
- **THEN** the system MUST reference a single logical metadata identity for those datapoints rather than creating a distinct identity per datapoint

#### Scenario: Metadata identity remains compact without using a 64-bit hash
- **WHEN** the system persists metadata references for gauge and sum datapoints
- **THEN** the metadata identity MUST use a compact 128-bit representation rather than a 64-bit hash value

#### Scenario: Description changes do not split metric identity
- **WHEN** two observations have the same identifying metadata and differ only in `MetricDescription`
- **THEN** the system MUST resolve both observations to the same metadata identity

#### Scenario: Schema URL changes do not split metric identity
- **WHEN** two observations have the same identifying metadata and differ only in resource or scope schema URL fields
- **THEN** the system MUST resolve both observations to the same metadata identity

### Requirement: Preserve identifying metadata drift as a new identity
The system SHALL treat a change to any metadata field included in the identifying normalization boundary as a new metadata identity rather than overwriting a previously stored identity.

#### Scenario: Resource attribute change creates a new metadata identity
- **WHEN** a later datapoint for the same metric name arrives with a changed resource attribute value
- **THEN** the system MUST persist or reference a different metadata identity for that datapoint

#### Scenario: Metric unit change creates a new metadata identity
- **WHEN** a datapoint arrives for the same metric name but with a changed metric unit
- **THEN** the system MUST preserve that change as a distinct metadata identity

### Requirement: Preserve non-identifying metadata outside the identity boundary
The system SHALL store `MetricDescription` and schema URL fields as descriptive metadata in the lookup table without treating them as identity-defining fields.

#### Scenario: Description is retained as descriptive metadata
- **WHEN** a datapoint is persisted with a metric description
- **THEN** the system MUST store that description in the metadata lookup record without using it to derive metadata identity

#### Scenario: Schema URLs are retained as descriptive metadata
- **WHEN** a datapoint is persisted with resource or scope schema URLs
- **THEN** the system MUST store those schema URLs in the metadata lookup record without using them to derive metadata identity

### Requirement: Metadata deduplication SHALL be eventual and deterministic
The system SHALL use a metadata storage strategy that tolerates duplicate physical rows for the same metadata identity during ingestion and converges to a single logical metadata record through eventual replacement semantics.

#### Scenario: Duplicate metadata rows remain logically valid during ingestion
- **WHEN** multiple metadata rows with the same metadata identity are inserted before ClickHouse merges them
- **THEN** datapoint rows referencing that identity MUST remain logically valid without waiting for immediate physical deduplication

#### Scenario: Replacement semantics converge on one logical metadata record
- **WHEN** duplicate metadata rows exist for the same identity with different non-identifying metadata values
- **THEN** the system MUST apply a deterministic replacement policy so the surviving logical metadata record is predictable after eventual deduplication

### Requirement: Keep datapoint storage optimized for time-bounded queries
The system SHALL store gauge and sum datapoints in time-partitioned tables that can be queried efficiently for a specified time range without requiring full table scans of all datapoints.

#### Scenario: Datapoint tables remain partitioned by event date
- **WHEN** the ClickHouse schema is created for normalized datapoint storage
- **THEN** the gauge and sum datapoint tables MUST partition rows using the datapoint event time

#### Scenario: Datapoint rows do not duplicate full metadata payloads
- **WHEN** a normalized datapoint row is persisted
- **THEN** the row MUST contain the datapoint value fields and a metadata reference instead of repeating the full metadata maps and descriptive fields inline
