// Package task defines the recoverable task queue: every generation,
// correction, and export is a trackable task persisted to a local SQLite queue
// with provider parameters, expected call counts, status, progress, errors, and
// retry counts; sessions stay continuous across app restarts (unfinished tasks
// resume with one action); identical tasks are deduplicated via a success-result
// cache (design D7, tasks spec 6.1–6.5).
//
// The SQLite schema and migrations live in core/store; this package owns the
// task model and the persisted store. The execution of tasks (which knows how
// to run a generation plan) lives in core/service.
package task
