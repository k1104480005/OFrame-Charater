// 制作 > 身份 sub-page: identity definition (text / reference image / sprite
// entries), logical canvas, anchor presets, materials, version history — all
// over the shared core identity services (tasks 2.3–2.5).
import { useCallback, useEffect, useState } from "react";
import type { AnchorPresetView, AnchorView, IdentityView, MaterialView } from "../../api/client";
import { addAnchorPreset, fetchAnchorPresets, fetchIdentity, importMaterial, pickMaterialFile, saveCanvas, saveDescription } from "../../api/client";
import { useSession } from "../../state/SessionContext";
import "./IdentityPage.css";

const ENTRY_LABEL: Record<string, string> = {
  text: "文字描述",
  reference_image: "参考图",
  sprite: "既有精灵",
};

const ROLE_LABEL: Record<string, string> = {
  main_reference: "主参考图",
  auxiliary_reference: "辅助参考图",
  sprite: "既有精灵",
};

export function IdentityPage() {
  const { pkg } = useSession();
  const [view, setView] = useState<IdentityView | null>(null);
  const [presets, setPresets] = useState<AnchorPresetView[]>([]);
  const [description, setDescription] = useState("");
  const [canvasW, setCanvasW] = useState("16");
  const [canvasH, setCanvasH] = useState("16");
  const [presetId, setPresetId] = useState("feet");
  const [anchorName, setAnchorName] = useState("");
  const [importRole, setImportRole] = useState("main_reference");
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [okMsg, setOkMsg] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const [v, ps] = await Promise.all([fetchIdentity(), fetchAnchorPresets()]);
      setView(v);
      setPresets(ps);
      setDescription(v.description);
      if (v.canvas) {
        setCanvasW(String(v.canvas.unitWidth));
        setCanvasH(String(v.canvas.unitHeight));
      }
    } catch (e) {
      setError(String(e));
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load, pkg]);

  const flash = (msg: string) => {
    setOkMsg(msg);
    window.setTimeout(() => setOkMsg(null), 2500);
  };

  const run = async (key: string, fn: () => Promise<void>) => {
    setBusy(key);
    setError(null);
    try {
      await fn();
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  };

  const handleSaveDescription = () =>
    run("desc", async () => {
      await saveDescription(description);
      flash("身份描述已保存");
    });

  const handleSaveCanvas = () => {
    const w = parseInt(canvasW, 10);
    const h = parseInt(canvasH, 10);
    if (!Number.isFinite(w) || !Number.isFinite(h) || w <= 0 || h <= 0) {
      setError("逻辑画布尺寸必须为正整数");
      return;
    }
    return run("canvas", async () => {
      await saveCanvas(w, h);
      flash("逻辑画布已保存");
    });
  };

  const handleAddPreset = () =>
    run("preset", async () => {
      await addAnchorPreset(presetId, anchorName.trim());
      flash("锚点已添加");
      setAnchorName("");
    });

  const handleImport = (kind: "reference_image" | "sprite") =>
    run(`import-${kind}`, async () => {
      // File picking happens Go-side (PickMaterialFile → native dialog);
      // the frontend just triggers it and passes the chosen path through.
      const path = await pickMaterialFile(`选择${kind === "reference_image" ? "参考图" : "既有精灵"}`);
      if (!path) return; // cancelled
      // 参考图按 1 主参考图 + 最多 2 辅助参考图语义导入（阶段 3）.
      await importMaterial(kind, path, "", kind === "reference_image" ? importRole : "");
      flash("素材已导入身份包素材区");
    });

  if (!view) {
    return <div className="muted">{error ? <span className="error-text">{error}</span> : "加载身份定义…"}</div>;
  }

  return (
    <div className="identity">
      <section className="pixel-panel">
        <h3 className="mono panel-heading">身份定义 / IDENTITY</h3>
        <hr className="pixel-rule" />
        <div className="identity__meta">
          <span className="mono">ID {view.id}</span>
          <span className="mono">入口 {ENTRY_LABEL[view.entryKind] ?? "未设置"}</span>
          <span className="mono">当前版本 {view.currentVersion}</span>
        </div>
        <div className="field-row">
          <span className="field-label">文字描述</span>
          <div className="col">
            <textarea value={description} onChange={(e) => setDescription(e.target.value)} placeholder="描述角色外观、配色、体型…" />
            <div>
              <button className="pixel-btn pixel-btn--primary" disabled={busy === "desc"} onClick={() => void handleSaveDescription()}>
                {busy === "desc" ? "保存中…" : "保存描述"}
              </button>
            </div>
          </div>
        </div>
        <div className="field-row">
          <span className="field-label">参考图 / 既有精灵</span>
          <div className="col">
            <div className="row">
              <label className="faint" htmlFor="ref-role">参考图角色</label>
              <select id="ref-role" value={importRole} onChange={(e) => setImportRole(e.target.value)} aria-label="参考图角色">
                <option value="main_reference">主参考图（最多 1 张）</option>
                <option value="auxiliary_reference">辅助参考图（最多 2 张）</option>
              </select>
            </div>
            <div className="row">
              <button className="pixel-btn" disabled={busy === "import-reference_image"} onClick={() => void handleImport("reference_image")}>
                添加参考图
              </button>
              <button className="pixel-btn" disabled={busy === "import-sprite"} onClick={() => void handleImport("sprite")}>
                导入既有精灵
              </button>
            </div>
          </div>
        </div>
        {view.materials.length > 0 && (
          <ul className="identity__materials">
            {view.materials.map((m: MaterialView) => (
              <li key={m.id} className="mono">
                {ROLE_LABEL[m.role] ?? m.kind} · {m.name} · {m.path}
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="pixel-panel">
        <h3 className="mono panel-heading">逻辑画布 / CANVAS</h3>
        <hr className="pixel-rule" />
        <div className="field-row">
          <span className="field-label">单元尺寸（宽 × 高）</span>
          <div className="row">
            <input className="identity__num" value={canvasW} onChange={(e) => setCanvasW(e.target.value)} aria-label="画布宽" />
            <span>×</span>
            <input className="identity__num" value={canvasH} onChange={(e) => setCanvasH(e.target.value)} aria-label="画布高" />
            <button className="pixel-btn" disabled={busy === "canvas"} onClick={() => void handleSaveCanvas()}>
              {busy === "canvas" ? "保存中…" : "保存规格"}
            </button>
          </div>
        </div>
      </section>

      <section className="pixel-panel">
        <h3 className="mono panel-heading">锚点 / ANCHORS</h3>
        <hr className="pixel-rule" />
        <div className="field-row">
          <span className="field-label">预设</span>
          <div className="row">
            <select value={presetId} onChange={(e) => setPresetId(e.target.value)} aria-label="锚点预设">
              {presets.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
            <input value={anchorName} onChange={(e) => setAnchorName(e.target.value)} placeholder="名称（可选）" aria-label="锚点名称" />
            <button className="pixel-btn" disabled={busy === "preset"} onClick={() => void handleAddPreset()}>
              添加锚点
            </button>
          </div>
        </div>
        {view.anchors.length === 0 ? (
          <div className="empty-state">尚未定义锚点 —— 需先设置逻辑画布</div>
        ) : (
          <ul className="identity__anchors">
            {view.anchors.map((a: AnchorView) => (
              <li key={a.id} className="mono identity__anchor">
                {a.name} <span className="faint">({a.preset})</span> · ({a.x}, {a.y})
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="pixel-panel">
        <h3 className="mono panel-heading">身份版本 / VERSIONS</h3>
        <hr className="pixel-rule" />
        {view.versions.length === 0 ? (
          <div className="empty-state">暂无身份版本</div>
        ) : (
          <ul className="identity__versions">
            {view.versions.map((v) => (
              <li key={v.id} className="mono">
                {v.id} · {v.reason} · {v.createdAt} {v.immutable ? "· 不可变" : "· 当前"}
              </li>
            ))}
          </ul>
        )}
      </section>

      {error && <div className="error-text">{error}</div>}
      {okMsg && <div className="status-ok">{okMsg}</div>}
    </div>
  );
}
