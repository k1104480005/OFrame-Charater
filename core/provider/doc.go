// Package provider defines the unified generation provider interface (text and
// image generation) with per-vendor adapters — Doubao (default), gpt-image-2,
// Agnes — plus generation confirmation, local key/credit/call statistics, rate
// limiting and retry with a hard cap (design D6).
//
// Phase 3 ships tasks 4.1–4.6: the unified interface, the three built-in
// adapters, runtime switching, local key/model configuration with offline
// validation, call statistics, and the retry-with-hard-cap policy (每方向最多
// 3 次总尝试). Adapters use an injectable *http.Client so tests run against a
// fake transport and never call real paid services. The full execution loop
// (filmstrip generation → slicing → correction) belongs to the filmstrip
// pipeline (tasks 5.x).
package provider
