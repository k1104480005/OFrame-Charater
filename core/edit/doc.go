// Package edit implements lightweight editing as the minimal closed loop for
// deterministic correction: frame-level, sequence-level, anchor-level, and
// batch capabilities, with every edit recorded as a replayable, append-only
// instruction so storage is non-destructive by construction (design D8).
//
// The package is used by the application layer for non-destructive frame and
// sequence revisions; callers can replay the instruction list and pass the
// resulting images back through the pipeline quality scorer.
package edit
