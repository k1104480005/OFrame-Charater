// PixiJS PixelPerfect preview canvas (filmstrip-pipeline spec 5.5): plays back
// sliced frames with nearest-neighbour (pixel-perfect) sampling so what is
// previewed is exactly what the engine consumes, with optional pixel-grid
// overlay and anchor visualization. The matting/alpha-check technical view —
// the ONLY place magenta is allowed (workbench-ui spec 10.8) — stays
// available for background-removal inspection.
//
// Performance (task 13.3): 纹理图集 —— all frame PNGs are decoded into ONE
// atlas canvas → a single GPU texture; each frame sprite references a
// sub-rectangle view of the same source (no N textures, no repeated uploads).
// The stage is built once per frames/geometry change; playback only toggles
// sprite visibility and redraws the anchor marks (no per-tick rebuild, no
// re-decoding). 降级缩放 —— when the rendered stage would exceed a pixel
// budget, the scale is capped and the pixel grid is dropped.
import { useEffect, useRef } from "react";
import { Application, Container, Graphics, Rectangle, Sprite, Texture } from "pixi.js";
import { decideRenderMode, layoutAtlas } from "./pixelAtlas";
import "./PixelCanvas.css";

/** one playable preview frame (base64 PNG) */
export interface PreviewFrame {
  png: string;
  durationMs?: number;
  anchors?: Array<{ name: string; x: number; y: number }>;
}

export interface PixelCanvasProps {
  /** logical canvas unit size (identity logical canvas); derived from frames when provided */
  unitWidth?: number;
  unitHeight?: number;
  /** scale factor for the pixel-perfect stage */
  scale?: number;
  /** frames to play back; when empty the canvas is empty */
  frames?: PreviewFrame[];
  /** play back the frames (advance per frame durationMs, default 120ms) */
  playing?: boolean;
  /** loop playback (default true); false = one-shot, stops on the last frame */
  loop?: boolean;
  /** called when a one-shot (loop=false) playback reaches its final frame */
  onPlaybackEnd?: () => void;
  /** show the alpha-check matting view (magenta technical background) */
  showMatting?: boolean;
  /** show the pixel grid overlay */
  showGrid?: boolean;
  /** show the anchor markers */
  showAnchors?: boolean;
  /** externally selected frame; playback and scrubbing share the same index */
  frameIndex?: number;
  /** stage pixel budget for degraded scaling (13.3); default 512×512 */
  maxStagePixels?: number;
  label?: string;
}

const MAGENTA = 0xff00ff;
const MAGENTA_DARK = 0xb300b3;
const GRID = 0xffffff;
const ANCHOR = 0x00ff88;

/** dynamic playback state kept in a ref so toggling `playing`/`showAnchors`
 * never rebuilds the stage (13.3: 静态/动态分层). */
interface DynState {
  sprites: Sprite[];
  marks: Graphics | null;
  current: number;
  elapsed: number;
}

export function PixelCanvas({
  unitWidth = 16,
  unitHeight = 16,
  scale = 16,
  frames = [],
  playing = false,
  loop = true,
  onPlaybackEnd,
  showMatting = false,
  showGrid = true,
  showAnchors = false,
  frameIndex,
  maxStagePixels,
  label,
}: PixelCanvasProps) {
  const hostRef = useRef<HTMLDivElement>(null);
  const appRef = useRef<Application | null>(null);
  const playingRef = useRef(playing);
  const loopRef = useRef(loop);
  const onPlaybackEndRef = useRef(onPlaybackEnd);
  const showAnchorsRef = useRef(showAnchors);
  const framesRef = useRef(frames);
  const frameIndexRef = useRef(frameIndex);
  const dynRef = useRef<DynState>({ sprites: [], marks: null, current: 0, elapsed: 0 });
  const drawMarksRef = useRef<(() => void) | null>(null);

  // These only flip refs — the stage is NOT rebuilt when they change.
  useEffect(() => {
    playingRef.current = playing;
  }, [playing]);
  useEffect(() => {
    loopRef.current = loop;
  }, [loop]);
  useEffect(() => {
    onPlaybackEndRef.current = onPlaybackEnd;
  }, [onPlaybackEnd]);
  useEffect(() => {
    showAnchorsRef.current = showAnchors;
    drawMarksRef.current?.();
  }, [showAnchors]);
  useEffect(() => {
    framesRef.current = frames;
  }, [frames]);

  // Scrubbing changes only sprite visibility and dynamic anchor marks; the
  // decoded atlas remains intact.
  useEffect(() => {
    frameIndexRef.current = frameIndex;
    const st = dynRef.current;
    if (st.sprites.length === 0 || frameIndex === undefined) return;
    const next = Math.max(0, Math.min(Math.floor(frameIndex), st.sprites.length - 1));
    st.current = next;
    st.elapsed = 0;
    for (let i = 0; i < st.sprites.length; i++) st.sprites[i].visible = i === next;
    drawMarksRef.current?.();
  }, [frameIndex]);

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;

    const app = new Application();
    appRef.current = app;
    let disposed = false;
    let initialized = false;

    const mode = decideRenderMode(unitWidth, unitHeight, frames.length, scale, { maxStagePixels });
    const effScale = mode.scale;
    const renderGrid = showGrid && mode.renderGrid;
    const w = unitWidth;
    const h = unitHeight;

    (async () => {
      await app.init({
        width: w * effScale,
        height: h * effScale,
        antialias: false,
        resolution: 1,
        backgroundAlpha: 0,
        autoDensity: true,
      });
      // 组件在 init 完成前已被卸载：此时 app 已初始化完整，安全销毁并退出，
      // 避免在 cleanup 中对未初始化实例调用 destroy（_cancelResize 竞态）。
      if (disposed) {
        try {
          app.destroy(true, { children: true });
        } catch {
          /* ignore teardown errors */
        }
        return;
      }
      initialized = true;
      if (!host.isConnected) {
        try {
          app.destroy(true, { children: true });
        } catch {
          /* ignore teardown errors */
        }
        return;
      }
      host.appendChild(app.canvas);
      app.canvas.style.width = `${w * effScale}px`;
      app.canvas.style.height = `${h * effScale}px`;
      app.canvas.style.imageRendering = "pixelated";
      if (mode.degraded) {
        app.canvas.dataset.degraded = "true";
      }

      const root = new Container();
      app.stage.addChild(root);

      // matting / alpha-check technical background (magenta checkerboard).
      if (showMatting) {
        const checker = new Graphics();
        const cell = 8;
        for (let y = 0; y < h * effScale; y += cell) {
          for (let x = 0; x < w * effScale; x += cell) {
            const dark = (x / cell + y / cell) % 2 === 0;
            checker.rect(x, y, cell, cell).fill(dark ? MAGENTA_DARK : MAGENTA);
          }
        }
        root.addChild(checker);
      }

      // pixel grid overlay (static; dropped in degraded mode — 13.3).
      if (renderGrid) {
        const grid = new Graphics();
        grid.setStrokeStyle({ width: 1, color: GRID, alpha: 0.35 });
        for (let x = 0; x <= w; x++) {
          grid.moveTo(x * effScale, 0).lineTo(x * effScale, h * effScale);
        }
        for (let y = 0; y <= h; y++) {
          grid.moveTo(0, y * effScale).lineTo(w * effScale, y * effScale);
        }
        grid.stroke();
        root.addChild(grid);
      }

      // --- texture atlas (13.3): one decode pass → one GPU texture; every
      // frame sprite is a sub-rectangle view of the same source. ---
      const valid = frames.filter((f) => f.png);
      const sprites: Sprite[] = [];
      if (valid.length > 0) {
        const rects = layoutAtlas(valid.length, w, h, 0);
        const atlasCanvas = document.createElement("canvas");
        atlasCanvas.width = rects.width;
        atlasCanvas.height = rects.height;
        const ctx = atlasCanvas.getContext("2d");
        if (ctx) {
          await Promise.all(
            valid.map(
              (f, i) =>
                new Promise<void>((resolve) => {
                  const img = new Image();
                  img.onload = () => {
                    ctx.drawImage(img, rects.rects[i].x, rects.rects[i].y);
                    resolve();
                  };
                  img.onerror = () => resolve();
                  img.src = `data:image/png;base64,${f.png}`;
                }),
            ),
          );
        }
        const atlasTex = Texture.from(atlasCanvas);
        atlasTex.source.scaleMode = "nearest";
        for (let i = 0; i < valid.length; i++) {
          const r = rects.rects[i];
          const sub = new Texture({ source: atlasTex.source, frame: new Rectangle(r.x, r.y, r.w, r.h) });
          const spr = new Sprite(sub);
          spr.width = w * effScale;
          spr.height = h * effScale;
          spr.visible = false;
          root.addChild(spr);
          sprites.push(spr);
        }
        if (sprites.length > 0) {
          sprites[0].visible = true;
        }
      }
      const initialFrame = frameIndexRef.current === undefined
        ? 0
        : Math.max(0, Math.min(Math.floor(frameIndexRef.current), Math.max(0, sprites.length - 1)));
      for (let i = 0; i < sprites.length; i++) sprites[i].visible = i === initialFrame;
      dynRef.current = { sprites, marks: null, current: initialFrame, elapsed: 0 };

      // dynamic anchor-marks layer: redrawn per frame without rebuilding
      // sprites/textures (13.3: 静态/动态分层).
      const marks = new Graphics();
      root.addChild(marks);
      dynRef.current.marks = marks;

      const drawMarks = () => {
        marks.clear();
        if (!showAnchorsRef.current) return;
        const f = framesRef.current[dynRef.current.current];
        if (!f?.anchors) return;
        for (const an of f.anchors) {
          const cx = an.x * effScale;
          const cy = an.y * effScale;
          marks.setStrokeStyle({ width: 2, color: ANCHOR, alpha: 1 });
          marks.moveTo(cx - 4, cy).lineTo(cx + 4, cy);
          marks.moveTo(cx, cy - 4).lineTo(cx, cy + 4);
          marks.stroke();
        }
      };
      drawMarksRef.current = drawMarks;
      drawMarks();

      // playback: advance per-frame with the frame's own duration (rhythm).
      // Only sprite visibility + anchor marks change — nothing is rebuilt.
      // loop=false (one-shot): playback stops on the last frame and the host
      // is notified so the UI can reset to the first frame for a replay.
      app.ticker.add((ticker) => {
        const st = dynRef.current;
        if (!playingRef.current || st.sprites.length <= 1) return;
        const dur = framesRef.current[st.current]?.durationMs && framesRef.current[st.current]!.durationMs! > 0
          ? framesRef.current[st.current]!.durationMs!
          : 120;
        st.elapsed += ticker.deltaMS;
        if (st.elapsed >= dur) {
          st.elapsed = 0;
          if (!loopRef.current && st.current >= st.sprites.length - 1) {
            // 一次性动作播放完成：停在末帧（不再回绕），通知宿主复位。
            onPlaybackEndRef.current?.();
            return;
          }
          st.current = (st.current + 1) % st.sprites.length;
          for (let i = 0; i < st.sprites.length; i++) st.sprites[i].visible = i === st.current;
          drawMarks();
        }
      });
    })();

    return () => {
      disposed = true;
      const app = appRef.current;
      appRef.current = null;
      // 仅在初始化完成后销毁；init 未完成时由 async 分支在完成后兜底销毁。
      if (app && initialized) {
        try {
          app.destroy(true, { children: true });
        } catch {
          /* ignore teardown errors */
        }
      }
      host.innerHTML = "";
      drawMarksRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [unitWidth, unitHeight, scale, frames, showMatting, showGrid, maxStagePixels]);

  return (
    <div className="pixel-canvas">
      {label && <div className="mono pixel-canvas__label">{label}</div>}
      <div
        ref={hostRef}
        className={`pixel-canvas__host${showMatting ? " pixel-canvas__host--matting" : ""}`}
        data-matting={showMatting ? "true" : "false"}
      />
      {frames.length === 0 && <div className="faint pixel-canvas__hint">PixelPerfect 预览 —— 选择方向后回放切片帧（最近邻采样）</div>}
    </div>
  );
}
