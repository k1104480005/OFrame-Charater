// 启动页 v2：右上角「创建/工作区」按钮 + 弹窗；身份包卡片网格；
// 卡片可拖到左侧分类改分类；分类右键删除（其下包自动归「未分类」）。
import { useEffect, useMemo, useRef, useState } from "react";
import type { DragEvent, FormEvent, MouseEvent } from "react";
import {
  deletePackage,
  fetchAppInfo,
  pickWorkspaceDir,
  renameIdentity,
  setPackageCategory,
} from "../api/client";
import { useSession } from "../state/SessionContext";
import { useTheme } from "../theme/ThemeProvider";
import { ConfirmModal } from "../components/ConfirmModal";
import { SettingsPanel } from "../components/SettingsPanel";
import type { SettingsPanelHandle } from "../components/SettingsPanel";
import "./LaunchPage.css";

/** 未分类的语义值（空字符串） */
const UNCATEGORIZED = "";
/** 「全部」分类的语义值 */
const ALL = "all";

/** 卡片排序方式：updated = 最近修改优先（默认）| created = 最近创建优先 | name = 名称 A→Z */
type SortMode = "updated" | "created" | "name";

const SORT_MODES: Array<{ value: SortMode; label: string }> = [
  { value: "updated", label: "最近修改优先" },
  { value: "created", label: "最近创建优先" },
  { value: "name", label: "名称 A→Z" },
];

/** 排序偏好持久化（跨启动记住用户选择） */
const SORT_STORAGE_KEY = "launch.sortMode";

function loadSortMode(): SortMode {
  try {
    const v = localStorage.getItem(SORT_STORAGE_KEY);
    if (v === "updated" || v === "created" || v === "name") return v;
  } catch {
    /* localStorage 不可用时使用默认值 */
  }
  return "updated";
}

/** 把 RFC3339 时间戳格式化为「MM-DD HH:mm」（本地时区） */
function fmtTime(iso?: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  const p = (n: number) => String(n).padStart(2, "0");
  return `${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

interface CtxMenuState {
  x: number;
  y: number;
  category: string;
}

export function LaunchPage() {
  const { workspace, packages, loading, error, open, create, setError, switchWorkspace, migrateWorkspace, refreshList } = useSession();
  const [busy, setBusy] = useState<string | null>(null);
  const [msg, setMsg] = useState<string | null>(null);
  const settingsHandle = useRef<SettingsPanelHandle>({ open: () => undefined });

  // 弹窗可见性
  const [createOpen, setCreateOpen] = useState(false);
  const [workspaceOpen, setWorkspaceOpen] = useState(false);
  const [createName, setCreateName] = useState("");
  const [createCategory, setCreateCategory] = useState(UNCATEGORIZED);
  const [workspacePath, setWorkspacePath] = useState<string | null>(null);

  // 分类
  const [category, setCategory] = useState<string>(ALL); // "all" = 全部 | "" = 未分类 | 分类名
  const [sortMode, setSortMode] = useState<SortMode>(loadSortMode);
  const [newCategory, setNewCategory] = useState("");
  const [extraCategories, setExtraCategories] = useState<string[]>([]); // 空分类（尚无包）
  const [ctxMenu, setCtxMenu] = useState<CtxMenuState | null>(null);
  const [deleteCatTarget, setDeleteCatTarget] = useState<string | null>(null);
  const [dragOverCat, setDragOverCat] = useState<string | null>(null);
  const [catMenuFor, setCatMenuFor] = useState<string | null>(null); // 卡片分类下拉当前打开的包路径
  const [createCatOpen, setCreateCatOpen] = useState(false); // 创建弹窗分类下拉
  const createCatRef = useRef<HTMLDivElement>(null); // 创建弹窗分类按钮容器（用于 fixed 下拉定位）
  const [createCatPos, setCreateCatPos] = useState<{ left: number; top: number; width: number } | null>(null);

  // 改名 / 删除
  const [renamePath, setRenamePath] = useState<string | null>(null);
  const [renameDraft, setRenameDraft] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<{ path: string; name: string } | null>(null);
  const [switchAsk, setSwitchAsk] = useState<string | null>(null); // 切换工作区是否移动包

  const { theme } = useTheme();
  const [appInfoText, setAppInfoText] = useState("");

  useEffect(() => {
    void fetchAppInfo().then((info) => {
      setAppInfoText(`OFrame Character Workbench v${info.version} · manifest format v${info.formatVersion} · ${info.go}`);
    });
  }, []);

  // 文件选择（工作区弹窗）
  const handleBrowse = async () => {
    const picked = await pickWorkspaceDir();
    if (picked) setWorkspacePath(picked);
  };

  // 分类列表（含计数）：「全部」→「未分类」→ 已使用分类 + 用户新建空分类
  const categories = useMemo(() => {
    const counts = new Map<string, number>();
    let uncategorized = 0;
    for (const p of packages) {
      const c = (p.category ?? "").trim();
      if (c) counts.set(c, (counts.get(c) ?? 0) + 1);
      else uncategorized++;
    }
    for (const c of extraCategories) {
      if (!counts.has(c)) counts.set(c, 0);
    }
    const rest = Array.from(counts.entries()).sort((a, b) => a[0].localeCompare(b[0], "zh-CN"));
    return [
      { name: ALL, count: packages.length },
      { name: UNCATEGORIZED, count: uncategorized },
      ...rest.map(([name, count]) => ({ name, count })),
    ];
  }, [packages, extraCategories]);

  const filtered =
    category === ALL
      ? packages
      : category === UNCATEGORIZED
        ? packages.filter((p) => !(p.category ?? "").trim())
        : packages.filter((p) => (p.category ?? "").trim() === category);

  // 卡片排序：时间戳无效时按 0 处理（排最后），保证比较结果确定。
  const sorted = useMemo(() => {
    const time = (iso?: string) => {
      const t = iso ? new Date(iso).getTime() : NaN;
      return Number.isNaN(t) ? 0 : t;
    };
    const list = [...filtered];
    switch (sortMode) {
      case "created":
        list.sort((a, b) => time(b.createdAt) - time(a.createdAt));
        break;
      case "name":
        list.sort((a, b) => a.name.localeCompare(b.name, "zh-CN"));
        break;
      case "updated":
      default:
        list.sort((a, b) => time(b.updatedAt) - time(a.updatedAt));
        break;
    }
    return list;
  }, [filtered, sortMode]);

  const changeSortMode = (mode: SortMode) => {
    setSortMode(mode);
    try {
      localStorage.setItem(SORT_STORAGE_KEY, mode);
    } catch {
      /* 忽略持久化失败 */
    }
  };

  const flash = (m: string) => {
    setMsg(m);
    window.setTimeout(() => setMsg(null), 4000);
  };

  // --- 分类操作 ---
  const addCategory = () => {
    const c = newCategory.trim();
    if (!c) return;
    setExtraCategories((prev) => (prev.includes(c) ? prev : [...prev, c]));
    setNewCategory("");
    setCategory(c);
  };

  const changeCategory = async (path: string, cat: string) => {
    setBusy(`cat:${path}`);
    try {
      await setPackageCategory(path, cat);
      setError(null);
      await refreshList();
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy(null);
    }
  };

  // 卡片拖到分类芯片 → 改分类
  const handleCardDragStart = (e: DragEvent, path: string) => {
    e.dataTransfer.setData("text/plain", path);
    e.dataTransfer.effectAllowed = "move";
  };

  const handleCatDrop = (e: DragEvent, cat: string) => {
    e.preventDefault();
    setDragOverCat(null);
    const path = e.dataTransfer.getData("text/plain");
    if (path) void changeCategory(path, cat);
  };

  // 右键分类 → 删除（其下包自动归「未分类」）
  const openCatMenu = (e: MouseEvent, cat: string) => {
    e.preventDefault();
    setCtxMenu({ x: e.clientX, y: e.clientY, category: cat });
  };

  useEffect(() => {
    if (!ctxMenu) return;
    const close = () => setCtxMenu(null);
    window.addEventListener("click", close);
    window.addEventListener("keydown", (e) => {
      if (e.key === "Escape") close();
    });
    return () => window.removeEventListener("click", close);
  }, [ctxMenu]);

  // 卡片分类下拉：点击外部 / Esc 关闭
  useEffect(() => {
    if (!catMenuFor) return;
    const close = () => setCatMenuFor(null);
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") close();
    };
    window.addEventListener("click", close);
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("click", close);
      window.removeEventListener("keydown", onKey);
    };
  }, [catMenuFor]);

  // 创建弹窗分类下拉：点击外部 / Esc / 滚动 关闭
  useEffect(() => {
    if (!createCatOpen) return;
    const close = () => setCreateCatOpen(false);
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") close();
    };
    window.addEventListener("click", close);
    window.addEventListener("keydown", onKey);
    window.addEventListener("scroll", close, true); // 页面/弹窗滚动时收起，避免 fixed 菜单错位
    return () => {
      window.removeEventListener("click", close);
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("scroll", close, true);
    };
  }, [createCatOpen]);

  const deleteCategory = async (cat: string) => {
    const affected = packages.filter((p) => (p.category ?? "").trim() === cat);
    setBusy("cat-clear");
    try {
      for (const p of affected) {
        await setPackageCategory(p.path, UNCATEGORIZED);
      }
      setExtraCategories((prev) => prev.filter((c) => c !== cat));
      if (category === cat) setCategory(ALL);
      flash(`已删除分类「${cat}」，${affected.length} 个身份包已归入未分类`);
      setError(null);
      await refreshList();
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy(null);
    }
  };

  // --- 创建 / 工作区 ---
  const handleCreate = async (e: FormEvent) => {
    e.preventDefault();
    const trimmed = createName.trim();
    if (!trimmed) return;
    setBusy("create");
    try {
      await create(trimmed, createCategory);
      setCreateName("");
      setCreateOpen(false);
      setError(null);
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy(null);
    }
  };

  const doSwitch = async (path: string, moved: boolean) => {
    setBusy("workspace");
    try {
      const info = moved ? await migrateWorkspace(path, true) : await switchWorkspace(path);
      setWorkspacePath(info.path);
      setWorkspaceOpen(false);
      flash(moved ? `已移动 ${packages.length} 个身份包并切换工作区：${info.path}` : `已切换工作区：${info.path}`);
      setError(null);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  };

  const handleSwitchClick = () => {
    if (!workspacePath || workspacePath === workspace?.path) return;
    if (packages.length > 0) {
      setSwitchAsk(workspacePath);
      return;
    }
    void doSwitch(workspacePath, false);
  };

  // --- 打开 / 改名 / 删除 ---
  const handleOpen = async (path: string) => {
    setBusy(path);
    try {
      await open(path);
      setError(null);
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy(null);
    }
  };

  const startRename = (path: string, current: string) => {
    setRenamePath(path);
    setRenameDraft(current);
  };

  const saveRename = async () => {
    const name = renameDraft.trim();
    if (!renamePath || !name) return;
    setBusy("rename");
    try {
      await renameIdentity(renamePath, name);
      setRenamePath(null);
      flash(`已重命名：${name}`);
      setError(null);
      await refreshList();
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy(null);
    }
  };

  const doDelete = async (path: string, name: string) => {
    setBusy(`delete:${path}`);
    try {
      await deletePackage(path);
      flash(`已删除：${name}（已移到工作区回收站）`);
      setError(null);
      await refreshList();
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy(null);
    }
  };

  const emptyHint =
    packages.length === 0
      ? "工作区中还没有身份包 —— 点右上角「＋ 创建身份包」开始"
      : category === UNCATEGORIZED
        ? "未分类中没有身份包 —— 把卡片拖到左侧分类，或新建一个"
        : "该分类下还没有身份包 —— 把卡片拖到这里，或点右上角创建";
  return (
    <div className="launch">
      <header className="launch__header">
        <div className="launch__title">
          <h1 className="launch__logo">OFrame Character</h1>
          <div className="launch__subtitle">角色动画资产工作台</div>
        </div>
        <div className="launch__header-actions">
          <button className="pixel-btn pixel-btn--primary" onClick={() => setCreateOpen(true)}>
            ＋ 创建身份包
          </button>
          <button className="pixel-btn" onClick={() => setWorkspaceOpen(true)}>
            切换工作区
          </button>
          <button className="pixel-btn" onClick={() => settingsHandle.current.open()} aria-label="打开设置" title="设置：provider / 密钥 / 统计">
            设置
          </button>
        </div>
      </header>

      <main className="launch__main">
        {/* 左侧：分类栏 */}
        <aside className="launch__side pixel-panel">
          <h2 className="mono launch__panel-title">分类 / CATEGORIES</h2>
          <hr className="pixel-rule" />
          <ul className="launch__cats">
            {categories.map(({ name, count }) => {
              const label = name === ALL ? "全部" : name === UNCATEGORIZED ? "未分类" : name;
              return (
                <li key={name}>
                  <button
                    className={`pixel-btn launch__cat${category === name ? " launch__cat--active" : ""}${dragOverCat === name ? " launch__cat--drop" : ""}`}
                    onClick={() => setCategory(name)}
                    onContextMenu={(e) => (name === ALL || name === UNCATEGORIZED ? undefined : openCatMenu(e, name))}
                    onDragOver={(e) => {
                      if (name === ALL) return;
                      e.preventDefault();
                      setDragOverCat(name);
                    }}
                    onDragLeave={() => setDragOverCat((v) => (v === name ? null : v))}
                    onDrop={(e) => (name === ALL ? undefined : handleCatDrop(e, name))}
                    title={
                      name === ALL
                        ? "全部身份包"
                        : name === UNCATEGORIZED
                          ? "未分类（默认）"
                          : `右键删除分类 · 拖卡片到此改分类`
                    }
                  >
                    <span className="launch__cat-name">{label}</span>
                    <span className="faint">({count})</span>
                  </button>
                </li>
              );
            })}
          </ul>
          <div className="row launch__newcat">
            <input
              className="grow"
              value={newCategory}
              onChange={(e) => setNewCategory(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") addCategory();
              }}
              placeholder="新建分类"
              aria-label="新建分类"
              maxLength={32}
            />
            <button className="pixel-btn" onClick={addCategory} disabled={!newCategory.trim()} title="添加分类">
              ＋
            </button>
          </div>
        </aside>

        {/* 右侧：画廊（带区域线框） */}
        <div className="launch__content">
          <div className="launch__gallery pixel-panel">
            <div className="launch__toolbar">
              <span className="mono launch__section-title">
                {category === ALL ? "全部" : category === UNCATEGORIZED ? "未分类" : category} · {filtered.length} 个身份包
              </span>
              {msg && <span className="status-ok mono">{msg}</span>}
              <label className="launch__sort">
                <span className="faint mono">排序</span>
                <select
                  value={sortMode}
                  onChange={(e) => changeSortMode(e.target.value as SortMode)}
                  aria-label="身份包排序方式"
                  title="身份包卡片排序方式"
                >
                  {SORT_MODES.map((m) => (
                    <option key={m.value} value={m.value}>
                      {m.label}
                    </option>
                  ))}
                </select>
              </label>
            </div>

            {loading ? (
              <div className="muted">加载工作区…</div>
            ) : filtered.length === 0 ? (
              <div className="empty-state">{emptyHint}</div>
            ) : (
              <ul className="launch__grid">
                {sorted.map((p) => (
                  <li key={p.path} className="launch__card" draggable onDragStart={(e) => handleCardDragStart(e, p.path)}>
                    <button
                      className="pixel-btn pixel-btn--warn launch__card-del"
                      disabled={busy === `delete:${p.path}`}
                      onClick={() => setDeleteTarget({ path: p.path, name: p.name })}
                      title="删除（移到回收站）"
                      aria-label={`删除 ${p.name}`}
                    >
                      ✕
                    </button>
                    <div className="launch__thumb checker-bg">
                      {p.baseCharacterThumb ? (
                        <img
                          className="launch__thumb-img"
                          src={`data:image/png;base64,${p.baseCharacterThumb}`}
                          alt={`${p.name} 身份基准缩略图`}
                          draggable={false}
                          loading="lazy"
                          title="已采用的身份基准"
                        />
                      ) : (
                        p.name.trim().charAt(0).toUpperCase() || "?"
                      )}
                    </div>
                    {renamePath === p.path ? (
                      <input
                        className="pixel-input launch__rename-input"
                        value={renameDraft}
                        onChange={(e) => setRenameDraft(e.target.value)}
                        onKeyDown={(e) => {
                          if (e.key === "Enter") void saveRename();
                          if (e.key === "Escape") setRenamePath(null);
                        }}
                        onBlur={() => {
                          if (renamePath === p.path) void saveRename();
                        }}
                        autoFocus
                        maxLength={64}
                        aria-label="新名称"
                      />
                    ) : (
                      <span className="launch__card-name launch__renameable" onClick={() => startRename(p.path, p.name)} title="点击改名 · 拖动卡片改分类">
                        {p.name}
                      </span>
                    )}
                    <span className="mono launch__item-meta">
                      角色来源：{p.baseCharacterSource === "ai" ? "AI 生成" : p.baseCharacterSource === "import" ? "本地导入" : "未选择"}
                    </span>
                    <span className="mono launch__item-meta">
                      版本 {p.currentVersion} · 格式 v{p.formatVersion}
                    </span>
                    <span className="mono launch__item-meta">
                      创建 {fmtTime(p.createdAt)} · 修改 {fmtTime(p.updatedAt)}
                    </span>
                    <div className="launch__catwrap" onClick={(e) => e.stopPropagation()}>
                      <button
                        className="pixel-btn launch__catbtn"
                        onClick={() => setCatMenuFor((cur) => (cur === p.path ? null : p.path))}
                        disabled={busy === `cat:${p.path}`}
                        aria-label="设置分类"
                        aria-haspopup="listbox"
                        aria-expanded={catMenuFor === p.path}
                      >
                        <span className="launch__catbtn-label">{(p.category ?? "").trim() || "未分类"}</span>
                        <span className="launch__catbtn-arrow" aria-hidden="true">
                          ▾
                        </span>
                      </button>
                      {catMenuFor === p.path && (
                        <div className="launch__catmenu pixel-panel" role="listbox">
                          {[
                            UNCATEGORIZED,
                            ...categories
                              .filter((c) => c.name !== ALL && c.name !== UNCATEGORIZED)
                              .map((c) => c.name),
                          ].map((cat) => (
                            <button
                              key={cat || "__uncat__"}
                              role="option"
                              aria-selected={(p.category ?? "") === cat}
                              className={`pixel-btn launch__catmenu-item${(p.category ?? "") === cat ? " pixel-btn--primary" : ""}`}
                              onClick={() => {
                                setCatMenuFor(null);
                                void changeCategory(p.path, cat);
                              }}
                            >
                              {cat === UNCATEGORIZED ? "未分类" : cat}
                            </button>
                          ))}
                        </div>
                      )}
                    </div>
                    <button className="pixel-btn pixel-btn--primary launch__card-open" disabled={busy === p.path} onClick={() => void handleOpen(p.path)}>
                      {busy === p.path ? "打开中…" : "打开"}
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
      </main>

      <footer className="launch__footer faint mono" data-theme={theme}>
        {appInfoText || "…"}
      </footer>

      {/* 创建身份包弹窗（右上角红色 ✕ 关闭，禁止点击遮罩关闭） */}
      {createOpen && (
        <div className="modal-scrim">
          <div className="pixel-panel launch-modal">
            <button type="button" className="modal-close" onClick={() => setCreateOpen(false)} aria-label="关闭创建弹窗" title="关闭">
              ✕
            </button>
            <h3 className="mono launch-modal__title">创建身份包</h3>
            <hr className="pixel-rule" />
            <form onSubmit={handleCreate} className="col">
              <input
                value={createName}
                onChange={(e) => setCreateName(e.target.value)}
                placeholder="身份包名称（如 Hero）"
                aria-label="身份包名称"
                maxLength={64}
                autoFocus
              />
              <div className="launch__catwrap" onClick={(e) => e.stopPropagation()} ref={createCatRef}>
                <button
                  type="button"
                  className="pixel-btn launch__catbtn"
                  onClick={() => {
                    const el = createCatRef.current;
                    if (el) {
                      const r = el.getBoundingClientRect();
                      const h = 224; // 下拉最大高度（约 6 项），超出内部滚动
                      let top = r.bottom + 6;
                      if (top + h > window.innerHeight - 8) top = Math.max(8, r.top - h - 6); // 底部放不下则向上弹
                      setCreateCatPos({ left: r.left, top, width: r.width });
                    }
                    setCreateCatOpen((v) => !v);
                  }}
                  aria-haspopup="listbox"
                  aria-expanded={createCatOpen}
                >
                  <span className="launch__catbtn-label">{createCategory || "未分类"}</span>
                  <span className="launch__catbtn-arrow" aria-hidden="true">
                    ▾
                  </span>
                </button>
                {createCatOpen && createCatPos && (
                  <div
                    className="launch__catmenu launch__catmenu--fixed pixel-panel"
                    role="listbox"
                    style={{ left: createCatPos.left, top: createCatPos.top, width: createCatPos.width }}
                  >
                    {[
                      UNCATEGORIZED,
                      ...categories
                        .filter((c) => c.name !== ALL && c.name !== UNCATEGORIZED)
                        .map((c) => c.name),
                    ].map((cat) => (
                      <button
                        key={cat || "__uncat__"}
                        type="button"
                        role="option"
                        aria-selected={createCategory === cat}
                        className={`pixel-btn launch__catmenu-item${createCategory === cat ? " pixel-btn--primary" : ""}`}
                        onClick={() => {
                          setCreateCategory(cat);
                          setCreateCatOpen(false);
                        }}
                      >
                        {cat === UNCATEGORIZED ? "未分类" : cat}
                      </button>
                    ))}
                  </div>
                )}
              </div>
              <div className="row launch-modal__actions">
                <button type="submit" className="pixel-btn pixel-btn--primary" disabled={busy === "create" || !createName.trim()}>
                  {busy === "create" ? "创建中…" : "创建并进入工作台"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* 工作区设置弹窗（右上角红色 ✕ 关闭，禁止点击遮罩关闭） */}
      {workspaceOpen && (
        <div className="modal-scrim">
          <div className="pixel-panel launch-modal">
            <button type="button" className="modal-close" onClick={() => setWorkspaceOpen(false)} aria-label="关闭工作区弹窗" title="关闭">
              ✕
            </button>
            <h3 className="mono launch-modal__title">工作区设置</h3>
            <hr className="pixel-rule" />
            <div className="row">
              <input
                className="grow"
                value={workspacePath ?? workspace?.path ?? ""}
                onChange={(e) => setWorkspacePath(e.target.value)}
                placeholder="工作区目录路径"
                aria-label="工作区目录路径"
              />
              <button className="pixel-btn" onClick={() => void handleBrowse()} aria-label="浏览工作区目录">
                浏览…
              </button>
            </div>
            <div className="row launch-modal__actions">
              <button
                className="pixel-btn pixel-btn--primary"
                disabled={busy === "workspace" || !workspacePath || workspacePath === workspace?.path}
                onClick={handleSwitchClick}
              >
                {busy === "workspace" ? "切换中…" : "切换到此工作区"}
              </button>
            </div>
            <div className="faint launch__hint">
              当前工作区：{workspace?.path ?? "—"}（{packages.length} 个包）。切换后列表将显示该目录下的身份包。
            </div>
            {error && <div className="error-text">{error}</div>}
          </div>
        </div>
      )}

      {/* 分类右键菜单 */}
      {ctxMenu && (
        <div
          className="ctx-menu pixel-panel"
          style={{ left: ctxMenu.x, top: ctxMenu.y }}
          onClick={(e) => e.stopPropagation()}
        >
          <button
            className="pixel-btn pixel-btn--warn ctx-menu__item"
            onClick={() => {
              setDeleteCatTarget(ctxMenu.category);
              setCtxMenu(null);
            }}
          >
            删除分类「{ctxMenu.category}」
          </button>
        </div>
      )}

      {/* 删除分类确认 */}
      <ConfirmModal
        open={deleteCatTarget !== null}
        title="删除分类"
        message={
          deleteCatTarget !== null
            ? `删除分类「${deleteCatTarget}」？\n其下 ${
                packages.filter((p) => (p.category ?? "").trim() === deleteCatTarget).length
              } 个身份包将自动归入「未分类」。`
            : ""
        }
        confirmLabel="删除分类"
        danger
        onConfirm={() => {
          const c = deleteCatTarget;
          setDeleteCatTarget(null);
          if (c !== null) void deleteCategory(c);
        }}
        onCancel={() => setDeleteCatTarget(null)}
      />

      {/* 切换工作区：是否移动包 */}
      <ConfirmModal
        open={switchAsk !== null}
        title="切换工作区"
        message={`当前工作区有 ${packages.length} 个身份包，新目录可能没有这些内容。\n要把它们一并移动过去吗？`}
        confirmLabel="移动并切换"
        cancelLabel="仅切换（不移动）"
        onConfirm={() => {
          const p = switchAsk;
          setSwitchAsk(null);
          if (p) void doSwitch(p, true);
        }}
        onCancel={() => {
          const p = switchAsk;
          setSwitchAsk(null);
          if (p) void doSwitch(p, false);
        }}
      />

      {/* 删除身份包确认 */}
      <ConfirmModal
        open={deleteTarget !== null}
        title="删除身份包"
        message={deleteTarget ? `将把「${deleteTarget.name}」移到工作区回收站（.trash），可从文件管理器找回。\n确定删除？` : ""}
        confirmLabel="确定删除"
        danger
        onConfirm={() => {
          const t = deleteTarget;
          setDeleteTarget(null);
          if (t) void doDelete(t.path, t.name);
        }}
        onCancel={() => setDeleteTarget(null)}
      />

      {/* 全局设置面板 — 启动页也可打开（主题切换已收拢到此面板内） */}
      <SettingsPanel handle={settingsHandle.current} />
    </div>
  );
}
