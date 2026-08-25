## Purpose

Defines versioned revisions: an immutable identity version is formed after an explicit appearance revision, candidate acceptance makes a generated candidate the current animation asset, and all changes are recorded in an append-only operation log supporting rollback to any historical point.

## ADDED Requirements

### Requirement: Immutable identity versions
The identity package SHALL form an immutable identity version after an explicit appearance revision; assets of older versions remain preserved but no longer represent the current identity by default.

#### Scenario: Create an identity version
- **WHEN** the user confirms an appearance revision
- **THEN** the system forms an immutable identity version while older versions remain preserved and accessible

#### Scenario: New version becomes current identity
- **WHEN** a newer identity version exists
- **THEN** the newer version represents the current identity by default and older version assets remain preserved

### Requirement: Candidate acceptance as business action
The system SHALL treat candidate acceptance as the explicit business action that makes a generated candidate the current animation asset; unaccepted candidates belong only to candidate history.

#### Scenario: Accept a candidate
- **WHEN** the user confirms a candidate on the acceptance tab
- **THEN** the candidate becomes the current animation asset and the acceptance is recorded in the operation log

### Requirement: Append-only revision history
All changes (generation, editing, acceptance, and mirror replacement) SHALL be recorded in an append-only operation log.

#### Scenario: Record every change
- **WHEN** any change happens to the identity package
- **THEN** the change is appended to the operation log

### Requirement: Rollback to any historical point
The system SHALL support rolling back the identity package to any historical point in the operation log.

#### Scenario: Rollback to a historical point
- **WHEN** the user rolls back to a historical point
- **THEN** the identity package content is restored to that point's state while later log entries remain preserved
