// “添加动画”弹窗：预设模式（搜索/选择预设动画，点击即创建）与
// 自定义模式（名称 + 动作描述 + 目标帧数）。对齐 perfectpixel 的
// “选择动画情景”交互：搜索框 + 预设 chip 网格，点击添加后关闭。
import { useEffect, useState } from "react";
import type { ActionPresetView } from "../api/client";
import "./MotionAddModal.css";

export type MotionAddMode = "preset" | "custom";

interface Props {
  open: boolean;
  mode: MotionAddMode;
  presets: ActionPresetView[];
  busy: boolean;
  onPickPreset: (presetId: string) => void;
  onCreateCustom: (name: string, description: string, frameCount: number) => void;
  onClose: () => void;
}

export function MotionAddModal({ open, mode, presets, busy, onPickPreset, onCreateCustom, onClose }: Props) {
  const [query, setQuery] = useState("");
  const [name, setName] = useState("");
  const [desc, setDesc] = useState("");
  const [frames, setFrames] = useState("6");

  // 每次打开时重置表单状态。
  useEffect(() => {
    if (open) {
      setQuery("");
      setName("");
      setDesc("");
      setFrames("6");
    }
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  if (!open) return null;

  const q = query.trim().toLowerCase();
  const filtered = q
    ? presets.filter(
        (p) =>
          p.name.toLowerCase().includes(q) ||
          p.promptText.toLowerCase().includes(q) ||
          p.description.toLowerCase().includes(q),
      )
    : presets;

  // 按分类分组（保持目录定义顺序），与 perfectpixel 的“情景分类 + 小 chip”一致。
  const groups: Array<{ category: string; items: ActionPresetView[] }> = [];
  for (const p of filtered) {
    const last = groups[groups.length - 1];
    if (last && last.category === p.category) {
      last.items.push(p);
    } else {
      groups.push({ category: p.category, items: [p] });
    }
  }

  return (
    <div className="modal-scrim" onClick={onClose}>
      <div
        className="motion-add-modal pixel-panel"
        role="dialog"
        aria-modal="true"
        aria-label={mode === "preset" ? "选择动画预设" : "添加自定义动画"}
        onClick={(e) => e.stopPropagation()}
      >
        <h3 className="mono motion-add-modal__title">
          {mode === "preset" ? "选择动画预设" : "添加自定义动画"}
        </h3>
        <hr className="pixel-rule" />

        {mode === "preset" ? (
          <>
            <input
              className="pixel-input motion-add-modal__search"
              placeholder="搜索动画（如：攻击 / walk / 死亡）"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              autoFocus
            />
            <div className="motion-add-modal__scroll">
              {filtered.length === 0 && <div className="faint">没有匹配的预设动画</div>}
              {groups.map((g) => (
                <section key={g.category} className="motion-add-modal__group">
                  <h4 className="motion-add-modal__group-title">{g.category}</h4>
                  <div className="motion-add-modal__chips">
                    {g.items.map((p) => (
                      <button
                        key={p.id}
                        className="pixel-btn motion-add-modal__chip"
                        disabled={busy}
                        onClick={() => onPickPreset(p.id)}
                        title={`${p.description}${p.frames ? `（${p.frames} 帧）` : ""}${p.promptText ? `\n提示词：${p.promptText}` : ""}`}
                      >
                        <span className="motion-add-modal__chip-name">{p.name}</span>
                        {p.frames ? <span className="faint motion-add-modal__chip-frames">{p.frames}帧</span> : null}
                      </button>
                    ))}
                  </div>
                </section>
              ))}
            </div>
            <div className="faint motion-add-modal__hint">点击预设即创建并选中该动作，可连续添加多个。</div>
          </>
        ) : (
          <div className="col">
            <div className="field-row">
              <label className="field-label" htmlFor="custom-motion-name">名称</label>
              <input id="custom-motion-name" className="pixel-input" value={name} onChange={(e) => setName(e.target.value)} placeholder="如：挥剑 / swing" autoFocus />
            </div>
            <div className="field-row">
              <label className="field-label" htmlFor="custom-motion-desc">动作描述</label>
              <textarea id="custom-motion-desc" className="pixel-input" rows={2} value={desc} onChange={(e) => setDesc(e.target.value)} placeholder="例如：挥剑攻击，先蓄力，再向前斩击，最后收势" />
            </div>
            <div className="field-row">
              <label className="field-label" htmlFor="custom-motion-frames">目标帧数</label>
              <input id="custom-motion-frames" className="pixel-input" type="number" min={1} max={10} value={frames} onChange={(e) => setFrames(e.target.value)} />
            </div>
            <div className="faint">动作描述会作为生成提示词的动作语义；方向策略可在添加后于动作卡上调整。</div>
          </div>
        )}

        <div className="row motion-add-modal__actions">
          {mode === "custom" && (
            <button
              className="pixel-btn pixel-btn--primary"
              disabled={busy || !name.trim() || !desc.trim()}
              onClick={() => onCreateCustom(name.trim(), desc.trim(), parseInt(frames, 10) || 6)}
            >
              {busy ? "创建中…" : "添加动作"}
            </button>
          )}
          <button className="pixel-btn" onClick={onClose}>
            关闭
          </button>
        </div>
      </div>
    </div>
  );
}
