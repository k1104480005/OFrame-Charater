// 设置面板（齿轮入口）：provider 选择、密钥/模型配置与验证、本地调用统计
// （workbench-ui spec 10.6 的前置子集 + generation spec 4.6）。全部经由共享
// application service（GUI/CLI 同源）—— 密钥仅本地保存，列表永不回传密钥。
import { useCallback, useEffect, useState } from "react";
import type { ProviderConfigView, ProviderInfoView, StatsView } from "../api/client";
import {
  fetchProviderConfig,
  fetchProviders,
  fetchProviderStats,
  saveProviderConfig,
  setActiveProvider,
  validateProvider,
} from "../api/client";
import { useTheme } from "../theme/ThemeProvider";
import type { Theme } from "../theme/ThemeProvider";
import "./SettingsPanel.css";

export interface SettingsPanelHandle {
  open: () => void;
}

const KEY_SOURCE_LABEL: Record<string, string> = {
  settings: "本地配置",
  env: "环境变量",
  none: "未配置",
};

export function SettingsPanel({ handle }: { handle: SettingsPanelHandle }) {
  const { theme, setTheme } = useTheme();
  const [visible, setVisible] = useState(false);
  const [providers, setProviders] = useState<ProviderInfoView[]>([]);
  const [stats, setStats] = useState<StatsView | null>(null);
  const [activeId, setActiveId] = useState("");
  const [cfg, setCfg] = useState<ProviderConfigView | null>(null);
  const [key, setKey] = useState("");
  const [model, setModel] = useState("");
  const [textModel, setTextModel] = useState("");
  const [maxAttempts, setMaxAttempts] = useState("3");
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [okMsg, setOkMsg] = useState<string | null>(null);

  useEffect(() => {
    handle.open = () => setVisible(true);
  }, [handle]);

  const load = useCallback(async () => {
    try {
      const [ps, st] = await Promise.all([fetchProviders(), fetchProviderStats()]);
      setProviders(ps);
      setStats(st);
      const active = ps.find((p) => p.active);
      setActiveId(active?.id ?? ps[0]?.id ?? "");
      await select(active?.id ?? ps[0]?.id ?? "");
    } catch (e) {
      setError(String(e));
    }
  }, []);

  const select = async (id: string) => {
    setActiveId(id);
    try {
      const c = await fetchProviderConfig(id);
      setCfg(c);
      setKey(c.apiKey ?? "");
      setModel(c.model ?? "");
      setTextModel(c.textModel ?? "");
      setMaxAttempts(String(c.maxAttempts > 0 ? c.maxAttempts : 3));
    } catch (e) {
      setError(String(e));
    }
  };

  const flash = (msg: string) => {
    setOkMsg(msg);
    window.setTimeout(() => setOkMsg(null), 2500);
  };

  const run = async (keyName: string, fn: () => Promise<void>) => {
    setBusy(keyName);
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

  const handleSave = () =>
    run("save", async () => {
      if (!cfg) return;
      await saveProviderConfig(cfg.providerId, {
        ...cfg,
        apiKey: key,
        model,
        textModel,
        maxAttempts: parseInt(maxAttempts, 10) || 0,
      });
      flash("配置已保存并即时生效");
    });

  const handleValidate = () =>
    run("validate", async () => {
      if (!cfg) return;
      await validateProvider(cfg.providerId);
      flash("配置验证通过（离线校验）");
    });

  const handleActivate = () =>
    run("activate", async () => {
      await setActiveProvider(activeId);
      flash("已切换当前 provider");
    });

  if (!visible) return null;

  return (
    <div className="settings-overlay" onClick={() => setVisible(false)}>
      <div className="settings-panel pixel-panel" onClick={(e) => e.stopPropagation()}>
        <div className="settings-panel__head">
          <h3 className="mono panel-heading">设置 / SETTINGS</h3>
          <button className="pixel-btn" onClick={() => setVisible(false)} aria-label="关闭设置">
            关闭
          </button>
        </div>

        <section>
          <h4 className="mono">Provider 选择</h4>
          <div className="row">
            <select value={activeId} onChange={(e) => void select(e.target.value)} aria-label="当前 provider">
              {providers.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name} {p.active ? "（当前）" : ""}
                </option>
              ))}
            </select>
            <button className="pixel-btn" disabled={busy === "activate"} onClick={() => void handleActivate()}>
              {busy === "activate" ? "切换中…" : "设为当前"}
            </button>
          </div>
          <ul className="settings-providers">
            {providers.map((p) => (
              <li key={p.id} className={`mono${p.active ? " settings-providers__active" : ""}`}>
                {p.id} · {p.name} · 模型 {p.imageModel} · 密钥：
                <span className={p.hasApiKey ? "status-ok" : "faint"}>{KEY_SOURCE_LABEL[p.keySource] ?? p.keySource}</span>
                · 每方向最多 {p.maxAttempts} 次尝试
              </li>
            ))}
          </ul>
        </section>

        <section>
          <h4 className="mono">密钥 / 模型配置（本地保存）</h4>
          {cfg && (
            <div className="col settings-config">
              <label className="faint">API Key（本地明文存储，仅本机可读）</label>
              <input type="password" value={key} onChange={(e) => setKey(e.target.value)} placeholder="sk-…" aria-label="API Key" />
              <label className="faint">图像模型</label>
              <input value={model} onChange={(e) => setModel(e.target.value)} aria-label="图像模型" />
              <label className="faint">文本模型（Doubao）</label>
              <input value={textModel} onChange={(e) => setTextModel(e.target.value)} aria-label="文本模型" />
              <label className="faint">每方向最多尝试次数（生成确认预算）</label>
              <input value={maxAttempts} onChange={(e) => setMaxAttempts(e.target.value)} aria-label="最大尝试次数" />
              <div className="row">
                <button className="pixel-btn pixel-btn--primary" disabled={busy === "save"} onClick={() => void handleSave()}>
                  {busy === "save" ? "保存中…" : "保存配置"}
                </button>
                <button className="pixel-btn" disabled={busy === "validate"} onClick={() => void handleValidate()}>
                  {busy === "validate" ? "校验中…" : "离线校验"}
                </button>
              </div>
            </div>
          )}
        </section>

        <section>
          <h4 className="mono">本地调用统计（次数 / 费用估算）</h4>
          {stats && stats.items.length === 0 ? (
            <div className="empty-state">暂无调用记录</div>
          ) : (
            <ul className="mono settings-stats">
              {stats?.items.map((s) => (
                <li key={`${s.providerId}-${s.model}`}>
                  {s.providerId} · {s.model} · {s.callCount} 次 · 估算 {s.estimatedCost.toFixed(4)} {s.currency}
                </li>
              ))}
            </ul>
          )}
        </section>

        <section>
          <h4 className="mono">外观主题</h4>
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
                <span className="mono">{t.label}</span>
              </button>
            ))}
          </div>
          <div className="faint">主题即时生效并本地持久化（spec 10.8 暖白/深墨双主题）</div>
        </section>

        {error && <div className="error-text">{error}</div>}
        {okMsg && <div className="status-ok">{okMsg}</div>}
      </div>
    </div>
  );
}
