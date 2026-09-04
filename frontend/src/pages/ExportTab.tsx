// 导出标签 (workbench-ui spec 10.2 top-level tab + export spec 11.x): 动画资产
// 检视（帧序列 + 锚点清单）、引擎目标选择（generic / godot / unity）、导出包生成
// 与校验、导出历史。后端为共享 service（core/export + core/service/export.go），
// 前端仅展示与交互（design D11）。
import { useCallback, useEffect, useMemo, useState } from "react";
import type {
  AcceptedAssetView,
  CandidatePreviewView,
  ExportHistoryRecord,
  ExportResult,
  MotionView,
} from "../api/client";
import {
  exportPackage,
  fetchCurrentAssets,
  fetchDirectionPreview,
  fetchExportHistory,
  fetchMotions,
  validateExport,
} from "../api/client";
import { PixelCanvas } from "../components/PixelCanvas";
import { useSession } from "../state/SessionContext";
import { useWork } from "../state/WorkContext";
import "./ExportTab.css";

const TARGETS = [
  { id: "generic", label: "通用序列帧 (generic)" },
  { id: "godot", label: "Godot" },
  { id: "unity", label: "Unity" },
] as const;

const ORIGIN_LABEL: Record<string, string> = {
  generated: "生成",
  mirrored: "镜像派生",
  replaced: "手动替换",
};

export function ExportTab() {
  const { pkg } = useSession();
  // 检视对象来自共享工作上下文：导出页选中的动作/方向与制作/验收/编辑一致。
  const { motionId, direction, selectMotion, selectDirection } = useWork();
  const [assets, setAssets] = useState<AcceptedAssetView[]>([]);
  const [motions, setMotions] = useState<MotionView[]>([]);
  const [history, setHistory] = useState<ExportHistoryRecord[]>([]);

  // 检视：当前上下文对象 → 帧序列 + 锚点清单 + PixelPerfect 预览
  const [preview, setPreview] = useState<CandidatePreviewView | null>(null);
  const [showGrid, setShowGrid] = useState(true);
  const [showAnchors, setShowAnchors] = useState(true);

  // 导出配置
  const [target, setTarget] = useState<string>("generic");
  const [outputDir, setOutputDir] = useState("");
  const [result, setResult] = useState<ExportResult | null>(null);
  const [okMsg, setOkMsg] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      const [a, m, h] = await Promise.all([fetchCurrentAssets(), fetchMotions(), fetchExportHistory()]);
      setAssets(a);
      setMotions(m);
      setHistory(h);
      setError(null);
    } catch (e) {
      setError(String(e));
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh, pkg?.path]);

  // 默认输出目录：身份包 exports/<target>，与导出历史 (exports/history.jsonl) 分离
  useEffect(() => {
    if (pkg) setOutputDir(`${pkg.path}/exports/${target}`);
  }, [pkg?.path, target]);

  const motionName = useCallback(
    (motionId?: string) => motions.find((m) => m.id === motionId)?.name ?? motionId ?? "—",
    [motions],
  );

  const selectedAsset = useMemo(() => {
    return assets.find((a) => a.motionId === motionId && a.direction === direction) ?? null;
  }, [assets, motionId, direction]);

  const inspect = useCallback(
    async (mid: string, dir: string) => {
      // 深链：把检视对象写入共享上下文（一次选择，所有视图同步）。
      selectMotion(mid);
      selectDirection(dir);
      try {
        const p = await fetchDirectionPreview(mid, dir);
        setPreview(p);
        setError(null);
      } catch (e) {
        setError(String(e));
      }
    },
    [selectMotion, selectDirection],
  );

  const run = async (key: string, fn: () => Promise<void>) => {
    setBusy(key);
    setError(null);
    setOkMsg(null);
    try {
      await fn();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  };

  const handleExport = () =>
    run("export", async () => {
      if (!pkg) return;
      const r = await exportPackage(outputDir.trim(), target, pkg.currentVersion);
      setResult(r);
      setOkMsg(`导出完成并校验通过：${r.manifest.animations.length} 个动画序列`);
      await refresh();
    });

  const handleValidate = () =>
    run("validate", async () => {
      await validateExport(outputDir.trim());
      setOkMsg("导出包校验通过（帧、锚点、清单完整）");
    });

  const totalFrames = useMemo(
    () => result?.manifest.animations.reduce((n, a) => n + (a.frames?.length ?? 0), 0) ?? 0,
    [result],
  );

  return (
    <div className="pixel-panel col">
      <h3 className="mono panel-heading">导出 / EXPORT</h3>
      <hr className="pixel-rule" />
      {error && <div className="error-text">{error}</div>}

      {/* 动画资产检视 (export spec: 帧序列 + 锚点清单) */}
      <section>
        <h4 className="mono">动画资产检视（仅验收通过的资产可导出）</h4>
        {assets.length === 0 ? (
          <div className="empty-state">暂无已接受资产 —— 在「验收」标签确认候选后，资产会出现在这里并可供导出</div>
        ) : (
          <ul className="mono gen-plan__list">
            {assets.map((a) => (
              <li key={`${a.motionId}-${a.direction}`} className="row">
                <button className="pixel-btn" disabled={busy !== null} onClick={() => void inspect(a.motionId!, a.direction)}>
                  检视
                </button>
                <span className="status-badge status-ok">已接受</span>
                <span>
                  {motionName(a.motionId)} / {a.direction}
                </span>
                <span className="faint">{a.frameCount} 帧 · 候选 {a.candidateId.slice(0, 8)}</span>
              </li>
            ))}
          </ul>
        )}

        {selectedAsset && preview && (
          <div className="pixel-panel pixel-panel--inset col">
            <div className="row">
              <h4 className="mono">
                检视：{motionName(selectedAsset.motionId)} / {selectedAsset.direction}
              </h4>
              <span className={`status-badge ${preview.origin === "mirrored" ? "status-muted" : "status-ok"}`}>
                {ORIGIN_LABEL[preview.origin] ?? preview.origin}
              </span>
              <span className="faint">
                逻辑画布 {preview.canvasWidth}×{preview.canvasHeight}
              </span>
            </div>
            <div className="row">
              <button className="pixel-btn" onClick={() => setShowGrid((v) => !v)}>
                网格 {showGrid ? "开" : "关"}
              </button>
              <button className="pixel-btn" onClick={() => setShowAnchors((v) => !v)}>
                锚点 {showAnchors ? "开" : "关"}
              </button>
            </div>
            <PixelCanvas
              unitWidth={preview.canvasWidth || 16}
              unitHeight={preview.canvasHeight || 16}
              scale={12}
              frames={(preview.frames ?? []).map((f) => ({
                png: f.png,
                durationMs: f.durationMs,
                anchors: (f.anchors ?? []).map((an) => ({ name: an.Name, x: an.X, y: an.Y })),
              }))}
              playing={false}
              showGrid={showGrid}
              showAnchors={showAnchors}
              label={`${motionName(selectedAsset.motionId)} / ${selectedAsset.direction}`}
            />

            {/* 帧序列 + 锚点清单 */}
            <table className="export-frames mono">
              <thead>
                <tr>
                  <th>帧</th>
                  <th>时长(ms)</th>
                  <th>锚点</th>
                </tr>
              </thead>
              <tbody>
                {(preview.frames ?? []).map((f) => (
                  <tr key={f.index}>
                    <td>{f.index}</td>
                    <td>{f.durationMs}</td>
                    <td>{(f.anchors ?? []).map((an) => `${an.Name}(${an.X},${an.Y})`).join(" ") || "—"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {/* 引擎目标选择 + 输出目录 + 生成 (export spec 11.1/11.3) */}
      <section>
        <h4 className="mono">引擎目标与导出</h4>
        <div className="row">
          <label className="faint" htmlFor="export-target">
            引擎目标
          </label>
          <select id="export-target" value={target} onChange={(e) => setTarget(e.target.value)} aria-label="引擎目标">
            {TARGETS.map((t) => (
              <option key={t.id} value={t.id}>
                {t.label}
              </option>
            ))}
          </select>
          <span className="faint mono">身份版本：{pkg?.currentVersion ?? "—"}</span>
        </div>
        <div className="row">
          <label className="faint" htmlFor="export-outdir">
            输出目录
          </label>
          <input
            id="export-outdir"
            className="pixel-input grow"
            value={outputDir}
            onChange={(e) => setOutputDir(e.target.value)}
            placeholder="例如 D:\mygame\assets\character"
          />
        </div>
        <div className="row">
          <button className="pixel-btn pixel-btn--primary" disabled={!assets.length || busy === "export"} onClick={() => void handleExport()}>
            {busy === "export" ? "导出中…" : "生成并校验导出包"}
          </button>
          <button className="pixel-btn" disabled={!outputDir.trim() || busy === "validate"} onClick={() => void handleValidate()}>
            {busy === "validate" ? "校验中…" : "校验现有导出包"}
          </button>
        </div>
        <div className="faint">
          产出 spritesheet.png + manifest.json + 逐帧 PNG + {target}.json 目标元数据；生成后自动校验完整性，失败即报错
        </div>
        {okMsg && <div className="status-ok">{okMsg}</div>}
      </section>

      {/* 导出结果 (task 11.3) */}
      {result && (
        <section className="pixel-panel pixel-panel--inset">
          <h4 className="mono">导出结果</h4>
          <ul className="mono gen-plan__list">
            <li>
              目标：{result.target} · 身份版本：{result.manifest.identityVersion}
            </li>
            <li>
              精灵表：{result.manifest.spriteSheet}（{result.manifest.cellWidth}×{result.manifest.cellHeight} / 单元）·{" "}
              {result.manifest.animations.length} 个序列 · {totalFrames} 帧
            </li>
            <li className="faint">输出目录：{result.outputDir}</li>
          </ul>
        </section>
      )}

      {/* 导出历史 (export spec 11.4) */}
      <section>
        <h4 className="mono">导出历史</h4>
        {history.length === 0 ? (
          <div className="empty-state">暂无导出记录 —— 每次导出（目标、身份版本、时间、结果）都会追加记录</div>
        ) : (
          <ul className="mono gen-plan__list">
            {history.map((h, i) => (
              <li key={i} className="row">
                <span className={`status-badge ${h.result === "succeeded" ? "status-ok" : "status-warn"}`}>
                  {h.result === "succeeded" ? "成功" : "失败"}
                </span>
                <span>{h.target}</span>
                <span className="faint">版本 {h.identityVersion}</span>
                <span className="faint">{h.createdAt}</span>
                {h.error && <span className="status-warn">{h.error}</span>}
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
