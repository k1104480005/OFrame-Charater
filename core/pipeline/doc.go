// Package pipeline implements the PerfectPixel filmstrip deterministic
// pipeline: normalize → generate one filmstrip → deterministic integer-pixel
// slicing → deterministic correction → PixelPerfect preview (PLAN §4.2,
// design D4).
//
// This package is a skeleton in phase 1 (Go core & workspace); the pipeline
// lands with the filmstrip-pipeline capability (tasks 5.1–5.6).
package pipeline
