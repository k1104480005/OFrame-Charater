// 制作 > 动作 sub-page（阶段 5：动作/方向集真实绑定）。创建/选择动作、方向策略
// （单方向 / 4 / 8 自动镜像 / 关闭镜像）、filmstrip 生成（生成确认 → 队列任务，
// 进度见全局任务抽屉）、方向集帧数与节奏展示。
import { useCallback, useEffect, useState } from "react";
import type { GenerationPlanView, MotionView } from "../../api/client";
import { confirmGeneration, createMotion, fetchMotions, prepareGeneration, setMotionFrameDurations, setMotionStrategy } from "../../api/client";
import { useSession } from "../../state/SessionContext";

const STRATEGY_LABEL: Record<string, string> = {
  "1": "单方向（down/正面）",
  "4": "4 方向（自动镜像）",
  "8": "8 方向（自动镜像）",
};

export function MotionPage() {
  const { pkg } = useSession();
  const [motions, setMotions] = useState<MotionView[]>([]);
  const [motionId, setMotionId] = useState("");
  const [name, setName] = useState("");
  const [count, setCount] = useState("1");
  const [mirror, setMirror] = useState(true);
  const [plan, setPlan] = useState<GenerationPlanView | null>(null);
  const [result, setResult] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [tempo, setTempo] = useState<number[]>([]);
  const [tempoDir, setTempoDir] = useState("");

  const refresh = useCallback(async () => {
    try {
      setMotions(await fetchMotions());
      setError(null);
    } catch (e) {
      setError(String(e));
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh, pkg?.path]);

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

  const motion = motions.find((m) => m.id === motionId) ?? null;

  const handleCreate = () =>
    run("create", async () => {
      const m = await createMotion(name.trim() || "新动作", parseInt(count, 10) || 1, mirror);
      setName("");
      await refresh();
      setMotionId(m.id);
    });

  const handleSetStrategy = () =>
    run("strategy", async () => {
      if (!motionId) return;
      await setMotionStrategy(motionId, parseInt(count, 10) || 1, mirror);
      await refresh();
    });

  const handlePrepare = () =>
    run("prepare", async () => {
      if (!motionId) return;
      setResult(null);
      setPlan(null);
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
      });
      setPlan(p);
    });

  const handleConfirm = (accept: boolean) =>
    run("confirm", async () => {
      if (!plan) return;
      const r = await confirmGeneration(plan.id, accept);
      if (accept) {
        setPlan(null);
        setResult(
          r.status === "executed"
            ? `已执行：${r.callsMade} 次调用 / ${r.attempts} 次尝试` + r.results.map((x) => ` · ${x.direction}`).join("")
            : `失败：${r.error || r.status}`,
        );
        await refresh();
      } else {
        setResult("已取消，未发起任何调用");
      }
    });

  const handleTempo = (dir: string) =>
    run("tempo", async () => {
      if (!motionId) return;
      setTempoDir(dir);
      const t = await setMotionFrameDurations(motionId, dir, tempo.filter((v) => Number.isFinite(v) && v > 0));
      setTempo(t.directions.find((d) => d.direction === dir)?.frames.map((f) => f.durationMs) ?? []);
      setResult(`已保存 ${dir} 方向帧时长（${tempo.length} 帧）`);
      await refresh();
    });

  const loadTempo = (dir: string) => {
    const d = motion?.directions.find((x) => x.direction === dir);
    setTempo(d?.frames.map((f) => f.durationMs) ?? []);
    setTempoDir(dir);
  };

  return (
    <div className="pixel-panel col">
      <h3 className="mono panel-heading">动作与方向集 / MOTION</h3>
      <hr className="pixel-rule" />
      {error && <div className="error-text">{error}</div>}

      {/* 创建动作 */}
      <section>
        <h4 className="mono">新建动作</h4>
        <div className="row">
          <label className="faint" htmlFor="motion-name">
            名称
          </label>
          <input id="motion-name" className="pixel-input" value={name} onChange={(e) => setName(e.target.value)} placeholder="如 walk / idle" />
          <label className="faint" htmlFor="motion-count">
            方向
          </label>
          <select id="motion-count" value={count} onChange={(e) => setCount(e.target.value)} aria-label="方向策略">
            <option value="1">单方向（1）</option>
            <option value="4">4 方向</option>
            <option value="8">8 方向</option>
          </select>
          <label className="faint" htmlFor="motion-mirror">
            <input id="motion-mirror" type="checkbox" checked={mirror} onChange={(e) => setMirror(e.target.checked)} /> 自动镜像
          </label>
          <button className="pixel-btn pixel-btn--primary" disabled={busy === "create"} onClick={() => void handleCreate()}>
            {busy === "create" ? "创建中…" : "创建动作"}
          </button>
        </div>
        {!mirror && <div className="faint">自动镜像关闭：所有方向独立生成（不再派生镜像方向）</div>}
      </section>

      {/* 动作选择 + 策略调整 */}
      <section>
        <div className="row">
          <label className="faint" htmlFor="motion-select">
            动作
          </label>
          <select
            id="motion-select"
            value={motionId}
            onChange={(e) => {
              setMotionId(e.target.value);
              setPlan(null);
            }}
            aria-label="选择动作"
          >
            <option value="">— 选择动作 —</option>
            {motions.map((m) => (
              <option key={m.id} value={m.id}>
                {m.name}（{m.strategy.count} 方向{m.strategy.mirror ? " · 镜像" : " · 全生成"}）
              </option>
            ))}
          </select>
          <button className="pixel-btn" disabled={!motionId || busy === "strategy"} onClick={() => void handleSetStrategy()}>
            应用左侧策略到当前动作
          </button>
        </div>
      </section>

      {/* 生成确认 */}
      <section>
        <h4 className="mono">filmstrip 生成（生成确认）</h4>
        <div className="row">
          <button className="pixel-btn pixel-btn--primary" disabled={!motionId || busy === "prepare"} onClick={() => void handlePrepare()}>
            {busy === "prepare" ? "计算中…" : "生成确认预览（不发起调用）"}
          </button>
          {motion && <span className="faint mono">当前策略：{STRATEGY_LABEL[String(motion.strategy.count)] ?? motion.strategy.count} 方向</span>}
        </div>
        {plan && (
          <div className="pixel-panel gen-plan">
            <ul className="mono gen-plan__list">
              <li>provider / model：{plan.providerId} / {plan.model}</li>
              <li>
                方向数：{plan.directions}（{plan.basicDirections} 生成 + {plan.mirroredDirections} 镜像）
              </li>
              <li>
                预计调用量：{plan.expectedCalls} 次 · 每方向最多 {plan.maxAttemptsPerDirection} 次总尝试 · 预算上限 {plan.maxTotalAttempts} 次
              </li>
              <li>
                预算：约 {plan.expectedCost.toFixed(2)} {plan.currency}（上限 {plan.maxCost.toFixed(2)} {plan.currency}）
              </li>
              <li className="gen-plan__prompt">
                提示词快照（{plan.prompt.stylePresetId} / {plan.prompt.actionPresetId}，{plan.prompt.frameCount} 帧）：
                <div className="faint">{plan.prompt.prompt}</div>
              </li>
            </ul>
            <div className="row">
              <button className="pixel-btn pixel-btn--primary" disabled={busy === "confirm"} onClick={() => void handleConfirm(true)}>
                {busy === "confirm" ? "执行中…" : "确认并执行"}
              </button>
              <button className="pixel-btn" disabled={busy === "confirm"} onClick={() => void handleConfirm(false)}>
                取消（不发起调用）
              </button>
            </div>
          </div>
        )}
        {result && <div className={result.startsWith("失败") ? "error-text" : "status-ok"}>{result}</div>}
      </section>

      {/* 方向集 + 节奏 */}
      {motion && (
        <section>
          <h4 className="mono">方向集（独立帧序列）</h4>
          <ul className="mono gen-plan__list">
            {motion.directions.map((d) => (
              <li key={d.direction} className="row">
                <span className={`status-badge status-${d.origin === "mirrored" ? "muted" : d.origin === "replaced" ? "warn" : "ok"}`}>
                  {d.origin === "generated" ? "生成" : d.origin === "mirrored" ? "镜像" : "替换"}
                </span>
                <span>{d.direction}</span>
                <span className="faint">
                  {d.frames.length} 帧{d.source ? ` ← ${d.source}` : ""}
                </span>
                <button className="pixel-btn" onClick={() => loadTempo(d.direction)} title="加载该方向帧时长到编辑器">
                  节奏
                </button>
              </li>
            ))}
          </ul>
          {tempoDir && (
            <div className="row">
              <span className="faint mono">帧时长（ms，{tempoDir}）：</span>
              <input
                className="pixel-input"
                value={tempo.join(", ")}
                onChange={(e) => setTempo(e.target.value.split(",").map((v) => parseInt(v.trim(), 10) || 0))}
                aria-label="帧时长列表"
              />
              <button className="pixel-btn" disabled={busy === "tempo"} onClick={() => void handleTempo(tempoDir)}>
                保存节奏
              </button>
            </div>
          )}
        </section>
      )}

      <div className="faint">生成进度与失败原因出现在全局任务抽屉（任务 6.x）；方向替换与质量验收在「验收」标签。</div>
    </div>
  );
}
