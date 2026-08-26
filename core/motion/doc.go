// Package motion defines motions, direction sets, and frame sequences — the
// structural model of animated behavior — together with the direction strategy
// (single direction by default, 4/8 directions with automatic mirroring) and
// the horizontal mirror geometry (motion spec, tasks 3.1–3.6).
//
// Domain language (CONTEXT.md): 动作 / 方向集 / 帧序列 / 锚点.
//
// Mirror semantics (复核报告语义点, made explicit in mirror.go): only
// horizontal (left-right) mirroring is used; down (正面/south) and up are
// self-symmetric; the mirror pairs are one-way — right→left, up-right→up-left,
// down-left→down-right (down-right 单向派生自 down-left).
package motion
