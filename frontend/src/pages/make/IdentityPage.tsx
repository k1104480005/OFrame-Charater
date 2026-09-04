// 制作 > 身份 sub-page: identity definition (text / reference image / sprite
// entries), logical canvas, anchor presets, materials, version history — all
// over the shared core identity services (tasks 2.3–2.5).
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { AnchorPresetView, AnchorView, BaseCharacterCandidateView, CurrentModelsView, GenerationPlanView, IdentityView, MaterialView, ProviderInfoView, StylePresetView } from "../../api/client";
import { addAnchorPreset, adoptBaseCharacter, confirmGeneration, deleteAnchor, deleteBaseCharacter, describeBaseCharacterImage, enhanceDescription, fetchAnchorPresets, fetchBaseCharacterCandidates, fetchCurrentModels, fetchDraft, fetchIdentity, fetchMaterialThumbs, fetchPresetCatalog, fetchProviders, fetchTask, fetchTasks, flipBaseCharacter, importBaseCharacter, importBaseCharacterCropped, importMaterial, lockBaseCharacterSource, pickMaterialFile, prepareGeneration, readImageForPreview, removeMaterial, saveCanvas, saveDescription, saveDraftPatch, setMainReference, setPerfectPixelStandard } from "../../api/client";
import { useSession } from "../../state/SessionContext";
import { PixelCanvas } from "../../components/PixelCanvas";
import { MaterialLightbox } from "../../components/MaterialLightbox";
import { ImageLightbox } from "../../components/ImageLightbox";
import type { ImageLightboxSource } from "../../components/ImageLightbox";
import { ConfirmModal } from "../../components/ConfirmModal";
import { ImageCropModal } from "../../components/ImageCropModal";
import type { CropRect } from "../../components/ImageCropModal";
import "./IdentityPage.css";

const ROLE_LABEL: Record<string, string> = {
  main_reference: "主参考图",
  auxiliary_reference: "辅助参考图",
  sprite: "既有角色图",
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

// 生成表单状态（身份页只有一张生成表单，没有任务卡列表）：
// 结果长效归属候选区；进行中状态由右上角任务抽屉承载。
type GenStatus = "idle" | "reviewing" | "generating" | "done" | "error";

const GEN_STATUS_LABEL: Record<GenStatus, string> = {
  idle: "未开始",
  reviewing: "待确认",
  generating: "生成中",
  done: "已完成",
  error: "失败",
};

const GEN_FORM_STORE_PREFIX = "identity.genForm.";

export function IdentityPage({ onOpenTasks }: { onOpenTasks?: () => void }) {
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
  // 生成表单（单实例）：
  const [genStyle, setGenStyle] = useState("pixel");
  const [genStyleCustom, setGenStyleCustom] = useState("");
  const [genCustomPrompt, setGenCustomPrompt] = useState(false);
  const [genDescription, setGenDescription] = useState("");
  const [genProvider, setGenProvider] = useState("");
  const [genModel, setGenModel] = useState("");
  const [genStatus, setGenStatus] = useState<GenStatus>("idle");
  const [genError, setGenError] = useState<string | null>(null);
  const [genPlan, setGenPlan] = useState<GenerationPlanView | null>(null);
  const [genPlanId, setGenPlanId] = useState<string | null>(null);
  const [genStartedAt, setGenStartedAt] = useState<number | null>(null);
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
  const [providers, setProviders] = useState<ProviderInfoView[]>([]);
  const [previewCandidate, setPreviewCandidate] = useState<ImageLightboxSource | null>(null);
  // 识图（AI 生成描述）的内联错误：显示在按钮旁，避免只沉在页面顶部反馈条。
  const [describeError, setDescribeError] = useState<string | null>(null);
  const [candidateToAdopt, setCandidateToAdopt] = useState<BaseCharacterCandidateView | null>(null);
  const [candidateToDelete, setCandidateToDelete] = useState<BaseCharacterCandidateView | null>(null);
  // 裁剪会话：导入图片与画布不符时弹出（比例锁定画布 + 辅助线框选）。
  const [cropSession, setCropSession] = useState<{ path: string; src: string } | null>(null);
  // 未保存草稿（.draft sidecar）：切换视图/任务运行/应用重启后恢复。
  const [draftLoaded, setDraftLoaded] = useState(false);
  const genInFlight = useRef(false);

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
    fetchProviders().then(setProviders).catch(() => setProviders([]));
    void fetchPresetCatalog().then((catalog) => {
      setStyles(catalog.styles);
      setStylesError(null);
    }).catch((e) => setStylesError(String(e)));
  }, [load, loadCandidates, loadThumbs, pkg]);

  // 生成表单初始化（每个身份包一次）：
  //   1. 恢复上次表单配置（风格/模型/自定义提示词）——切标签/重启不丢；
  //   2. 探测队列中本会话之前未完成的生成任务 → 恢复“生成中”并轮询终态。
  const genFormRestored = useRef(false);
  const genFormKey = pkg ? `${GEN_FORM_STORE_PREFIX}${pkg.path}` : null;

  // 身份包切换时，先清理上一个包的本地表单和任务状态，避免短暂沿用
  // 上一个包的配置或继续轮询其任务。
  useEffect(() => {
    genFormRestored.current = false;
    setGenStyle("pixel");
    setGenStyleCustom("");
    setGenCustomPrompt(false);
    setGenDescription("");
    setGenProvider("");
    setGenModel("");
    setGenStatus("idle");
    setGenError(null);
    setGenPlan(null);
    setGenPlanId(null);
    setGenStartedAt(null);
  }, [genFormKey]);

  useEffect(() => {
    if (!view || !genFormKey || genFormRestored.current) return;
    genFormRestored.current = true;
    try {
      const raw = localStorage.getItem(genFormKey);
      if (raw) {
        const saved = JSON.parse(raw) as { style?: string; styleCustom?: string; customPrompt?: boolean; description?: string; provider?: string; model?: string };
        if (saved.style) setGenStyle(saved.style);
        if (typeof saved.styleCustom === "string") setGenStyleCustom(saved.styleCustom);
        if (typeof saved.customPrompt === "boolean") setGenCustomPrompt(saved.customPrompt);
        if (typeof saved.description === "string") setGenDescription(saved.description);
        if (typeof saved.provider === "string") setGenProvider(saved.provider);
        if (typeof saved.model === "string") setGenModel(saved.model);
      }
    } catch {
      /* 配置存档损坏时使用默认值 */
    }
    // 进行中的生成（上次会话遗留）：恢复“生成中”并轮询终态（任务 id == 计划 id）。
    // 只恢复归属当前身份包的任务：packagePath 不匹配（含旧版本任务的空归属）
    // 一律忽略，避免把其他包的生成中任务误显示成本包的进度。
    void fetchTasks()
      .then((rows) => {
        const live = rows.find((t) => t.kind === "base-character" && t.packagePath === pkg?.path && (t.status === "running" || t.status === "queued"));
        if (!live) return;
        setGenStatus("generating");
        setGenPlanId(live.id);
        setGenStartedAt(new Date(live.createdAt).getTime() || Date.now());
      })
      .catch(() => undefined);
  }, [view, genFormKey]);

  // 表单配置存档：变化即写 localStorage（不含生成状态——那归任务队列管）。
  useEffect(() => {
    if (!genFormKey || !genFormRestored.current) return;
    try {
      localStorage.setItem(genFormKey, JSON.stringify({ style: genStyle, styleCustom: genStyleCustom, customPrompt: genCustomPrompt, description: genDescription, provider: genProvider, model: genModel }));
    } catch {
      /* 存储失败不阻塞生成 */
    }
  }, [genStyle, genStyleCustom, genCustomPrompt, genDescription, genProvider, genModel, genFormKey]);

  // 生成中轮询任务队列：切标签/重启后也能感知终态（任务 id == 计划 id）。
  useEffect(() => {
    if (genStatus !== "generating" || !genPlanId) return;
    const iv = window.setInterval(() => {
      void fetchTask(genPlanId)
        .then((row) => {
          if (row.status === "succeeded") {
            setGenStatus("done");
            void loadCandidates();
          } else if (row.status === "failed") {
            setGenStatus("error");
            setGenError(row.error || "生成失败");
          } else if (row.status === "abandoned") {
            setGenStatus("error");
            setGenError("任务已放弃");
          }
        })
        .catch(() => undefined);
    }, 2500);
    return () => window.clearInterval(iv);
  }, [genStatus, genPlanId, loadCandidates]);

  // 生成中每秒跳动一次，驱动“已用 N 秒”计时显示。
  const [, setElapsedTick] = useState(0);
  useEffect(() => {
    if (genStatus !== "generating") return;
    const iv = window.setInterval(() => setElapsedTick((x) => x + 1), 1000);
    return () => window.clearInterval(iv);
  }, [genStatus]);

  const flash = (msg: string) => {
    setOkMsg(msg);
    window.setTimeout(() => setOkMsg(null), 2500);
  };

  const savedDescription = view?.description ?? "";
  const descriptionDirty = draftLoaded && description !== savedDescription;
  // 画布输入与已保存规格不一致（或尚无画布 / 输入非法）时，才显示保存按钮。
  const canvasWNum = parsePositiveInt(canvasW);
  const canvasHNum = parsePositiveInt(canvasH);
  const canvasDirty = view?.canvas
    ? canvasWNum === null || canvasHNum === null || canvasWNum !== view.canvas.unitWidth || canvasHNum !== view.canvas.unitHeight
    : true;
  const isAI = view?.baseCharacterSource === "ai";
  // 采用即定稿：身份基准一旦采用，描述/参考图/画布/生成（导入）阶段全部锁定。
  const basisAdopted = Boolean(view?.baseCharacterId);
  // 本地导入画布优先：画布规格已保存且无未保存修改时才允许导入（后端仍兜底校验尺寸）。
  const importCanvasReady = Boolean(view?.canvas) && !canvasDirty;
  // 参考图容量：1 主参考图 + 最多 2 辅助参考图（界面前置拦截，后端仍兜底校验）。
  const mainRefCount = view?.materials.filter((m) => m.role === "main_reference").length ?? 0;
  const auxRefCount = view?.materials.filter((m) => m.role === "auxiliary_reference").length ?? 0;
  const refsFull = mainRefCount >= 1 && auxRefCount >= 2;
  const imageProviders = providers.filter((p) => p.image && (p.imageModels.length > 0 || p.imageModel));
  const selectedProvider = imageProviders.find((p) => p.id === genProvider)
    ?? imageProviders.find((p) => p.id === models?.providerId)
    ?? imageProviders[0];
  const selectedProviderId = selectedProvider?.id ?? "";
  const selectedImageModels = selectedProvider
    ? Array.from(new Set([...(selectedProvider.imageModels ?? []), selectedProvider.imageModel].filter(Boolean)))
    : (models?.imageModels ?? []);

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

  // 生成确认门（generation spec）：prepare 只计算计划不外呼；用户在确认弹窗
  // 看到 provider/model/预算/提示词后，确认才执行，取消零调用。
  const genBusy = genStatus === "generating";

  const resetGenResult = () => {
    setGenPlan(null);
    if (genStatus === "done") setGenStatus("idle");
  };

  const startGeneration = async () => {
    if (genInFlight.current || genBusy) return;
    // 后端生成读取的是"已保存"的描述与画布，未保存内容不会进入请求，
    // 因此存在未保存修改时禁止生成确认，避免用户误以为草稿已生效。
    // 勾选了任务级自定义提示词时不受身份描述未保存的影响。
    const blockers: string[] = [];
    if (descriptionDirty && !genCustomPrompt) blockers.push("角色描述");
    if (canvasDirty) blockers.push("单元尺寸");
    if (blockers.length > 0) {
      setGenStatus("error");
      setGenError(`${blockers.join("与")}尚未保存 —— 请先保存，再生成`);
      return;
    }
    genInFlight.current = true;
    setGenStatus("reviewing");
    setGenError(null);
    try {
      const plan = await prepareGeneration({
        packagePath: "",
        baseCharacter: true,
        motionId: "",
        providerId: genProvider,
        model: genModel,
        directions: 0,
        stylePresetId: genStyle === "custom" ? "" : genStyle,
        styleCustom: genStyle === "custom" ? genStyleCustom.trim() : "",
        description: genCustomPrompt ? genDescription : "",
        actionPresetId: "",
        frameCount: 0,
        maxAttemptsPerDirection: 0,
      });
      setGenPlan(plan);
    } catch (e) {
      setGenStatus("error");
      setGenError(String(e));
    } finally {
      genInFlight.current = false;
    }
  };

  const confirmGenerationForm = async () => {
    if (!genPlan || genInFlight.current) return;
    genInFlight.current = true;
    setGenStatus("generating");
    setGenError(null);
    setGenPlanId(genPlan.id);
    setGenStartedAt(Date.now());
    try {
      const result = await confirmGeneration(genPlan.id, true);
      if (result.status !== "executed" || !result.results?.[0]?.candidateId) {
        throw new Error(result.error || `生成${result.status || "失败"}`);
      }
      setGenStatus("done");
      await loadCandidates();
    } catch (e) {
      setGenStatus("error");
      setGenError(String(e));
    } finally {
      genInFlight.current = false;
    }
  };

  const cancelGenerationForm = async () => {
    if (genPlan) await confirmGeneration(genPlan.id, false).catch(() => undefined);
    setGenStatus("idle");
    setGenPlan(null);
  };

  const confirmAdoptCandidate = () => {
    if (!candidateToAdopt) return;
    const id = candidateToAdopt.id;
    setCandidateToAdopt(null);
    void (async () => {
      setBusy(`adopt-${id}`);
      setError(null);
      try {
        await adoptBaseCharacter(id);
        await load();
        await loadCandidates();
        flash("候选已采用为身份基准，其余候选已自动弃用");
      } catch (e) {
        setError(String(e));
      } finally {
        setBusy(null);
      }
    })();
  };

  const confirmDeleteCandidate = () => {
    if (!candidateToDelete) return;
    const cand = candidateToDelete;
    setCandidateToDelete(null);
    void run("candidate-delete", async () => {
      await deleteBaseCharacter(cand.id);
      // 同步刷新：候选列表（缩略图/确认锁定/候选网格）+ 身份视图（锚点面板等）。
      await loadCandidates();
      await load();
      flash("已删除：记录与图片文件均已移除，相关区域已同步更新");
    });
  };

  // 水平翻转候选/已采用角色图：AI 生图经常把角色朝向画反，翻转是自逆的
  // 真实像素修正（再点一次翻回），已采用的基准与生成读取的是同一文件。
  const handleFlipCandidate = (candidate: BaseCharacterCandidateView) =>
    void run("candidate-flip", async () => {
      await flipBaseCharacter(candidate.id);
      await loadCandidates();
      flash("已水平翻转 —— 朝向相反时再点一次即可翻回");
    });

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
        // 画布优先：画布规格未设置/未保存时不打开文件选择，先引导去保存画布规格。
        if (!importCanvasReady) {
          flash("来源已锁定为本地导入 —— 请先在“逻辑画布”步骤设置并保存规格，再导入角色图");
          return;
        }
        const path = await pickMaterialFile("选择角色图（一次一张，尺寸需与逻辑画布一致）");
        if (!path) return;
        await importBaseCharacter(path);
        await loadCandidates();
        flash("角色图已登记为候选 —— 确认无误后点“确认锁定”");
      }
    });
  };

  const handleImportBase = () =>
    run("import-base", async () => {
      setDescribeError(null); // 换图后旧的识图错误不再相关
      if (!view?.baseCharacterSource) {
        setSourceConfirm("import");
        return;
      }
      const path = await pickMaterialFile("选择角色图（一次一张，尺寸需与逻辑画布一致）");
      if (!path) return; // cancelled
      const canvas = view.canvas;
      if (canvas) {
        // 探针：尺寸与画布一致 → 直接导入；不符 → 打开裁剪工具（比例锁定画布）。
        const preview = await readImageForPreview(path);
        if (preview.width > 0 && (preview.width !== canvas.unitWidth || preview.height !== canvas.unitHeight)) {
          setCropSession({ path, src: `data:${preview.mime || "image/png"};base64,${preview.data}` });
          return;
        }
      }
      await importBaseCharacter(path);
      await loadCandidates();
      flash("角色图已登记为候选 —— 确认无误后点“确认锁定”");
    });

  const confirmCrop = (rect: CropRect) => {
    if (!cropSession) return;
    const session = cropSession;
    setDescribeError(null);
    void run("import-crop", async () => {
      await importBaseCharacterCropped(session.path, rect);
      setCropSession(null);
      await loadCandidates();
      flash("已按画布规格裁剪并登记 —— 在步骤 4 确认锁定");
    });
  };

  // 把识图失败的原始错误翻译成可操作的提示（跟随在内联错误下方）。
  const describeErrorHint = (raw: string): string => {
    if (raw.includes("超时") || raw.includes("deadline") || raw.includes("timeout")) {
      return "识图调用超时 —— 免费网关高峰期常见：可稍后重试，或在设置中更换更快的视觉模型";
    }
    if (raw.includes("400") || raw.includes("vision") || raw.includes("image") || raw.includes("不支持") || raw.includes("invalid") || raw.includes("multimodal")) {
      return "当前增强模型很可能不支持图像输入 —— 请在设置中把增强模型换成支持识图的视觉模型（如 qwen-vl / gemini / gpt-4o 类）";
    }
    return "";
  };

  // 识图与“AI 增强描述”同理：不能用 run()（成功后会 load() 用旧草稿回填，
  // 把刚生成的描述覆盖掉），自管 busy + 内联错误。
  const handleDescribeImage = async () => {
    if (!pendingImportDraft) return;
    setDescribeError(null);
    setBusy("describe-image");
    try {
      const text = await describeBaseCharacterImage(pendingImportDraft.id);
      setDescription(text);
      flash("AI 已根据角色图生成描述 —— 请核对与修改后保存");
    } catch (e) {
      setDescribeError(String(e));
    } finally {
      setBusy(null);
    }
  };

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
  // 本地导入流程的待锁定草稿（一次一张；重新导入会替换上一张未锁定图）。
  const pendingImportDraft = view?.baseCharacterSource === "import"
    ? candidates.find((c) => c.status === "pending") ?? null
    : null;

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
            <div className="faint">身份包的基础角色来源不可切换。{view.baseCharacterSource === "ai" ? "当前可使用提示词、风格和参考图生成候选。" : "流程：逻辑画布 → 导入角色图 → 角色描述 → 确认锁定；描述会作为后续动作生成的提示词基础。"}</div>
          </div>
        ) : (
          <div className="identity__source-choice">
            <div className="faint">请选择基础角色来源。确定后不可切换，请确认身份包的制作方式。</div>
            <div className="identity__source-options">
              <button className="pixel-btn" disabled={busy !== null} onClick={() => void handleLockAI()}>
                AI 生成
                <span className="faint">提示词 + 风格 + 参考图</span>
              </button>
              <button className="pixel-btn" disabled={busy !== null} onClick={() => void handleImportBase()} title="锁定来源后，先在“逻辑画布”步骤确定规格，再选择一张与画布同尺寸的本地角色图">
                导入已有角色图
                <span className="faint">本地角色图 · 一次一张 · 尺寸校验</span>
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
              <section className="identity__step-card identity__step-card--description">
                <span className="identity__step-marker mono">{isAI ? "1" : "3"}</span>
                <div className="identity__step-head">
                  <span className="identity__step-title mono">角色描述</span>
                  <span className="identity__step-hint faint">{isAI ? "保存为身份档案，并作为生成提示词的基础" : "会作为后续动作生成的提示词基础 —— 请尽量准确描述这张本地角色图的外观、配色与体型"}</span>
                </div>
                <textarea value={description} onChange={(e) => setDescription(e.target.value)} placeholder="描述角色外观、配色、体型…" disabled={basisAdopted} />
                {basisAdopted && (
                  <div className="identity__locked-note">已采用身份基准 —— 身份生成阶段已完成，角色描述已锁定。</div>
                )}
                {descriptionDirty && !basisAdopted && (
                  <div className="identity__actions">
                    <button className="pixel-btn pixel-btn--ok" disabled={busy === "desc"} onClick={() => void handleSaveDescription()} title="有未保存的描述修改">
                      {busy === "desc" ? "保存中…" : "保存描述"}
                    </button>
                  </div>
                )}
                {isAI && (
                  <div className="identity__actions identity__actions--left">
                    <button
                      className="pixel-btn"
                      disabled={busy !== null || !description.trim() || models?.enhanceSupported === false || basisAdopted}
                      onClick={() => void handleEnhanceDescription()}
                      title={basisAdopted ? "已采用身份基准 —— 角色描述已锁定" : models?.enhanceSupported === false ? "当前未配置可用的文本模型 —— 请在设置中配置增强模型" : "调用文本模型把简短描述扩写成结构化角色设定 —— 将产生一次文本调用费用；结果填入上方文本框，请检查后保存"}
                    >
                      {busy === "enhance" ? "增强中…（最长约 90 秒）" : "AI 增强描述"}
                    </button>
                    <span className="faint">
                      {models?.enhanceSupported
                        ? `文本模型：${models.enhanceProviderId || models.providerName || models.providerId} / ${models.enhanceModel} · 慢模型可能需要 30–90 秒`
                        : "当前未配置可用的文本模型 —— 请在设置中配置增强模型"}
                    </span>
                  </div>
                )}
                {isAI && enhanceError && <div className="error-text">{enhanceError}</div>}
                {!isAI && (
                  <div className="identity__actions identity__actions--left">
                    <button
                      className="pixel-btn"
                      disabled={busy !== null || basisAdopted || !pendingImportDraft || models?.enhanceSupported === false}
                      onClick={() => void handleDescribeImage()}
                      title={basisAdopted ? "已锁定身份基准 —— 角色描述已锁定" : !pendingImportDraft ? "先在步骤 2 导入角色图，才能识图生成描述" : models?.enhanceSupported === false ? "当前未配置可用的文本模型 —— 请在设置中配置增强模型（需支持视觉识图）" : "调用视觉文本模型读取角色图并生成描述 —— 结果填入上方文本框，请核对后保存"}
                    >
                      {busy === "describe-image" ? "识图中…（最长约 90 秒）" : "AI 识图生成描述"}
                    </button>
                    <span className="faint">
                      {models?.enhanceSupported === false
                        ? "当前未配置可用的文本模型 —— 请在设置中配置增强模型（需支持视觉识图）"
                        : pendingImportDraft
                          ? `识图模型：${models?.enhanceProviderId || models?.providerName || models?.providerId || "?"} / ${models?.enhanceModel || "?"} · 需支持视觉的模型，90 秒超时`
                          : "先在步骤 2 导入角色图"}
                    </span>
                  </div>
                )}
                {!isAI && describeError && (
                  <div className="error-text">
                    {describeError}
                    {describeErrorHint(describeError) && <div className="faint">{describeErrorHint(describeError)}</div>}
                  </div>
                )}
              </section>

              {view.baseCharacterSource === "ai" && (
                <section className="identity__step-card identity__step-card--references">
                  <span className="identity__step-marker mono">2</span>
                  <div className="identity__step-head">
                    <span className="identity__step-title mono">参考图</span>
                    <span className="identity__step-hint faint">主参考图最多 1 张 · 辅助参考图最多 2 张 —— 基础角色与每次动作生成都随请求外发，数量越多花费越高</span>
                  </div>
                  {basisAdopted && (
                    <div className="identity__locked-note">已采用身份基准 —— 身份生成阶段已完成，参考图已锁定。</div>
                  )}
                  <div className="row">
                    <button className="pixel-btn" disabled={busy === "import-reference_image" || refsFull || basisAdopted} onClick={() => void handleImport()}>
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
                              <button className="pixel-btn" disabled={busy !== null || basisAdopted} onClick={() => void handleSetMain(m.id)} aria-label={`设为主参考图 ${m.name}`} title={basisAdopted ? "已采用身份基准 —— 参考图已锁定" : "把这张辅助参考图设为主参考图（当前主参考图自动改为辅助）"}>设为主参考</button>
                            )}
                            <button className="pixel-btn pixel-btn--warn" disabled={busy !== null || basisAdopted} onClick={() => setMaterialToDelete(m)} aria-label={`删除素材 ${m.name}`} title={basisAdopted ? "已采用身份基准 —— 参考图已锁定" : "从身份包中删除此素材文件"}>删除</button>
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </section>
              )}

              <section className="identity__step-card identity__step-card--canvas">
                <span className="identity__step-marker mono">{isAI ? "3" : "1"}</span>
                <div className="identity__step-head">
                  <span className="identity__step-title mono">逻辑画布</span>
                  <span className="identity__step-hint faint">每帧图像的像素网格大小 —— 生成、导入与锚点都以此为准</span>
                </div>
                {basisAdopted && (
                  <div className="identity__locked-note">已采用身份基准 —— 身份生成阶段已完成，单元尺寸已锁定。</div>
                )}
                <div className="row">
                  <select value={sizeChoice} onChange={(e) => handleSizeChoice(e.target.value)} aria-label="常用画布尺寸" title="选择常用尺寸，或选自定义输入任意宽高" disabled={basisAdopted}>
                    {CANVAS_PRESETS.map((n) => <option key={n} value={`${n}x${n}`}>{n === 256 ? "256 × 256 · PerfectPixel 标准" : `${n} × ${n}`}</option>)}
                    <option value="custom">自定义{canvasW === canvasH && CANVAS_PRESETS.includes(parseInt(canvasW, 10)) ? "" : `（${canvasW} × ${canvasH}）`}</option>
                  </select>
                  {sizeChoice === "custom" && (
                    <>
                      <input className="identity__num" value={canvasW} onChange={(e) => setCanvasW(e.target.value)} aria-label="画布宽（像素）" disabled={basisAdopted} />
                      <span>×</span>
                      <input className="identity__num" value={canvasH} onChange={(e) => setCanvasH(e.target.value)} aria-label="画布高（像素）" disabled={basisAdopted} />
                    </>
                  )}
                  {canvasDirty && !basisAdopted && (
                    <button className="pixel-btn pixel-btn--ok" disabled={busy === "canvas"} onClick={() => void handleSaveCanvas()} title="有未保存的尺寸修改">
                      {busy === "canvas" ? "保存中…" : "保存规格"}
                    </button>
                  )}
                </div>
                <div className="faint identity__step-note">PerfectPixel 标准：256 × 256，每帧保留 24px 安全边距，并自动按质量中心与脚底基线对齐。其他尺寸可自定义。</div>
                {canvasW === "256" && canvasH === "256" && (
                  <label className="identity__pixelperfect-toggle">
                    <input type="checkbox" checked={perfectPixelStandard} onChange={handlePerfectPixelToggle} disabled={busy !== null || basisAdopted} />
                    <span>启用 PerfectPixel 标准</span>
                    <span className="faint">24px 安全边距 · 质量中心对齐 · 自动脚底基线</span>
                  </label>
                )}
              </section>

              {view.baseCharacterSource === "ai" && (
                <section className="identity__step-card identity__step-card--generate">
                  <span className="identity__step-marker mono">4</span>
                  <div className="identity__step-head">
                    <span className="identity__step-title mono">生成角色</span>
                    <span className="identity__step-hint faint">确认门之后才会调用 · 结果长效保存在候选区</span>
                  </div>
                  {basisAdopted ? (
                    <div className="identity__locked-note">已采用身份基准 —— 身份生成阶段已完成，不可再生成新的候选。</div>
                  ) : (
                    <>
                  <div className="row">
                    <span className="faint">选择风格与模型后生成{models?.imageModel ? ` · 当前 provider：${models.providerName || models.providerId}` : ""} · 结果保存在下方候选区</span>
                  </div>
                  {stylesError && <div className="error-text">风格目录加载失败：{stylesError}</div>}
                  <div className="identity__generation-form">
                    <div className="identity__generation-row">
                          <select value={genStyle} onChange={(e) => { setGenStyle(e.target.value); resetGenResult(); }} aria-label="生成风格" title="生成风格" disabled={genBusy}>
                            {styles.length === 0 && genStyle !== "custom" && <option value={genStyle}>{genStyle}</option>}
                            {styles.map((style) => <option key={style.id} value={style.id}>{style.name}</option>)}
                            <option value="custom">自定义风格…</option>
                          </select>
                          <select
                            value={selectedProviderId}
                            onChange={(e) => { setGenProvider(e.target.value); setGenModel(""); resetGenResult(); }}
                            aria-label="生成提供商"
                            title="选择任务级图像提供商；可使用设置中已配置的其他提供商"
                            disabled={genBusy || imageProviders.length === 0}
                          >
                            {imageProviders.length === 0 && <option value="">暂无可用图像提供商</option>}
                            {imageProviders.map((p) => <option key={p.id} value={p.id}>{p.name || p.id}</option>)}
                          </select>
                          <select value={genModel} onChange={(e) => { setGenModel(e.target.value); resetGenResult(); }} aria-label="生成模型" title="选择所选提供商的图像模型；可与当前提供商不同" disabled={genBusy || !selectedProvider}>
                            <option value="">默认{selectedProvider?.imageModel ? `（${selectedProvider.imageModel}）` : ""}</option>
                            {selectedImageModels.filter((m) => m !== (selectedProvider?.imageModel ?? "")).map((m) => <option key={m} value={m}>{m}</option>)}
                          </select>
                          <span className={`mono ${genStatus === "error" ? "error-text" : genStatus === "done" ? "status-ok" : genStatus === "reviewing" ? "status-warn" : genStatus === "generating" ? "status-busy" : "faint"}`}>
                            {genStatus === "generating"
                              ? `生成中 · 已用 ${Math.max(0, Math.floor((Date.now() - (genStartedAt ?? Date.now())) / 1000))} 秒`
                              : GEN_STATUS_LABEL[genStatus]}
                          </span>
                          <span className="identity__generation-spacer" />
                          {(genStatus === "idle" || genStatus === "error" || genStatus === "done") && (
                            <button
                              className="pixel-btn pixel-btn--primary"
                              onClick={() => { resetGenResult(); void startGeneration(); }}
                              aria-label="生成基础角色"
                              title="先弹出确认（不发起调用），再决定是否执行；可使用同一模型再次生成候选"
                            >
                              {genStatus === "error" ? "重试" : "生成"}
                            </button>
                          )}
                        </div>
                        {genStyle === "custom" && (
                          <input className="pixel-input" value={genStyleCustom} onChange={(e) => { setGenStyleCustom(e.target.value); resetGenResult(); }} placeholder="自定义风格提示词（英文更稳定），例：dark fantasy pixel art, muted colors" aria-label="自定义风格提示词" title="作为风格片段写入生成提示词，与内置预设同等生效" disabled={genBusy} />
                        )}
                        <div className="faint identity__step-note">
                          {genStyle === "custom"
                            ? "自定义风格：上方输入的文字会作为风格片段写入提示词。"
                            : (() => { const s = styles.find((x) => x.id === genStyle); return s ? `${s.name}：${s.description}` : ""; })()}
                        </div>
                        {genStyle !== "custom" && (() => {
                          const style = styles.find((x) => x.id === genStyle);
                          return style?.negativePrompt ? (
                            <div className="faint identity__negative-prompt">
                              <span className="mono">内置负面提示词：</span>{style.negativePrompt}
                            </div>
                          ) : null;
                        })()}
                        <label className="identity__generation-custom" title="勾选后可为本次生成改写提示词；不勾选则使用身份描述">
                          <input type="checkbox" checked={genCustomPrompt} disabled={genBusy} onChange={(e) => { setGenCustomPrompt(e.target.checked); if (e.target.checked && !genDescription) setGenDescription(description); resetGenResult(); }} />
                          <span className="faint">自定义提示词（默认使用身份描述）</span>
                        </label>
                        {genCustomPrompt && (
                          <input className="pixel-input" value={genDescription} onChange={(e) => { setGenDescription(e.target.value); resetGenResult(); }} aria-label="生成提示词" title="本次生成的提示词覆盖 —— 只影响本次生成，不改变身份描述" placeholder="生成提示词（默认取身份描述）" disabled={genBusy} />
                        )}
                        {genStatus === "error" && genError && (
                          <div className="error-text">
                            {genError}
                            {genError.includes("context deadline exceeded") && (
                              <div className="faint">
                                免费网关高峰期常见：请求已发出但网关迟迟不返回。可稍后重试、减少参考图（尤其大图）再试，或在设置中更换 Provider / 调整单次超时秒数。
                              </div>
                            )}
                          </div>
                        )}
                        {genStatus === "generating" && (
                          <div className="faint identity__step-note">
                            已提交任务队列 —— provider 通常需要 1-3 分钟，进度与失败原因见右上角「任务」抽屉。
                            {onOpenTasks && (
                              <button className="pixel-btn" onClick={onOpenTasks} aria-label="查看任务进度" title="打开任务抽屉查看实时进度">
                                查看进度
                              </button>
                            )}
                          </div>
                        )}
                  </div>
                    </>
                  )}
                </section>
              )}

              {view.baseCharacterSource === "import" && (
                <section className="identity__step-card identity__step-card--import-base">
                  <span className="identity__step-marker mono">2</span>
                  <div className="identity__step-head">
                    <span className="identity__step-title mono">导入角色图</span>
                    <span className="identity__step-hint faint">一次一张 · 重新导入会替换未锁定的上一张 · 与逻辑画布尺寸完全一致（不符将被拦截）· 不调用外部 AI</span>
                  </div>
                  <div className="row">
                    <button
                      className="pixel-btn pixel-btn--primary"
                      disabled={busy !== null || basisAdopted || !importCanvasReady}
                      onClick={() => void handleImportBase()}
                      title={basisAdopted ? "已采用身份基准 —— 身份生成阶段已完成" : !view.canvas ? "请先在上方“逻辑画布”步骤设置并保存规格，才能导入角色图" : canvasDirty ? "画布规格有未保存的修改 —— 请先保存，再导入角色图" : pendingImportDraft ? "重新选择一张本地角色图（一次一张），替换当前未锁定图；尺寸不符时将打开裁剪工具" : "选择一张本地角色图（一次一张）；尺寸不符时将打开裁剪工具"}
                    >
                      {pendingImportDraft && !basisAdopted ? "重新导入角色图" : "导入角色图"}
                    </button>
                    <span className="faint">
                      当前画布：{view.canvas ? `${view.canvas.unitWidth} × ${view.canvas.unitHeight}${canvasDirty ? "（有未保存修改）" : ""}` : "未设置"}
                    </span>
                  </div>
                  {!basisAdopted && pendingImportDraft && (
                    <div className="identity__lock-preview">
                      <img src={`data:image/png;base64,${pendingImportDraft.png}`} alt="已导入的角色图" title="点击放大预览" onClick={() => setPreviewCandidate({ src: `data:image/png;base64,${pendingImportDraft.png}`, title: "已导入的角色图" })} />
                      <div className="col">
                        <span className="faint">已导入：本地角色图{view.canvas ? ` · 画布规格 ${view.canvas.unitWidth} × ${view.canvas.unitHeight}` : ""}</span>
                        <div className="row">
                          <button className="pixel-btn" disabled={busy !== null} onClick={() => handleFlipCandidate(pendingImportDraft)} aria-label="水平翻转已导入的角色图" title="水平翻转（左右镜像）—— 朝向相反时用它纠正；真实像素翻转，再点一次翻回">
                            ⇄ 水平翻转
                          </button>
                          <button className="pixel-btn pixel-btn--warn" disabled={busy !== null} onClick={() => setCandidateToDelete(pendingImportDraft)} aria-label="删除已导入的角色图" title="删除这张已导入的角色图（记录与图片文件一并移除，需确认）">
                            删除角色图
                          </button>
                          <span className="faint">重新导入会替换这张未锁定图</span>
                        </div>
                      </div>
                    </div>
                  )}
                  {basisAdopted ? (
                    <div className="identity__locked-note">已采用身份基准 —— 身份生成阶段已完成，不可再导入新的候选。</div>
                  ) : !importCanvasReady ? (
                    <div className="identity__step-note faint">画布优先：在上方“逻辑画布”步骤设置并保存尺寸后，才能导入角色图；导入图片的尺寸必须与画布完全一致。</div>
                  ) : null}
                </section>
              )}

              {isAI && (
              <section className="identity__step-card identity__step-card--candidates">
                <span className="identity__step-marker mono">5</span>
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
                  <div className="empty-state">尚未生成候选 —— 完成生成任务并确认执行</div>
                ) : candidates.length > 0 ? (
                  <div className="identity__candidates">
                    {[...candidates]
                      .sort((a, b) => Number(b.id === view.baseCharacterId) - Number(a.id === view.baseCharacterId))
                      .map((candidate) => {
                        const adopted = view.baseCharacterId === candidate.id;
                        const pending = candidate.status === "pending";
                        const rejected = candidate.status === "rejected";
                        return (
                          <div className={`pixel-panel identity__candidate${adopted ? " identity__candidate--adopted" : ""}`} key={candidate.id}>
                            {adopted && (
                              <span className="identity__candidate-adopted-mark mono">✓ 已采用</span>
                            )}
                            {!adopted && (
                              <button className="identity__candidate-delete" disabled={busy !== null} onClick={() => setCandidateToDelete(candidate)} aria-label={`删除候选 ${candidate.id.slice(0, 8)}`} title={pending ? "删除此候选图（记录与图片文件一并移除，需确认）" : "删除此已弃用候选图（记录与图片文件一并移除，需确认）"}>
                                ✕
                              </button>
                            )}
                            <button className="identity__candidate-flip" disabled={busy !== null} onClick={() => handleFlipCandidate(candidate)} aria-label={`水平翻转候选 ${candidate.id.slice(0, 8)}`} title="水平翻转（左右镜像）—— 大模型常把角色朝向画反，用它纠正；真实像素翻转，再点一次翻回">
                              ⇄
                            </button>
                            <img src={`data:image/png;base64,${candidate.png}`} alt="基础角色候选" onClick={() => setPreviewCandidate({ src: `data:image/png;base64,${candidate.png}`, title: `候选 ${candidate.id.slice(0, 8)}` })} />
                            <div className={`mono identity__candidate-badge${adopted ? " identity__candidate-badge--adopted" : rejected ? " identity__candidate-badge--rejected" : ""}`}>
                              {adopted ? "已采用" : pending ? "待采用" : rejected ? "已弃用" : candidate.status}
                            </div>
                            <div className="faint">{candidate.provider === "import" ? "本地导入" : `${candidate.provider} · ${candidate.model}`}</div>
                            {pending && (
                              <div className="row">
                                <button
                                  className="pixel-btn pixel-btn--primary"
                                  disabled={busy !== null}
                                  onClick={() => setCandidateToAdopt(candidate)}
                                  aria-label="采用此候选"
                                  title="采用此候选前需要二次确认"
                                >
                                  采用
                                </button>
                              </div>
                            )}
                          </div>
                        );
                      })}
                  </div>
                ) : null}
              </section>
              )}
              {view.baseCharacterSource === "import" && (
                <section className="identity__step-card identity__step-card--lock">
                  <span className="identity__step-marker mono">4</span>
                  <div className="identity__step-head">
                    <span className="identity__step-title mono">确认锁定</span>
                    <span className="identity__step-hint faint">把导入的角色图锁定为身份基准 —— 锁定后画布、角色图与描述均不可再修改</span>
                  </div>
                  {basisAdopted ? (
                    <div className="identity__lock-preview">
                      {adoptedCandidate && (
                        <img src={`data:image/png;base64,${adoptedCandidate.png}`} alt="已锁定的身份基准" title="点击放大预览" onClick={() => setPreviewCandidate({ src: `data:image/png;base64,${adoptedCandidate.png}`, title: "已锁定的身份基准" })} />
                      )}
                      {adoptedCandidate && (
                        <div className="col">
                          <button className="pixel-btn" disabled={busy !== null} onClick={() => handleFlipCandidate(adoptedCandidate)} aria-label="水平翻转已锁定的身份基准" title="水平翻转（左右镜像）—— 朝向相反时用它纠正；后续动作生成读取同一文件，翻转即时生效；再点一次翻回">
                            ⇄ 水平翻转
                          </button>
                        </div>
                      )}
                      <div className="identity__locked-note">✓ 已锁定为身份基准 —— 身份生成阶段已完成，可进入动作生成阶段。</div>
                    </div>
                  ) : pendingImportDraft ? (
                    <div className="identity__lock-preview">
                      <img src={`data:image/png;base64,${pendingImportDraft.png}`} alt="待锁定的角色图" title="点击放大预览" onClick={() => setPreviewCandidate({ src: `data:image/png;base64,${pendingImportDraft.png}`, title: "待锁定的角色图" })} />
                      <div className="col">
                        <span className="faint">来源：本地导入{view.canvas ? ` · 画布规格 ${view.canvas.unitWidth} × ${view.canvas.unitHeight}` : ""}</span>
                        <div className="row">
                          <button className="pixel-btn pixel-btn--primary" disabled={busy !== null} onClick={() => setCandidateToAdopt(pendingImportDraft)} aria-label="确认锁定身份基准" title="锁定前需要二次确认">
                            确认锁定为身份基准
                          </button>
                          <button className="pixel-btn" disabled={busy !== null} onClick={() => handleFlipCandidate(pendingImportDraft)} aria-label="水平翻转待锁定的角色图" title="水平翻转（左右镜像）—— 朝向相反时用它纠正；真实像素翻转，再点一次翻回">
                            ⇄ 水平翻转
                          </button>
                          <span className="faint">锁定前仍可在步骤 2 重新导入替换这张图</span>
                        </div>
                      </div>
                    </div>
                  ) : (
                    <div className="empty-state">尚未导入角色图 —— 完成步骤 2 后，在这里确认锁定。</div>
                  )}
                </section>
              )}
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

      {cropSession && view?.canvas && (
        <ImageCropModal
          src={cropSession.src}
          targetWidth={view.canvas.unitWidth}
          targetHeight={view.canvas.unitHeight}
          busy={busy === "import-crop"}
          onConfirm={confirmCrop}
          onCancel={() => setCropSession(null)}
        />
      )}

      <ImageLightbox source={previewCandidate} onClose={() => setPreviewCandidate(null)} />

      <ConfirmModal
        open={candidateToAdopt !== null}
        title={isAI ? "采用候选" : "确认锁定身份基准"}
        message={candidateToAdopt ? (
          <div className="candidate-confirm">
            <img src={`data:image/png;base64,${candidateToAdopt.png}`} alt={isAI ? "待采用候选预览" : "待锁定角色图预览"} />
            <div className="col">
              {isAI ? (
                <>
                  <span>确定采用这张候选图作为身份基准吗？</span>
                  <span className="faint">采用后其他候选将自动标记为“已弃用”，不能再采用。</span>
                </>
              ) : (
                <>
                  <span>确定把这张本地角色图锁定为身份基准吗？</span>
                  <span className="faint">锁定后画布、角色图与角色描述均不可再修改，不能更换。</span>
                </>
              )}
            </div>
          </div>
        ) : ""}
        confirmLabel={isAI ? "确认采用" : "确认锁定"}
        cancelLabel="取消"
        onConfirm={confirmAdoptCandidate}
        onCancel={() => setCandidateToAdopt(null)}
      />

      <ConfirmModal
        open={candidateToDelete !== null}
        title={isAI ? "删除候选" : "删除角色图"}
        message={candidateToDelete ? (
          <div className="candidate-confirm">
            <img src={`data:image/png;base64,${candidateToDelete.png}`} alt="待删除预览" />
            <div className="col">
              {isAI ? (
                <>
                  <span>确定删除这张候选图吗？</span>
                  <span className="faint">候选记录与图片文件将从身份包中永久移除，无法恢复。</span>
                </>
              ) : (
                <>
                  <span>确定删除这张已导入的角色图吗？</span>
                  <span className="faint">删除后可重新导入；记录与图片文件将从身份包中永久移除，无法恢复。</span>
                </>
              )}
            </div>
          </div>
        ) : ""}
        confirmLabel="确认删除"
        cancelLabel="取消"
        danger
        onConfirm={confirmDeleteCandidate}
        onCancel={() => setCandidateToDelete(null)}
      />

      {genStatus === "reviewing" && genPlan && (
        <ConfirmModal
          open
          title="生成确认"
          confirmLabel="确认执行"
          cancelLabel="取消（不发起调用）"
          onConfirm={() => void confirmGenerationForm()}
          onCancel={() => void cancelGenerationForm()}
          message={
            <ul className="mono gen-plan__list">
              <li>
                provider / model：{genPlan.providerId} / {genPlan.model}
                {genPlan.providerType ? `（协议：${genPlan.providerType}）` : ""}
              </li>
              <li>预计调用量：{genPlan.expectedCalls} 次 · 总尝试上限 {genPlan.maxTotalAttempts} 次</li>
              <li>预算：约 {genPlan.expectedCost.toFixed(2)} {genPlan.currency}（上限 {genPlan.maxCost.toFixed(2)} {genPlan.currency}）</li>
              <li>外发素材：{(genPlan.outboundMaterials ?? []).length} 个{(genPlan.outboundMaterials ?? []).length > 0 ? "（主参考图 / 辅助参考图随请求发送）" : ""}</li>
              <li className="gen-plan__prompt">
                提示词快照（{genPlan.prompt.stylePresetId === "custom" ? "自定义风格" : styles.find((s) => s.id === genPlan.prompt.stylePresetId)?.name ?? genPlan.prompt.stylePresetId}，{genPlan.prompt.frameCount} 帧）：
                <div className="faint">{genPlan.prompt.prompt}</div>
              </li>
            </ul>
          }
        />
      )}

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
