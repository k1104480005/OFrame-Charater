// 验收标签 (workbench-ui spec 10.4): quality scores display, PixelPerfect
// preview playback + confirmation, candidate accept/reject, and direction
// replacement. Backed by the quality (8.2–8.4) and versioning (9.2–9.4)
// capabilities through the shared service.
import { useCallback, useEffect, useMemo, useState } from "react";
import type {
  AcceptanceDecisionView,
  AcceptedAssetView,
  CandidateHistoryView,
  CandidatePreviewView,
  ConsistencyScoreView,
  GenerationPlanView,
  MotionView,
  OperationLogEntryView,
} from "../api/client";
import {
  confirmGeneration,
  decideCandidate,
  fetchCandidateHistory,
  fetchConsistencyScore,
  fetchCurrentAssets,
  fetchDirectionPreview,
  fetchMotions,
  fetchOperationLog,
  prepareGeneration,
  rollbackTo,
} from "../api/client";
import { PixelCanvas } from "../components/PixelCanvas";
import { ConfirmModal } from "../components/ConfirmModal";
import { EditPage } from "./make/EditPage";
import { useSession } from "../state/SessionContext";
import { useWork } from "../state/WorkContext";

const STATUS_LABEL: Record<string, string> = {
  pending: "未验收",
  accepted: "已接受",
  rejected: "已拒绝",
};

const ACTION_LABEL: Record<string, string> = {
  generation: "生成",
  regeneration: "重新生成",
  mirror_replacement: "镜像替换",
  acceptance: "候选接受",
  version_commit: "外观修订",
  rollback: "回退",
  edit: "编辑",
};

export function AcceptanceTab() {
  const { pkg } = useSession();
  // 动作/方向/预览控件全部来自共享工作上下文：一次选择，所有视图同步；
  // 局部 preview（候选帧数据）与上下文 preview（播放控件）重名，解构改名。
  const {
    motionId, direction, candidateId, selectMotion, selectDirection, focusCandidate,
    preview: previewControls, setPreview: setPreviewControls, bumpPreview, previewNonce,
  } = useWork();
  const [motions, setMotions] = useState<MotionView[]>([]);
  const [preview, setPreview] = useState<CandidatePreviewView | null>(null);
  const [history, setHistory] = useState<CandidateHistoryView[]>([]);
  const [assets, setAssets] = useState<AcceptedAssetView[]>([]);
  const [log, setLog] = useState<OperationLogEntryView[]>([]);
  const [score, setScore] = useState<ConsistencyScoreView | null>(null);
  const [showMatting, setShowMatting] = useState(false);
  const [frameIndex, setFrameIndex] = useState(0);
  const [zoom, setZoom] = useState(12);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [note, setNote] = useState("");
  const [regenFeedback, setRegenFeedback] = useState("");
  const [regenPlan, setRegenPlan] = useState<GenerationPlanView | null>(null);
  const [replacePlan, setReplacePlan] = useState<GenerationPlanView | null>(null);
  const [rollbackSeq, setRollbackSeq] = useState<number | null>(null);
  const [editOpen, setEditOpen] = useState(false);

  const refresh = useCallback(async () => {
    try {
      const [ms, h, a, l] = await Promise.all([
        fetchMotions(),
        fetchCandidateHistory(),
        fetchCurrentAssets(),
        fetchOperationLog(),
      ]);
      setMotions(ms);
      setHistory(h);
      setAssets(a);
      setLog(l);
      setError(null);
    } catch (e) {
      setError(String(e));
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh, pkg?.path]);

  const loadPreview = useCallback(async (mid: string, dir: string) => {
    try {
      const p = await fetchDirectionPreview(mid, dir);
      setPreview(p);
      setPreviewControls({ playing: true });
      setError(null);
    } catch (e) {
      setError(String(e));
    }
  }, [setPreviewControls]);

  useEffect(() => {
    if (motionId && direction) void loadPreview(motionId, direction);
  }, [motionId, direction, loadPreview]);

  // 工件变更信号（生成完成/验收决策/替换/回滚）：同方向也强制重载预览，
  // 无需用户手动重新选择（task 3.2 / workbench-context-preview spec）。
  useEffect(() => {
    if (previewNonce === 0) return;
    if (motionId && direction) void loadPreview(motionId, direction);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [previewNonce]);

  const run = async (key: string, fn: () => Promise<void>) => {
    setBusy(key);
    setError(null);
    try {
      await fn();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  };

  const motion = useMemo(() => motions.find((m) => m.id === motionId) ?? null, [motions, motionId]);
  const selectedDirection = useMemo(() => motion?.directions.find((d) => d.direction === direction) ?? null, [motion, direction]);
  const previewFrames = preview?.frames ?? [];
  // 稳定引用：预览内容不变时 PixelCanvas 不重建（避免播放逐帧渲染触发
  // PixiJS 场景销毁/初始化竞态，见 PixelCanvas 的 initialized 保护）。
  const previewFrameData = useMemo(
    () =>
      previewFrames.map((f) => ({
        png: f.png,
        durationMs: f.durationMs,
        anchors: (f.anchors ?? []).map((a) => ({ name: a.Name, x: a.X, y: a.Y })),
      })),
    [previewFrames],
  );
  const currentFrame = previewFrames[frameIndex];
  const currentFps = currentFrame?.durationMs && currentFrame.durationMs > 0
    ? Math.round(1000 / currentFrame.durationMs)
    : 0;

  useEffect(() => {
    setFrameIndex(0);
  }, [motionId, direction, previewNonce]);

  const handleDecide = (candidateId: string, confirm: boolean) =>
    run(`decide:${candidateId}`, async () => {
      const d: AcceptanceDecisionView = await decideCandidate(candidateId, confirm, note);
      if (confirm && d.decision !== "accepted") setError(`未通过：${d.note || "评分未达阈值"}`);
      setNote("");
      await refresh();
      bumpPreview();
      if (motionId && direction) await loadPreview(motionId, direction);
    });

  const handleReplacePrepare = () =>
    run("replace-prepare", async () => {
      if (!motionId || !direction) return;
      setReplacePlan(null);
      const p = await prepareGeneration({
        packagePath: "",
        motionId,
        providerId: "",
        model: "",
        directions: 0,
        stylePresetId: "",
        actionPresetId: "",
        frameCount: 0,
        maxAttemptsPerDirection: 0,
        replaceDirections: [direction],
      });
      setReplacePlan(p);
    });

  const handleReplaceConfirm = (accept: boolean) =>
    run("replace-confirm", async () => {
      if (!replacePlan) return;
      await confirmGeneration(replacePlan.id, accept);
      setReplacePlan(null);
      await refresh();
      bumpPreview();
      if (motionId && direction) await loadPreview(motionId, direction);
    });

  const handleRegeneratePrepare = () =>
    run("regenerate-prepare", async () => {
      if (!motionId || !direction || !candidateId) return;
      if (!regenFeedback.trim()) {
        setError("请先填写本次重新生成希望改进的内容");
        return;
      }
      setRegenPlan(null);
      const p = await prepareGeneration({
        packagePath: "",
        motionId,
        providerId: "",
        model: "",
        directions: 0,
        stylePresetId: "",
        actionPresetId: "",
        feedback: regenFeedback.trim(),
        frameCount: 0,
        maxAttemptsPerDirection: 0,
        regenerateOf: candidateId,
      });
      setRegenPlan(p);
    });

  const handleRegenerateConfirm = (accept: boolean) =>
    run("regenerate-confirm", async () => {
      if (!regenPlan) return;
      const r = await confirmGeneration(regenPlan.id, accept);
      setRegenPlan(null);
      if (accept && r.status !== "executed") {
        setError(`重新生成失败：${r.error || r.status}`);
      } else if (accept) {
        setRegenFeedback("");
      }
      await refresh();
      bumpPreview();
      if (motionId && direction) await loadPreview(motionId, direction);
    });

  const handleScore = (useAI: boolean) =>
    run("score", async () => {
      const s: ConsistencyScoreView = await fetchConsistencyScore(useAI);
      setScore(s);
    });

  const handleRollback = (seq: number) => {
    setRollbackSeq(seq);
  };

  const doRollback = async (seq: number) => {
    void run(`rollback:${seq}`, async () => {
      const l = await rollbackTo(seq);
      setLog(l);
      await refresh();
      bumpPreview();
      if (motionId && direction) await loadPreview(motionId, direction);
    });
  };

  return (
    <div className="pixel-panel col">
      <h3 className="mono panel-heading">验收 / ACCEPTANCE</h3>
      <hr className="pixel-rule" />
      {error && <div className="error-text">{error}</div>}

      {/* 动作与方向选择 */}
      <section>
        <div className="row">
          <label className="faint" htmlFor="acc-motion">
            动作
          </label>
          <select
            id="acc-motion"
            value={motionId}
            onChange={(e) => selectMotion(e.target.value)}
            aria-label="选择动作"
          >
            <option value="">— 选择动作 —</option>
            {motions.map((m) => (
              <option key={m.id} value={m.id}>
                {m.name}
              </option>
            ))}
          </select>
          <label className="faint" htmlFor="acc-direction">
            方向
          </label>
          <select
            id="acc-direction"
            value={direction}
            onChange={(e) => selectDirection(e.target.value)}
            aria-label="选择方向"
          >
            <option value="">— 选择方向 —</option>
            {motion?.directions.map((d) => (
              <option key={d.direction} value={d.direction}>
                {d.direction}
                {d.origin === "mirrored" ? " (镜像)" : d.origin === "replaced" ? " (已替换)" : ""}
              </option>
            ))}
          </select>
        </div>
        {motion && motion.directions.length > 0 && (
          <div className="direction-grid" aria-label="方向网格">
            {motion.directions.map((item) => (
              <button
                key={item.direction}
                className={`pixel-btn ${item.direction === direction ? "pixel-btn--primary" : ""}`}
                onClick={() => selectDirection(item.direction)}
                aria-label={`选择方向 ${item.direction}`}
                title={item.origin === "mirrored" ? "镜像方向" : item.origin === "replaced" ? "替换方向" : "生成方向"}
              >
                {item.direction}
              </button>
            ))}
          </div>
        )}
      </section>

      {/* PixelPerfect 预览 (task 5.5 / 10.4) */}
      <section className="pixel-panel">
        <div className="row">
          <h4 className="mono">PixelPerfect 预览</h4>
          {selectedDirection && (
            <span className={`status-badge status-${selectedDirection.origin === "mirrored" ? "muted" : "ok"} mono`}>
              {selectedDirection.origin === "generated" ? "已生成" : selectedDirection.origin === "mirrored" ? "镜像派生" : "手动替换"}
            </span>
          )}
        </div>
        <div className="row preview-toolbar">
          <button className="pixel-btn" disabled={!preview} onClick={() => setPreviewControls({ playing: !previewControls.playing })}>
            {previewControls.playing ? "暂停" : "播放"}
          </button>
          <button className="pixel-btn" onClick={() => setPreviewControls({ showGrid: !previewControls.showGrid })}>
            网格 {previewControls.showGrid ? "开" : "关"}
          </button>
          <button className="pixel-btn" onClick={() => setPreviewControls({ showAnchors: !previewControls.showAnchors })}>
            锚点 {previewControls.showAnchors ? "开" : "关"}
          </button>
          <button className="pixel-btn" onClick={() => setShowMatting((v) => !v)}>
            洋红检查 {showMatting ? "开" : "关"}
          </button>
          <button className="pixel-btn" disabled={!motionId || !direction} onClick={() => setEditOpen((open) => !open)}>
             {editOpen ? "收起编辑" : "编辑当前资产"}
           </button>
           <button className="pixel-btn" disabled={zoom <= 4} onClick={() => setZoom((v) => Math.max(4, v - 2))} aria-label="缩小预览" title="缩小预览">−</button>
          <span className="mono faint">{zoom}x</span>
          <button className="pixel-btn" disabled={zoom >= 24} onClick={() => setZoom((v) => Math.min(24, v + 2))} aria-label="放大预览" title="放大预览">+</button>
          {preview && <span className="mono faint">帧 {Math.min(frameIndex + 1, previewFrames.length)} / {previewFrames.length} · {currentFps} FPS</span>}
        </div>
        {preview ? (
          <>
          <PixelCanvas
            unitWidth={preview.canvasWidth || 16}
            unitHeight={preview.canvasHeight || 16}
            scale={zoom}
            frameIndex={frameIndex}
            frames={previewFrameData}
            playing={previewControls.playing}
            loop={motion?.loop !== false}
            onPlaybackEnd={() => {
              setPreviewControls({ playing: false });
              setFrameIndex(0);
            }}
            showMatting={showMatting}
            showGrid={previewControls.showGrid}
            showAnchors={previewControls.showAnchors}
            label={direction ? `${motion?.name ?? ""} / ${direction}` : undefined}
          />
          <label className="preview-scrubber mono" htmlFor="preview-frame">
            <span>逐帧</span>
            <input
              id="preview-frame"
              type="range"
              min={0}
              max={Math.max(0, previewFrames.length - 1)}
              value={Math.min(frameIndex, Math.max(0, previewFrames.length - 1))}
              onChange={(e) => {
                setPreviewControls({ playing: false });
                setFrameIndex(Number(e.target.value));
              }}
              disabled={previewFrames.length === 0}
              aria-label="选择预览帧"
            />
            <span className="faint">{currentFrame?.durationMs ?? 0} ms</span>
          </label>
          </>
        ) : (
          <div className="empty-state">请选择动作和方向后查看 PixelPerfect 预览</div>
        )}
        <div className="faint">最近邻采样回放 —— 预览渲染与切片结果逐像素一致</div>
      </section>

      {motion && direction && selectedDirection?.origin !== "mirrored" && (
        <section>
          <h4 className="mono">反馈重新生成</h4>
          <div className="faint">对当前候选提出具体修改意见，确认后会作为提示词的一部分发送。</div>
          <div className="row">
            <textarea
              className="pixel-input"
              rows={2}
              value={regenFeedback}
              onChange={(e) => setRegenFeedback(e.target.value)}
              placeholder="例如：手臂动作更大一些，保持帽子颜色，不要裁掉脚"
              aria-label="重新生成反馈"
              disabled={!candidateId || busy === "regenerate-prepare" || busy === "regenerate-confirm"}
            />
            <button
              className="pixel-btn pixel-btn--primary"
              disabled={!candidateId || !regenFeedback.trim() || busy === "regenerate-prepare"}
              onClick={() => void handleRegeneratePrepare()}
            >
              {busy === "regenerate-prepare" ? "计算中…" : "生成确认预览"}
            </button>
          </div>
          {regenPlan && (
            <div className="pixel-panel gen-plan">
              <ul className="mono gen-plan__list">
                <li>重新生成方向：{(regenPlan.basicLabels ?? []).join(", ") || direction} · 预计调用量 {regenPlan.expectedCalls} 次</li>
                <li>provider / model：{regenPlan.providerId} / {regenPlan.model} · 预算上限 {regenPlan.maxTotalAttempts} 次尝试</li>
                <li>反馈快照：{regenPlan.prompt.feedback || regenFeedback}</li>
              </ul>
              <div className="row">
                <button className="pixel-btn pixel-btn--primary" disabled={busy === "regenerate-confirm"} onClick={() => void handleRegenerateConfirm(true)}>
                  {busy === "regenerate-confirm" ? "执行中…" : "确认并重新生成"}
                </button>
                <button className="pixel-btn" disabled={busy === "regenerate-confirm"} onClick={() => void handleRegenerateConfirm(false)}>取消</button>
              </div>
            </div>
          )}
        </section>
      )}

      {editOpen && motionId && direction && <EditPage />}

      {/* 一致性粗评分 (task 8.2: 仅参考、不阻塞) */}
      <section>
        <div className="row">
          <h4 className="mono">一致性粗评分（参考）</h4>
          <button className="pixel-btn" disabled={busy === "score"} onClick={() => void handleScore(false)}>
            本地评估
          </button>
          <button className="pixel-btn" disabled={busy === "score"} onClick={() => void handleScore(true)}>
            AI 评估
          </button>
          {score && (
            <span className={`mono status-badge status-${score.source === "ai" ? "ok" : "muted"}`}>
              {score.score.toFixed(2)} · {score.source === "ai" ? "AI" : "本地"}
            </span>
          )}
        </div>
        {score?.detail && <div className="faint">{score.detail} —— 仅作参考，不阻塞验收流程</div>}
      </section>

      {/* 候选历史 + 接受/拒绝 (task 8.3/8.4/9.2) */}
      <section>
        <h4 className="mono">候选历史</h4>
        {history.length === 0 ? (
          <div className="empty-state">暂无候选 —— 在「制作」中生成后，候选会连同评分与验收结果记录在这里</div>
        ) : (
          <ul className="mono gen-plan__list">
            {history.map((c) => (
              <li key={c.id} className={`row ${candidateId === c.id ? "candidate-row--current" : ""}`}>
                <button
                  className="pixel-btn"
                  onClick={() => focusCandidate(c.id, c.motionId, c.direction)}
                  aria-label={`预览候选 ${c.id.slice(0, 8)}`}
                  title="将此候选设为当前预览对象"
                >
                  预览
                </button>
                <span className={`status-badge status-${c.status === "accepted" ? "ok" : c.status === "rejected" ? "warn" : "muted"}`}>
                  {STATUS_LABEL[c.status] ?? c.status}
                </span>
                <span className="faint">{c.direction || "—"}</span>
                <span>
                  {c.id.slice(0, 8)} · 综合 {c.overall.toFixed(2)}
                </span>
                {c.acceptanceNote && <span className="faint">{c.acceptanceNote}</span>}
                {c.status === "pending" && (
                  <span className="row">
                    <input
                      className="pixel-input"
                      placeholder="验收备注（可选）"
                      value={note}
                      onChange={(e) => setNote(e.target.value)}
                    />
                    <button className="pixel-btn pixel-btn--primary" disabled={busy === `decide:${c.id}`} onClick={() => void handleDecide(c.id, true)}>
                      预览确认通过
                    </button>
                    <button className="pixel-btn" disabled={busy === `decide:${c.id}`} onClick={() => void handleDecide(c.id, false)}>
                      拒绝
                    </button>
                  </span>
                )}
              </li>
            ))}
          </ul>
        )}
      </section>

      {/* 方向替换 (task 3.5/10.4: 验收时以额外调用替换) */}
      <section>
        <h4 className="mono">方向替换</h4>
        <div className="row">
          <button className="pixel-btn" disabled={!motionId || !direction || busy === "replace-prepare"} onClick={() => void handleReplacePrepare()}>
            替换当前方向（生成确认预览）
          </button>
        </div>
        {replacePlan && (
          <div className="pixel-panel gen-plan">
            <ul className="mono gen-plan__list">
              <li>
                替换方向：{(replacePlan.basicLabels ?? []).join(", ") || direction} · 预计调用量 {replacePlan.expectedCalls} 次 · 预算上限{" "}
                {replacePlan.maxTotalAttempts} 次
              </li>
              <li>
                provider / model：{replacePlan.providerId} / {replacePlan.model}
                {replacePlan.providerType ? `（协议：${replacePlan.providerType}）` : ""} · 能力：{replacePlan.capability}
              </li>
              <li>
                预算：约 {replacePlan.expectedCost.toFixed(2)} {replacePlan.currency}
              </li>
            </ul>
            <div className="row">
              <button className="pixel-btn pixel-btn--primary" disabled={busy === "replace-confirm"} onClick={() => void handleReplaceConfirm(true)}>
                确认并执行替换
              </button>
              <button className="pixel-btn" disabled={busy === "replace-confirm"} onClick={() => void handleReplaceConfirm(false)}>
                取消
              </button>
            </div>
          </div>
        )}
      </section>

      {/* 当前动画资产 (task 9.2) */}
      <section>
        <h4 className="mono">当前动画资产</h4>
        {assets.length === 0 ? (
          <div className="empty-state">暂无已接受资产 —— 候选经预览确认并通过阈值后成为当前动画资产</div>
        ) : (
          <ul className="mono gen-plan__list">
            {assets.map((a) => (
              <li key={`${a.motionId}-${a.direction}`} className="row">
                <span className="status-badge status-ok">已接受</span>
                <span>
                  {a.motionId || "—"} / {a.direction} · {a.frameCount} 帧 · 候选 {a.candidateId.slice(0, 8)}
                </span>
              </li>
            ))}
          </ul>
        )}
      </section>

      {/* 操作日志 + 回退 (task 9.3/9.4) */}
      <section>
        <h4 className="mono">操作日志（追加式）</h4>
        {log.length === 0 ? (
          <div className="empty-state">日志为空 —— 生成/编辑/接受/镜像替换等所有变更会逐条追加记录</div>
        ) : (
          <ul className="mono gen-plan__list">
            {log.map((e) => (
              <li key={e.seq} className="row">
                <span className="faint">#{e.seq}</span>
                <span>{ACTION_LABEL[e.action] ?? e.action}</span>
                <span className="faint">{e.at}</span>
                {e.action !== "rollback" && (
                  <button className="pixel-btn" onClick={() => handleRollback(e.seq)} title="回退到该历史点（后续日志保留）">
                    回退至此
                  </button>
                )}
              </li>
            ))}
          </ul>
        )}
      </section>

      <div className="faint">验收关口：评分达阈值 且 在 PixelPerfect 预览中确认，方可通过</div>

      {/* 回退确认（动森弹窗，替代原生 confirm） */}
      <ConfirmModal
        open={rollbackSeq !== null}
        title="回退操作日志"
        message={rollbackSeq !== null ? `回退到操作日志第 ${rollbackSeq} 条？\n身份包内容将恢复该点状态，后续日志保留。` : ""}
        confirmLabel="回退"
        danger
        onConfirm={() => {
          const s = rollbackSeq;
          setRollbackSeq(null);
          if (s !== null) void doRollback(s);
        }}
        onCancel={() => setRollbackSeq(null)}
      />
    </div>
  );
}
