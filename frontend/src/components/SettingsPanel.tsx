// 设置面板 v3（齿轮入口；align-framebaker-providers 任务 5.1–5.6）：FrameBaker
// 七预设快速添加（后端 ProviderPresets 提供协议/字段说明/模型分类）、卡片字段按
// 协议切换（HTTP API / 自定义 CLI）、图片/视频/文本模型目录独立管理、"测试连接"
// 与"获取模型"使用当前未保存草稿值（零持久化）、模型列表过滤 + 能力目标点选、
// 提示词增强关联（复用已有 Provider 的文本模型，无独立凭证）、密钥遮罩与环境
// 变量回退、本地调用统计与主题。全部经由共享 application service（GUI/CLI 同源）
// —— 密钥仅本地保存，列表永不回传密钥。
import { useCallback, useEffect, useState } from "react";
import type {
  EnhanceSettingsView,
  ProviderConfigView,
  ProviderInfoView,
  ProviderPresetView,
  ProviderTestView,
  StatsView,
} from "../api/client";
import {
  addProvider,
  fetchEnhanceSettings,
  fetchProviderConfig,
  fetchProviderModelsDraft,
  fetchProviderOptions,
  fetchProviderPresets,
  fetchProviders,
  fetchProviderStats,
  removeProvider,
  saveProviderConfig,
  setActiveProvider,
  setEnhanceSettings,
  testProviderDraft,
} from "../api/client";
import { ConfirmModal } from "./ConfirmModal";
import { useTheme } from "../theme/ThemeProvider";
import type { Theme } from "../theme/ThemeProvider";
import "./SettingsPanel.css";

export interface SettingsPanelHandle {
  open: () => void;
}

type Capability = "image" | "video" | "text";

/** 每张 provider 卡片的编辑草稿（目录化 + CLI 字段，任务 5.2） */
interface Draft {
  name: string;
  apiKey: string;
  baseUrl: string;
  defaultSize: string;
  imageModels: string[];
  videoModels: string[];
  textModels: string[];
  cliCommand: string;
  cliPromptArg: string;
  cliOutputArg: string;
  cliModelArg: string;
  cliRefImageArg: string;
  cliExtraArgs: string; // 每行一个参数
  maxAttempts: string;
  timeoutSec: string;
  pricePerCall: string;
  showKey: boolean;
}

interface TestState {
  testing: boolean;
  result: ProviderTestView | null;
}

interface ModelListState {
  loading: boolean;
  models: string[] | null;
  error: string | null;
  filter: string;
  target: Capability;
}

/** 类型徽标：颜色与文案（七种协议 + 三种内置身份） */
const TYPE_BADGE: Record<string, { label: string; cls: string }> = {
  doubao: { label: "豆包", cls: "type-doubao" },
  openai: { label: "OpenAI", cls: "type-openai" },
  agnes: { label: "Agnes", cls: "type-agnes" },
  compatible: { label: "自定义兼容", cls: "type-compatible" },
  api: { label: "自定义 API", cls: "type-api" },
  dashscope: { label: "百炼", cls: "type-dashscope" },
  gemini: { label: "Gemini", cls: "type-gemini" },
  minimax: { label: "MiniMax", cls: "type-minimax" },
  volcengine: { label: "方舟/豆包", cls: "type-volcengine" },
  cli: { label: "自定义 CLI", cls: "type-cli" },
};

const CAP_LABEL: Record<Capability, string> = {
  image: "图像",
  video: "视频（预留）",
  text: "文本",
};

const toDraft = (c: ProviderConfigView): Draft => ({
  name: c.name ?? "",
  apiKey: c.apiKey ?? "",
  baseUrl: c.baseUrl ?? "",
  defaultSize: c.defaultSize ?? "",
  // 旧单模型字段 → 目录回退（后端 Effective* 同规则，前端展示保持一致）
  imageModels: c.imageModels?.length ? c.imageModels : c.model ? [c.model] : [],
  videoModels: c.videoModels?.length ? c.videoModels : c.videoModel ? [c.videoModel] : [],
  textModels: c.textModels?.length ? c.textModels : c.textModel ? [c.textModel] : [],
  cliCommand: c.cliCommand ?? "",
  cliPromptArg: c.cliPromptArg ?? "",
  cliOutputArg: c.cliOutputArg ?? "",
  cliModelArg: c.cliModelArg ?? "",
  cliRefImageArg: c.cliRefImageArg ?? "",
  cliExtraArgs: (c.cliExtraArgs ?? []).join("\n"),
  maxAttempts: String(c.maxAttempts > 0 ? c.maxAttempts : 3),
  timeoutSec: String(c.timeoutSec > 0 ? c.timeoutSec : 300),
  pricePerCall: c.pricePerCall > 0 ? String(c.pricePerCall) : "",
  showKey: false,
});

const splitModels = (text: string) =>
  text
    .split(/[,，\n]+/)
    .map((s) => s.trim())
    .filter(Boolean);

/** 提示词增强可选项：具备文本能力且目录非空的 Provider（任务 5.5） */
interface EnhanceOption {
  id: string;
  name: string;
  type: string;
  models: string[];
}

export function SettingsPanel({ handle }: { handle: SettingsPanelHandle }) {
  const { theme, setTheme } = useTheme();
  const [visible, setVisible] = useState(false);
  const [providers, setProviders] = useState<ProviderInfoView[]>([]);
  const [presets, setPresets] = useState<ProviderPresetView[]>([]);
  const [stats, setStats] = useState<StatsView | null>(null);
  const [drafts, setDrafts] = useState<Record<string, Draft>>({});
  const [tests, setTests] = useState<Record<string, TestState>>({});
  const [modelLists, setModelLists] = useState<Record<string, ModelListState>>({});
  const [busy, setBusy] = useState<Record<string, string | null>>({});
  const [removing, setRemoving] = useState<ProviderInfoView | null>(null);
  const [enhance, setEnhance] = useState<EnhanceSettingsView>({ providerId: "", model: "" });
  const [enhanceOptions, setEnhanceOptions] = useState<EnhanceOption[]>([]);
  // 人工验收反馈：卡片默认收起，需要配置时再展开（减少占位）
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const [error, setError] = useState<string | null>(null);
  const [okMsg, setOkMsg] = useState<string | null>(null);

  useEffect(() => {
    handle.open = () => setVisible(true);
  }, [handle]);

  const flash = (msg: string) => {
    setOkMsg(msg);
    window.setTimeout(() => setOkMsg(null), 2500);
  };

  const patchDraft = (id: string, patch: Partial<Draft>) =>
    setDrafts((prev) => (prev[id] ? { ...prev, [id]: { ...prev[id], ...patch } } : prev));

  const setBusyFor = (id: string, key: string | null) =>
    setBusy((prev) => ({ ...prev, [id]: key }));

  const toggleExpanded = (id: string) =>
    setExpanded((prev) => ({ ...prev, [id]: !prev[id] }));

  // 打开面板时加载 providers / 七预设 / 统计 / 编辑草稿 / 提示词增强关联
  const load = useCallback(async () => {
    try {
      const [ps, presetList, st, enhanceSettings, textOptions] = await Promise.all([
        fetchProviders(),
        fetchProviderPresets().catch(() => [] as ProviderPresetView[]),
        fetchProviderStats(),
        fetchEnhanceSettings().catch(() => ({ providerId: "", model: "" }) as EnhanceSettingsView),
        fetchProviderOptions("text").catch(() => []),
      ]);
      setProviders(ps);
      setPresets(presetList);
      setStats(st);
      setEnhance(enhanceSettings);
      setEnhanceOptions(textOptions.filter((o) => !o.reason && o.models.length > 0).map((o) => ({
        id: o.id,
        name: o.name,
        type: o.type,
        models: o.models,
      })));
      const draftsMap: Record<string, Draft> = {};
      const cfgs = await Promise.all(ps.map((p) => fetchProviderConfig(p.id).catch(() => null)));
      ps.forEach((p, i) => {
        const c = cfgs[i];
        if (c) draftsMap[p.id] = toDraft(c);
      });
      setDrafts(draftsMap);
    } catch (e) {
      setError(String(e));
    }
  }, []);

  useEffect(() => {
    if (visible) void load();
  }, [visible, load]);

  const run = async (id: string, busyKey: string, fn: () => Promise<void>) => {
    setBusyFor(id, busyKey);
    setError(null);
    try {
      await fn();
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusyFor(id, null);
    }
  };

  /** 当前草稿 → 完整配置视图（测试/取模型使用未保存值，任务 5.3） */
  const draftConfig = (info: ProviderInfoView): ProviderConfigView => {
    const d = drafts[info.id];
    if (!d) throw new Error("草稿尚未加载");
    return {
      providerId: info.id,
      type: info.type,
      name: d.name,
      apiKey: d.apiKey,
      model: d.imageModels[0] ?? "",
      videoModel: d.videoModels[0] ?? "",
      textModel: d.textModels[0] ?? "",
      imageModels: d.imageModels,
      videoModels: d.videoModels,
      textModels: d.textModels,
      baseUrl: d.baseUrl,
      defaultSize: d.defaultSize,
      maxAttempts: parseInt(d.maxAttempts, 10) || 0,
      timeoutSec: 0,
      pricePerCall: parseFloat(d.pricePerCall) || 0,
      cliCommand: d.cliCommand,
      cliPromptArg: d.cliPromptArg,
      cliOutputArg: d.cliOutputArg,
      cliModelArg: d.cliModelArg,
      cliRefImageArg: d.cliRefImageArg,
      cliExtraArgs: splitModels(d.cliExtraArgs),
    };
  };

  // ---- 卡片操作 ----

  const handleSave = (info: ProviderInfoView) =>
    run(info.id, "save", async () => {
      const d = drafts[info.id];
      if (!d) return;
      await saveProviderConfig(info.id, {
        providerId: info.id,
        type: info.type,
        name: d.name,
        apiKey: d.apiKey,
        model: d.imageModels[0] ?? "",
        videoModel: d.videoModels[0] ?? "",
        textModel: d.textModels[0] ?? "",
        imageModels: d.imageModels,
        videoModels: d.videoModels,
        textModels: d.textModels,
        baseUrl: d.baseUrl,
        defaultSize: d.defaultSize,
        maxAttempts: parseInt(d.maxAttempts, 10) || 0,
        timeoutSec: parseInt(d.timeoutSec, 10) || 0,
        pricePerCall: parseFloat(d.pricePerCall) || 0,
        cliCommand: d.cliCommand,
        cliPromptArg: d.cliPromptArg,
        cliOutputArg: d.cliOutputArg,
        cliModelArg: d.cliModelArg,
        cliRefImageArg: d.cliRefImageArg,
        cliExtraArgs: splitModels(d.cliExtraArgs),
      });
      flash("配置已保存并即时生效");
    });

  const handleActivate = (id: string) =>
    run(id, "activate", async () => {
      await setActiveProvider(id);
      flash("已切换当前 provider");
    });

  /** 测试连接：使用当前未保存草稿值（任务 5.3），不触发保存 */
  const handleTest = (info: ProviderInfoView) => {
    const id = info.id;
    if (!drafts[id]) return; // 草稿尚未加载完成时忽略点击
    setTests((prev) => ({ ...prev, [id]: { testing: true, result: null } }));
    setBusyFor(id, "test");
    testProviderDraft(draftConfig(info))
      .then((result) => {
        setTests((prev) => ({ ...prev, [id]: { testing: false, result } }));
        if (result.ok && result.models && result.models.length > 0) {
          setModelLists((prev) => ({
            ...prev,
            [id]: { ...(prev[id] ?? { filter: "", target: "image" as Capability }), loading: false, models: result.models ?? null, error: null },
          }));
        }
      })
      .catch((e) => setTests((prev) => ({ ...prev, [id]: { testing: false, result: { ok: false, error: String(e) } } })))
      .finally(() => setBusyFor(id, null));
  };

  /** 获取模型：使用当前未保存草稿值（任务 5.3），不触发保存 */
  const handleFetchModels = (info: ProviderInfoView) => {
    const id = info.id;
    if (!drafts[id]) return; // 草稿尚未加载完成时忽略点击
    setModelLists((prev) => ({
      ...prev,
      [id]: { ...(prev[id] ?? { filter: "", target: "image" as Capability }), loading: true, models: null, error: null },
    }));
    setBusyFor(id, "fetch");
    fetchProviderModelsDraft(draftConfig(info))
      .then((models) => setModelLists((prev) => ({
        ...prev,
        [id]: { ...(prev[id] ?? { filter: "", target: "image" as Capability }), loading: false, models, error: null },
      })))
      .catch((e) => setModelLists((prev) => ({
        ...prev,
        [id]: { ...(prev[id] ?? { filter: "", target: "image" as Capability }), loading: false, models: null, error: String(e) },
      })))
      .finally(() => setBusyFor(id, null));
  };

  const catalogOf = (d: Draft, cap: Capability): string[] =>
    cap === "image" ? d.imageModels : cap === "video" ? d.videoModels : d.textModels;

  const setCatalog = (id: string, cap: Capability, list: string[]) => {
    const key = cap === "image" ? { imageModels: list } : cap === "video" ? { videoModels: list } : { textModels: list };
    patchDraft(id, key as Partial<Draft>);
  };

  /** 点选模型 chip：把模型加入/移出当前能力目标目录（保留手输项，任务 5.4） */
  const toggleModel = (id: string, model: string) => {
    const d = drafts[id];
    const ml = modelLists[id];
    if (!d || !ml) return;
    const cur = catalogOf(d, ml.target);
    const next = cur.includes(model) ? cur.filter((m) => m !== model) : [...cur, model];
    setCatalog(id, ml.target, next);
  };

  const handleRemove = async () => {
    const target = removing;
    if (!target) return;
    setRemoving(null);
    await run(target.id, "remove", async () => {
      await removeProvider(target.id);
      flash(`已删除 ${target.name}`);
    });
  };

  /** 快速添加：以七预设的后端描述填充新卡片草稿（任务 5.1，协议类型显式保留） */
  const addPreset = (preset: ProviderPresetView) =>
    run("__add__", preset.key, async () => {
      const added = await addProvider({
        providerId: "",
        type: preset.type,
        name: preset.name,
        apiKey: "",
        model: preset.imageModels?.[0] ?? "",
        videoModel: preset.videoModels?.[0] ?? "",
        textModel: preset.textModels?.[0] ?? "",
        imageModels: preset.imageModels ?? [],
        videoModels: preset.videoModels ?? [],
        textModels: preset.textModels ?? [],
        baseUrl: preset.baseUrl ?? "",
        maxAttempts: 0,
        timeoutSec: 0,
        pricePerCall: 0,
        cliCommand: "",
        cliPromptArg: preset.cliPromptArg ?? "",
        cliOutputArg: preset.cliOutputArg ?? "",
        cliModelArg: preset.cliModelArg ?? "",
        cliRefImageArg: preset.cliRefImageArg ?? "",
        cliExtraArgs: [],
      });
      // 新卡自动展开，便于立刻填写密钥
      setExpanded((prev) => ({ ...prev, [added.id]: true }));
      flash(`已添加「${preset.name}」，请填写密钥后保存`);
    });

  const handleSaveEnhance = () =>
    run("__enhance__", "enhance", async () => {
      await setEnhanceSettings(enhance.providerId, enhance.model);
      flash(enhance.providerId ? "提示词增强模型已关联" : "提示词增强已改为跟随当前 Provider");
    });

  if (!visible) return null;

  const keyStatus = (p: ProviderInfoView) =>
    p.keySource === "settings"
      ? { cls: "key-ok", label: "密钥已配置" }
      : p.keySource === "env"
        ? { cls: "key-env", label: "环境变量" }
        : { cls: "key-missing", label: "未配置密钥" };

  const configuredCount = (type: string) => providers.filter((p) => p.type === type).length;

  return (
    <div className="settings-overlay">
      <div className="settings-panel pixel-panel">
        <button className="modal-close" onClick={() => setVisible(false)} aria-label="关闭设置" title="关闭">
          ✕
        </button>
        <div className="settings-panel__head">
          <h3 className="settings-panel__title">设置</h3>
        </div>

        <div className="settings-panel__body">
          {/* ===== 生成 Provider ===== */}
          <section>
            <div className="settings-sec-head">
              <h4>生成 Provider（图像 / 文本生成能力来源）</h4>
            </div>
            {/* 快速添加：后端七预设（协议标签 + 字段说明 + 配置状态） */}
            <div className="settings-presets">
              <span className="faint">快速添加：</span>
              {presets.map((p) => {
                const badge = TYPE_BADGE[p.type] ?? { label: p.type, cls: "type-compatible" };
                const n = configuredCount(p.type);
                return (
                  <button
                    key={p.key}
                    className="pixel-btn pixel-btn--small preset-btn"
                    disabled={busy["__add__"] != null}
                    title={p.description}
                    onClick={() => void addPreset(p)}
                  >
                    ＋ {p.name}
                    <span className={`preset-tag ${badge.cls}`}>{badge.label}</span>
                    {n > 0 && <span className="preset-count">已配置 {n}</span>}
                  </button>
                );
              })}
            </div>

            <div className="settings-cards">
              {providers.map((p) => {
                const d = drafts[p.id];
                const tst = tests[p.id];
                const ml = modelLists[p.id];
                const ks = keyStatus(p);
                const badge = TYPE_BADGE[p.type] ?? TYPE_BADGE.compatible;
                const b = busy[p.id];
                const isCLI = p.type === "cli";
                const isDashscope = p.type === "dashscope";
                const isOpen = !!expanded[p.id];
                const cnt = (arr?: string[]) => arr?.length ?? 0;
                return (
                  <div key={p.id} className={`provider-card${p.active ? " provider-card--active" : ""}${isOpen ? " provider-card--open" : ""}`}>
                    <div className="provider-head">
                      <button
                        className="provider-toggle"
                        onClick={() => toggleExpanded(p.id)}
                        aria-expanded={isOpen}
                        title={isOpen ? "收起配置" : "展开配置"}
                      >
                        {isOpen ? "▾" : "▸"}
                      </button>
                      <span className={`provider-type ${badge.cls}`}>{badge.label}</span>
                      {isOpen ? (
                        <input
                          className="px-input provider-name"
                          value={d?.name ?? p.name}
                          onChange={(e) => patchDraft(p.id, { name: e.target.value })}
                          aria-label="provider 名称"
                        />
                      ) : (
                        // 收起时名称为纯文本，点击即展开（避免误改）
                        <button
                          className="provider-name provider-name--static provider-name--click"
                          onClick={() => toggleExpanded(p.id)}
                          title="点击展开配置"
                        >
                          {d?.name || p.name}
                        </button>
                      )}
                      {isOpen && p.image && <span className="cap-badge">图像</span>}
                      {isOpen && p.text && <span className="cap-badge">文本</span>}
                      {!isOpen && (
                        // 收起摘要：数量为 0 的目录红色高亮（不能生成的信号）；
                        // CLI 显示命令配置状态
                        <span className="provider-summary">
                          {isCLI ? (
                            <span className={d?.cliCommand ? undefined : "sum-zero"}>
                              {d?.cliCommand ? "命令已配置" : "未配置命令"}
                            </span>
                          ) : (
                            <>
                              <span className={cnt(d?.imageModels) === 0 ? "sum-zero" : undefined}>图 {cnt(d?.imageModels)}</span>
                              <span className={cnt(d?.textModels) === 0 ? "sum-zero" : undefined}>文 {cnt(d?.textModels)}</span>
                              <span className={cnt(d?.videoModels) === 0 ? "sum-zero" : undefined}>视 {cnt(d?.videoModels)}</span>
                            </>
                          )}
                        </span>
                      )}
                      {!isCLI && <span className={`key-badge ${ks.cls}`} title={ks.label}>{ks.label}</span>}
                      {p.active && <span className="active-badge">当前</span>}
                      <span className="provider-head__spacer" />
                      {!p.active && (
                        <button className="pixel-btn pixel-btn--small" disabled={b != null} onClick={() => void handleActivate(p.id)}>
                          {b === "activate" ? "切换中…" : "设为当前"}
                        </button>
                      )}
                      {/* 保存仅在展开态出现：收起行看不到字段，不该提供误存入口 */}
                      {isOpen && (
                        <button className="pixel-btn pixel-btn--small pixel-btn--primary" disabled={b != null} onClick={() => void handleSave(p)}>
                          {b === "save" ? "保存中…" : "保存"}
                        </button>
                      )}
                      <button className="pixel-btn pixel-btn--small pixel-btn--warn" disabled={b != null} onClick={() => setRemoving(p)}>
                        删除
                      </button>
                    </div>

                    {/* 展开区：字段 / 说明 / 模型目录 / 测试（人工验收反馈：默认收起省空间） */}
                    {isOpen && (
                      <>
                        <div className="provider-fields">
                      {isCLI ? (
                        <>
                          {/* ===== 自定义 CLI 字段（argv 数组执行，不经 shell） ===== */}
                          <label className="field field--wide">
                            <span>可执行文件路径</span>
                            <input
                              className="px-input"
                              placeholder="C:\tools\my-tool.exe 或 /usr/local/bin/my-tool"
                              value={d?.cliCommand ?? ""}
                              onChange={(e) => patchDraft(p.id, { cliCommand: e.target.value })}
                            />
                          </label>
                          <label className="field">
                            <span>提示词参数（留空 = 位置参数）</span>
                            <input
                              className="px-input"
                              placeholder="--prompt"
                              value={d?.cliPromptArg ?? ""}
                              onChange={(e) => patchDraft(p.id, { cliPromptArg: e.target.value })}
                            />
                          </label>
                          <label className="field">
                            <span>输出文件参数（必填）</span>
                            <input
                              className="px-input"
                              placeholder="--output"
                              value={d?.cliOutputArg ?? ""}
                              onChange={(e) => patchDraft(p.id, { cliOutputArg: e.target.value })}
                            />
                          </label>
                          <label className="field">
                            <span>模型参数（可选）</span>
                            <input
                              className="px-input"
                              placeholder="--model"
                              value={d?.cliModelArg ?? ""}
                              onChange={(e) => patchDraft(p.id, { cliModelArg: e.target.value })}
                            />
                          </label>
                          <label className="field">
                            <span>引用图参数（可重复，留空 = 不支持引用图）</span>
                            <input
                              className="px-input"
                              placeholder="--ref"
                              value={d?.cliRefImageArg ?? ""}
                              onChange={(e) => patchDraft(p.id, { cliRefImageArg: e.target.value })}
                            />
                          </label>
                          <label className="field field--wide">
                            <span>固定额外参数（每行一个，按序原样传递）</span>
                            <textarea
                              className="px-input cli-extra"
                              rows={2}
                              placeholder={"--verbose\n--seed 42"}
                              value={d?.cliExtraArgs ?? ""}
                              onChange={(e) => patchDraft(p.id, { cliExtraArgs: e.target.value })}
                            />
                          </label>
                        </>
                      ) : (
                        <>
                          <label className="field">
                            <span>API Key</span>
                            <div className="field-key-row">
                              <input
                                className="px-input"
                                type={d?.showKey ? "text" : "password"}
                                autoComplete="off"
                                placeholder="sk-…"
                                value={d?.apiKey ?? ""}
                                onChange={(e) => patchDraft(p.id, { apiKey: e.target.value })}
                              />
                              <button
                                className="pixel-btn pixel-btn--small"
                                onClick={() => patchDraft(p.id, { showKey: !d?.showKey })}
                                title={d?.showKey ? "隐藏密钥" : "显示密钥"}
                              >
                                {d?.showKey ? "隐藏" : "显示"}
                              </button>
                            </div>
                          </label>

                          <label className="field">
                            <span>接口地址 Base URL{isDashscope ? "（原生 /compatible-mode/v1 自动识别）" : ""}</span>
                            <input
                              className="px-input"
                              placeholder="https://api.openai.com/v1"
                              value={d?.baseUrl ?? ""}
                              onChange={(e) => patchDraft(p.id, { baseUrl: e.target.value })}
                            />
                          </label>

                          <label className="field">
                            <span>默认尺寸 WxH（建议值，供生成参考）</span>
                            <input
                              className="px-input"
                              placeholder="1024x1024"
                              value={d?.defaultSize ?? ""}
                              onChange={(e) => patchDraft(p.id, { defaultSize: e.target.value })}
                            />
                          </label>
                        </>
                      )}

                      {/* ===== 模型目录：图像 / 视频（预留）/ 文本 独立输入 ===== */}
                      {(["image", "video", "text"] as Capability[]).map((cap) => (
                        <label className="field" key={cap}>
                          <span>{CAP_LABEL[cap]}模型{cap === "video" ? "（预留目录，接入前不可调用）" : ""}（逗号或换行分隔，可手输）</span>
                          <input
                            className="px-input"
                            placeholder={cap === "image" ? "doubao-seedream-4-0 / gpt-image-2…" : cap === "video" ? "例如 seedance-1-0-pro / wan2.2-t2v-flash…" : "doubao-1-5-pro-32k…"}
                            value={(d ? catalogOf(d, cap) : []).join(", ")}
                            onChange={(e) => setCatalog(p.id, cap, splitModels(e.target.value))}
                          />
                        </label>
                      ))}

                      {!isCLI && (
                        <>
                          <label className="field">
                            <span>每方向最多尝试次数</span>
                            <input
                              className="px-input num"
                              type="number"
                              min={1}
                              max={10}
                              value={d?.maxAttempts ?? "3"}
                              onChange={(e) => patchDraft(p.id, { maxAttempts: e.target.value })}
                            />
                          </label>

                          <label className="field">
                            <span>单次费用估算（{p.currency}）</span>
                            <input
                              className="px-input num"
                              type="number"
                              min={0}
                              step={0.01}
                              placeholder={p.pricePerCall > 0 ? String(p.pricePerCall) : "默认"}
                              value={d?.pricePerCall ?? ""}
                              onChange={(e) => patchDraft(p.id, { pricePerCall: e.target.value })}
                            />
                          </label>
                        </>
                      )}
                    </div>

                    <div className="provider-hints">
                      {isCLI ? (
                        <>
                          <p>执行方式：命令与参数按 argv 数组直接传给系统（不经 shell），提示词/路径中的空格与特殊字符不会破坏参数边界。</p>
                          <p>输出文件由应用创建临时路径并通过输出参数传入，生成后校验存在性与图片格式；引用图会写入临时文件并按引用图参数逐个传递。</p>
                          <p>预检：缺少命令、输出参数，或选择了引用图但未配置引用图参数时，任务在启动前即被拒绝并说明原因。</p>
                        </>
                      ) : (
                        <>
                          <p>密钥：仅本机保存；也可用环境变量 <code>OFRAME_{p.id.toUpperCase()}_API_KEY</code> 注入，留空自动读取。</p>
                          {isDashscope && <p>百炼：原生地址走通义万相/Qwen 端点；将地址改为 …/compatible-mode/v1 则按 OpenAI 兼容模式调用。</p>}
                          {p.type === "gemini" && <p>Gemini：密钥经 x-goog-api-key 请求头发送；引用图以 inlineData 按序附加。</p>}
                          {p.type === "minimax" && <p>MiniMax：单张引用图作为 subject_reference 传入；超过一张会在调用前被拒绝。</p>}
                          <p>
                            尝试次数：每个生成方向的总尝试上限（默认 3 = 首次 + 2 次重试），直接影响生成确认时的预算；费用估算仅用于预算，非实际计费。
                          </p>
                        </>
                      )}
                    </div>

                    {ml?.error && <div className="hint warn">拉取模型失败：{ml.error}（可直接手填模型名）</div>}
                    {ml?.models && !isCLI && (
                      <div className="model-chips">
                        <div className="model-chips__bar">
                          <span className="faint">点选加入</span>
                          <select
                            className="px-input cap-select"
                            value={ml.target}
                            onChange={(e) => setModelLists((prev) => ({
                              ...prev,
                              [p.id]: { ...(prev[p.id] as ModelListState), target: e.target.value as Capability },
                            }))}
                            aria-label="能力分类目标"
                          >
                            {(["image", "video", "text"] as Capability[]).map((cap) => (
                              <option key={cap} value={cap}>{CAP_LABEL[cap]}</option>
                            ))}
                          </select>
                          <input
                            className="px-input model-filter"
                            placeholder="过滤模型名…"
                            value={ml.filter}
                            onChange={(e) => setModelLists((prev) => ({
                              ...prev,
                              [p.id]: { ...(prev[p.id] as ModelListState), filter: e.target.value },
                            }))}
                          />
                        </div>
                        <div className="model-chips__list">
                          {ml.models
                            .filter((m) => m.toLowerCase().includes(ml.filter.toLowerCase()))
                            .map((m) => (
                              <button
                                key={m}
                                className={`model-chip${(d ? catalogOf(d, ml.target) : []).includes(m) ? " active" : ""}`}
                                onClick={() => toggleModel(p.id, m)}
                                title={ml.target === "video" ? "视频目录为预留配置" : `加入/移出${CAP_LABEL[ml.target]}目录`}
                              >
                                {m}
                              </button>
                            ))}
                        </div>
                      </div>
                    )}

                    <div className="provider-test">
                      {!isCLI ? (
                        <>
                          <button className="pixel-btn pixel-btn--small" disabled={tst?.testing || b === "test"} onClick={() => handleTest(p)}>
                            {tst?.testing || b === "test" ? "测试中…" : "测试连接"}
                          </button>
                          <button
                            className="pixel-btn pixel-btn--small"
                            disabled={ml?.loading || b === "fetch"}
                            onClick={() => void handleFetchModels(p)}
                            title="用当前表单值从接口拉取模型列表"
                          >
                            {ml?.loading || b === "fetch" ? "拉取中…" : "获取模型"}
                          </button>
                          {tst?.result && (
                            <span className={`test-result ${tst.result.ok ? "ok" : "bad"}`}>
                              {tst.result.ok
                                ? `✓ 连通正常 · ${tst.result.latencyMs}ms${tst.result.models && tst.result.models.length > 0 ? ` · 发现 ${tst.result.models.length} 个模型` : ""}`
                                : `✗ ${tst.result.error || "测试失败"}`}
                            </span>
                          )}
                          {tst?.result?.ok && !d?.apiKey && <span className="test-result ok">（当前密钥来自环境变量）</span>}
                        </>
                      ) : (
                        <span className="faint">本地 CLI 无需连接测试；执行失败（未找到 / 退出码 / 输出缺失 / 格式错误）会在生成结果中报告原因。</span>
                      )}
                    </div>
                      </>
                    )}
                  </div>
                );
              })}
            </div>
          </section>

          {/* ===== 提示词增强关联（任务 5.5：复用已有 Provider 的文本模型） ===== */}
          <section>
            <div className="settings-sec-head">
              <h4>提示词增强模型</h4>
              <span className="faint">关联已有 Provider 的文本模型，无需独立凭证</span>
            </div>
            <p className="settings-hint">
              留空 Provider 表示跟随当前生成 Provider 的文本模型。关联在对应 Provider 被删除后自动失效回退。
            </p>
            <div className="enhance-row">
              <select
                className="px-input"
                value={enhance.providerId}
                onChange={(e) => {
                  // 切换 provider 后模型清空（后端保存时按目录校验，默认取目录第一个）
                  setEnhance({ providerId: e.target.value, model: "" });
                }}
                aria-label="提示词增强 Provider"
              >
                <option value="">跟随当前 Provider</option>
                {enhanceOptions.map((o) => (
                  <option key={o.id} value={o.id}>{o.name}（{o.type}）</option>
                ))}
              </select>
              <select
                className="px-input"
                value={enhance.model}
                onChange={(e) => setEnhance((prev) => ({ ...prev, model: e.target.value }))}
                disabled={!enhance.providerId}
                aria-label="提示词增强文本模型"
              >
                <option value="">默认（目录第一个）</option>
                {(enhanceOptions.find((o) => o.id === enhance.providerId)?.models ?? []).map((m) => (
                  <option key={m} value={m}>{m}</option>
                ))}
              </select>
              <button className="pixel-btn pixel-btn--small pixel-btn--primary" disabled={busy["__enhance__"] != null} onClick={() => void handleSaveEnhance()}>
                {busy["__enhance__"] === "enhance" ? "保存中…" : "保存关联"}
              </button>
            </div>
          </section>

          {/* ===== 本地调用统计 ===== */}
          <section>
            <h4>本地调用统计（次数 / 费用估算）</h4>
            {stats && stats.items.length === 0 ? (
              <div className="empty-state">暂无调用记录 — 生成确认执行后自动记录</div>
            ) : (
              <ul className="settings-stats">
                {stats?.items.map((s) => (
                  <li key={`${s.providerId}-${s.model}`}>
                    <span className="stat-provider">{s.providerId}</span> · {s.model} · {s.callCount} 次 · 估算{" "}
                    {s.estimatedCost.toFixed(4)} {s.currency}
                  </li>
                ))}
              </ul>
            )}
          </section>

          {/* ===== 外观主题 ===== */}
          <section>
            <h4>外观主题</h4>
            <div className="row">
              {(
                [
                  { id: "warm-white", label: "暖白 WARM" },
                  { id: "dark-ink", label: "深墨 DARK" },
                ] as const
              ).map((t) => (
                <button
                  key={t.id}
                  className={`pixel-btn${theme === t.id ? " pixel-btn--primary" : ""}`}
                  onClick={() => setTheme(t.id as Theme)}
                  aria-pressed={theme === t.id}
                >
                  {t.label}
                </button>
              ))}
            </div>
            <div className="faint">主题即时生效并本地持久化</div>
          </section>

          {error && <div className="error-text">{error}</div>}
          {okMsg && <div className="status-ok">{okMsg}</div>}
        </div>
      </div>

      {removing && (
        <ConfirmModal
          open={removing != null}
          title="删除 Provider"
          message={`确定删除「${removing.name}」吗？删除后其配置将从本机移除；若它是当前 provider，将自动切换到剩余的第一个 Provider（无剩余则清空，生成前需重新配置）。内置身份（豆包/OpenAI/Agnes）删除后可随时重新添加。`}
          confirmLabel="删除"
          danger
          onCancel={() => setRemoving(null)}
          onConfirm={() => void handleRemove()}
        />
      )}
    </div>
  );
}
