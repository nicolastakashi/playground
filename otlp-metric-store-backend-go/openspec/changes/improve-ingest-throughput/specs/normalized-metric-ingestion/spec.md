## ADDED Requirements

### Requirement: Batch normalized writes per export request and metric kind
The system SHALL derive normalized gauge and sum rows from one export request and SHALL persist each populated metric kind with request-scoped batch writes rather than issuing storage writes per datapoint.

#### Scenario: One gauge export request writes one metadata batch and one datapoint batch
- **WHEN** an export request contains one or more gauge datapoints and no sum datapoints
- **THEN** the system MUST issue exactly one gauge metadata write and exactly one gauge datapoint write for that request

#### Scenario: One sum export request writes one metadata batch and one datapoint batch
- **WHEN** an export request contains one or more sum datapoints and no gauge datapoints
- **THEN** the system MUST issue exactly one sum metadata write and exactly one sum datapoint write for that request

#### Scenario: Empty kinds do not trigger storage writes
- **WHEN** an export request produces no normalized rows for a metric kind
- **THEN** the system MUST skip storage writes for that kind

### Requirement: Process populated gauge and sum kinds independently within one export
The system SHALL treat populated gauge and sum kinds as independent persistence pipelines within one export request. Neither kind SHALL be required to wait for the other kind to complete its storage work before starting its own pipeline.

#### Scenario: Mixed export request starts both persistence pipelines independently
- **WHEN** an export request contains both gauge datapoints and sum datapoints
- **THEN** the system MUST be allowed to begin gauge persistence and sum persistence independently within the same export call

#### Scenario: Per-kind ordering remains metadata before datapoints
- **WHEN** the system persists one populated metric kind
- **THEN** it MUST write that kind's metadata batch before writing that kind's datapoint batch

### Requirement: Surface storage failures without retries or atomicity guarantees
The system SHALL fail the export call when a storage step returns an error. The system SHALL NOT automatically retry failed storage writes, and it SHALL NOT claim all-or-nothing persistence across tables or metric kinds.

#### Scenario: Metadata failure aborts that export call
- **WHEN** a metadata write for a populated metric kind returns an error
- **THEN** the export call MUST return an error for that request

#### Scenario: Datapoint failure aborts that export call
- **WHEN** a datapoint write for a populated metric kind returns an error after that kind's metadata write succeeded
- **THEN** the export call MUST return an error for that request without retrying the failed write

#### Scenario: Partial persistence remains possible across independent kinds
- **WHEN** one metric kind finishes persisting successfully and another metric kind later fails during the same export request
- **THEN** the export call MUST return an error and the already-persisted rows MAY remain stored
