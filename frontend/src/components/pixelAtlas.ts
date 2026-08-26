// Pure pixel-canvas performance helpers (no PixiJS dependency, so the layout
// math is unit-checkable without a browser — task 13.3: 纹理图集 / 降级缩放).
//
// 纹理图集: all preview frames are decoded into ONE atlas canvas → one GPU
// texture; each frame sprite references a sub-rectangle view of the same
// source, so memory is bounded and texture switches are avoided.
// 降级缩放: when the rendered stage would exceed a pixel budget (large canvas
// × many frames), the scale is capped and the per-pixel grid is dropped so
// playback stays smooth.

export interface AtlasRect {
  x: number;
  y: number;
  w: number;
  h: number;
}

export interface AtlasLayout {
  width: number;
  height: number;
  rects: AtlasRect[];
}

/** Square-ish atlas layout for `count` frames of cellW×cellH, with an
 * optional per-cell padding. Deterministic and integer-pixel exact. */
export function layoutAtlas(count: number, cellW: number, cellH: number, pad = 0): AtlasLayout {
  if (count < 0 || cellW <= 0 || cellH <= 0) {
    throw new Error(`pixelAtlas: invalid layout inputs count=${count} cell=${cellW}x${cellH}`);
  }
  if (count === 0) {
    return { width: 0, height: 0, rects: [] };
  }
  const cols = Math.ceil(Math.sqrt(count));
  const rows = Math.ceil(count / cols);
  const width = cols * cellW + (cols > 1 ? pad * (cols - 1) : 0);
  const height = rows * cellH + (rows > 1 ? pad * (rows - 1) : 0);
  const rects: AtlasRect[] = [];
  for (let i = 0; i < count; i++) {
    const c = i % cols;
    const r = Math.floor(i / cols);
    rects.push({ x: c * (cellW + pad), y: r * (cellH + pad), w: cellW, h: cellH });
  }
  return { width, height, rects };
}

export interface RenderMode {
  /** effective nearest-neighbour scale of the stage */
  scale: number;
  /** whether the per-pixel grid should be drawn */
  renderGrid: boolean;
  /** degraded rendering is active (scale capped or grid dropped) */
  degraded: boolean;
}

export interface RenderModeOptions {
  /** stage pixel budget (unitW*scale * unitH*scale); default 512×512 */
  maxStagePixels?: number;
  /** minimum scale at which the pixel grid stays legible; default 8 */
  minGridScale?: number;
  /** frame count above which rendering is degraded (many frames); default 64 */
  maxFrames?: number;
}

/** Decide the effective render mode for a playback stage. */
export function decideRenderMode(
  unitW: number,
  unitH: number,
  frameCount: number,
  baseScale: number,
  opts: RenderModeOptions = {},
): RenderMode {
  const maxPixels = opts.maxStagePixels ?? 512 * 512;
  const minGridScale = opts.minGridScale ?? 8;
  const maxFrames = opts.maxFrames ?? 64;
  const safeBase = baseScale > 0 ? baseScale : 12;

  let scale = safeBase;
  const stagePixels = unitW * scale * unitH * scale;
  if (stagePixels > maxPixels) {
    // 降级缩放: cap the stage so the whole canvas fits the pixel budget.
    scale = Math.max(1, Math.floor(Math.sqrt(maxPixels / (unitW * unitH))));
  }
  // Degraded = the stage had to be downscaled, or the sequence is very long.
  // The pixel grid is dropped only when it is no longer legible (scale below
  // minGridScale) — grid visibility is decoupled from degraded mode.
  const degraded = scale < safeBase || frameCount > maxFrames;
  const renderGrid = scale >= minGridScale;
  return { scale, renderGrid, degraded };
}
