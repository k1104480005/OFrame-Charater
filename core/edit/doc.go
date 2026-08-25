// Package edit implements lightweight editing as the minimal closed loop for
// deterministic correction: frame-level, sequence-level, anchor-level, and
// batch capabilities, with every edit recorded as a replayable, append-only
// instruction so storage is non-destructive by construction (design D8).
//
// This package is a skeleton in phase 1 (Go core & workspace); the editing
// capability lands with tasks 7.1–7.5.
package edit
