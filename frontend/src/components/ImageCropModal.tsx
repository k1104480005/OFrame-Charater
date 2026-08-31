import { useEffect, useRef, useState } from "react";
import "./ImageCropModal.css";

// 裁剪结果：源图像素坐标下的矩形（后端按画布比例校验 + 最近邻缩放）。
export interface CropRect {
  x: number;
  y: number;
  w: number;
  h: number;
}

interface ImageCropModalProps {
  src: string; // data URL
  targetWidth: number; // 逻辑画布宽
  targetHeight: number; // 逻辑画布高
  busy?: boolean;
  onConfirm: (rect: CropRect) => void;
  onCancel: () => void;
}

type Corner = "nw" | "ne" | "sw" | "se";

const MIN_SIZE = 4;
const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v));
const cornerSign = (c: Corner) => ({ sx: c === "ne" || c === "se" ? 1 : -1, sy: c === "sw" || c === "se" ? 1 : -1 });

// 快速裁剪工具（本地导入：图片尺寸与画布不符时弹出）：
// 框选比例锁定为画布宽高比，框内显示三分线与中心十字辅助构图，
// 四角拖拽改变大小、框内拖动移动位置，确认后按最近邻缩放到画布规格。
export function ImageCropModal({ src, targetWidth, targetHeight, busy, onConfirm, onCancel }: ImageCropModalProps) {
  const [imgSize, setImgSize] = useState<{ w: number; h: number } | null>(null);
  const [rect, setRect] = useState<CropRect | null>(null);
  const [scale, setScale] = useState(1);
  const drag = useRef<{ mode: "move" | "resize"; corner?: Corner; startX: number; startY: number; origin: CropRect } | null>(null);

  const aspect = targetWidth / targetHeight;

  // 图片加载后：按显示区适配缩放，并初始化为居中的最大同比例框。
  useEffect(() => {
    if (!imgSize) return;
    const maxW = Math.min(window.innerWidth * 0.72, 860);
    const maxH = window.innerHeight * 0.6;
    setScale(Math.min(maxW / imgSize.w, maxH / imgSize.h));
    let w = imgSize.w;
    let h = w / aspect;
    if (h > imgSize.h) {
      h = imgSize.h;
      w = h * aspect;
    }
    setRect({
      x: Math.max(0, Math.round((imgSize.w - w) / 2)),
      y: Math.max(0, Math.round((imgSize.h - h) / 2)),
      w: Math.max(MIN_SIZE, Math.round(w)),
      h: Math.max(MIN_SIZE, Math.round(h)),
    });
  }, [imgSize, aspect]);

  const toImagePoint = (e: React.PointerEvent) => {
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
    return { x: (e.clientX - r.left) / scale, y: (e.clientY - r.top) / scale };
  };

  const beginMove = (e: React.PointerEvent) => {
    if (!rect || busy) return;
    e.preventDefault();
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    const p = toImagePoint(e);
    drag.current = { mode: "move", startX: p.x, startY: p.y, origin: rect };
  };

  const beginResize = (e: React.PointerEvent, corner: Corner) => {
    if (!rect || busy) return;
    e.preventDefault();
    e.stopPropagation();
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    const p = toImagePoint(e);
    drag.current = { mode: "resize", corner, startX: p.x, startY: p.y, origin: rect };
  };

  const onPointerMove = (e: React.PointerEvent) => {
    const d = drag.current;
    if (!d || !imgSize) return;
    const p = toImagePoint(e);
    if (d.mode === "move") {
      setRect({
        ...d.origin,
        x: Math.round(clamp(d.origin.x + (p.x - d.startX), 0, imgSize.w - d.origin.w)),
        y: Math.round(clamp(d.origin.y + (p.y - d.startY), 0, imgSize.h - d.origin.h)),
      });
      return;
    }
    const { sx, sy } = cornerSign(d.corner!);
    const anchorX = sx > 0 ? d.origin.x : d.origin.x + d.origin.w;
    const anchorY = sy > 0 ? d.origin.y : d.origin.y + d.origin.h;
    const dist = Math.abs(p.x - anchorX);
    const maxWx = sx > 0 ? imgSize.w - anchorX : anchorX;
    const maxHy = sy > 0 ? imgSize.h - anchorY : anchorY;
    // 比例锁定：h = w / aspect，横向纵向边界同时约束宽度。
    const w = Math.round(clamp(dist, MIN_SIZE, Math.max(MIN_SIZE, Math.min(maxWx, maxHy * aspect))));
    const h = Math.round(w / aspect);
    setRect({
      x: Math.round(clamp(sx > 0 ? anchorX : anchorX - w, 0, imgSize.w)),
      y: Math.round(clamp(sy > 0 ? anchorY : anchorY - h, 0, imgSize.h)),
      w,
      h,
    });
  };

  const endDrag = () => {
    drag.current = null;
  };

  return (
    <div className="crop-modal__overlay" role="dialog" aria-modal="true" aria-label="裁剪角色图">
      <div className="pixel-panel crop-modal">
        <h3 className="mono crop-modal__title">裁剪角色图 · 输出 {targetWidth} × {targetHeight}</h3>
        <div className="faint crop-modal__hint">
          拖动框选与画布同比例的区域：四角拖拽改变大小（比例锁定），框内拖动移动位置；框内三分线与中心十字辅助构图。
        </div>
        <div className="crop-modal__stage">
          <div className="crop-modal__frame" style={imgSize ? { width: imgSize.w * scale, height: imgSize.h * scale } : undefined}>
            <img
              src={src}
              alt="待裁剪角色图"
              className="crop-modal__img"
              draggable={false}
              onLoad={(e) => setImgSize({ w: e.currentTarget.naturalWidth, h: e.currentTarget.naturalHeight })}
            />
            {imgSize && rect && (
              <div
                className="crop-modal__layer"
                style={{ width: imgSize.w * scale, height: imgSize.h * scale }}
                onPointerMove={onPointerMove}
                onPointerUp={endDrag}
                onPointerCancel={endDrag}
              >
                <div
                  className="crop-modal__box"
                  style={{ left: rect.x * scale, top: rect.y * scale, width: rect.w * scale, height: rect.h * scale }}
                  onPointerDown={beginMove}
                >
                  <div className="crop-modal__guide crop-modal__guide--v" style={{ left: "33.333%" }} />
                  <div className="crop-modal__guide crop-modal__guide--v" style={{ left: "66.667%" }} />
                  <div className="crop-modal__guide crop-modal__guide--h" style={{ top: "33.333%" }} />
                  <div className="crop-modal__guide crop-modal__guide--h" style={{ top: "66.667%" }} />
                  <div className="crop-modal__guide crop-modal__guide--v crop-modal__guide--center" style={{ left: "50%" }} />
                  <div className="crop-modal__guide crop-modal__guide--h crop-modal__guide--center" style={{ top: "50%" }} />
                  {(["nw", "ne", "sw", "se"] as Corner[]).map((c) => (
                    <div key={c} className={`crop-modal__handle crop-modal__handle--${c}`} onPointerDown={(e) => beginResize(e, c)} />
                  ))}
                </div>
              </div>
            )}
          </div>
        </div>
        <div className="mono faint crop-modal__meta">
          框选：{rect ? `${rect.w} × ${rect.h}` : "—"} 像素 → 输出 {targetWidth} × {targetHeight}（最近邻缩放，不模糊）
        </div>
        <div className="crop-modal__actions">
          <button className="pixel-btn pixel-btn--primary" disabled={busy || !rect} onClick={() => rect && onConfirm(rect)} title="按当前框选裁剪，并按画布规格最近邻缩放后登记为候选">
            {busy ? "裁剪中…" : "确认裁剪并导入"}
          </button>
          <button className="pixel-btn" disabled={busy} onClick={onCancel}>
            取消
          </button>
        </div>
      </div>
    </div>
  );
}
