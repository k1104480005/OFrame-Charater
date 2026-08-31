// ImageLightbox: generic click-to-zoom preview for an already-available image
// source (data URI or URL) — used by base-character candidates and generated
// results. Zoom cycles 适应/1×/2×/4× with pixelated rendering; Esc / mask
// click / ✕ close it. Reuses the MaterialLightbox stylesheet.
import { useEffect, useState } from "react";
import "./MaterialLightbox.css";

const ZOOM_STEPS = ["适应", 1, 2, 4] as const;

export interface ImageLightboxSource {
  /** data URI or URL of the full image */
  src: string;
  title: string;
}

export function ImageLightbox({ source, onClose }: { source: ImageLightboxSource | null; onClose: () => void }) {
  const [zoomIndex, setZoomIndex] = useState(0);
  const [natural, setNatural] = useState<{ w: number; h: number } | null>(null);

  useEffect(() => {
    setZoomIndex(0);
    setNatural(null);
  }, [source]);

  useEffect(() => {
    if (!source) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [source, onClose]);

  if (!source) return null;

  const zoom = ZOOM_STEPS[zoomIndex];
  const scaleStyle =
    zoom !== "适应" && natural
      ? { width: natural.w * zoom, height: natural.h * zoom, maxWidth: "none", maxHeight: "none" }
      : undefined;

  return (
    <div className="material-lightbox__mask" onClick={onClose} role="dialog" aria-modal="true" aria-label={`大图预览 ${source.title}`}>
      <div className="material-lightbox__bar" onClick={(e) => e.stopPropagation()}>
        <span className="mono">{source.title}</span>
        <span className="faint">{natural ? `${natural.w} × ${natural.h}` : ""}</span>
        <button className="pixel-btn" onClick={() => setZoomIndex((i) => (i + 1) % ZOOM_STEPS.length)} aria-label="切换缩放" title="切换缩放：适应窗口 → 1× → 2× → 4×">
          缩放：{zoom}
        </button>
        <button className="pixel-btn pixel-btn--warn" onClick={onClose} aria-label="关闭预览" title="关闭（Esc）">✕</button>
      </div>
      <div className="material-lightbox__stage" onClick={onClose}>
        <img
          className="material-lightbox__image"
          src={source.src}
          alt={source.title}
          style={scaleStyle}
          onClick={(e) => e.stopPropagation()}
          onLoad={(e) => setNatural({ w: e.currentTarget.naturalWidth, h: e.currentTarget.naturalHeight })}
        />
      </div>
    </div>
  );
}
