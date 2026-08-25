// Package provider defines the unified generation provider interface (text and
// image generation) with per-vendor adapters — Doubao (default), gpt-image-2,
// Agnes — plus generation confirmation, local key/credit/call statistics, rate
// limiting and retry with a hard cap (design D6).
//
// This package is a skeleton in phase 1 (Go core & workspace); adapters and
// accounting land with the generation capability (tasks 4.1–4.8).
package provider
