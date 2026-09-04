// 生成确认弹窗：点击「生成所选方向」后直接弹出，展示完整方案（模型 /
// 方向明细 / 调用量 / 预算 / 提示词快照），确认后才执行，取消不发起任何调用。
// 替代旧的「生成确认」常驻区域（该区域已改为批量操作区）。
import { useEffect } from "react";
import type { GenerationPlanView } from "../api/client";
import "./GenerationPlanModal.css";

interface Props {
  plan: GenerationPlanView | null;
  /** 动作名（标题中显示，帮助确认生成对象） */
  motionName?: string;
  /** 执行中：按钮禁用并显示进度文案 */
  busy?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

export function GenerationPlanModal({ plan, motionName, busy = false, onConfirm, onCancel }: Props) {
  useEffect(() => {
    if (!plan || busy) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onCancel();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [plan, busy, onCancel]);

  if (!plan) return null;

  return (
    <div className="modal-scrim" onClick={busy ? undefined : onCancel}>
      <div
        className="gen-plan-modal pixel-panel"
        role="dialog"
        aria-modal="true"
        aria-label="生成确认"
        onClick={(e) => e.stopPropagation()}
      >
        <h3 className="mono gen-plan-modal__title">生成确认{motionName ? ` · ${motionName}` : ""}</h3>
        <hr className="pixel-rule" />
        <ul className="mono gen-plan__list gen-plan-modal__list">
          <li>
            provider / model：{plan.providerId} / {plan.model}
            {plan.providerType ? `（协议：${plan.providerType}）` : ""}
          </li>
          <li>能力：{plan.capability}（视频能力未接入，仅图像生成可执行）</li>
          <li>
            方向数：{plan.directions}（{plan.basicDirections} 生成 + {plan.mirroredDirections} 镜像）
          </li>
          <li>
            本次生成：{(plan.basicLabels ?? []).map((d) => d).join("、") || "—"}
            {(plan.mirroredLabels ?? []).length > 0 && (
              <span className="faint"> ＋ 镜像派生：{(plan.mirroredLabels ?? []).join("、")}</span>
            )}
            （AI {plan.basicDirections} 次调用）
          </li>
          <li>
            预计调用量：{plan.expectedCalls} 次 · 每方向最多 {plan.maxAttemptsPerDirection} 次总尝试 · 预算上限 {plan.maxTotalAttempts} 次
          </li>
          <li>
            预算：约 {plan.expectedCost.toFixed(2)} {plan.currency}（上限 {plan.maxCost.toFixed(2)} {plan.currency}）
          </li>
          <li className="gen-plan-modal__prompt">
            提示词快照（{plan.prompt.stylePresetId} / {plan.prompt.actionPresetId}，{plan.prompt.frameCount} 帧）：
            <div className="faint gen-plan-modal__prompt-text">{plan.prompt.prompt}</div>
          </li>
        </ul>
        <div className="row gen-plan-modal__actions">
          <button className="pixel-btn pixel-btn--primary" disabled={busy} onClick={onConfirm}>
            {busy ? "执行中…" : "确认并执行"}
          </button>
          <button className="pixel-btn" disabled={busy} onClick={onCancel}>
            取消（不发起调用）
          </button>
        </div>
        <div className="faint gen-plan-modal__hint">确认后执行：进度与结果会显示在批量操作区与任务抽屉，生成期间动作卡锁定。</div>
      </div>
    </div>
  );
}
