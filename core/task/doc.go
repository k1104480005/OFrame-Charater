// Package task defines the recoverable task queue: every generation,
// correction, and export is a trackable task persisted to a local queue with
// provider parameters, expected call counts, status, progress, errors, and
// retry counts; sessions stay continuous across app restarts; identical tasks
// are deduplicated (design D7).
//
// The local SQLite schema and migrations used by this queue live in
// core/store; the task model and queue behavior land with the tasks
// capability (tasks 6.1–6.5).
//
// This package is a skeleton in phase 1 (Go core & workspace).
package task
