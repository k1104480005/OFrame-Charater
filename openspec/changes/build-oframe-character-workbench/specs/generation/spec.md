## Purpose

Defines the generation capability: a pluggable provider abstraction with adapters (Doubao, gpt-image-2, Agnes), the generation confirmation gate before any external call, local key/credit/call statistics, and rate limiting and retry with hard caps to prevent silent cost overruns.

## ADDED Requirements

### Requirement: Unified provider interface
The system SHALL expose generation capability (text and image generation) through a unified provider interface, with per-vendor adapters that can be switched at runtime.

#### Scenario: Switch provider at runtime
- **WHEN** the user switches the active provider in settings
- **THEN** subsequent generation calls use the new provider while the previous provider's configuration is preserved

### Requirement: Built-in provider adapters
The system SHALL ship adapters for Doubao (default primary), gpt-image-2 (high-quality fallback), and Agnes (specialized supplementary).

#### Scenario: Default provider used on first generation
- **WHEN** generation is triggered without an explicit provider choice
- **THEN** the system uses the Doubao adapter by default

#### Scenario: Select an alternative provider
- **WHEN** the user selects gpt-image-2 or Agnes in settings
- **THEN** generation calls are routed through the corresponding adapter

### Requirement: Generation confirmation before external calls
The system SHALL, before any operation that may cause external calls, present the expected call count and maximum retry count and execute only after the user confirms.

#### Scenario: Confirmation before generation
- **WHEN** the user triggers generation (including mirrored-direction replacement or retries)
- **THEN** the system shows the expected call count and maximum retry count and waits for confirmation before issuing any external call

#### Scenario: Cancel confirmation
- **WHEN** the user cancels the generation confirmation
- **THEN** no external call is made and the operation is aborted

### Requirement: Local key, credit, and call statistics management
Provider keys, credit limits, and call statistics SHALL be managed locally.

#### Scenario: Configure provider key
- **WHEN** the user configures a provider key in settings
- **THEN** the key is stored locally and used for subsequent calls

#### Scenario: Record call statistics
- **WHEN** a generation call completes
- **THEN** the system updates local call statistics including call count and estimated cost

### Requirement: Retry with hard cap and rate limiting
Generation calls SHALL be retried automatically on failure up to a configured maximum, with rate limiting, and SHALL NOT silently exceed the cap.

#### Scenario: Automatic retry below the cap
- **WHEN** a provider call fails and the retry count is below the maximum
- **THEN** the system retries the call with backoff

#### Scenario: Failure at the retry cap
- **WHEN** a provider call fails and the retry count has reached the maximum
- **THEN** the task is marked failed with the recorded reason and no further retries are issued

### Requirement: Idempotent result caching
The system SHALL cache successful results per task so identical tasks do not incur repeated billable calls.

#### Scenario: Cache hit for identical task
- **WHEN** a task identical to an already successful task is submitted again
- **THEN** the system reuses the cached result and issues no new external call
