## ADDED Requirements

### Requirement: Persist reusable metric metadata separately from datapoints
The system SHALL persist gauge and sum metric metadata in dedicated per-kind lookup tables instead of embedding full metadata inline in every datapoint row. The stored metadata SHALL include the resource, scope, metric, and datapoint attribute fields needed to reconstruct the current denormalized context for a datapoint, and the sum metadata table SHALL additionally store sum-specific semantic fields.

#### Scenario: Gauge datapoint writes metadata and datapoint rows separately
- **WHEN** the service receives a gauge datapoint with valid resource, scope, metric, and datapoint attributes
- **THEN** the system MUST persist the metadata in the gauge metadata table and persist the datapoint in the gauge table with a reference to that metadata record

#### Scenario: Sum datapoint writes metadata and datapoint rows separately
- **WHEN** the service receives a sum datapoint with valid resource, scope, metric, and datapoint attributes
- **THEN** the system MUST persist the metadata in the sum metadata table and persist the datapoint in the sum table with a reference to that metadata record

#### Scenario: Metadata writes do not require synchronous uniqueness checks
- **WHEN** the service receives datapoints whose metadata identity has been seen before
- **THEN** the system MUST be allowed to append metadata records without a synchronous existence check before writing datapoint references

### Requirement: Reuse metadata identity for identical metric series
The system SHALL derive a deterministic 128-bit metadata identity from the normalized identifying metadata fields for a metric series within its corresponding metadata lookup table. Logically identical metadata MUST resolve to the same identity even when OTLP attribute ordering differs.

#### Scenario: Reordered attributes resolve to the same metadata identity
- **WHEN** two datapoints carry the same metadata values but present resource, scope, or datapoint attributes in different orders
- **THEN** the system MUST resolve both datapoints to the same metadata identity

#### Scenario: Repeated series reuse an existing metadata record
- **WHEN** multiple datapoints in one or more export requests describe the same metric metadata
- **THEN** the system MUST reference a single logical metadata identity for those datapoints rather than creating a distinct identity per datapoint

#### Scenario: Gauge and sum series do not share a lookup record
- **WHEN** a gauge datapoint and a sum datapoint share the same resource, scope, metric name, metric unit, and attribute values
- **THEN** the system MUST persist or reference separate metadata records in the gauge and sum metadata tables rather than sharing one lookup record across kinds

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

#### Scenario: Sum semantic change creates a new metadata identity
- **WHEN** a sum datapoint arrives for the same metric name but with changed aggregation temporality or monotonicity
- **THEN** the system MUST preserve that change as a distinct metadata identity

### Requirement: Preserve non-identifying metadata outside the identity boundary
The system SHALL store `MetricDescription` and schema URL fields as descriptive metadata in the lookup tables without treating them as identity-defining fields.

#### Scenario: Description is retained as descriptive metadata
- **WHEN** a datapoint is persisted with a metric description
- **THEN** the system MUST store that description in the metadata lookup record without using it to derive metadata identity

#### Scenario: Schema URLs are retained as descriptive metadata
- **WHEN** a datapoint is persisted with resource or scope schema URLs
- **THEN** the system MUST store those schema URLs in the metadata lookup record without using them to derive metadata identity

### Requirement: Metadata deduplication SHALL be eventual and deterministic
The system SHALL use metadata storage strategies for the gauge and sum metadata tables that tolerate duplicate physical rows for the same metadata identity during ingestion and converge to a single logical metadata record through eventual replacement semantics.

#### Scenario: Duplicate metadata rows remain logically valid during ingestion
- **WHEN** multiple metadata rows with the same metadata identity are inserted before ClickHouse merges them
- **THEN** datapoint rows referencing that identity MUST remain logically valid without waiting for immediate physical deduplication

#### Scenario: Replacement semantics converge on one logical metadata record
- **WHEN** duplicate metadata rows exist for the same identity with different non-identifying metadata values
- **THEN** the system MUST apply a deterministic whole-row replacement policy derived from canonical non-identifying metadata so the surviving logical metadata record is predictable independent of insert order

### Requirement: Support metadata-first series discovery
The system SHALL allow normalized gauge and sum reads to discover matching metric series from the metadata lookup tables before fetching datapoints. The discovery step SHALL return the logical metadata rows and their `MetadataKey` values without requiring datapoint rows to duplicate the metadata payload inline.

#### Scenario: Query identifies matching metric series before fetching datapoints
- **WHEN** a query filters by metric, resource, scope, service, or datapoint attribute metadata and needs to know which series exist
- **THEN** the system MUST be able to return matching metadata rows together with their `MetadataKey` values for a later datapoint query

### Requirement: Keep datapoint storage optimized for time-bounded queries
The system SHALL store gauge and sum datapoints in time-partitioned tables and SHALL support fetching datapoints for a specified time range, optionally constrained by a previously selected set of `MetadataKey` values, without requiring full table scans of all datapoints.

#### Scenario: Datapoint tables remain partitioned by event date
- **WHEN** the ClickHouse schema is created for normalized datapoint storage
- **THEN** the gauge and sum datapoint tables MUST partition rows using the datapoint event time

#### Scenario: Datapoint rows do not duplicate full metadata payloads
- **WHEN** a normalized datapoint row is persisted
- **THEN** the row MUST contain the datapoint value fields and a metadata reference instead of repeating the full metadata maps and descriptive fields inline

#### Scenario: Time-bounded datapoint reads use selected metadata keys
- **WHEN** a gauge or sum query fetches datapoints for a specific time range after determining a relevant set of `MetadataKey` values
- **THEN** the query MUST apply the requested time predicate to the datapoint table and use those `MetadataKey` values to constrain the datapoint rows it reads

#### Scenario: Supported normalized query flow is documented and verifiable
- **WHEN** the repository defines the normalized query path for gauge or sum reads
- **THEN** the documented or shared query pattern MUST make the metadata-discovery step and the time-bounded datapoint retrieval step explicit so integration tests can verify the intended flow
