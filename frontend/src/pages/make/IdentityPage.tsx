// 制作 > 身份 sub-page: identity definition (text / reference image / sprite
// entries), logical canvas, anchor presets, materials, version history — all
// over the shared core identity services (tasks 2.3–2.5).
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { AnchorPresetView, AnchorView, BaseCharacterCandidateView, CurrentModelsView, GenerationPlanView, IdentityView, MaterialView, StylePresetView } from "../../api/client";
import { addAnchorPreset, adoptBaseCharacter, confirmGeneration, deleteAnchor, enhanceDescription, fetchAnchorPresets, fetchBaseCharacterCandidates, fetchCurrentModels, fetchDraft, fetchIdentity, fetchMaterialThumbs, fetchPresetCatalog, importBaseCharacter, importMaterial, lockBaseCharacterSource, pickMaterialFile, prepareGeneration, removeMaterial, saveCanvas, saveDescription, saveDraftPatch, setMainReference, setPerfectPixelStandard } from "../../api/client";
import { useSession } from "../../state/SessionContext";
import { PixelCanvas } from "../../components/PixelCanvas";
import { MaterialLightbox } from "../../components/MaterialLightbox";
import { ConfirmModal } from "../../components/ConfirmModal";
import "./IdentityPage.css";

const ROLE_LABEL: Record<string, string> = {
  main_reference: "主参考图",
  auxiliary_reference: "辅助参考图",
  sprite: "既有角色图",
};

const TASK_STATUS_LABEL: Record<string, string> = {
  config: "未开始",
  reviewing: "待确认",
  generating: "生成中",
  done: "已完成",
  error: "失败",
};

// 常用像素画布预设（正方形）；256×256 是 PerfectPixel 标准，其他尺寸走"自定义"输入。
const CANVAS_PRESETS = [16, 24, 32, 48, 64, 96, 128, 256, 512];

// 严格正整数解析：只接受完整的十进制正整数字符串（拒绝 "12.5"、"12abc"、""）。
const parsePositiveInt = (value: string): number | null => {
  const trimmed = value.trim();
  if (!/^\d+$/.test(trimmed)) return null;
  const n = Number(trimmed);
  return Number.isSafeInteger(n) && n > 0 ? n : null;
};

export function IdentityPage() {
  const { pkg } = useSession();
  const [view, setView] = useState<IdentityView | null>(null);
  const [presets, setPresets] = useState<AnchorPresetView[]>([]);
  const [description, setDescription] = useState("");
  const [canvasW, setCanvasW] = useState("256");
  const [canvasH, setCanvasH] = useState("256");
  // 画布尺寸选择："WxH" = 预设；"custom" = 自定义输入。
  const [sizeChoice, setSizeChoice] = useState("256x256");
  const [perfectPixelStandard, setPerfectPixelStandardState] = useState(false);
  const [presetId, setPresetId] = useState("feet");
  const [anchorName, setAnchorName] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [okMsg, setOkMsg] = useState<string | null>(null);
  const [candidates, setCandidates] = useState<BaseCharacterCandidateView[]>([]);
  const [styles, setStyles] = useState<StylePresetView[]>([]);
  const [tasks, setTasks] = useState<Array<{ id: number; description: string; style: string; styleCustom?: string; customPrompt?: boolean; status: "config" | "reviewing" | "generating" | "done" | "error"; error?: string; plan?: GenerationPlanView }>>([]);
  const [candidateLoading, setCandidateLoading] = useState(true);
  const [candidateError, setCandidateError] = useState<string | null>(null);
  const [stylesError, setStylesError] = useState<string | null>(null);
  const [sourceConfirm, setSourceConfirm] = useState<"ai" | "import" | null>(null);
  const [anchorToDelete, setAnchorToDelete] = useState<AnchorView | null>(null);
  const [materialToDelete, setMaterialToDelete] = useState<MaterialView | null>(null);
  const [thumbs, setThumbs] = useState<Record<string, string>>({});
  const [previewMaterial, setPreviewMaterial] = useState<MaterialView | null>(null);
  const [enhanceError, setEnhanceError] = useState<string | null>(null);
  const [models, setModels] = useState<CurrentModelsView | null>(null);
  const [nextTaskId, setNextTaskId] = useState(1);
  // 未保存草稿（.draft sidecar）：切换视图/任务运行/应用重启后恢复。
  const [draftLoaded, setDraftLoaded] = useState(false);
  const taskActionInFlight = useRef(new Set<number>());

  const loadCandidates = useCallback(async () => {
    setCandidateLoading(true);
    setCandidateError(null);
    try { setCandidates(await fetchBaseCharacterCandidates()); }
    catch (e) { setCandidateError(String(e)); }
    finally { setCandidateLoading(false); }
  }, []);

  const loadThumbs = useCallback(async () => {
    try {
      const list = await fetchMaterialThumbs();
      const map: Record<string, string> = {};
      for (const t of list) map[t.materialId] = t.png;
      setThumbs(map);
    } catch { setThumbs({}); }
  }, []);

  const load = useCallback(async () => {
    try {
      const [v, ps] = await Promise.all([fetchIdentity(), fetchAnchorPresets()]);
      setView(v);
      setPresets(ps);
      // 草稿优先：有未保存描述则恢复它，否则展示已保存描述。
      const draft = await fetchDraft().catch(() => null);
      setDescription(draft?.description ? draft.description : v.description);
      setDraftLoaded(true);
      if (v.canvas) {
        setCanvasW(String(v.canvas.unitWidth));
        setCanvasH(String(v.canvas.unitHeight));
        const preset = v.canvas.unitWidth === v.canvas.unitHeight && CANVAS_PRESETS.includes(v.canvas.unitWidth);
        setSizeChoice(preset ? `${v.canvas.unitWidth}x${v.canvas.unitHeight}` : "custom");
      }
      setPerfectPixelStandardState(v.perfectPixelStandard);
    } catch (e) {
      setError(String(e));
    }
  }, []);

  useEffect(() => {
    void load();
    void loadCandidates();
    void loadThumbs();
    fetchCurrentModels().then(setModels).catch(() => setModels(null));
    void fetchPresetCatalog().then((catalog) => {
      setStyles(catalog.styles);
      setStylesError(null);
    }).catch((e) => setStylesError(String(e)));
  }, [load, loadCandidates, loadThumbs, pkg]);

  useEffect(() => {
    if (view && tasks.length === 0) {
      const style = styles[0]?.id || "pixel";
      setTasks([{ id: nextTaskId, description: "", style, status: "config" }]);
      setNextTaskId((id) => id + 1);
    }
  }, [view, styles, nextTaskId, tasks.length]);

  const flash = (msg: string) => {
    setOkMsg(msg);
    window.setTimeout(() => setOkMsg(null), 2500);
  };

  const savedDescription = view?.description ?? "";
  const descriptionDirty = draftLoaded && description !== savedDescription;
  // 画布输入与已保存规格不一致（或尚无画布）时，才显示保存按钮。
  const canvasWNum = parsePositiveInt(canvasW);
  const canvasHNum = parsePositiveInt(canvasH);
  const canvasDirty = view?.canvas
    ? canvasWNum === null || canvasHNum === null || canvasWNum !== view.canvas.unitWidth || canvasHNum !== view.canvas.unitHeight
    : canvasWNum === null || canvasHNum === null;
  const isAI = view?.baseCharacterSource === "ai";
  // 参考图容量：1 主参考图 + 最多 2 辅助参考图（界面前置拦截，后端仍兜底校验）。
  const mainRefCount = view?.materials.filter((m) => m.role === "main_reference").length ?? 0;
  const auxRefCount = view?.materials.filter((m) => m.role === "auxiliary_reference").length ?? 0;
  const refsFull = mainRefCount >= 1 && auxRefCount >= 2;

  // 描述变更防抖写入草稿 sidecar（仅在有实际改动时）。
  useEffect(() => {
    if (!draftLoaded || description === savedDescription) return;
    const t = window.setTimeout(() => {
      void saveDraftPatch({ description }).catch(() => undefined);
    }, 600);
    return () => window.clearTimeout(t);
  }, [description, draftLoaded, savedDescription]);

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
      await saveDraftPatch({ description: "" }); // 保存成功后清除描述草稿
      flash("身份描述已保存");
    });

  const handleSaveCanvas = () => {
    const w = parsePositiveInt(canvasW);
    const h = parsePositiveInt(canvasH);
    if (w === null || h === null) {
      setError("逻辑画布尺寸必须为正整数");
      return;
    }
    return run("canvas", async () => {
      await saveCanvas(w, h);
      flash("逻辑画布已保存");
    });
  };

  const handlePerfectPixelToggle = () => {
    const next = !perfectPixelStandard;
    void run("perfectpixel", async () => {
      await setPerfectPixelStandard(next);
      setPerfectPixelStandardState(next);
      flash(next ? "已启用 PerfectPixel 标准" : "已关闭 PerfectPixel 标准");
    });
  };

  const handleSizeChoice = (value: string) => {
    if (value === "custom") {
      setSizeChoice("custom");
      return;
    }
    const [w, h] = value.split("x").map((n) => parseInt(n, 10));
    setCanvasW(String(w));
    setCanvasH(String(h));
    setSizeChoice(value);
  };

  const handleEnhanceDescription = async () => {
    setEnhanceError(null);
    setBusy("enhance");
    try {
      const enhanced = await enhanceDescription(description);
      setDescription(enhanced);
      flash("描述已由 AI 增强 —— 请检查内容并按需修改，再点“保存描述”");
    } catch (e) {
      setEnhanceError(String(e));
    } finally {
      setBusy(null);
    }
  };

  const handleAddPreset = () =>
    run("preset", async () => {
      await addAnchorPreset(presetId, anchorName.trim());
      flash("固定锚点已添加");
      setAnchorName("");
    });

  const handleDeleteMaterial = () => {
    if (!materialToDelete) return;
    const material = materialToDelete;
    setMaterialToDelete(null);
    void run("material-delete", async () => {
      await removeMaterial(material.id);
      await load();
      await loadThumbs();
      flash(`已删除素材：${material.name}`);
    });
  };

  const handleDeleteAnchor = () => {
    if (!anchorToDelete) return;
    const anchor = anchorToDelete;
    setAnchorToDelete(null);
    void run("anchor-delete", async () => {
      await deleteAnchor(anchor.id);
      flash(`已删除自定义锚点：${anchor.name}`);
    });
  };

  const updateTask = (id: number, patch: Partial<(typeof tasks)[number]>) =>
    setTasks((items) => items.map((task) => task.id === id ? { ...task, ...patch } : task));

  // 生成确认门（generation spec）：prepare 只计算计划不外呼；用户在
  // reviewing 状态看到 provider/model/预算/提示词后，确认才执行，取消零调用。
  const prepareTask = async (task: (typeof tasks)[number]) => {
    if (taskActionInFlight.current.has(task.id)) return;
    // 后端生成读取的是"已保存"的描述与画布，未保存内容不会进入请求，
    // 因此存在未保存修改时禁止生成确认，避免用户误以为草稿已生效。
    // 勾选了任务级自定义提示词的任务不受身份描述未保存的影响。
    const blockers: string[] = [];
    if (descriptionDirty && !task.customPrompt) blockers.push("角色描述");
    if (canvasDirty) blockers.push("单元尺寸");
    if (blockers.length > 0) {
      updateTask(task.id, { error: `${blockers.join("与")}尚未保存 —— 请先保存，再生成确认` });
      return;
    }
    updateTask(task.id, { status: "reviewing", error: undefined });
    taskActionInFlight.current.add(task.id);
    try {
      const plan = await prepareGeneration({
        packagePath: "",
        baseCharacter: true,
        motionId: "",
        providerId: "",
        model: "",
        directions: 0,
        stylePresetId: task.style === "custom" ? "" : task.style,
        styleCustom: task.style === "custom" ? (task.styleCustom ?? "").trim() : "",
        description: task.customPrompt ? task.description : "",
        actionPresetId: "",
        frameCount: 0,
        maxAttemptsPerDirection: 0,
      });
      updateTask(task.id, { plan, status: "reviewing" });
    } catch (e) {
      updateTask(task.id, { status: "error", error: String(e) });
    } finally {
      taskActionInFlight.current.delete(task.id);
    }
  };

  const confirmTask = async (task: (typeof tasks)[number]) => {
    if (!task.plan || taskActionInFlight.current.has(task.id)) return;
    taskActionInFlight.current.add(task.id);
    updateTask(task.id, { status: "generating", error: undefined });
    try {
      const result = await confirmGeneration(task.plan.id, true);
      if (result.status !== "executed" || !result.results?.[0]?.candidateId) {
        throw new Error(result.error || `生成${result.status || "失败"}`);
      }
      updateTask(task.id, { status: "done" });
      await loadCandidates();
    } catch (e) {
      updateTask(task.id, { status: "error", error: String(e) });
    } finally {
      taskActionInFlight.current.delete(task.id);
    }
  };

  const cancelTask = async (task: (typeof tasks)[number]) => {
    if (task.plan) await confirmGeneration(task.plan.id, false).catch(() => undefined);
    updateTask(task.id, { status: "config", plan: undefined });
  };

  const removeTask = (task: (typeof tasks)[number]) => {
    if (task.plan && task.status === "reviewing") void confirmGeneration(task.plan.id, false).catch(() => undefined);
    setTasks((items) => items.filter((t) => t.id !== task.id));
  };

  const handleAdopt = async (id: string) => {
    setBusy(`adopt-${id}`);
    try { await adoptBaseCharacter(id); await load(); await loadCandidates(); }
    catch (e) { setError(String(e)); }
    finally { setBusy(null); }
  };

  const handleLockAI = () => setSourceConfirm("ai");

  const confirmSource = () => {
    if (!sourceConfirm) return;
    const source = sourceConfirm;
    setSourceConfirm(null);
    void run("source", async () => {
      await lockBaseCharacterSource(source);
      setView(await fetchIdentity());
      flash(source === "ai" ? "基础角色来源已锁定为 AI 生成" : "基础角色来源已锁定为本地导入");
      if (source === "import") {
        const path = await pickMaterialFile("选择角色图（尺寸需与逻辑画布一致）");
        if (!path) return;
        await importBaseCharacter(path);
        await loadCandidates();
        flash("角色图已登记为候选 —— 确认无误后点“采用”");
      }
    });
  };

  const handleImportBase = () =>
    run("import-base", async () => {
      if (!view?.baseCharacterSource) {
        setSourceConfirm("import");
        return;
      }
      const path = await pickMaterialFile("选择角色图（尺寸需与逻辑画布一致）");
      if (!path) return; // cancelled
      await importBaseCharacter(path);
      await loadCandidates();
      flash("角色图已登记为候选 —— 确认无误后点“采用”");
    });

  const handleImport = () =>
    run("import-reference_image", async () => {
      // 角色自动分配：无主参考图时成为主参考图，否则成为辅助参考图。
      // 满员（1 主 + 2 辅）时按钮已禁用；后端仍会做最终校验。
      const path = await pickMaterialFile("选择参考图");
      if (!path) return; // cancelled
      const role = mainRefCount === 0 ? "main_reference" : "auxiliary_reference";
      await importMaterial("reference_image", path, "", role);
      await load();
      await loadThumbs();
      flash(role === "main_reference" ? "参考图已导入并设为主参考图" : "参考图已导入为辅助参考图");
    });

  const handleSetMain = (materialId: string) =>
    run("material-main", async () => {
      await setMainReference(materialId);
      await load();
      await loadThumbs();
      flash("已更新主参考图 —— 原主参考图自动改为辅助");
    });

  // 当前采用的基准候选（hooks 必须在早退前调用）。
  const adoptedCandidate = useMemo(
    () => (view ? candidates.find((c) => c.id === view.baseCharacterId) ?? null : null),
    [candidates, view],
  );

  if (!view) {
    // 完整占位面板：加载/错误态不塌缩成一行，页面高度与其他标签一致。
    return (
      <div className="empty-state">
        {error ? <span className="error-text">{error}</span> : "加载身份定义…"}
      </div>
    );
  }

  return (
    <div className="identity">
      {/* 反馈前置：保存/生成结果在页面顶部可见，不沉底 */}
      {(error || okMsg) && (
        <div className={`identity__feedback ${error ? "identity__feedback--error" : "identity__feedback--ok"}`} role="status">
          {error ?? okMsg}
        </div>
      )}

      <section className="pixel-panel identity__source-panel">
        <h3 className="mono panel-heading">基础角色来源 / SOURCE</h3>
        <hr className="pixel-rule" />
        {view.baseCharacterSource ? (
          <div className="identity__source-locked">
            <div className="row">
              <strong>{view.baseCharacterSource === "ai" ? "AI 生成" : "导入已有角色图"}</strong>
              <span className="status-ok">已锁定</span>
            </div>
            <div className="faint">身份包的基础角色来源不可切换。{view.baseCharacterSource === "ai" ? "当前可使用提示词、风格和参考图生成候选。" : "当前只接受本地角色图候选，身份描述仅作为角色档案。"}</div>
          </div>
        ) : (
          <div className="identity__source-choice">
            <div className="faint">请选择基础角色来源。确定后不可切换，请确认身份包的制作方式。</div>
            <div className="identity__source-options">
              <button className="pixel-btn" disabled={busy !== null} onClick={() => void handleLockAI()}>
                AI 生成
                <span className="faint">提示词 + 风格 + 参考图</span>
              </button>
              <button className="pixel-btn" disabled={busy !== null} onClick={() => void handleImportBase()}>
                导入已有角色图
                <span className="faint">本地角色图 + 尺寸校验</span>
              </button>
            </div>
          </div>
        )}
        <div className="identity__meta identity__meta--tight">
          <span className="mono">ID {view.id}</span>
          <span className="mono">当前版本 {view.currentVersion}</span>
        </div>
        {view.versions.length > 0 && (
          <details className="identity__versions-box">
            <summary className="mono">身份版本（{view.versions.length}）</summary>
            <ul className="identity__versions">
              {view.versions.map((v) => (
                <li key={v.id} className="mono">
                  {v.id} · {v.reason} · {v.createdAt} {v.immutable ? "· 历史" : "· 当前"}
                </li>
              ))}
            </ul>
          </details>
        )}
      </section>

      {view.baseCharacterSource && (
        <div className="identity__grid">
        {/* 合并工作流面板：AI 来源 = 描述→参考图→画布→任务→候选；本地导入 = 描述→画布→导入→候选 */}
        <div className="identity__rail">
          <section className={`pixel-panel ${view.baseCharacterSource === "ai" ? "identity__ai-workflow" : "identity__import-workflow"}`}>
            <h3 className="mono panel-heading">
              {view.baseCharacterSource === "ai" ? "创建角色 · AI 生成 / CREATE" : "创建角色 · 本地导入 / CREATE"}
              {descriptionDirty && (
                <span className="status-badge identity__draft-badge" title="描述尚未保存 —— 已自动暂存，切换视图或重启后仍会恢复">
                  未保存草稿
                </span>
              )}
            </h3>
            <hr className="pixel-rule" />
            <div className="identity__steps">
              <section className="identity__step-card">
                <span className="identity__step-marker mono">1</span>
                <div className="identity__step-head">
                  <span className="identity__step-title mono">角色描述</span>
                  <span className="identity__step-hint faint">{isAI ? "保存为身份档案，并作为生成提示词的基础" : "仅作为角色档案，不参与本体生成"}</span>
                </div>
                <textarea value={description} onChange={(e) => setDescription(e.target.value)} placeholder="描述角色外观、配色、体型…" />
                {descriptionDirty && (
                  <div className="identity__actions">
                    <button className="pixel-btn pixel-btn--ok" disabled={busy === "desc"} onClick={() => void handleSaveDescription()} title="有未保存的描述修改">
                      {busy === "desc" ? "保存中…" : "保存描述"}
                    </button>
                  </div>
                )}
                <div className="identity__actions identity__actions--left">
                  <button
                    className="pixel-btn"
                    disabled={busy !== null || !description.trim() || models?.enhanceSupported === false}
                    onClick={() => void handleEnhanceDescription()}
                    title={models?.enhanceSupported === false ? "当前未配置可用的文本模型 —— 请在设置中配置增强模型" : "调用文本模型把简短描述扩写成结构化角色设定 —— 将产生一次文本调用费用；结果填入上方文本框，请检查后保存"}
                  >
                    {busy === "enhance" ? "增强中…（最长约 90 秒）" : "AI 增强描述"}
                  </button>
                  <span className="faint">
                    {models?.enhanceSupported
                      ? `文本模型：${models.enhanceProviderId || models.providerName || models.providerId} / ${models.enhanceModel} · 慢模型可能需要 30–90 秒`
                      : "当前未配置可用的文本模型 —— 请在设置中配置增强模型"}
                  </span>
                </div>
                {enhanceError && <div className="error-text">{enhanceError}</div>}
              </section>

              {view.baseCharacterSource === "ai" && (
                <section className="identity__step-card">
                  <span className="identity__step-marker mono">2</span>
                  <div className="identity__step-head">
                    <span className="identity__step-title mono">参考图</span>
                    <span className="identity__step-hint faint">主参考图最多 1 张 · 辅助参考图最多 2 张 —— 基础角色与每次动作生成都随请求外发，数量越多花费越高</span>
                  </div>
                  <div className="row">
                    <button className="pixel-btn" disabled={busy === "import-reference_image" || refsFull} onClick={() => void handleImport()}>
                      添加参考图
                    </button>
                    <span className="faint">{refsFull ? "已达上限：主参考图 1 张 + 辅助参考图 2 张 —— 可删除素材，或点辅助卡的“设为主参考”调整" : `还可添加 ${3 - mainRefCount - auxRefCount} 张（主参考图 1 张 · 辅助参考图 2 张）`}</span>
                  </div>
                  {view.materials.length > 0 && (
                    <div className="identity__material-grid">
                      {view.materials.map((m: MaterialView) => (
                        <div className="pixel-panel identity__material" key={m.id}>
                          {thumbs[m.id]
                            ? <img src={`data:image/png;base64,${thumbs[m.id]}`} alt={m.name} className="identity__material-thumb" title="点击放大预览" onClick={() => setPreviewMaterial(m)} />
                            : <button type="button" className="identity__material-placeholder faint" title="点击放大预览" onClick={() => setPreviewMaterial(m)}>无预览<br />点击查看原图</button>}
                          <div className="mono">{ROLE_LABEL[m.role] ?? m.kind}</div>
                          <div className="faint identity__material-name" title={m.path}>{m.name}</div>
                          <div className="identity__material-actions">
                            {m.role === "auxiliary_reference" && (
                              <button className="pixel-btn" disabled={busy !== null} onClick={() => void handleSetMain(m.id)} aria-label={`设为主参考图 ${m.name}`} title="把这张辅助参考图设为主参考图（当前主参考图自动改为辅助）">设为主参考</button>
                            )}
                            <button className="pixel-btn pixel-btn--warn" disabled={busy !== null} onClick={() => setMaterialToDelete(m)} aria-label={`删除素材 ${m.name}`} title="从身份包中删除此素材文件">删除</button>
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </section>
              )}

              <section className="identity__step-card">
                <span className="identity__step-marker mono">{isAI ? "3" : "2"}</span>
                <div className="identity__step-head">
                  <span className="identity__step-title mono">逻辑画布</span>
                  <span className="identity__step-hint faint">每帧图像的像素网格大小 —— 生成、导入与锚点都以此为准</span>
                </div>
                <div className="row">
                  <select value={sizeChoice} onChange={(e) => handleSizeChoice(e.target.value)} aria-label="常用画布尺寸" title="选择常用尺寸，或选自定义输入任意宽高">
                    {CANVAS_PRESETS.map((n) => <option key={n} value={`${n}x${n}`}>{n === 256 ? "256 × 256 · PerfectPixel 标准" : `${n} × ${n}`}</option>)}
                    <option value="custom">自定义{canvasW === canvasH && CANVAS_PRESETS.includes(parseInt(canvasW, 10)) ? "" : `（${canvasW} × ${canvasH}）`}</option>
                  </select>
                  {sizeChoice === "custom" && (
                    <>
                      <input className="identity__num" value={canvasW} onChange={(e) => setCanvasW(e.target.value)} aria-label="画布宽（像素）" />
                      <span>×</span>
                      <input className="identity__num" value={canvasH} onChange={(e) => setCanvasH(e.target.value)} aria-label="画布高（像素）" />
                    </>
                  )}
                  {canvasDirty && (
                    <button className="pixel-btn pixel-btn--ok" disabled={busy === "canvas"} onClick={() => void handleSaveCanvas()} title="有未保存的尺寸修改">
                      {busy === "canvas" ? "保存中…" : "保存规格"}
                    </button>
                  )}
                </div>
                <div className="faint identity__step-note">PerfectPixel 标准：256 × 256，每帧保留 24px 安全边距，并自动按质量中心与脚底基线对齐。其他尺寸可自定义。</div>
                {canvasW === "256" && canvasH === "256" && (
                  <label className="identity__pixelperfect-toggle">
                    <input type="checkbox" checked={perfectPixelStandard} onChange={handlePerfectPixelToggle} disabled={busy !== null} />
                    <span>启用 PerfectPixel 标准</span>
                    <span className="faint">24px 安全边距 · 质量中心对齐 · 自动脚底基线</span>
                  </label>
                )}
              </section>

              {view.baseCharacterSource === "ai" && (
                <section className="identity__step-card">
                  <span className="identity__step-marker mono">4</span>
                  <div className="identity__step-head">
                    <span className="identity__step-title mono">生成任务</span>
                    <span className="identity__step-hint faint">每个任务独立提示词与风格 · 确认门之后才会调用</span>
                  </div>
                  <div className="row">
                    <span className="faint">每个任务独立选择风格与提示词{models?.imageModel ? ` · 图像模型：${models.providerName || models.providerId} / ${models.imageModel}` : ""}</span>
                    <button className="pixel-btn" disabled={tasks.length >= 10 || busy !== null} onClick={() => { const style = tasks.length ? tasks[tasks.length - 1].style : (styles[0]?.id || "pixel"); setTasks((items) => [...items, { id: nextTaskId, description: "", style, status: "config" }]); setNextTaskId((id) => id + 1); }} aria-label="添加生成任务" title="添加生成任务">添加任务</button>
                  </div>
                  {stylesError && <div className="error-text">风格目录加载失败：{stylesError}</div>}
                  <div className="identity__task-list">
                    {tasks.map((task) => (
                      <div className="identity__task-card" key={task.id}>
                        <div className="identity__task-row">
                          <select value={task.style} onChange={(e) => updateTask(task.id, { style: e.target.value })} aria-label={`任务 ${task.id} 风格`} title="任务风格" disabled={task.status !== "config"}>
                            {styles.length === 0 && task.style !== "custom" && <option value={task.style}>{task.style}</option>}
                            {styles.map((style) => <option key={style.id} value={style.id}>{style.name}</option>)}
                            <option value="custom">自定义风格…</option>
                          </select>
                          <span className={`mono ${task.status === "error" ? "error-text" : task.status === "done" ? "status-ok" : task.status === "reviewing" ? "status-warn" : "faint"}`}>{TASK_STATUS_LABEL[task.status] ?? task.status}</span>
                          <span className="identity__task-spacer" />
                          {task.status === "config" && (
                            <button className="pixel-btn pixel-btn--primary" onClick={() => void prepareTask(task)} aria-label={`生成确认任务 ${task.id}`} title="先查看生成确认（不发起调用），再决定是否执行">生成确认</button>
                          )}
                          <button className="pixel-btn" disabled={task.status === "generating" || tasks.length <= 1} onClick={() => removeTask(task)} aria-label={`移除任务 ${task.id}`} title="移除任务">移除</button>
                        </div>
                        {task.style === "custom" && (
                          <input className="pixel-input" value={task.styleCustom ?? ""} onChange={(e) => updateTask(task.id, { styleCustom: e.target.value })} placeholder="自定义风格提示词（英文更稳定），例：dark fantasy pixel art, muted colors" aria-label={`任务 ${task.id} 自定义风格提示词`} title="作为风格片段写入生成提示词，与内置预设同等生效" disabled={task.status !== "config"} />
                        )}
                        <div className="faint identity__step-note">
                          {task.style === "custom"
                            ? "自定义风格：上方输入的文字会作为风格片段写入提示词。"
                            : (() => { const s = styles.find((x) => x.id === task.style); return s ? `${s.name}：${s.description}` : ""; })()}
                        </div>
                                                 {task.style !== "custom" && (() => {
                           const style = styles.find((x) => x.id === task.style);
                           return style?.negativePrompt ? (
                             <div className="faint identity__negative-prompt">
                               <span className="mono">内置负面提示词：</span>{style.negativePrompt}
                             </div>
                           ) : null;
                         })()}
                         <label className="identity__task-custom" title="勾选后可为本任务改写提示词；不勾选则使用身份描述">
                          <input type="checkbox" checked={!!task.customPrompt} disabled={task.status !== "config"} onChange={(e) => updateTask(task.id, { customPrompt: e.target.checked, description: e.target.checked && !task.description ? description : task.description })} />
                          <span className="faint">自定义提示词（默认使用身份描述）</span>
                        </label>
                        {task.customPrompt && (
                          <input className="pixel-input" value={task.description} onChange={(e) => updateTask(task.id, { description: e.target.value })} aria-label={`任务 ${task.id} 生成提示词`} title="本次生成的提示词覆盖 —— 只影响此任务，不改变身份描述" placeholder="生成提示词（默认取身份描述）" disabled={task.status !== "config"} />
                        )}
                        {task.error && <div className="error-text">{task.error}</div>}
                        {task.status === "reviewing" && task.plan && (
                          <details className="identity__task-plan" open>
                            <summary className="mono">生成确认详情</summary>
                            <div className="pixel-panel gen-plan">
                              <ul className="mono gen-plan__list">
                                <li>
                                  provider / model：{task.plan.providerId} / {task.plan.model}
                                  {task.plan.providerType ? `（协议：${task.plan.providerType}）` : ""}
                                </li>
                                <li>预计调用量：{task.plan.expectedCalls} 次 · 总尝试上限 {task.plan.maxTotalAttempts} 次</li>
                                <li>预算：约 {task.plan.expectedCost.toFixed(2)} {task.plan.currency}（上限 {task.plan.maxCost.toFixed(2)} {task.plan.currency}）</li>
                                <li>外发素材：{(task.plan.outboundMaterials ?? []).length} 个{(task.plan.outboundMaterials ?? []).length > 0 ? "（主参考图 / 辅助参考图随请求发送）" : ""}</li>
                                <li className="gen-plan__prompt">
                                  提示词快照（{task.plan?.prompt.stylePresetId === "custom" ? "自定义风格" : styles.find((s) => s.id === task.plan?.prompt.stylePresetId)?.name ?? task.plan?.prompt.stylePresetId}，{task.plan?.prompt.frameCount} 帧）：
                                  <div className="faint">{task.plan.prompt.prompt}</div>
                                </li>
                              </ul>
                              <div className="row">
                                <button className="pixel-btn pixel-btn--primary" onClick={() => void confirmTask(task)} aria-label="确认执行生成">确认执行</button>
                                <button className="pixel-btn" onClick={() => void cancelTask(task)} aria-label="取消生成">取消（不发起调用）</button>
                              </div>
                            </div>
                          </details>
                        )}
                      </div>
                    ))}
                  </div>
                </section>
              )}

              {view.baseCharacterSource === "import" && (
                <section className="identity__step-card">
                  <span className="identity__step-marker mono">3</span>
                  <div className="identity__step-head">
                    <span className="identity__step-title mono">导入角色图</span>
                    <span className="identity__step-hint faint">与逻辑画布尺寸一致 · 不调用外部 AI</span>
                  </div>
                  <div className="row">
                    <button className="pixel-btn pixel-btn--primary" disabled={busy !== null} onClick={() => void handleImportBase()}>
                      导入角色图
                    </button>
                    <span className="faint">当前画布：{view.canvas ? `${view.canvas.unitWidth} × ${view.canvas.unitHeight}` : "未设置"}</span>
                  </div>
                </section>
              )}

              <section className="identity__step-card">
                <span className="identity__step-marker mono">{isAI ? "5" : "4"}</span>
                <div className="identity__step-head">
                  <span className="identity__step-title mono">候选与采用</span>
                  <span className="identity__step-hint faint">先评审再采用 —— 采用后成为身份基准</span>
                </div>
                {candidateLoading ? (
                  <div className="empty-state">加载基础角色候选…</div>
                ) : candidateError ? (
                  <div className="error-text">
                    {candidateError} <button className="pixel-btn" onClick={() => void loadCandidates()} aria-label="重试加载候选" title="重试加载候选">重试</button>
                  </div>
                ) : candidates.length === 0 ? (
                  <div className="empty-state">{view.baseCharacterSource === "ai" ? "尚未生成候选 —— 完成生成任务并确认执行" : "尚未导入角色图"}</div>
                ) : candidates.length > 0 ? (
                  <div className="identity__candidates">
                    {candidates.map((candidate) => {
                      const adopted = view.baseCharacterId === candidate.id;
                      return (
                        <div className={`pixel-panel identity__candidate${adopted ? " identity__candidate--adopted" : ""}`} key={candidate.id}>
                          <img src={`data:image/png;base64,${candidate.png}`} alt="基础角色候选" />
                          <div className="mono">{adopted ? "已采用" : candidate.status === "pending" ? "待采用" : candidate.status === "rejected" ? "已弃用" : candidate.status}</div>
                          <div className="faint">{candidate.provider === "import" ? "本地导入" : `${candidate.provider} · ${candidate.model}`}</div>
                          <button className="pixel-btn" disabled={busy !== null || adopted || candidate.status === "rejected"} onClick={() => void handleAdopt(candidate.id)} aria-label="采用此候选" title={candidate.status === "rejected" ? "已弃用候选不能重新采用" : "采用此候选"}>采用</button>
                        </div>
                      );
                    })}
                  </div>
                ) : null}
              </section>
            </div>
          </section>

          <section className="pixel-panel">
            <h3 className="mono panel-heading">自定义固定锚点 / FIXED RUNTIME MARKERS</h3>
            <hr className="pixel-rule" />
            <div className="identity__anchor-intro faint">
              可选配置：固定在身份基准画布中的运行时参考点，用于游戏引擎定位、道具挂载和特效位置。预设只提供初始坐标，请根据当前基准角色图调整；不会逐帧追踪身体部位，只会随整帧对齐位移同步移动。PerfectPixel 的脚底对齐会自动处理，不需要在这里添加锚点。
            </div>
            <div className="field-row">
              <span className="field-label">位置预设</span>
              <div className="row">
                <select value={presetId} onChange={(e) => setPresetId(e.target.value)} aria-label="锚点预设" disabled={!adoptedCandidate || busy !== null}>
                  {presets.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                    </option>
                  ))}
                </select>
                <input value={anchorName} onChange={(e) => setAnchorName(e.target.value)} placeholder="名称（可选）" aria-label="锚点名称" disabled={!adoptedCandidate || busy !== null} />
                <button className="pixel-btn" disabled={!adoptedCandidate || busy === "preset"} onClick={() => void handleAddPreset()}>
                  添加锚点
                </button>
              </div>
            </div>
            {view.anchors.length === 0 ? (
              <div className="empty-state">{adoptedCandidate ? "尚未添加自定义锚点" : "请先采用基础角色图，再添加自定义锚点。"}</div>
            ) : (
              <ul className="identity__anchors">
                {view.anchors.map((a: AnchorView) => (
                  <li key={a.id} className="mono identity__anchor">
                    <span>{a.name} <span className="faint">({a.preset})</span> · ({a.x}, {a.y})</span>
                    <button className="pixel-btn pixel-btn--warn identity__anchor-delete" disabled={!adoptedCandidate || busy !== null} onClick={() => setAnchorToDelete(a)} aria-label={`删除锚点 ${a.name}`} title="删除此自定义锚点">删除</button>
                  </li>
                ))}
              </ul>
            )}
            {adoptedCandidate && view.anchors.length > 0 && (
              <div className="identity__anchor-preview">
                <PixelCanvas
                  unitWidth={view.canvas?.unitWidth || 16}
                  unitHeight={view.canvas?.unitHeight || 16}
                  scale={8}
                  frames={[{
                    png: adoptedCandidate.png,
                    durationMs: 100,
                    anchors: view.anchors.map((a: AnchorView) => ({ name: a.name, x: a.x, y: a.y })),
                  }]}
                  playing={false}
                  showGrid
                  showAnchors
                  label="当前基准 + 固定锚点"
                />
              </div>
            )}
          </section>
        </div>
        </div>
      )}

      <MaterialLightbox material={previewMaterial} onClose={() => setPreviewMaterial(null)} />

      <ConfirmModal
        open={materialToDelete !== null}
        title="删除素材"
        message={materialToDelete ? `确定删除素材“${materialToDelete.name}”吗？素材文件将从身份包的素材区中移除。` : ""}
        confirmLabel="删除"
        cancelLabel="取消"
        danger
        onConfirm={handleDeleteMaterial}
        onCancel={() => setMaterialToDelete(null)}
      />

      <ConfirmModal
        open={anchorToDelete !== null}
        title="删除自定义锚点"
        message={anchorToDelete ? `确定删除“${anchorToDelete.name}”吗？删除后，该锚点的运行时定位信息将从身份包中移除。` : ""}
        confirmLabel="删除"
        cancelLabel="取消"
        danger
        onConfirm={handleDeleteAnchor}
        onCancel={() => setAnchorToDelete(null)}
      />

      <ConfirmModal
        open={sourceConfirm !== null}
        title="锁定基础角色来源"
        message={sourceConfirm === "ai" ? "确定使用“AI 生成”作为此身份包的基础角色来源？锁定后将无法切换为导入已有角色图。" : "确定使用“导入已有角色图”作为此身份包的基础角色来源？锁定后将无法切换为 AI 生成。"}
        confirmLabel="确认并锁定"
        cancelLabel="再想想"
        onConfirm={confirmSource}
        onCancel={() => setSourceConfirm(null)}
      />
    </div>
  );
}
