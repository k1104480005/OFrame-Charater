// Package pipeline implements the PerfectPixel filmstrip deterministic
// pipeline: normalize → generate one filmstrip → deterministic integer-pixel
// slicing → deterministic correction → PixelPerfect preview (PLAN §4.2,
// design D4).
//
// Phase 4 (filmstrip-pipeline + quality capabilities) lands the deterministic
// image-processing stages in this package:
//
//   - 5.1 帧清单规格化 (normalize.go)
//   - 5.2 filmstrip 生成 + 保留原始产物与 prompt 快照 (filmstrip.go, persist.go)
//   - 5.3 确定性切片: alpha projection + DP 最优切分, 整数像素级 (projection.go, slice.go)
//   - 5.4 确定性校正: YCbCr 色度抠图含 despill/flood fill (keying.go),
//     alpha-weighted centroid 对齐与共享基线 + 锚点校正 (align.go),
//     共享调色板量化 (palette.go), 像素网格校正 (grid.go)
//   - 5.6 重新生成 + 保留最佳候选 (candidate.go, process.go)
//   - 8.1 质量评分指标 (quality.go)
//
// Everything is pure Go (standard library only) and deterministic: identical
// input always produces identical output, which the tests verify with
// synthetic images.
package pipeline
