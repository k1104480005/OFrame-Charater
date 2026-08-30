// MaterialLightbox: click-to-zoom preview for stored reference images.
// The image renders at "fit" size first; zoom cycles 1×/2×/4× with pixelated
// rendering so pixel art stays crisp. Esc / mask click / ✕ all close it.
import { useEffect, useState } from "react";
import { fetchMaterialImage } from "../api/client";
import type { MaterialView } from "../api/client";
import "./MaterialLightbox.css";

const ZOOM_STEPS = ["适应", 1, 2, 4] as const;

export function MaterialLightbox({ material, onClose }: { material: MaterialView | null; onClose: () => void }) {
  const [image, setImage] = useState<{ mime: string; data: string } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [zoomIndex, setZoomIndex] = useState(0);
  const [natural, setNatural] = useState<{ w: number; h: number } | null>(null);

  useEffect(() => {
    setImage(null);
    setError(null);
    setZoomIndex(0);
    setNatural(null);
    if (!material) return;
    let cancelled = false;
    fetchMaterialImage(material.id)
      .then((res) => {
        if (!cancelled) setImage({ mime: res.mime, data: res.data });
      })
      .catch((e) => {
        if (!cancelled) setError(String(e));
      });
    return () => {
      cancelled = true;
    };
  }, [material]);

  useEffect(() => {
    if (!material) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [material, onClose]);

  if (!material) return null;

  const zoom = ZOOM_STEPS[zoomIndex];
  const scaleStyle =
    zoom !== "适应" && natural
      ? { width: natural.w * zoom, height: natural.h * zoom, maxWidth: "none", maxHeight: "none" }
      : undefined;

  return (
    <div className="material-lightbox__mask" onClick={onClose} role="dialog" aria-modal="true" aria-label={`素材大图预览 ${material.name}`}>
      <div className="material-lightbox__bar" onClick={(e) => e.stopPropagation()}>
        <span className="mono">{material.name}</span>
        <span className="faint">{natural ? `${natural.w} × ${natural.h}` : ""}</span>
        <button className="pixel-btn" onClick={() => setZoomIndex((i) => (i + 1) % ZOOM_STEPS.length)} aria-label="切换缩放" title="切换缩放：适应窗口 → 1× → 2× → 4×">
          缩放：{zoom}
        </button>
        <button className="pixel-btn pixel-btn--warn" onClick={onClose} aria-label="关闭预览" title="关闭（Esc）">✕</button>
      </div>
      <div className="material-lightbox__stage" onClick={onClose}>
        {error ? (
          <div className="error-text" onClick={(e) => e.stopPropagation()}>{error}</div>
        ) : image ? (
          <img
            className="material-lightbox__image"
            src={`data:${image.mime};base64,${image.data}`}
            alt={material.name}
            style={scaleStyle}
            onClick={(e) => e.stopPropagation()}
            onLoad={(e) => setNatural({ w: e.currentTarget.naturalWidth, h: e.currentTarget.naturalHeight })}
          />
        ) : (
          <div className="faint" onClick={(e) => e.stopPropagation()}>加载原图…</div>
        )}
      </div>
    </div>
  );
}
