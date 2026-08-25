## Purpose

Defines quality scoring and the quality acceptance gate: quantified metrics for each generated candidate, a gate that passes a candidate only when scores meet thresholds and the user confirms in the PixelPerfect preview, and candidate history recorded in the identity package metadata.

## ADDED Requirements

### Requirement: Quality score metrics
The system SHALL compute quantified metrics for each generated candidate: structural metrics (slice completeness, anchor deviation, inter-frame area/center-of-mass jitter) and rule metrics (color count/palette consistency, mirror symmetry, out-of-bounds pixel ratio).

#### Scenario: Output quality scores
- **WHEN** the pipeline produces a candidate
- **THEN** the system computes the structural and rule metrics and displays them to the user

### Requirement: Optional AI-assisted consistency score
The system SHALL support an optional coarse consistency score for same-character consistency from a provider or local model, as reference only and never blocking the workflow.

#### Scenario: AI-assisted coarse scoring
- **WHEN** AI-assisted scoring is enabled
- **THEN** a coarse consistency score is displayed as reference and does not block the acceptance flow

### Requirement: Quality acceptance gate
A candidate SHALL pass only when its scores meet thresholds AND the user confirms in the PixelPerfect preview; otherwise it enters retry or lightweight editing.

#### Scenario: Candidate passes acceptance
- **WHEN** the candidate's scores meet the thresholds and the user confirms in the preview
- **THEN** the candidate is marked as passed and eligible to become animation asset

#### Scenario: Candidate fails acceptance
- **WHEN** the candidate's scores do not meet the thresholds or the user rejects it
- **THEN** the candidate does not become animation asset and may enter retry or lightweight editing

### Requirement: Candidate history
Unaccepted candidates SHALL belong only to candidate history, and scoring and acceptance results SHALL be written into the identity package metadata.

#### Scenario: Record candidate history
- **WHEN** a candidate is rejected or left unaccepted
- **THEN** the candidate together with its scores and acceptance result is retained in the candidate history inside the identity package
