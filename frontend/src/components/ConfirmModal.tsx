// 动森风格确认弹窗：替代原生 window.confirm，融入主题皮肤
// （scrim 淡入 + 面板弹入动效，按钮走 pixel-btn 3D 按下感）。
import { useEffect } from "react";
import type { ReactNode } from "react";
import "./ConfirmModal.css";

export interface ConfirmModalProps {
  open: boolean;
  title: string;
  message: ReactNode;
  confirmLabel?: string;
  cancelLabel?: string;
  /** 确认按钮使用危险样式（红底） */
  danger?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

export function ConfirmModal({
  open,
  title,
  message,
  confirmLabel = "确定",
  cancelLabel = "取消",
  danger = false,
  onConfirm,
  onCancel,
}: ConfirmModalProps) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onCancel();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onCancel]);

  if (!open) return null;

  return (
    <div className="modal-scrim" onClick={onCancel}>
      <div
        className="confirm-modal pixel-panel"
        role="dialog"
        aria-modal="true"
        aria-label={title}
        onClick={(e) => e.stopPropagation()}
      >
        <h3 className="mono confirm-modal__title">{title}</h3>
        <hr className="pixel-rule" />
        <div className="confirm-modal__message">{message}</div>
        <div className="row confirm-modal__actions">
          <button className={`pixel-btn${danger ? " pixel-btn--warn" : " pixel-btn--primary"}`} autoFocus onClick={onConfirm}>
            {confirmLabel}
          </button>
          <button className="pixel-btn" onClick={onCancel}>
            {cancelLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
