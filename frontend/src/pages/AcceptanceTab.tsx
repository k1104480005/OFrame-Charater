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
import { useSession } from "../state/SessionContext";

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
  const [motions, setMotions] = useState<MotionView[]>([]);
  const [motionId, setMotionId] = useState("");
  const [direction, setDirection] = useState("");
  const [preview, setPreview] = useState<CandidatePreviewView | null>(null);
  const [history, setHistory] = useState<CandidateHistoryView[]>([]);
  const [assets, setAssets] = useState<AcceptedAssetView[]>([]);
  const [log, setLog] = useState<OperationLogEntryView[]>([]);
  const [score, setScore] = useState<ConsistencyScoreView | null>(null);
  const [playing, setPlaying] = useState(false);
  const [showGrid, setShowGrid] = useState(true);
  const [showAnchors, setShowAnchors] = useState(true);
  const [showMatting, setShowMatting] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [note, setNote] = useState("");
  const [replacePlan, setReplacePlan] = useState<GenerationPlanView | null>(null);

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
      setPlaying(true);
      setError(null);
    } catch (e) {
      setError(String(e));
    }
  }, []);

  useEffect(() => {
    if (motionId && direction) void loadPreview(motionId, direction);
  }, [motionId, direction, loadPreview]);

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

  const handleDecide = (candidateId: string, confirm: boolean) =>
    run(`decide:${candidateId}`, async () => {
      const d: AcceptanceDecisionView = await decideCandidate(candidateId, confirm, note);
      if (confirm && d.decision !== "accepted") setError(`未通过：${d.note || "评分未达阈值"}`);
      setNote("");
      await refresh();
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
      if (motionId && direction) await loadPreview(motionId, direction);
    });

  const handleScore = (useAI: boolean) =>
    run("score", async () => {
      const s: ConsistencyScoreView = await fetchConsistencyScore(useAI);
      setScore(s);
    });

  const handleRollback = (seq: number) => {
    if (!window.confirm(`回退到操作日志第 ${seq} 条？身份包内容将恢复该点状态，后续日志保留。`)) return;
    void run(`rollback:${seq}`, async () => {
      const l = await rollbackTo(seq);
      setLog(l);
      await refresh();
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
            onChange={(e) => {
              setMotionId(e.target.value);
              setDirection("");
            }}
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
            onChange={(e) => setDirection(e.target.value)}
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
        <div className="row">
          <button className="pixel-btn" disabled={!preview} onClick={() => setPlaying((p) => !p)}>
            {playing ? "暂停" : "播放"}
          </button>
          <button className="pixel-btn" onClick={() => setShowGrid((v) => !v)}>
            网格 {showGrid ? "开" : "关"}
          </button>
          <button className="pixel-btn" onClick={() => setShowAnchors((v) => !v)}>
            锚点 {showAnchors ? "开" : "关"}
          </button>
          <button className="pixel-btn" onClick={() => setShowMatting((v) => !v)}>
            洋红检查 {showMatting ? "开" : "关"}
          </button>
        </div>
        <PixelCanvas
          unitWidth={preview?.canvasWidth || 16}
          unitHeight={preview?.canvasHeight || 16}
          scale={12}
          frames={(preview?.frames ?? []).map((f) => ({
            png: f.png,
            durationMs: f.durationMs,
            anchors: (f.anchors ?? []).map((a) => ({ name: a.Name, x: a.X, y: a.Y })),
          }))}
          playing={playing}
          showMatting={showMatting}
          showGrid={showGrid}
          showAnchors={showAnchors}
          label={direction ? `${motion?.name ?? ""} / ${direction}` : undefined}
        />
        <div className="faint">最近邻采样回放 —— 预览渲染与切片结果逐像素一致（任务 5.5）</div>
      </section>

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
              <li key={c.id} className="row">
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

      <div className="faint">验收关口：评分达阈值 且 在 PixelPerfect 预览中确认，方可通过（任务 8.3）</div>
    </div>
  );
}
