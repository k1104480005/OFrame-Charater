// 制作 > 动作 sub-page —— 新版动作与方向集工作台（2026-09-05 验收版）：
// 多动作卡（各自可独立展开/折叠）、点亮式九宫格生成、右键菜单（预览原图 /
// 水平翻转 / 带反馈重新生成 / 删除该格动画）、自动镜像、方向模式快捷点亮、
// 三栏配置布局（预设与生成参数 / 方向模式与生成 / 罗盘点亮）、批量操作区、
// 全屏动画预览与原始条带查看器。
// 数据流：本页只负责展示与会话包读写；生成经 prepareGeneration →
// GenerationPlanModal 确认 → confirmGeneration 执行，队列任务通过
// task:changed 事件 + 轮询同步进度（任务行 id = plan id）。
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type {
  ActionPresetView,
  CandidatePreviewView,
  GenerationPlanView,
  MotionBatchSummaryView,
  MotionView,
  ProviderOptionView,
  TaskSummary,
} from "../../api/client";
import {
  clearMotionDirection,
  confirmGeneration,
  createMotion,
  deleteMotion,
  fetchBatchSummary,
  fetchDirectionPreview,
  fetchDirectionRawStrip,
  fetchDirectionThumbnail,
  fetchMotions,
  fetchPresetCatalog,
  fetchProviderOptions,
  fetchTasks,
  flipMotionDirection,
  onTasksChanged,
  prepareGeneration,
  renameMotion,
  setMotionGenerationSettings,
  setMotionLoop,
  setMotionProviderSettings,
  setMotionStrategy,
} from "../../api/client";
import { ConfirmModal } from "../../components/ConfirmModal";
import { GenerationPlanModal } from "../../components/GenerationPlanModal";
import { MotionAddModal } from "../../components/MotionAddModal";
import type { MotionAddMode } from "../../components/MotionAddModal";
import { PixelCanvas } from "../../components/PixelCanvas";
import { useSession } from "../../state/SessionContext";
import { useWork } from "../../state/WorkContext";
import "./MotionPage.css";

/* ------------------------------------------------------------------ */
/* 方向常量                                                             */
/* ------------------------------------------------------------------ */

const DIR_LABEL: Record<string, string> = {
  down: "下",
  up: "上",
  left: "左",
  right: "右",
  "up-left": "左上",
  "up-right": "右上",
  "down-left": "左下",
  "down-right": "右下",
};

/** 罗盘九宫格逐格渲染顺序（3×3 行优先；中心 null = 中心格） */
const COMPASS_ORDER: ReadonlyArray<string | null> = [
  "up-left",
  "up",
  "up-right",
  "left",
  null,
  "right",
  "down-left",
  "down",
  "down-right",
];

/** 水平镜像派生槽（单向 source → derived：right→left 等） */
const MIRROR_SLOTS: readonly string[] = ["left", "up-left", "down-right"];

/** derived → source 反向查找 */
const MIRROR_SOURCE: Record<string, string> = {
  left: "right",
  "up-left": "up-right",
  "down-right": "down-left",
};

const KIND_LABEL: Record<string, string> = {
  generate: "方向生成",
  replace: "方向替换",
  regenerate: "重新生成",
};

const ORIGIN_LABEL: Record<string, string> = {
  generated: "生成",
  mirrored: "镜像",
  replaced: "替换",
};

/** 生成中任务种类（决定 genLocked 是否由该任务触发） */
const GEN_TASK_KINDS = new Set(["generate", "replace", "regenerate"]);

/** 缩略图/预览缓存键 */
const cacheKey = (mid: string, dir: string): string => `${mid}:${dir}`;

type DirMode = "base" | "four" | "eight" | "custom";

interface DirModeOption {
  id: DirMode;
  label: string;
  title: string;
  count: number | null;
}

const DIR_MODE_OPTIONS: DirModeOption[] = [
  { id: "base", label: "原图方向", title: "仅正面（下）：单方向动画，适合基础角色待机", count: 1 },
  { id: "four", label: "4 方向", title: "下 / 上 / 左 / 右 四个方向（开启自动镜像时左侧派生）", count: 4 },
  { id: "eight", label: "8 方向", title: "四方向 + 四个斜角方向（开启自动镜像时镜像对派生）", count: 8 },
  { id: "custom", label: "自定义点亮", title: "不改变方向集，仅手动在罗盘上点亮要生成的方向", count: null },
];

/** 各方向策略下需要独立 AI 调用的方向（镜像开启时派生槽不在其中） */
function strategyBasicDirs(count: number, mirror: boolean): string[] {
  switch (count) {
    case 1:
      return ["down"];
    case 4:
      return mirror ? ["down", "up", "right"] : ["down", "up", "left", "right"];
    case 8:
      return mirror
        ? ["down", "up", "right", "up-right", "down-left"]
        : ["down", "up", "left", "right", "up-left", "up-right", "down-left", "down-right"];
    default:
      return [];
  }
}

/* ------------------------------------------------------------------ */
/* 类型                                                                */
/* ------------------------------------------------------------------ */

interface MotionPageProps {
  onOpenAcceptance?: () => void;
  onOpenTasks?: () => void;
}

/** 一个在途生成运行（task id = plan id）。local=true 表示本页确认发起、方向信息已知。 */
interface GenRun {
  taskId: string;
  motionId: string;
  kind: string;
  label: string;
  status: string;
  progress: number;
  error: string;
  /** 该运行涉及的方向（basic + 镜像派生），用于格子/圆点的 genning 展示 */
  cells: string[];
  local: boolean;
}

/** 动作卡上编辑中的表单字段（覆盖后端视图值，直到保存成功） */
interface FormPatch {
  presetID?: string;
  description?: string;
  frames?: number;
  providerId?: string;
  model?: string;
}

/** 动作卡的“当前生效”表单值（本地未保存修改优先于后端视图） */
interface MotionFormValues {
  presetID: string;
  description: string;
  frames: number;
  providerId: string;
  model: string;
}

interface PlanModalState {
  plan: GenerationPlanView;
  motionId: string;
  motionName: string;
}

interface CtxMenuState {
  x: number;
  y: number;
  motionId: string;
  dir: string;
}

interface OverlayState {
  motionId: string;
  dir: string;
}

interface RawViewerState {
  motionId: string;
  dir: string;
}

interface BatchExecState {
  running: boolean;
  idx: number;
  total: number;
  failed: string[];
  currentName: string;
}

type Message = { kind: "ok" | "err"; text: string } | null;

/** PixelCanvas 播放帧映射 */
function pixelFramesOf(preview: CandidatePreviewView | null, speed: number): Array<{ png: string; durationMs?: number; anchors?: Array<{ name: string; x: number; y: number }> }> {
  if (!preview) return [];
  return preview.frames.map((f) => ({
    png: f.png,
    durationMs: f.durationMs && f.durationMs > 0 ? f.durationMs / speed : undefined,
    anchors: (f.anchors ?? []).map((a) => ({ name: a.Name, x: a.X, y: a.Y })),
  }));
}

function presetCardName(m: MotionView, presetsById: Map<string, ActionPresetView>): string {
  if (m.actionPresetId === "custom") return "自定义";
  return presetsById.get(m.actionPresetId)?.name ?? m.name;
}

function presetSnapshotName(m: MotionView, presetsById: Map<string, ActionPresetView>): string {
  if (m.actionPresetId === "custom") return "自定义动作";
  return presetsById.get(m.actionPresetId)?.name ?? "动作预设";
}

/* ================================================================== */
/* MotionPage                                                          */
/* ================================================================== */

export function MotionPage({ onOpenAcceptance, onOpenTasks }: MotionPageProps) {
  const { pkg } = useSession();
  const { bumpPreview } = useWork();

  /* --- 数据源 --- */
  const [motions, setMotions] = useState<MotionView[]>([]);
  const [presets, setPresets] = useState<ActionPresetView[]>([]);
  const [imageOptions, setImageOptions] = useState<ProviderOptionView[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<Message>(null);
  const [busy, setBusy] = useState<string | null>(null);

  /* --- 多动作卡 UI 状态 --- */
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const [modes, setModes] = useState<Record<string, DirMode>>({});
  const [lit, setLit] = useState<Record<string, string[]>>({});
  const [formPatch, setFormPatch] = useState<Record<string, FormPatch>>({});
  const formPatchRef = useRef<Record<string, FormPatch>>({});
  formPatchRef.current = formPatch;

  /* --- 在途生成运行 / 任务同步 --- */
  const [runsById, setRunsById] = useState<Record<string, GenRun>>({});
  const runsRef = useRef<Record<string, GenRun>>({});
  const prevActiveRef = useRef<Set<string>>(new Set());

  /* --- 缩略图缓存 --- */
  const [thumbs, setThumbs] = useState<Record<string, string>>({});
  const thumbInflight = useRef<Set<string>>(new Set());

  /* --- 弹窗/浮层 --- */
  const [addOpen, setAddOpen] = useState(false);
  const [addMode, setAddMode] = useState<MotionAddMode>("preset");
  const [planModal, setPlanModal] = useState<PlanModalState | null>(null);
  const [deleteMotionId, setDeleteMotionId] = useState<string | null>(null);
  const [ctxMenu, setCtxMenu] = useState<CtxMenuState | null>(null);
  const [overlay, setOverlay] = useState<OverlayState | null>(null);
  const [rawViewer, setRawViewer] = useState<RawViewerState | null>(null);
  const [regen, setRegen] = useState<CtxMenuState | null>(null);
  const [regenFeedback, setRegenFeedback] = useState("");

  /* --- 批量操作区 --- */
  const [batchSummary, setBatchSummary] = useState<MotionBatchSummaryView | null>(null);
  const [batchConfirmOpen, setBatchConfirmOpen] = useState(false);
  const [batchExec, setBatchExec] = useState<BatchExecState | null>(null);

  const presetsById = useMemo(() => {
    const m = new Map<string, ActionPresetView>();
    for (const p of presets) m.set(p.id, p);
    return m;
  }, [presets]);

  const pkgPath = pkg?.path ?? "";

  const activeRuns = useMemo(
    () => Object.values(runsById).filter((r) => r.status === "queued" || r.status === "running"),
    [runsById],
  );
  const genLocked = activeRuns.length > 0;
  const activeProgress =
    activeRuns.length === 0 ? 0 : activeRuns.reduce((sum, r) => sum + r.progress, 0) / activeRuns.length;

  /* ============================================================= */
  /* 基础数据加载                                                     */
  /* ============================================================= */

  const refreshMotions = useCallback(async () => {
    try {
      const ms = await fetchMotions();
      setMotions(ms);
      return ms;
    } catch (e) {
      setError(String(e));
      return null;
    }
  }, []);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const [catalog, opts] = await Promise.all([
          fetchPresetCatalog(),
          fetchProviderOptions("image").catch(() => [] as ProviderOptionView[]),
        ]);
        if (!cancelled) {
          setPresets(catalog.actions);
          setImageOptions(opts);
        }
      } catch (e) {
        if (!cancelled) setError(String(e));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [pkg?.path]);

  useEffect(() => {
    void refreshMotions();
  }, [refreshMotions, pkg?.path]);

  // 换包时清空所有卡片/缓存状态，避免引用上一包的旧 id。
  useEffect(() => {
    setExpanded({});
    setModes({});
    setLit({});
    setFormPatch({});
    setThumbs({});
    thumbInflight.current.clear();
    runsRef.current = {};
    setRunsById({});
    setBatchSummary(null);
    setBatchExec(null);
    setPlanModal(null);
    setOverlay(null);
    setRawViewer(null);
    setCtxMenu(null);
    setRegen(null);
    setError(null);
    setMessage(null);
  }, [pkg?.path]);

  // 初始化每张卡的状态：新卡默认展开；首屏没有任何卡片展开时展开第一张。
  useEffect(() => {
    if (motions.length === 0) return;
    setExpanded((prev) => {
      const next = { ...prev };
      for (const m of motions) {
        if (next[m.id] === undefined) next[m.id] = false;
      }
      const anyOpen = motions.some((m) => next[m.id]);
      if (!anyOpen) next[motions[0].id] = true;
      return next;
    });
    setModes((prev) => {
      const next = { ...prev };
      for (const m of motions) {
        if (next[m.id] === undefined) {
          next[m.id] = m.strategy.count === 1 ? "base" : m.strategy.count === 4 ? "four" : m.strategy.count === 8 ? "eight" : "custom";
        }
      }
      return next;
    });
    setLit((prev) => {
      const next = { ...prev };
      for (const m of motions) {
        if (next[m.id] === undefined) next[m.id] = [];
      }
      return next;
    });
  }, [motions]);

  /* ============================================================= */
  /* 任务事件 / 轮询：在途生成运行                                        */
  /* ============================================================= */

  const commitRuns = useCallback((next: Record<string, GenRun>) => {
    runsRef.current = next;
    setRunsById(next);
  }, []);

  const addRun = useCallback(
    (run: GenRun) => {
      commitRuns({ ...runsRef.current, [run.taskId]: run });
    },
    [commitRuns],
  );

  const removeRun = useCallback(
    (taskId: string) => {
      if (!runsRef.current[taskId]) return;
      const next = { ...runsRef.current };
      delete next[taskId];
      commitRuns(next);
    },
    [commitRuns],
  );

  const hasActiveRun = (runs: Record<string, GenRun>): boolean =>
    Object.values(runs).some((r) => r.status === "queued" || r.status === "running");

  // 生成完成后统一刷新：动作视图（帧/方向更新）与缩略图缓存。
  const afterRunsFinished = useCallback(() => {
    void refreshMotions();
    thumbInflight.current.clear();
    setThumbs({});
  }, [refreshMotions]);

  // 任务事件 + 轮询共同维护 runsById：只跟踪当前包的生成类任务；本页确认后
  // 乐观登记的 local run 含 motionId/方向，事件到达后按 id 对齐。
  const syncTasks = useCallback(
    (tasks: TaskSummary[]) => {
      const prev = runsRef.current;
      const hadActive = hasActiveRun(prev);
      const next: Record<string, GenRun> = {};
      for (const [id, run] of Object.entries(prev)) {
        if (run.status === "queued" || run.status === "running") next[id] = run;
      }
      let changed = Object.keys(next).length !== Object.keys(prev).length;
      for (const t of tasks) {
        if (!GEN_TASK_KINDS.has(t.kind)) continue;
        if (pkgPath && t.packagePath && t.packagePath !== pkgPath) continue;
        const isActive = t.status === "queued" || t.status === "running";
        const existing = next[t.id];
        if (isActive) {
          const merged: GenRun = {
            taskId: t.id,
            motionId: existing?.motionId ?? "",
            kind: t.kind,
            label: KIND_LABEL[t.kind] ?? t.kind,
            status: t.status,
            progress: t.progress,
            error: t.error ?? "",
            cells: existing?.cells ?? [],
            local: existing?.local ?? false,
          };
          const prevRun = prev[t.id];
          if (
            !prevRun ||
            prevRun.status !== merged.status ||
            Math.abs(prevRun.progress - merged.progress) > 0.001 ||
            prevRun.error !== merged.error
          ) {
            changed = true;
          }
          next[t.id] = merged;
        } else if (existing) {
          delete next[t.id];
          changed = true;
        }
      }
      const nowActive = hasActiveRun(next);
      const activeNow = new Set(
        Object.keys(next).filter((k) => {
          const r = next[k];
          return r && (r.status === "queued" || r.status === "running");
        }),
      );
      const prevActiveSet = prevActiveRef.current;
      const transitioned = (prevActiveSet.size > 0 && activeNow.size === 0) || hadActive !== nowActive;
      prevActiveRef.current = activeNow;
      if (!changed && !transitioned) return;
      commitRuns(next);
      if (hadActive && !nowActive) afterRunsFinished();
    },
    [pkgPath, commitRuns, afterRunsFinished],
  );

  useEffect(() => {
    const off = onTasksChanged((tasks) => syncTasks(tasks));
    void fetchTasks().then(syncTasks).catch(() => undefined);
    const iv = window.setInterval(() => {
      void fetchTasks().then(syncTasks).catch(() => undefined);
    }, 2500);
    return () => {
      off();
      window.clearInterval(iv);
    };
  }, [syncTasks]);

  /* ============================================================= */
  /* 缩略图预取                                                        */
  /* ============================================================= */

  useEffect(() => {
    const need: Array<{ mid: string; dir: string; key: string }> = [];
    for (const m of motions) {
      for (const d of m.directions) {
        if (d.frames.length === 0) continue;
        const key = cacheKey(m.id, d.direction);
        if (thumbs[key] !== undefined || thumbInflight.current.has(key)) continue;
        thumbInflight.current.add(key);
        need.push({ mid: m.id, dir: d.direction, key });
      }
    }
    if (need.length === 0) return;
    let cancelled = false;
    for (const n of need) {
      void fetchDirectionThumbnail(n.mid, n.dir)
        .then((png) => {
          if (!cancelled) setThumbs((prev) => ({ ...prev, [n.key]: png }));
        })
        .catch(() => undefined)
        .finally(() => {
          thumbInflight.current.delete(n.key);
        });
    }
    return () => {
      cancelled = true;
    };
  }, [motions, thumbs]);

  /* ============================================================= */
  /* 卡片基础 helpers                                                  */
  /* ============================================================= */

  const dirOf = (m: MotionView, dir: string) => m.directions.find((d) => d.direction === dir);
  const isMirrorDerivedCell = (m: MotionView, dir: string): boolean => m.strategy.mirror && MIRROR_SLOTS.includes(dir);

  /** 勾选且未生成、且可直接点亮的方向（生成所选方向的统计口径） */
  const pendingLights = (m: MotionView): string[] => {
    const raw = lit[m.id] ?? [];
    return raw.filter((dir) => {
      const d = dirOf(m, dir);
      if (!d || d.frames.length > 0) return false;
      if (isMirrorDerivedCell(m, dir)) return false;
      return true;
    });
  };

  const activeCellsForMotion = (mid: string): Set<string> => {
    const out = new Set<string>();
    for (const r of activeRuns) {
      if (r.motionId === mid) for (const c of r.cells) out.add(c);
    }
    return out;
  };

  /** 当前生效的表单值（本地未保存修改优先） */
  const effectiveForm = (m: MotionView): MotionFormValues => {
    const p = formPatch[m.id];
    return {
      presetID: (p?.presetID ?? m.actionPresetId) || "walk",
      description: p?.description ?? m.actionDescription,
      frames: p?.frames ?? m.frameCount,
      providerId: p?.providerId ?? m.providerId,
      model: p?.model ?? m.model,
    };
  };

  /* ============================================================= */
  /* 动作创建                                                          */
  /* ============================================================= */

  const openAdd = (mode: MotionAddMode) => {
    setAddMode(mode);
    setAddOpen(true);
  };

  const handlePickPreset = (presetId: string) => {
    const preset = presetsById.get(presetId);
    setAddOpen(false);
    setBusy("add");
    setError(null);
    setMessage(null);
    void (async () => {
      try {
        // 新建动作默认 8 方向 + 自动镜像（可在卡上随时切换方向模式/镜像）。
        let mv = await createMotion(preset?.name ?? presetId, 8, true);
        mv = await setMotionGenerationSettings(mv.id, presetId, "", preset?.frames ?? 0);
        if (preset) mv = await setMotionLoop(mv.id, preset.loop);
        setMotions((prev) => [...prev, mv]);
        setExpanded((prev) => ({ ...prev, [mv.id]: true }));
        setLit((prev) => ({ ...prev, [mv.id]: [] }));
        setModes((prev) => ({ ...prev, [mv.id]: "custom" }));
        setMessage({ kind: "ok", text: `已添加动作「${mv.name}」—— 展开配置点亮方向后即可生成` });
      } catch (e) {
        setError(String(e));
      } finally {
        setBusy(null);
      }
    })();
  };

  const handleCreateCustom = (name: string, description: string, frameCount: number) => {
    setAddOpen(false);
    setBusy("add");
    setError(null);
    setMessage(null);
    void (async () => {
      try {
        const m = await createMotion(name, 8, true);
        const mv = await setMotionGenerationSettings(m.id, "custom", description, frameCount);
        setMotions((prev) => [...prev, mv]);
        setExpanded((prev) => ({ ...prev, [m.id]: true }));
        setLit((prev) => ({ ...prev, [m.id]: [] }));
        setModes((prev) => ({ ...prev, [m.id]: "custom" }));
        setMessage({ kind: "ok", text: `已添加自定义动作「${m.name}」` });
      } catch (e) {
        setError(String(e));
      } finally {
        setBusy(null);
      }
    })();
  };

  const handleDeleteMotion = () => {
    const id = deleteMotionId;
    setDeleteMotionId(null);
    if (!id) return;
    setBusy(`delete:${id}`);
    setError(null);
    void (async () => {
      try {
        await deleteMotion(id);
        setMotions((prev) => prev.filter((m) => m.id !== id));
        setExpanded((prev) => {
          const n = { ...prev };
          delete n[id];
          return n;
        });
        setLit((prev) => {
          const n = { ...prev };
          delete n[id];
          return n;
        });
        setModes((prev) => {
          const n = { ...prev };
          delete n[id];
          return n;
        });
        setFormPatch((prev) => {
          const n = { ...prev };
          delete n[id];
          return n;
        });
        setMessage({ kind: "ok", text: "动作已删除" });
      } catch (e) {
        setError(String(e));
      } finally {
        setBusy(null);
      }
    })();
  };

  /* ============================================================= */
  /* 表单自动保存（preset / 描述 / 帧数 / Provider / 模型）                 */
  /* ============================================================= */

  const saveTimers = useRef<Record<string, number>>({});

  const patchForm = useCallback((mid: string, patch: Partial<FormPatch>) => {
    setFormPatch((prev) => ({ ...prev, [mid]: { ...(prev[mid] ?? {}), ...patch } }));
  }, []);

  const clearAppliedPatch = useCallback((mid: string, mv: MotionView) => {
    setFormPatch((prev) => {
      const cur = prev[mid];
      if (!cur) return prev;
      const next: FormPatch = {};
      if (cur.presetID !== undefined && cur.presetID !== (mv.actionPresetId || "walk")) next.presetID = cur.presetID;
      if (cur.description !== undefined && cur.description !== mv.actionDescription) next.description = cur.description;
      if (cur.frames !== undefined && cur.frames !== mv.frameCount) next.frames = cur.frames;
      if (cur.providerId !== undefined && cur.providerId !== mv.providerId) next.providerId = cur.providerId;
      if (cur.model !== undefined && cur.model !== mv.model) next.model = cur.model;
      const updated = { ...prev };
      if (Object.keys(next).length === 0) delete updated[mid];
      else updated[mid] = next;
      return updated;
    });
  }, []);

  // 可复用的保存执行体：无本地差异时直接清理 patch；否则落库并按返回刷新动作行。
  const persistFormPatch = useCallback(
    async (mid: string, patch: FormPatch) => {
      const m = motions.find((x) => x.id === mid);
      if (!m) {
        setFormPatch((prev) => {
          const n = { ...prev };
          delete n[mid];
          return n;
        });
        return;
      }
      const prevPreset = m.actionPresetId || "walk";
      const effPreset = patch.presetID ?? prevPreset;
      const effDesc = patch.description ?? m.actionDescription;
      const effFrames = patch.frames ?? m.frameCount;
      const effProvider = patch.providerId ?? m.providerId;
      const effModel = patch.model ?? m.model;
      const genChange =
        (patch.presetID !== undefined && effPreset !== prevPreset) ||
        (patch.description !== undefined && patch.description !== m.actionDescription) ||
        (patch.frames !== undefined && patch.frames !== m.frameCount);
      const provChange =
        (patch.providerId !== undefined && patch.providerId !== m.providerId) ||
        (patch.model !== undefined && patch.model !== m.model);
      if (!genChange && !provChange) {
        setFormPatch((prev) => {
          const n = { ...prev };
          delete n[mid];
          return n;
        });
        return;
      }
      let mv: MotionView | null = null;
      if (genChange) mv = await setMotionGenerationSettings(mid, effPreset, effDesc, effFrames);
      if (provChange) mv = await setMotionProviderSettings(mid, effProvider, effModel);
      // 非 custom 且预设确实变更：卡名自动跟随预设名。
      if (mv && patch.presetID !== undefined && effPreset !== prevPreset && effPreset !== "custom") {
        const preset = presetsById.get(effPreset);
        if (preset) mv = await renameMotion(mid, preset.name);
      }
      if (mv) {
        setMotions((prev) => prev.map((row) => (row.id === mid ? (mv as MotionView) : row)));
        clearAppliedPatch(mid, mv);
      } else {
        setFormPatch((prev) => {
          const n = { ...prev };
          delete n[mid];
          return n;
        });
      }
    },
    [motions, presetsById, clearAppliedPatch],
  );

  const saveForm = useCallback(
    (mid: string, patch: FormPatch) => {
      void persistFormPatch(mid, patch).catch((e) => setError(`自动保存失败：${String(e)}`));
    },
    [persistFormPatch],
  );

  // 立即落盘该卡的未保存表单修改（生成前确保用最新设置；无修改时直接返回）。
  const flushPendingForm = useCallback(
    async (mid: string): Promise<void> => {
      const pending = formPatchRef.current[mid];
      if (!pending || Object.keys(pending).length === 0) return;
      if (saveTimers.current[mid] !== undefined) {
        window.clearTimeout(saveTimers.current[mid]);
        delete saveTimers.current[mid];
      }
      await persistFormPatch(mid, pending);
    },
    [persistFormPatch],
  );

  // 字段变更后防抖保存（每卡一个合并计时器：保存时取该卡最新 patch）。
  useEffect(() => {
    for (const mid of Object.keys(formPatch)) {
      if (saveTimers.current[mid] !== undefined) continue;
      saveTimers.current[mid] = window.setTimeout(() => {
        delete saveTimers.current[mid];
        const p = formPatchRef.current[mid];
        if (p) saveForm(mid, p);
      }, 600);
    }
  }, [formPatch, saveForm]);

  /* ============================================================= */
  /* 方向模式 / 镜像 / 循环                                               */
  /* ============================================================= */

  const applyDirMode = (m: MotionView, mode: DirMode) => {
    if (genLocked) return;
    const opt = DIR_MODE_OPTIONS.find((o) => o.id === mode);
    if (!opt) return;
    setError(null);
    if (opt.count === null) {
      setModes((prev) => ({ ...prev, [m.id]: mode }));
      return;
    }
    const targetCount = opt.count;
    setBusy(`mode:${m.id}`);
    void (async () => {
      try {
        let mv = m;
        if (m.strategy.count !== targetCount) {
          mv = await setMotionStrategy(m.id, targetCount, m.strategy.mirror);
          setMotions((prev) => prev.map((row) => (row.id === m.id ? mv : row)));
        }
        const basic = strategyBasicDirs(targetCount, mv.strategy.mirror);
        setLit((prev) => {
          const old = prev[m.id] ?? [];
          const keep = old.filter((dir) => {
            const d = mv.directions.find((x) => x.direction === dir);
            return d && d.frames.length === 0 && !(mv.strategy.mirror && MIRROR_SLOTS.includes(dir));
          });
          const add = basic.filter((dir) => {
            const d = mv.directions.find((x) => x.direction === dir);
            return d && d.frames.length === 0 && !(mv.strategy.mirror && MIRROR_SLOTS.includes(dir)) && !keep.includes(dir);
          });
          return { ...prev, [m.id]: [...keep, ...add] };
        });
        setModes((prev) => ({ ...prev, [m.id]: mode }));
        setMessage({
          kind: "ok",
          text: mode === "base" ? "已切为原图方向（单方向）" : mode === "four" ? "已切为 4 方向并点亮对应方向" : "已切为 8 方向并点亮对应方向",
        });
      } catch (e) {
        setError(String(e));
      } finally {
        setBusy(null);
      }
    })();
  };

  const toggleLit = (m: MotionView, dir: string) => {
    if (genLocked) return;
    const d = dirOf(m, dir);
    if (!d || d.frames.length > 0 || isMirrorDerivedCell(m, dir)) return;
    setLit((prev) => {
      const cur = prev[m.id] ?? [];
      const next = cur.includes(dir) ? cur.filter((x) => x !== dir) : [...cur, dir];
      return { ...prev, [m.id]: next };
    });
    setModes((prev) => {
      if ((prev[m.id] ?? "custom") === "custom") return prev;
      return { ...prev, [m.id]: "custom" };
    });
  };

  const clearLitFor = (m: MotionView) => {
    setLit((prev) => ({ ...prev, [m.id]: [] }));
    setModes((prev) => ({ ...prev, [m.id]: "custom" }));
  };

  const toggleMirror = (m: MotionView, next: boolean) => {
    if (genLocked) return;
    setBusy(`strategy:${m.id}`);
    setError(null);
    void (async () => {
      try {
        const mv = await setMotionStrategy(m.id, m.strategy.count, next);
        setMotions((prev) => prev.map((row) => (row.id === m.id ? mv : row)));
        if (next) {
          setLit((prev) => {
            const cur = prev[m.id] ?? [];
            return { ...prev, [m.id]: cur.filter((dir) => !MIRROR_SLOTS.includes(dir)) };
          });
        }
        setMessage({ kind: "ok", text: next ? "已开启自动镜像：对侧方向自动派生，节省调用" : "已关闭自动镜像：各方向独立生成" });
      } catch (e) {
        setError(String(e));
      } finally {
        setBusy(null);
      }
    })();
  };

  const handleLoopChange = (m: MotionView, loop: boolean) => {
    if (genLocked) return;
    setBusy(`loop:${m.id}`);
    void (async () => {
      try {
        const mv = await setMotionLoop(m.id, loop);
        setMotions((prev) => prev.map((row) => (row.id === m.id ? mv : row)));
      } catch (e) {
        setError(String(e));
      } finally {
        setBusy(null);
      }
    })();
  };

  /* ============================================================= */
  /* 点亮式生成：prepare → GenerationPlanModal → confirm                  */
  /* ============================================================= */

  const handlePrepare = (m: MotionView) => {
    const dirs = pendingLights(m);
    if (dirs.length === 0 || genLocked) return;
    setError(null);
    setMessage(null);
    setBusy(`prepare:${m.id}`);
    void (async () => {
      try {
        // 生成方案基于动作卡已保存的 Provider/模型/预设 —— 先把未落盘的
        // 表单修改立即保存，避免生成用了旧设置。
        await flushPendingForm(m.id);
        const plan = await prepareGeneration({
          packagePath: "",
          motionId: m.id,
          providerId: "",
          model: "",
          directions: 0,
          stylePresetId: "",
          actionPresetId: "",
          frameCount: 0,
          maxAttemptsPerDirection: 0,
          generateDirections: dirs,
          forceRegenerate: false,
        });
        setPlanModal({ plan, motionId: m.id, motionName: m.name });
      } catch (e) {
        setError(String(e));
      } finally {
        setBusy(null);
      }
    })();
  };

  const handlePlanDecision = (accept: boolean) => {
    if (busy === "confirm") return;
    const pm = planModal;
    if (!pm) return;
    setBusy("confirm");
    setError(null);
    if (!accept) {
      void confirmGeneration(pm.plan.id, false)
        .catch((e) => setError(String(e)))
        .finally(() => {
          setPlanModal(null);
          setBusy(null);
          setMessage({ kind: "ok", text: "已取消，未发起任何调用" });
        });
      return;
    }
    // 乐观登记 local run：事件到达前即点亮生成中状态
    addRun({
      taskId: pm.plan.id,
      motionId: pm.motionId,
      kind: pm.plan.kind,
      label: KIND_LABEL[pm.plan.kind] ?? pm.plan.kind,
      status: "queued",
      progress: 0,
      error: "",
      cells: [...(pm.plan.basicLabels ?? []), ...(pm.plan.mirroredLabels ?? [])],
      local: true,
    });
    void (async () => {
      try {
        const r = await confirmGeneration(pm.plan.id, true);
        if (r.status !== "executed") {
          setMessage({ kind: "err", text: `生成失败：${r.error || r.status}` });
        } else {
          const dirs = (r.results ?? []).map((x) => x.direction).filter(Boolean);
          const extra = dirs.length > 0 ? ` · ${dirs.map((d) => DIR_LABEL[d] ?? d).join("、")}` : "";
          setMessage({ kind: "ok", text: `已执行：${r.callsMade} 次调用 / ${r.attempts} 次尝试${extra}` });
          thumbInflight.current.clear();
          setThumbs({});
          bumpPreview();
          void refreshMotions();
        }
      } catch (e) {
        setMessage({ kind: "err", text: `执行失败：${String(e)}` });
      } finally {
        removeRun(pm.plan.id);
        setPlanModal(null);
        setBusy(null);
      }
    })();
  };

  /* ============================================================= */
  /* 九宫格右键菜单动作                                                  */
  /* ============================================================= */

  const invalidateMotionVisuals = (mid: string) => {
    thumbInflight.current.clear();
    setThumbs((prev) => {
      const next = { ...prev };
      for (const key of Object.keys(next)) {
        if (key.startsWith(`${mid}:`)) delete next[key];
      }
      return next;
    });
  };

  const handleFlipDirection = (motion: MotionView, dir: string) => {
    setCtxMenu(null);
    setBusy(`flip:${motion.id}:${dir}`);
    void (async () => {
      try {
        const msg = await flipMotionDirection(motion.id, dir);
        invalidateMotionVisuals(motion.id);
        await refreshMotions();
        setMessage({ kind: "ok", text: msg });
      } catch (e) {
        setError(String(e));
      } finally {
        setBusy(null);
      }
    })();
  };

  const handleClearDirection = (motion: MotionView, dir: string) => {
    setCtxMenu(null);
    setBusy(`clear:${motion.id}:${dir}`);
    void (async () => {
      try {
        await clearMotionDirection(motion.id, dir);
        invalidateMotionVisuals(motion.id);
        await refreshMotions();
        setMessage({ kind: "ok", text: `已删除「${DIR_LABEL[dir] ?? dir}」动画 —— 该格已回到未生成状态，可重新点亮生成` });
      } catch (e) {
        setError(String(e));
      } finally {
        setBusy(null);
      }
    })();
  };

  /** 该格重新生成的目标方向：镜像派生格 → 重新生成源方向（镜像自动同步） */
  const regenTargetOf = (motion: MotionView, dir: string): string => {
    const d = dirOf(motion, dir);
    if (d?.origin === "mirrored" && d.source) return d.source;
    return dir;
  };

  const openRegenConfirm = (motion: MotionView, dir: string) => {
    setCtxMenu(null);
    setRegen({ x: 0, y: 0, motionId: motion.id, dir });
    setRegenFeedback("");
  };

  // 带反馈重新生成：forceRegenerate 单方向计划（不命中幂等缓存）
  const handleRegenPrepare = () => {
    if (!regen) return;
    const motion = motions.find((m) => m.id === regen.motionId);
    if (!motion) {
      setRegen(null);
      return;
    }
    const target = regenTargetOf(motion, regen.dir);
    const fb = regenFeedback.trim();
    setRegen(null);
    setBusy(`regen-prepare:${motion.id}`);
    setError(null);
    void (async () => {
      try {
        await flushPendingForm(motion.id);
        const plan = await prepareGeneration({
          packagePath: "",
          motionId: motion.id,
          providerId: "",
          model: "",
          directions: 0,
          stylePresetId: "",
          actionPresetId: "",
          feedback: fb,
          frameCount: 0,
          maxAttemptsPerDirection: 0,
          generateDirections: [target],
          forceRegenerate: true,
        });
        setPlanModal({ plan, motionId: motion.id, motionName: motion.name });
      } catch (e) {
        setError(String(e));
      } finally {
        setBusy(null);
      }
    })();
  };

  /* ============================================================= */
  /* 全屏预览 / 原图查看器                                               */
  /* ============================================================= */

  const openOverlay = (mid: string, dir: string) => {
    setOverlay({ motionId: mid, dir });
  };

  const openRawViewer = (mid: string, dir: string) => {
    setCtxMenu(null);
    setRawViewer({ motionId: mid, dir });
  };

  /* ============================================================= */
  /* 批量操作区                                                         */
  /* ============================================================= */

  const batchSelections = useMemo(() => {
    const out: Array<{ motionId: string; directions: string[] }> = [];
    for (const m of motions) {
      const dirs = pendingLights(m);
      if (dirs.length > 0) out.push({ motionId: m.id, directions: dirs });
    }
    return out;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [motions, lit, pkgPath]);

  useEffect(() => {
    let cancelled = false;
    const t = window.setTimeout(() => {
      void fetchBatchSummary(batchSelections)
        .then((s) => {
          if (!cancelled) setBatchSummary(s);
        })
        .catch(() => {
          if (!cancelled) setBatchSummary(null);
        });
    }, 300);
    return () => {
      cancelled = true;
      window.clearTimeout(t);
    };
  }, [batchSelections]);

  const batchExecutable = (batchSummary?.items ?? []).some((it) => it.basicDirs.length > 0);

  const executeBatch = () => {
    const items = (batchSummary?.items ?? []).filter((it) => it.basicDirs.length > 0);
    if (items.length === 0) return;
    setBatchConfirmOpen(false);
    setError(null);
    setMessage(null);
    setBatchExec({ running: true, idx: 0, total: items.length, failed: [], currentName: items[0]?.motionName ?? "" });
    void (async () => {
      const failed: string[] = [];
      for (let i = 0; i < items.length; i++) {
        const it = items[i];
        setBatchExec({ running: true, idx: i, total: items.length, failed: [...failed], currentName: it.motionName });
        try {
          await flushPendingForm(it.motionId);
          const plan = await prepareGeneration({
            packagePath: "",
            motionId: it.motionId,
            providerId: "",
            model: "",
            directions: 0,
            stylePresetId: "",
            actionPresetId: "",
            frameCount: 0,
            maxAttemptsPerDirection: 0,
            generateDirections: it.basicDirs,
            forceRegenerate: false,
          });
          addRun({
            taskId: plan.id,
            motionId: it.motionId,
            kind: plan.kind,
            label: "批量生成",
            status: "queued",
            progress: 0,
            error: "",
            cells: [...(plan.basicLabels ?? []), ...(plan.mirroredLabels ?? [])],
            local: true,
          });
          try {
            const r = await confirmGeneration(plan.id, true);
            if (r.status !== "executed") failed.push(`${it.motionName}（${r.error || r.status}）`);
          } finally {
            removeRun(plan.id);
          }
          thumbInflight.current.clear();
          setThumbs({});
          await refreshMotions();
        } catch (e) {
          failed.push(`${it.motionName}（${String(e)}）`);
        }
      }
      setBatchExec({ running: false, idx: items.length, total: items.length, failed: [...failed], currentName: "" });
      setMessage({
        kind: failed.length === 0 ? "ok" : "err",
        text:
          failed.length === 0
            ? `批量生成完成：${items.length}/${items.length} 个动作已执行`
            : `批量生成结束：${items.length - failed.length}/${items.length} 成功，失败：${failed.join("；")}`,
      });
      void refreshMotions();
    })();
  };

  const batchCostText = (summary: MotionBatchSummaryView | null): string => {
    if (!summary || summary.costs.length === 0) return "预估 0（未解析到 Provider）";
    return summary.costs.map((c) => `约 ${c.amount.toFixed(2)} ${c.currency}`).join(" + ");
  };

  /* ============================================================= */
  /* 渲染                                                             */
  /* ============================================================= */

  const overlayMotion = overlay ? (motions.find((m) => m.id === overlay.motionId) ?? null) : null;

  return (
    <div className="pixel-panel col">
      <h3 className="mono panel-heading">动作与方向集 / MOTION</h3>
      <hr className="pixel-rule" />
      {error && (
        <div className="col">
          <div className="error-text">{error}</div>
          <div className="row">
            <button className="pixel-btn" onClick={() => onOpenTasks?.()} title="任务抽屉中可查看失败原因并重试/放弃">
              查看任务
            </button>
          </div>
        </div>
      )}
      {message && <div className={message.kind === "ok" ? "status-ok" : "error-text"}>{message.text}</div>}

      {/* 生成进行中：顶层任务态 */}
      {activeRuns.length > 0 && (
        <div className="motion-task-status">
          <div className="row">
            <span className="motion-spin" aria-hidden="true" />
            <span>
              生成进行中：{activeRuns.length} 个任务 —— 全部动作卡已锁定
              {activeRuns.every((r) => r.status === "queued") ? "（排队中）" : ""}
            </span>
            <span className="grow" />
            <button className="pixel-btn" onClick={() => onOpenTasks?.()} title="打开全局任务抽屉查看进度与失败原因">
              查看任务
            </button>
          </div>
          <div
            className="motion-task-progress"
            role="progressbar"
            aria-valuenow={Math.round(activeProgress * 100)}
            aria-valuemin={0}
            aria-valuemax={100}
          >
            <div className="motion-task-progress__fill" style={{ width: `${Math.round(activeProgress * 100)}%` }} />
          </div>
        </div>
      )}

      {/* ① 动作 */}
      <div className="motion-flow">
      <section className="motion-card">
        <div className="motion-card__head">
          <span className="motion-step" aria-hidden="true">
            1
          </span>
          <h4>动作</h4>
          <span className="grow" />
          <div className="motion-card__head-actions">
            <button className="pixel-btn" disabled={busy === "add" || genLocked} onClick={() => openAdd("preset")} title="从动作预设库选择一个动作（点击即创建）">
              ＋ 添加动画
            </button>
            <button className="pixel-btn" disabled={busy === "add" || genLocked} onClick={() => openAdd("custom")} title="自己编写动作描述与帧数">
              ＋ 添加自定义动画
            </button>
          </div>
        </div>
        <div className="motion-card__body">
          {motions.length === 0 ? (
            <div className="empty-state">还没有动作 —— 点击“＋ 添加动画”选择预设，或“＋ 添加自定义动画”</div>
          ) : (
            <ul className="motion-list">
              {motions.map((m) => (
                <MotionItem
                  key={m.id}
                  motion={m}
                  presets={presets}
                  presetsById={presetsById}
                  imageOptions={imageOptions}
                  expanded={expanded[m.id] ?? false}
                  mode={modes[m.id] ?? "custom"}
                  form={effectiveForm(m)}
                  litRaw={lit[m.id] ?? []}
                  pendingCount={pendingLights(m).length}
                  activeCells={activeCellsForMotion(m.id)}
                  runsForMotion={activeRuns.filter((r) => r.motionId === m.id)}
                  genLocked={genLocked}
                  busy={busy}
                  thumbs={thumbs}
                  onToggleExpand={() => setExpanded((prev) => ({ ...prev, [m.id]: !(prev[m.id] ?? false) }))}
                  onDelete={() => setDeleteMotionId(m.id)}
                  onApplyMode={(mode) => applyDirMode(m, mode)}
                  onToggleLit={(dir) => toggleLit(m, dir)}
                  onClearLit={() => clearLitFor(m)}
                  onPatch={(patch) => patchForm(m.id, patch)}
                  onToggleMirror={(next) => toggleMirror(m, next)}
                  onLoop={(loop) => handleLoopChange(m, loop)}
                  onPrepare={() => handlePrepare(m)}
                  prepareBusy={busy === `prepare:${m.id}`}
                  onOpenView={() => {
                    const first = m.directions.find((d) => d.frames.length > 0);
                    if (first) openOverlay(m.id, first.direction);
                  }}
                  onOpenDir={(dir) => openOverlay(m.id, dir)}
                  onMenu={(dir, x, y) => setCtxMenu({ x, y, motionId: m.id, dir })}
                />
              ))}
            </ul>
          )}
        </div>
      </section>

      {/* ② 批量操作 */}
      <section className="motion-card" id="motion-batch">
        <div className="motion-card__head">
          <span className="motion-step" aria-hidden="true">
            2
          </span>
          <h4>批量操作</h4>
          <span className="grow" />
          <span className="faint">按各动作卡勾选的方向统计 · 一键批量生成</span>
        </div>
        <div className="motion-card__body">
          {genLocked && (
            <div className="motion-lock-banner">
              <span className="motion-spin" aria-hidden="true" />
              <span>生成进行中：全部动作卡已锁定</span>
            </div>
          )}
          <div className="motion-batch__totals">
            <span>
              勾选未生成 <b>{batchSummary?.pendingCells ?? 0}</b> 格
            </span>
            <span>
              预计 AI 调用 <b>{batchSummary?.pendingCalls ?? 0}</b> 次
            </span>
            <span>
              预估 <b>{batchCostText(batchSummary)}</b>
            </span>
          </div>
          {batchSummary && batchSummary.items.length > 0 && (
            <ul className="motion-batch__items">
              {batchSummary.items.map((it) => (
                <li key={it.motionId} className="motion-batch__item">
                  <span className="motion-batch__item-name">{it.motionName}</span>
                  <span className="faint">
                    {it.basicDirs.length > 0
                      ? `生成：${it.basicDirs.map((d) => DIR_LABEL[d] ?? d).join("、")}（${it.calls} 次调用）`
                      : "无待生成方向"}
                    {it.mirrorDirs.length > 0 ? ` ＋镜像派生：${it.mirrorDirs.map((d) => DIR_LABEL[d] ?? d).join("、")}` : ""}
                    {it.stuckDirs && it.stuckDirs.length > 0
                      ? ` ｜需关闭镜像才能生成：${it.stuckDirs.map((d) => DIR_LABEL[d] ?? d).join("、")}`
                      : ""}
                  </span>
                  <span className="motion-batch__item-cost">
                    {it.calls > 0 ? `约 ${it.expectedCost.toFixed(2)} ${it.currency}` : "镜像派生不额外计费"}
                  </span>
                </li>
              ))}
            </ul>
          )}
          {!genLocked && batchSummary && batchSummary.items.length === 0 && (
            <div className="faint">当前没有勾选未生成的方向 —— 先在动作卡罗盘上点亮方向</div>
          )}
          {imageOptions.every((o) => o.reason) && (
            <div className="faint">暂无可用图像 Provider —— 请先在设置中完成配置（无可用 Provider 时生成会被拒绝）</div>
          )}
          {batchExec && (
            <div className="motion-task-status">
              <div className="row">
                <span className="motion-spin" aria-hidden="true" />
                <span>
                  {batchExec.running
                    ? `批量生成中：${batchExec.currentName}（${batchExec.idx + 1}/${batchExec.total}）`
                    : `批量结束：${batchExec.failed.length === 0 ? "全部成功" : `${batchExec.total - batchExec.failed.length}/${batchExec.total} 成功`}`}
                </span>
                <span className="grow" />
                <button className="pixel-btn" onClick={() => onOpenTasks?.()}>
                  查看任务
                </button>
              </div>
              {batchExec.running && (
                <div
                  className="motion-task-progress"
                  role="progressbar"
                  aria-valuenow={Math.round(activeProgress * 100)}
                  aria-valuemin={0}
                  aria-valuemax={100}
                >
                  <div className="motion-task-progress__fill" style={{ width: `${Math.round(activeProgress * 100)}%` }} />
                </div>
              )}
              {batchExec.failed.length > 0 && <div className="error-text">失败：{batchExec.failed.join("；")}</div>}
            </div>
          )}
          <div className="motion-card__actions">
            <button
              className="pixel-btn pixel-btn--primary"
              disabled={genLocked || batchExec?.running === true || !batchSummary || batchSummary.pendingCalls === 0 || !batchExecutable}
              onClick={() => setBatchConfirmOpen(true)}
              title="一键执行所有动作卡勾选且未生成的方向"
            >
              {genLocked ? "生成中…" : batchExec?.running ? "批量生成中…" : "开始批量生成"}
            </button>
          </div>
        </div>
      </section>
      </div>

      <div className="faint">
        生成进度与失败原因出现在全局任务抽屉；方向替换与质量验收在
        <button className="pixel-btn" onClick={() => onOpenAcceptance?.()}>
          验收标签
        </button>
        中完成。
      </div>

      {/* 弹窗 / 浮层 */}
      <MotionAddModal
        open={addOpen}
        mode={addMode}
        presets={presets}
        busy={busy === "add"}
        onPickPreset={(pid) => handlePickPreset(pid)}
        onCreateCustom={(name, desc, frames) => handleCreateCustom(name, desc, frames)}
        onClose={() => setAddOpen(false)}
      />

      <GenerationPlanModal
        plan={planModal?.plan ?? null}
        motionName={planModal?.motionName}
        busy={busy === "confirm"}
        onConfirm={() => handlePlanDecision(true)}
        onCancel={() => handlePlanDecision(false)}
      />

      <ConfirmModal
        open={deleteMotionId !== null}
        title="删除动作"
        message={`删除动作「${motions.find((m) => m.id === deleteMotionId)?.name ?? ""}」？\n该动作的全部已生成动画与帧数据将被移除。`}
        confirmLabel="删除"
        danger
        onConfirm={() => handleDeleteMotion()}
        onCancel={() => setDeleteMotionId(null)}
      />

      <ConfirmModal
        open={regen !== null}
        title={regen ? `重新生成 · ${motions.find((m) => m.id === regen?.motionId)?.name ?? ""} ${DIR_LABEL[regen?.dir ?? ""] ?? ""}` : "重新生成"}
        message={
          regen ? (
            <div className="col">
              {(() => {
                const motion = motions.find((m) => m.id === regen.motionId);
                const cell = motion ? dirOf(motion, regen.dir) : undefined;
                const isMirrored = cell?.origin === "mirrored";
                const target = motion ? regenTargetOf(motion, regen.dir) : regen.dir;
                return (
                  <div className="col">
                    <div className="faint">
                      {isMirrored
                        ? `「${DIR_LABEL[regen.dir] ?? regen.dir}」是镜像派生格 —— 将重新生成其源方向「${DIR_LABEL[target] ?? target}」，镜像会同步自动派生；同源方向会一并更新。`
                        : `将重新生成「${DIR_LABEL[regen.dir] ?? regen.dir}」方向的动画，沿用该动作已保存的预设与帧数；本次生成不会被幂等缓存拦截。`}
                    </div>
                    <textarea
                      className="pixel-input motion-textarea motion-regen__feedback"
                      rows={3}
                      value={regenFeedback}
                      onChange={(e) => setRegenFeedback(e.target.value)}
                      placeholder="这次希望改进什么？（可选）例如：挥手幅度更大、保持帽子颜色、不要裁掉脚"
                      aria-label="重新生成反馈"
                    />
                  </div>
                );
              })()}
            </div>
          ) : (
            ""
          )
        }
        confirmLabel="计算生成方案"
        onConfirm={() => handleRegenPrepare()}
        onCancel={() => setRegen(null)}
      />

      <ConfirmModal
        open={batchConfirmOpen}
        title="批量生成确认"
        message={
          <div className="col">
            <div className="faint">将按以下动作逐一执行（每张动作卡使用各自保存的 Provider/模型）：</div>
            <ul className="mono gen-plan__list">
              {(batchSummary?.items ?? [])
                .filter((it) => it.basicDirs.length > 0)
                .map((it) => (
                  <li key={it.motionId}>
                    {it.motionName}：{it.basicDirs.map((d) => DIR_LABEL[d] ?? d).join("、")}（{it.calls} 次调用，约 {it.expectedCost.toFixed(2)} {it.currency}）
                  </li>
                ))}
            </ul>
            <div className="faint">
              合计 {batchSummary?.pendingCalls ?? 0} 次 AI 调用，{batchCostText(batchSummary)}。确认后执行：生成期间动作卡锁定，进度显示在批量操作区与任务抽屉。
            </div>
          </div>
        }
        confirmLabel="开始批量生成"
        onConfirm={() => executeBatch()}
        onCancel={() => setBatchConfirmOpen(false)}
      />

      {ctxMenu && (
        <ContextMenuPanel
          menu={ctxMenu}
          onClose={() => setCtxMenu(null)}
          onRaw={(dir) => {
            const m = motions.find((x) => x.id === ctxMenu.motionId);
            if (m) openRawViewer(m.id, dir);
          }}
          onFlip={(dir) => {
            const m = motions.find((x) => x.id === ctxMenu.motionId);
            if (m) handleFlipDirection(m, dir);
          }}
          onRegen={(dir) => {
            const m = motions.find((x) => x.id === ctxMenu.motionId);
            if (m) openRegenConfirm(m, dir);
          }}
          onClear={(dir) => {
            const m = motions.find((x) => x.id === ctxMenu.motionId);
            if (m) handleClearDirection(m, dir);
          }}
        />
      )}

      {overlayMotion && overlay && <PreviewOverlay key={`${overlay.motionId}:${overlay.dir}`} motion={overlayMotion} dir={overlay.dir} onClose={() => setOverlay(null)} />}

      {rawViewer && <RawStripViewer motionId={rawViewer.motionId} dir={rawViewer.dir} onClose={() => setRawViewer(null)} />}
    </div>
  );
}

/* ================================================================== */
/* 子组件：动作卡                                                        */
/* ================================================================== */

interface MotionItemProps {
  motion: MotionView;
  presets: ActionPresetView[];
  presetsById: Map<string, ActionPresetView>;
  imageOptions: ProviderOptionView[];
  expanded: boolean;
  mode: DirMode;
  form: MotionFormValues;
  litRaw: string[];
  pendingCount: number;
  activeCells: Set<string>;
  runsForMotion: GenRun[];
  genLocked: boolean;
  busy: string | null;
  thumbs: Record<string, string>;
  onToggleExpand: () => void;
  onDelete: () => void;
  onApplyMode: (mode: DirMode) => void;
  onToggleLit: (dir: string) => void;
  onClearLit: () => void;
  onPatch: (patch: Partial<FormPatch>) => void;
  onToggleMirror: (next: boolean) => void;
  onLoop: (loop: boolean) => void;
  onPrepare: () => void;
  prepareBusy: boolean;
  onOpenView: () => void;
  onOpenDir: (dir: string) => void;
  onMenu: (dir: string, x: number, y: number) => void;
}

function MotionItem(props: MotionItemProps) {
  const {
    motion: m,
    presets,
    presetsById,
    imageOptions,
    expanded,
    mode,
    form,
    litRaw,
    pendingCount,
    activeCells,
    runsForMotion,
    genLocked,
    busy,
    thumbs,
    onToggleExpand,
    onDelete,
    onApplyMode,
    onToggleLit,
    onClearLit,
    onPatch,
    onToggleMirror,
    onLoop,
    onPrepare,
    prepareBusy,
    onOpenView,
    onOpenDir,
    onMenu,
  } = props;

  const total = m.directions.length;
  const done = m.directions.filter((d) => d.frames.length > 0).length;
  const allDone = total > 0 && done === total;
  const generated = done > 0;
  const cardRun = runsForMotion[0];

  const cardPresetName = presetCardName(m, presetsById);
  const snapshotName = presetSnapshotName(m, presetsById);

  // 镜像 / 循环：本地乐观状态，等后端返回后由视图同步
  const [mirrorDraft, setMirrorDraft] = useState<boolean | null>(null);
  const [loopDraft, setLoopDraft] = useState<boolean | null>(null);
  useEffect(() => {
    setMirrorDraft(null);
  }, [m.strategy.mirror]);
  useEffect(() => {
    setLoopDraft(null);
  }, [m.loop]);
  const mirrorOn = mirrorDraft ?? m.strategy.mirror;
  const loopOn = loopDraft ?? m.loop;

  const dirOfLocal = (dir: string) => m.directions.find((d) => d.direction === dir);
  const framesOfLocal = (dir: string) => dirOfLocal(dir)?.frames.length ?? 0;

  const providerOpt = imageOptions.find((o) => o.id === form.providerId);

  const modelTitle = form.providerId
    ? `${providerOpt?.name ?? form.providerId} / ${form.model || "Provider 默认"}`
    : "跟随全局默认";
  const modelValue = modelTitle;

  const loopText = loopOn ? "循环播放" : "播放一次";
  const mirrorText = mirrorOn ? "自动镜像" : "独立方向";

  const lightableEmpty = m.directions.filter(
    (d) => d.frames.length === 0 && !(mirrorOn && MIRROR_SLOTS.includes(d.direction)),
  ).length;
  const unlitCount = Math.max(0, lightableEmpty - pendingCount);

  // 预设分组（按目录顺序）
  const grouped: Array<{ category: string; items: ActionPresetView[] }> = [];
  for (const p of presets) {
    const last = grouped[grouped.length - 1];
    if (last && last.category === p.category) last.items.push(p);
    else grouped.push({ category: p.category, items: [p] });
  }

  const lockTitle = genLocked ? "生成进行中：该动作卡已整体锁定，不可修改" : undefined;
  const headTitle = lockTitle ?? (expanded ? "点击收起动作配置" : "点击展开动作配置");

  const dotTitle = (slot: string): string => {
    const frames = framesOfLocal(slot);
    if (activeCells.has(slot)) return `「${DIR_LABEL[slot] ?? slot}」正在生成`;
    if (frames > 0) return `「${DIR_LABEL[slot] ?? slot}」已生成 ${frames} 帧`;
    if (mirrorOn && MIRROR_SLOTS.includes(slot)) return `「${DIR_LABEL[slot] ?? slot}」等待源方向生成后镜像派生`;
    return `「${DIR_LABEL[slot] ?? slot}」未生成`;
  };

  return (
    <li
      className={[
        "motion-item",
        expanded ? "motion-item--active" : "",
        genLocked ? "motion-item--locked" : "",
        cardRun ? "motion-item--generating" : "",
      ].join(" ")}
    >
      {/* 卡头：整行可点击展开/收起 */}
      <div className="motion-item__head" onClick={onToggleExpand} title={headTitle}>
        <button
          type="button"
          className="motion-item__delete"
          onClick={(e) => {
            e.stopPropagation();
            onDelete();
          }}
          disabled={genLocked}
          title={genLocked ? "生成进行中，不能删除动作" : "删除动作"}
          aria-label="删除动作"
        >
          ×
        </button>
        <div className="motion-item__topline">
          <span className={`motion-item__caret${expanded ? " motion-item__caret--open" : ""}`} aria-hidden="true">
            ▶
          </span>
          <span className="motion-item__name">{m.name}</span>
          {cardRun && (
            <span className="motion-item__genbadge" title={cardRun.label}>
              <span className="motion-spin" aria-hidden="true" />
              生成中
            </span>
          )}
        </div>
        <div className="motion-item__meta-row">
          <span className="motion-item__summary">
            {cardPresetName} · {m.frameCount} 帧
          </span>
          <span className="motion-item__headright">
            <span className={`motion-item__status status-${done > 0 ? "ok" : "muted"}`}>
              {done}/{total} 方向
            </span>
            <button
              type="button"
              className="pixel-btn motion-item__view"
              onClick={(e) => {
                e.stopPropagation();
                onOpenView();
              }}
              disabled={done === 0}
              title={done === 0 ? "还没有已生成的格子" : "全屏预览该动作"}
            >
              查看
            </button>
          </span>
        </div>
        {cardRun && (
          <div className="motion-item__genbar" aria-hidden="true">
            {cardRun.status === "queued" ? (
              <div className="motion-item__genbar-fill motion-item__genbar-fill--indeterminate" />
            ) : (
              <div className="motion-item__genbar-fill" style={{ width: `${Math.round(Math.max(0, Math.min(1, cardRun.progress)) * 100)}%` }} />
            )}
          </div>
        )}
      </div>

      {/* 折叠态摘要 */}
      {!expanded && (
        <div className="motion-item__snapshot">
          <div className="motion-item__snapshot-status">
            <div className="motion-item__dots" aria-label="方向完成度">
              {COMPASS_ORDER.map((slot, i) => {
                if (slot === null) {
                  return (
                    <span
                      key={`c${i}`}
                      className={`motion-item__dot motion-item__dot--center${allDone ? " motion-item__dot--ok" : ""}`}
                      title={allDone ? "全部方向已生成" : "尚有方向未生成"}
                    />
                  );
                }
                const d = dirOfLocal(slot);
                const cls = !d
                  ? "motion-item__dot motion-item__dot--absent"
                  : activeCells.has(slot)
                    ? "motion-item__dot motion-item__dot--genning"
                    : d.frames.length > 0
                      ? "motion-item__dot motion-item__dot--done"
                      : mirrorOn && MIRROR_SLOTS.includes(slot)
                        ? "motion-item__dot motion-item__dot--mirror"
                        : "motion-item__dot";
                return <span key={slot} className={cls} title={dotTitle(slot)} />;
              })}
            </div>
            <span className="motion-item__snapshot-count">
              {done}/{total} 方向
            </span>
          </div>
          <div className="motion-item__snapshot-info">
            <div className="motion-item__snapshot-primary">
              <span>{snapshotName}</span>
              <span className="faint">{m.frameCount} 帧</span>
            </div>
            <div className="motion-item__snapshot-secondary">
              <span className="faint">{loopText}</span>
              <span className="faint">{mirrorText}</span>
              <span className="motion-item__model">
                <span className="motion-item__model-label">Provider / 模型</span>
                <span className="motion-item__model-value" title={modelTitle}>
                  {modelValue}
                </span>
              </span>
            </div>
          </div>
        </div>
      )}

      {/* 展开配置区（fieldset disabled = 生成中整体冻结） */}
      {expanded && (
        <fieldset className="motion-item__config" disabled={genLocked}>
          <div className="motion-item__config-grid">
            {/* 左栏：预设 / 描述 / 帧数 / Provider / 模型 / 循环 */}
            <div className="motion-item__config-main">
              <div className="field-row">
                <label className="field-label" htmlFor={`motion-preset-${m.id}`}>
                  动作预设
                </label>
                <select
                  id={`motion-preset-${m.id}`}
                  value={form.presetID}
                  disabled={generated}
                  title={generated ? "🔒 已生成 —— 动作预设已锁定（需先删除该动作的全部已生成动画）" : "切换预设后卡名自动跟随"}
                  onChange={(e) => {
                    onPatch({ presetID: e.target.value });
                    if (e.target.value !== "custom") onPatch({ description: "" });
                  }}
                >
                  {grouped.map((g) => (
                    <optgroup key={g.category} label={g.category}>
                      {g.items.map((p) => (
                        <option key={p.id} value={p.id}>
                          {p.name}
                        </option>
                      ))}
                    </optgroup>
                  ))}
                  <option value="custom">自定义动作</option>
                </select>
              </div>
              {form.presetID === "custom" ? (
                <div className="field-row">
                  <label className="field-label" htmlFor={`motion-desc-${m.id}`}>
                    动作描述
                  </label>
                  <textarea
                    id={`motion-desc-${m.id}`}
                    className="pixel-input motion-textarea"
                    rows={1}
                    value={form.description}
                    disabled={generated}
                    onChange={(e) => onPatch({ description: e.target.value })}
                    placeholder="描述动作语义：例如 向前斩击，先蓄力再挥出…"
                  />
                </div>
              ) : (
                <div className="field-row">
                  <label className="field-label">预设提示词</label>
                  <div className="motion-note" title={presetsById.get(form.presetID)?.promptText ?? ""}>
                    {presetsById.get(form.presetID)?.promptText ?? "（该预设暂无提示词快照）"}
                  </div>
                </div>
              )}
              <div className="field-row">
                <label className="field-label" htmlFor={`motion-frames-${m.id}`}>
                  目标帧数
                </label>
                <input
                  id={`motion-frames-${m.id}`}
                  className="pixel-input"
                  type="number"
                  min={1}
                  max={10}
                  value={String(form.frames)}
                  onChange={(e) => {
                    const v = parseInt(e.target.value, 10);
                    if (Number.isNaN(v)) return;
                    const clamped = Math.max(1, Math.min(10, v));
                    if (clamped !== form.frames) onPatch({ frames: clamped });
                  }}
                  aria-label="目标帧数"
                />
              </div>
              <div className="field-row">
                <label className="field-label" htmlFor={`motion-provider-${m.id}`}>
                  图像 Provider
                </label>
                <select
                  id={`motion-provider-${m.id}`}
                  value={form.providerId}
                  onChange={(e) => {
                    onPatch({ providerId: e.target.value });
                    onPatch({ model: "" });
                  }}
                >
                  <option value="">跟随全局默认</option>
                  {imageOptions.map((o) => (
                    <option key={o.id} value={o.id} disabled={!!o.reason}>
                      {o.name}
                      {o.reason ? ` — 不可用：${o.reason}` : `（${o.models.length} 个图像模型）`}
                    </option>
                  ))}
                </select>
              </div>
              <div className="field-row">
                <label className="field-label" htmlFor={`motion-model-${m.id}`}>
                  模型
                </label>
                <select
                  id={`motion-model-${m.id}`}
                  value={form.model}
                  disabled={!form.providerId}
                  onChange={(e) => onPatch({ model: e.target.value })}
                >
                  <option value="">Provider 默认</option>
                  {(providerOpt?.models ?? []).map((md) => (
                    <option key={md} value={md}>
                      {md}
                    </option>
                  ))}
                </select>
              </div>
              <div className="field-row">
                <label className="field-label" htmlFor={`motion-loop-${m.id}`}>
                  循环播放
                </label>
                <select
                  id={`motion-loop-${m.id}`}
                  value={loopOn ? "loop" : "once"}
                  onChange={(e) => {
                    const next = e.target.value === "loop";
                    setLoopDraft(next);
                    onLoop(next);
                  }}
                >
                  <option value="loop">循环</option>
                  <option value="once">播放一次</option>
                </select>
              </div>
              {generated && <div className="motion-autosave-hint">🔒 已生成 —— 预设与描述已锁定</div>}
            </div>

            {/* 中栏：方向模式 / 自动镜像 / 生成按钮 */}
            <div className="motion-item__config-middle">
              <div className="motion-dir-modes" aria-label="方向模式">
                {DIR_MODE_OPTIONS.map((opt) => (
                  <button
                    key={opt.id}
                    type="button"
                    className={`pixel-btn${mode === opt.id ? " pixel-btn--primary" : ""}`}
                    onClick={() => onApplyMode(opt.id)}
                    title={opt.title}
                  >
                    {opt.label}
                  </button>
                ))}
              </div>
              <label
                className="motion-switch motion-switch--compact"
                title={
                  mirrorOn
                    ? "开启：左右对应方向由源方向自动派生，可减少 AI 调用；源方向重新生成后镜像方向同步更新"
                    : "关闭：每个方向独立生成，可分别控制每个方向的画面"
                }
              >
                <input
                  type="checkbox"
                  className="pixel-switch"
                  checked={mirrorOn}
                  onChange={(e) => {
                    const next = e.target.checked;
                    setMirrorDraft(next);
                    onToggleMirror(next);
                  }}
                />
                <span>自动镜像</span>
              </label>
              <div className="faint">{mirrorOn ? "对侧方向自动派生，节省调用" : "各方向独立生成"}</div>
              <div className="motion-card__actions">
                <button
                  type="button"
                  className="pixel-btn pixel-btn--primary"
                  disabled={pendingCount === 0 || genLocked || prepareBusy}
                  onClick={onPrepare}
                  title={pendingCount === 0 ? "先在罗盘上点亮要生成的方向" : "生成所选方向（生成前会先弹出确认方案）"}
                >
                  {genLocked ? "生成中…" : prepareBusy ? "计算中…" : `生成所选方向（${pendingCount}）`}
                </button>
              </div>
            </div>

            {/* 右栏：罗盘点亮九宫格 */}
            <div className="motion-item__config-side">
              {m.directions.length < 8 && (
                <div className="faint">
                  当前为 {m.strategy.count} 方向动作 —— 点击上方方向模式可调整方向集（已有方向的动画会保留）
                </div>
              )}
              <div className="motion-compass" role="group" aria-label="方向点亮九宫格">
                {COMPASS_ORDER.map((slot, i) => {
                  if (slot === null) {
                    return (
                      <div key={`center${i}`} className="motion-compass__center">
                        <span className="mono">已点亮 {pendingCount}</span>
                        <span className="faint">未点亮 {unlitCount}</span>
                        {pendingCount > 0 && (
                          <button type="button" className="pixel-btn motion-compass__clear" onClick={onClearLit} title="清空全部点亮并回到自定义模式">
                            清除
                          </button>
                        )}
                      </div>
                    );
                  }
                  const d = dirOfLocal(slot);
                  const frames = framesOfLocal(slot);
                  const key = cacheKey(m.id, slot);
                  return (
                    <CompassCell
                      key={slot}
                      dir={slot}
                      dirLabel={DIR_LABEL[slot] ?? slot}
                      present={!!d}
                      frameCount={frames}
                      origin={d?.origin}
                      source={d?.source}
                      mirrorWaiting={mirrorOn && MIRROR_SLOTS.includes(slot) && frames === 0}
                      genning={activeCells.has(slot)}
                      lit={litRaw.includes(slot)}
                      thumb={thumbs[key]}
                      busy={busy !== null && (busy.startsWith(`flip:${m.id}:`) || busy.startsWith(`clear:${m.id}:`))}
                      onToggle={() => onToggleLit(slot)}
                      onOpen={() => onOpenDir(slot)}
                      onMenu={(x, y) => onMenu(slot, x, y)}
                    />
                  );
                })}
              </div>
            </div>
          </div>
        </fieldset>
      )}
    </li>
  );
}

/* ================================================================== */
/* 子组件：罗盘单元格                                                     */
/* ================================================================== */

interface CompassCellProps {
  dir: string;
  dirLabel: string;
  present: boolean;
  frameCount: number;
  origin?: string;
  source?: string;
  mirrorWaiting: boolean;
  genning: boolean;
  lit: boolean;
  thumb?: string;
  busy: boolean;
  onToggle: () => void;
  onOpen: () => void;
  onMenu: (x: number, y: number) => void;
}

function CompassCell(props: CompassCellProps) {
  const { dir, dirLabel, present, frameCount, origin, source, mirrorWaiting, genning, lit, thumb, busy, onToggle, onOpen, onMenu } = props;
  const hasFrames = frameCount > 0;
  const originTag = origin && origin !== "generated" ? ORIGIN_LABEL[origin] ?? origin : "";

  if (!present) {
    return (
      <div className="motion-compass__cell motion-compass__cell--absent" title="该动作不包含此方向（可切换方向模式补全）">
        <span className="motion-compass__dir">{dirLabel}</span>
        <span className="motion-compass__origin" />
      </div>
    );
  }

  if (mirrorWaiting) {
    const src = source ?? (MIRROR_SOURCE[dir] ?? "");
    return (
      <div
        className="motion-compass__cell motion-compass__cell--mirror-auto"
        title={`「${dirLabel}」为镜像派生槽：${src ? `由「${DIR_LABEL[src] ?? src}」生成后自动派生` : "待源方向生成后自动派生"}`}
      >
        <span className="motion-compass__dir">{dirLabel}</span>
        <span className="motion-compass__meta">镜像派生</span>
        <span className="motion-compass__origin">{src ? `← ${DIR_LABEL[src] ?? src}` : ""}</span>
      </div>
    );
  }

  if (genning) {
    return (
      <div className="motion-compass__cell motion-compass__cell--genning" title={`「${dirLabel}」正在生成…`}>
        <span className="motion-compass__dir">{dirLabel}</span>
        <span className="motion-compass__meta">
          <span className="motion-spin motion-compass__spin" aria-hidden="true" />
          生成中
        </span>
        <span className="motion-compass__origin" />
      </div>
    );
  }

  if (hasFrames) {
    return (
      <button
        type="button"
        className="motion-compass__cell motion-compass__cell--done"
        style={thumb ? { backgroundImage: `url(data:image/png;base64,${thumb})` } : undefined}
        title={`「${dirLabel}」${frameCount} 帧（${originTag || "已生成"}）—— 点击预览动画，右键更多操作`}
        onClick={onOpen}
        onContextMenu={(e) => {
          e.preventDefault();
          onMenu(e.clientX, e.clientY);
        }}
      >
        <span className="motion-compass__dir">{dirLabel}</span>
        <span className="motion-compass__meta">
          {originTag ? originTag : `${frameCount} 帧`}
          {source ? ` ← ${DIR_LABEL[source] ?? source}` : ""}
        </span>
        <span className="motion-compass__origin">{source ? "镜像" : ""}</span>
      </button>
    );
  }

  const cls = ["motion-compass__cell", lit ? "motion-compass__cell--lit" : ""].join(" ");
  return (
    <button
      type="button"
      className={cls}
      disabled={busy}
      onClick={onToggle}
      title={lit ? `已点亮「${dirLabel}」—— 点击取消` : `点亮「${dirLabel}」以生成该方向动画`}
    >
      <span className="motion-compass__dir">{dirLabel}</span>
      <span className="motion-compass__meta">{lit ? "已点亮" : "未生成"}</span>
      <span className="motion-compass__origin" />
    </button>
  );
}

/* ================================================================== */
/* 子组件：右键菜单                                                     */
/* ================================================================== */

interface ContextMenuPanelProps {
  menu: CtxMenuState;
  onClose: () => void;
  onRaw: (dir: string) => void;
  onFlip: (dir: string) => void;
  onRegen: (dir: string) => void;
  onClear: (dir: string) => void;
}

function ContextMenuPanel(props: ContextMenuPanelProps) {
  const { menu, onClose, onRaw, onFlip, onRegen, onClear } = props;
  const dirLabel = DIR_LABEL[menu.dir] ?? menu.dir;

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const item = (label: string, danger: boolean, title: string | undefined, fn: () => void) => (
    <button
      type="button"
      className={`pixel-btn motion-ctx-menu__item${danger ? " motion-ctx-menu__item--danger" : ""}`}
      onClick={fn}
      title={title}
    >
      {label}
    </button>
  );

  return (
    <>
      <div className="motion-ctx-scrim" onClick={onClose} />
      <div className="motion-ctx-menu" style={{ left: menu.x, top: menu.y }} role="menu" aria-label={`${dirLabel} 操作`}>
        {item("🔍 预览原图", false, "查看大模型返回的原始条带图", () => onRaw(menu.dir))}
        {item("⇄ 水平翻转", false, "水平翻转该格动画（镜像对同步翻转，再点一次翻回）", () => onFlip(menu.dir))}
        {item("↻ 重新生成（可带反馈）", false, "填写改进意见后重新生成该方向（强制新生成，不走缓存）", () => onRegen(menu.dir))}
        {item("删除该格动画", true, "该方向回到未生成状态，可重新点亮生成", () => onClear(menu.dir))}
      </div>
    </>
  );
}

/* ================================================================== */
/* 子组件：全屏动画预览                                                    */
/* ================================================================== */

interface PreviewOverlayProps {
  motion: MotionView;
  dir: string;
  onClose: () => void;
}

function PreviewOverlay(props: PreviewOverlayProps) {
  const { motion, dir, onClose } = props;
  const [activeDir, setActiveDir] = useState(dir);
  const [previewCache, setPreviewCache] = useState<Record<string, CandidatePreviewView>>({});
  const previewCacheRef = useRef<Record<string, CandidatePreviewView>>({});
  const [playing, setPlaying] = useState(true);
  const [loop, setLoop] = useState(motion.loop !== false);
  const [speed, setSpeed] = useState(1);
  const [frameIndex, setFrameIndex] = useState(0);
  const [stageBox, setStageBox] = useState<{ w: number; h: number } | null>(null);
  const stageRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    previewCacheRef.current = previewCache;
  }, [previewCache]);

  const dirs = motion.directions.filter((d) => d.frames.length > 0);

  // 若传入的 dir 没有帧（如来自查看按钮兜底），自动落到首个有帧方向。
  useEffect(() => {
    const hasFramesFor = (d: string) => motion.directions.some((x) => x.direction === d && x.frames.length > 0);
    setActiveDir((cur) => {
      if (hasFramesFor(cur)) return cur;
      return motion.directions.find((x) => x.frames.length > 0)?.direction ?? cur;
    });
  }, [motion]);

  useEffect(() => {
    if (dirs.length === 0) return;
    const k = cacheKey(motion.id, activeDir);
    if (previewCacheRef.current[k]) {
      setFrameIndex(0);
      setPlaying(true);
      return;
    }
    let cancelled = false;
    void fetchDirectionPreview(motion.id, activeDir)
      .then((p) => {
        if (!cancelled) {
          previewCacheRef.current[k] = p;
          setPreviewCache((prev) => ({ ...prev, [k]: p }));
        }
      })
      .catch(() => undefined);
    setFrameIndex(0);
    setPlaying(true);
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [motion.id, activeDir]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  useEffect(() => {
    const el = stageRef.current;
    if (!el) return;
    const ro = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const r = entry.contentRect;
        setStageBox({ w: r.width, h: r.height });
      }
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const preview = previewCache[cacheKey(motion.id, activeDir)] ?? null;
  const unitW = preview?.canvasWidth || 16;
  const unitH = preview?.canvasHeight || 16;
  const scale = useMemo(() => {
    if (!stageBox || stageBox.w < 8 || stageBox.h < 8) return undefined;
    const byW = Math.floor((stageBox.w - 16) / Math.max(1, unitW));
    const byH = Math.floor((stageBox.h - 16) / Math.max(1, unitH));
    return Math.max(2, Math.min(96, Math.min(byW, byH)));
  }, [stageBox, unitW, unitH]);

  const frames = useMemo(() => pixelFramesOf(preview, speed), [preview, speed]);
  const totalFrames = frames.length;
  const activeDirLabel = DIR_LABEL[activeDir] ?? activeDir;
  const d = motion.directions.find((x) => x.direction === activeDir);
  const originTag = d ? ORIGIN_LABEL[d.origin] ?? d.origin : "";
  const speedOptions = [0.5, 1, 1.5, 2];

  return (
    <div className="motion-overlay" role="dialog" aria-modal="true" aria-label={`动画预览：${motion.name}`}>
      <div className="motion-overlay__panel">
        <div className="motion-overlay__bar">
          <h3 className="mono">{motion.name} / 预览</h3>
          <span className="faint">
            {activeDirLabel} · {totalFrames} 帧
          </span>
          <button className="pixel-btn" onClick={onClose} title="关闭预览（Esc）">
            ✕ 关闭
          </button>
        </div>
        <div className="motion-overlay__body">
          <aside className="motion-overlay__side">
            <div className="motion-overlay__sub">方向</div>
            <div className="motion-preview__directions">
              {dirs.map((x) => (
                <button
                  key={x.direction}
                  type="button"
                  className={`pixel-btn${x.direction === activeDir ? " pixel-btn--primary" : ""}`}
                  onClick={() => setActiveDir(x.direction)}
                  title={`预览「${DIR_LABEL[x.direction] ?? x.direction}」（${x.frames.length} 帧）`}
                >
                  {DIR_LABEL[x.direction] ?? x.direction}
                </button>
              ))}
              {dirs.length === 0 && <div className="faint">该动作还没有已生成的格子</div>}
            </div>
          </aside>

          <div className="motion-overlay__stage">
            <div className="motion-preview__stage motion-preview__stage--large" ref={stageRef}>
              {preview && scale !== undefined ? (
                <PixelCanvas
                  unitWidth={unitW}
                  unitHeight={unitH}
                  scale={scale}
                  frames={frames}
                  playing={playing}
                  loop={loop}
                  frameIndex={frameIndex}
                  onPlaybackEnd={() => {
                    setPlaying(false);
                    setFrameIndex(0);
                  }}
                  showGrid={false}
                  showAnchors={false}
                  label={`${activeDirLabel}（${unitW}×${unitH} ×${scale}）`}
                />
              ) : (
                <div className="faint">加载预览帧…</div>
              )}
            </div>
            <div className="motion-preview__controls">
              <button className="pixel-btn" disabled={totalFrames === 0} onClick={() => setPlaying((v) => !v)} title={playing ? "暂停" : "播放"}>
                {playing ? "⏸ 暂停" : "▶ 播放"}
              </button>
              <label className="preview-scrubber mono" htmlFor="motion-preview-scrubber">
                <span>帧</span>
                <input
                  id="motion-preview-scrubber"
                  type="range"
                  min={0}
                  max={Math.max(0, totalFrames - 1)}
                  value={Math.min(frameIndex, Math.max(0, totalFrames - 1))}
                  onChange={(e) => {
                    setPlaying(false);
                    setFrameIndex(Number(e.target.value));
                  }}
                  disabled={totalFrames === 0}
                  aria-label="逐帧"
                />
                <span className="faint">
                  {totalFrames > 0 ? `${Math.min(frameIndex + 1, totalFrames)} / ${totalFrames}` : "—"}
                </span>
              </label>
              <button
                className="pixel-btn"
                disabled={totalFrames === 0}
                onClick={() => {
                  const i = speedOptions.indexOf(speed);
                  setSpeed(speedOptions[(i + 1) % speedOptions.length] ?? 1);
                }}
                title="切换回放速度"
              >
                速度 {speed}×
              </button>
              <label className="motion-switch" title={loop ? "循环播放（无缝动作）" : "播放一次（一次性动作）"}>
                <input
                  type="checkbox"
                  className="pixel-switch"
                  checked={loop}
                  onChange={(e) => setLoop(e.target.checked)}
                />
                <span className="faint">循环 {loop ? "开" : "关"}</span>
              </label>
            </div>
          </div>

          <aside className="motion-overlay__side">
            <div className="motion-overlay__sub">方向信息</div>
            {d ? (
              <div className="col">
                <div className="row">
                  <span className="faint">来源</span>
                  <span className={`status-badge status-${d.origin === "generated" ? "ok" : d.origin === "replaced" ? "warn" : "muted"} mono`}>
                    {originTag}
                  </span>
                </div>
                {d.source && (
                  <div className="row">
                    <span className="faint">镜像源</span>
                    <span>{DIR_LABEL[d.source] ?? d.source}</span>
                  </div>
                )}
                <div className="row">
                  <span className="faint">逻辑画布</span>
                  <span className="mono">
                    {unitW}×{unitH}
                  </span>
                </div>
                <div className="row">
                  <span className="faint">播放</span>
                  <span>{motion.loop === false ? "播放一次" : "循环播放"}</span>
                </div>
                <div className="row">
                  <span className="faint">帧节奏（ms）</span>
                  <span className="faint">{preview ? preview.frames.map((f) => f.durationMs || 100).join(" · ") : "—"}</span>
                </div>
              </div>
            ) : (
              <div className="faint">选择方向后显示详情</div>
            )}
          </aside>
        </div>
      </div>
    </div>
  );
}

/* ================================================================== */
/* 子组件：原始条带查看器                                                  */
/* ================================================================== */

interface RawStripViewerProps {
  motionId: string;
  dir: string;
  onClose: () => void;
}

function RawStripViewer(props: RawStripViewerProps) {
  const { motionId, dir, onClose } = props;
  const [png, setPng] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);
  const [zoom, setZoom] = useState(1);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setErr(null);
    setZoom(1);
    void fetchDirectionRawStrip(motionId, dir)
      .then((data) => {
        if (!cancelled) setPng(data);
      })
      .catch((e) => {
        if (!cancelled) setErr(String(e));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [motionId, dir]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div className="motion-raw" role="dialog" aria-modal="true" aria-label="原始条带查看">
      <div className="motion-raw__panel" style={zoom > 1 ? { overflow: "auto" } : undefined}>
        <div className="motion-raw__bar">
          <h3 className="mono">原始条带 · {DIR_LABEL[dir] ?? dir}</h3>
          <span className="faint">大模型返回、未切分的 filmstrip</span>
          <button className="pixel-btn" disabled={zoom <= 1} onClick={() => setZoom((z) => Math.max(1, z - 1))} title="缩小">
            −
          </button>
          <span className="mono faint">{zoom}×</span>
          <button className="pixel-btn" disabled={zoom >= 4} onClick={() => setZoom((z) => Math.min(4, z + 1))} title="放大">
            ＋
          </button>
          <button className="pixel-btn" onClick={onClose} title="关闭（Esc）">
            ✕ 关闭
          </button>
        </div>
        <hr className="pixel-rule" />
        {loading && <div className="empty-state">加载原图中…</div>}
        {err && <div className="error-text">{err}</div>}
        {png && (
          <img
            className="motion-raw__img"
            src={`data:image/png;base64,${png}`}
            alt={`${DIR_LABEL[dir] ?? dir} 原始条带`}
            style={{ width: `${zoom * 100}%`, maxWidth: "none" }}
          />
        )}
      </div>
    </div>
  );
}
