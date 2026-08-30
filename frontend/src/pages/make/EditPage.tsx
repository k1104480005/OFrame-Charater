// 制作 > 编辑 sub-page (workbench-ui spec 10.3): lightweight editing wired to
// core/service.EditDirection (editing spec 7.1–7.5). 帧级(去背景)、序列级(删除/
// 顺序/时长)、锚点级(偏移, 单帧或整方向集)、批量(统一去背景) —— 全部以可回放
// 编辑指令发送, 服务端追加式记录并写回当前版本资产。
import { useCallback, useEffect, useMemo, useState } from "react";
import type {
  CandidatePreviewView,
  EditInstructionInput,
  EditResultView,
  MotionView,
} from "../../api/client";
import { editDirection, fetchDirectionPreview, fetchMotions } from "../../api/client";
import { PixelCanvas } from "../../components/PixelCanvas";
import { useSession } from "../../state/SessionContext";
import { useWork } from "../../state/WorkContext";
import "./EditPage.css";

export function EditPage() {
  const { pkg } = useSession();
  // 编辑对象来自共享工作上下文：与制作/验收/导出共享同一动作与方向选择。
  const { motionId, selectMotion, direction, selectDirection, bumpPreview } = useWork();
  const [motions, setMotions] = useState<MotionView[]>([]);
  const [preview, setPreview] = useState<CandidatePreviewView | null>(null);
  const [selected, setSelected] = useState(0);
  const [duration, setDuration] = useState("100");
  const [deltaX, setDeltaX] = useState("0");
  const [deltaY, setDeltaY] = useState("0");
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [okMsg, setOkMsg] = useState<string | null>(null);

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

  const loadPreview = useCallback(async (mid: string, dir: string) => {
    try {
      const p = await fetchDirectionPreview(mid, dir);
      setPreview(p);
      setSelected(0);
      setDuration(String(p.frames[0]?.durationMs ?? 100));
      setError(null);
    } catch (e) {
      setError(String(e));
    }
  }, []);

  useEffect(() => {
    if (motionId && direction) void loadPreview(motionId, direction);
  }, [motionId, direction, loadPreview]);

  const motion = useMemo(() => motions.find((m) => m.id === motionId) ?? null, [motions, motionId]);
  const frames = preview?.frames ?? [];
  const frameCount = frames.length;

  const apply = async (instructions: EditInstructionInput[], label: string) => {
    if (!motionId || !direction) return;
    setBusy(label);
    setError(null);
    setOkMsg(null);
    try {
      const res: EditResultView = await editDirection(motionId, direction, instructions);
      setOkMsg(`${label} 已应用 —— ${res.frameCount} 帧 · 操作日志 #${res.logSeq}`);
      await loadPreview(motionId, direction);
      bumpPreview();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  };

  const handleDelete = () =>
    apply([{ kind: "delete", frameIndex: selected }], `删除帧 ${selected}`);

  const handleMove = (delta: -1 | 1) => {
    const target = selected + delta;
    if (target < 0 || target >= frameCount) return;
    const order = frames.map((_, i) => i);
    order[selected] = target;
    order[target] = selected;
    void apply([{ kind: "reorder", order }], `帧 ${selected} ${delta < 0 ? "上移" : "下移"}`);
  };

  const handleDuration = () => {
    const ms = parseInt(duration, 10);
    if (!Number.isFinite(ms) || ms <= 0) {
      setError("帧时长必须是正整数（毫秒）");
      return;
    }
    void apply([{ kind: "duration", frameIndex: selected, durationMs: ms }], "帧时长");
  };

  const handleCleanupAll = () => {
    // 批量: 统一去背景应用到方向集全部帧 (spec: 同一校正批量应用)
    const instructions = frames.map((_, i) => ({ kind: "cleanup", frameIndex: i }));
    void apply(instructions, "批量去背景");
  };

  const handleAnchorDelta = (all: boolean) => {
    const dx = parseInt(deltaX, 10) || 0;
    const dy = parseInt(deltaY, 10) || 0;
    if (dx === 0 && dy === 0) {
      setError("锚点偏移量不能全为 0");
      return;
    }
    const instructions = all
      ? frames.map((_, i) => ({ kind: "anchor-delta", frameIndex: i, deltaX: dx, deltaY: dy }))
      : [{ kind: "anchor-delta", frameIndex: selected, deltaX: dx, deltaY: dy }];
    void apply(instructions, all ? "锚点偏移(整方向集)" : `锚点偏移(帧 ${selected})`);
  };

  const selectedFrame = frames[selected] ?? null;

  return (
    <div className="pixel-panel col">
      <h3 className="mono panel-heading">轻量编辑 / EDIT</h3>
      <hr className="pixel-rule" />
      {error && <div className="error-text">{error}</div>}
      {okMsg && <div className="status-ok">{okMsg}</div>}

      {/* 动作 / 方向选择 */}
      <section>
        <div className="row">
          <label className="faint" htmlFor="edit-motion">
            动作
          </label>
          <select
            id="edit-motion"
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
          <label className="faint" htmlFor="edit-direction">
            方向
          </label>
          <select id="edit-direction" value={direction} onChange={(e) => selectDirection(e.target.value)} aria-label="选择方向">
            <option value="">— 选择方向 —</option>
            {motion?.directions.map((d) => (
              <option key={d.direction} value={d.direction}>
                {d.direction}
                {d.origin === "mirrored" ? " (镜像)" : d.origin === "replaced" ? " (已替换)" : ""}
              </option>
            ))}
          </select>
          <span className="faint mono">编辑对象：已接受资产（帧文件 + 动作元数据）</span>
        </div>
      </section>

      {!motionId || !direction ? (
        <div className="empty-state">选择动作与方向后，可编辑其已接受的动画资产（帧/序列/锚点，指令可回放）</div>
      ) : (
        <>
          {/* 预览 */}
          <section className="pixel-panel">
            <h4 className="mono">PixelPerfect 预览</h4>
            <PixelCanvas
              unitWidth={preview?.canvasWidth || 16}
              unitHeight={preview?.canvasHeight || 16}
              scale={12}
               frameIndex={selected}
              frames={(preview?.frames ?? []).map((f) => ({
                png: f.png,
                durationMs: f.durationMs,
                anchors: (f.anchors ?? []).map((an) => ({ name: an.Name, x: an.X, y: an.Y })),
              }))}
              playing={false}
              showGrid={true}
              showAnchors={true}
              label={`${motion?.name ?? ""} / ${direction}`}
            />
          </section>

          {/* 帧条 + 序列级编辑 */}
          <section>
            <h4 className="mono">帧序列（{frameCount} 帧）</h4>
            <div className="row edit-frames">
              {frames.map((f, i) => (
                <button
                  key={i}
                  className={`pixel-btn edit-frame${i === selected ? " edit-frame--selected" : ""}`}
                  onClick={() => {
                    setSelected(i);
                    setDuration(String(f.durationMs ?? 100));
                  }}
                  title={`帧 ${i} · ${f.durationMs}ms`}
                >
                  <img src={`data:image/png;base64,${f.png}`} alt={`帧 ${i}`} />
                  <span className="mono">{i}</span>
                </button>
              ))}
            </div>
            <div className="row">
              <button className="pixel-btn pixel-btn--warn" disabled={busy !== null || frameCount <= 1} onClick={() => void handleDelete()}>
                删除选中帧
              </button>
              <button className="pixel-btn" disabled={busy !== null || selected <= 0} onClick={() => void handleMove(-1)}>
                上移
              </button>
              <button className="pixel-btn" disabled={busy !== null || selected >= frameCount - 1} onClick={() => void handleMove(1)}>
                下移
              </button>
              <label className="faint" htmlFor="edit-duration">
                帧时长(ms)
              </label>
              <input id="edit-duration" className="pixel-input edit-num" value={duration} onChange={(e) => setDuration(e.target.value)} />
              <button className="pixel-btn" disabled={busy !== null || !selectedFrame} onClick={() => void handleDuration()}>
                应用时长
              </button>
            </div>
            {selectedFrame && (
              <div className="faint mono">
                选中帧 {selected} · 时长 {selectedFrame.durationMs}ms
                {selectedFrame.anchors?.length ? ` · 锚点 ${selectedFrame.anchors.map((an) => `${an.Name}(${an.X},${an.Y})`).join(" ")}` : " · 无锚点"}
              </div>
            )}
          </section>

          {/* 批量 + 锚点级编辑 */}
          <section>
            <h4 className="mono">批量 / 锚点</h4>
            <div className="row">
              <button className="pixel-btn" disabled={busy !== null || frameCount === 0} onClick={() => void handleCleanupAll()}>
                批量去背景（全部帧）
              </button>
            </div>
            <div className="row">
              <label className="faint" htmlFor="edit-dx">
                ΔX
              </label>
              <input id="edit-dx" className="pixel-input edit-num" value={deltaX} onChange={(e) => setDeltaX(e.target.value)} />
              <label className="faint" htmlFor="edit-dy">
                ΔY
              </label>
              <input id="edit-dy" className="pixel-input edit-num" value={deltaY} onChange={(e) => setDeltaY(e.target.value)} />
              <button className="pixel-btn" disabled={busy !== null || !selectedFrame} onClick={() => void handleAnchorDelta(false)}>
                锚点偏移（选中帧）
              </button>
              <button className="pixel-btn" disabled={busy !== null || frameCount === 0} onClick={() => void handleAnchorDelta(true)}>
                锚点偏移（整方向集）
              </button>
            </div>
            <div className="faint">所有编辑以可回放指令追加记录（操作日志支持回退）；帧级像素笔刷/橡皮/裁切由核心支持，画布交互为后续增强。</div>
          </section>
        </>
      )}
    </div>
  );
}
