## Purpose

Defines the recoverable task queue: every generation, correction, and export in the workflow is a trackable task persisted to a local queue, sessions stay continuous across app restarts, failed tasks can be inspected and retried or abandoned, and identical tasks are deduplicated.

## ADDED Requirements

### Requirement: Task model and persistence
Each generation, correction, and export SHALL be a trackable task persisted to the local queue, including provider parameters, expected call counts, status, progress, errors, and retry counts.

#### Scenario: Create a task
- **WHEN** the user triggers a generation, correction, or export
- **THEN** the system creates a task with its parameters and persists it to the local queue

### Requirement: Task status and progress visibility
Tasks SHALL expose statuses including running, queued, and failed, with visible progress.

#### Scenario: View tasks in the drawer
- **WHEN** the user opens the task drawer
- **THEN** running, queued, and failed tasks are shown with their progress

### Requirement: Persistent session across restarts
The task queue SHALL remain continuous across application restarts, allowing unfinished tasks to be resumed with one action after an interruption (crash, shutdown, or network failure).

#### Scenario: Resume after interruption
- **WHEN** the application is restarted after an interruption with unfinished tasks
- **THEN** the user can resume the unfinished tasks with a single action

### Requirement: Idempotent deduplication
The system SHALL cache successful results per task so identical tasks do not incur repeated billable calls.

#### Scenario: Re-submit identical task
- **WHEN** the user submits a task identical to an already successful one
- **THEN** the system reuses the cached result without issuing a new external call

### Requirement: Failed task handling
Failed tasks SHALL show their reason and support retry or abandon, with retries obeying the maximum retry count agreed in generation confirmation.

#### Scenario: Retry a failed task
- **WHEN** a task has failed and the user chooses to retry
- **THEN** the system retries under the generation confirmation rules

#### Scenario: Abandon a failed task
- **WHEN** the user abandons a failed task
- **THEN** the task is marked as abandoned and is not executed further
