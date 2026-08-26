// 启动页 (workbench-ui spec 10.1): offers ONLY selecting an existing identity
// package or creating a new one — no editing capability anywhere on this page.
import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { fetchAppInfo, openWorkspace } from "../api/client";
import { useSession } from "../state/SessionContext";
import { useTheme } from "../theme/ThemeProvider";
import { ThemeToggle } from "../components/ThemeToggle";
import "./LaunchPage.css";

export function LaunchPage() {
  const { workspace, packages, loading, error, open, create, refreshList, setError } = useSession();
  const [name, setName] = useState("");
  const [busy, setBusy] = useState<string | null>(null); // "create" | package path
  const [workspacePath, setWorkspacePath] = useState<string | null>(null);
  const [appInfo, setAppInfo] = useState<string>("");
  const { theme } = useTheme();

  useEffect(() => {
    void fetchAppInfo().then((info) => {
      setAppInfo(`OFrame Character Workbench v${info.version} · manifest format v${info.formatVersion} · ${info.go}`);
    });
  }, []);

  // pick an explicit workspace (still only lists packages — no editing here)
  const handleWorkspace = async () => {
    if (!workspacePath) return;
    setBusy("workspace");
    try {
      const info = await openWorkspace(workspacePath);
      setWorkspacePath(info.path);
      await refreshList();
      setError(null);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  };

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault();
    const trimmed = name.trim();
    if (!trimmed) return;
    setBusy("create");
    try {
      await create(trimmed);
      setName("");
      setError(null);
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy(null);
    }
  };

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

  return (
    <div className="launch">
      <header className="launch__header">
        <div className="launch__title">
          <h1 className="launch__logo mono">OFrame Character</h1>
          <div className="launch__subtitle">角色动画资产工作台</div>
        </div>
        <div className="launch__header-actions">
          <ThemeToggle />
        </div>
      </header>

      <main className="launch__main">
        <section className="launch__panel pixel-panel">
          <h2 className="mono launch__panel-title">OPEN / 选择身份包</h2>
          <hr className="pixel-rule" />
          {loading ? (
            <div className="muted">加载工作区…</div>
          ) : (
            <>
              <div className="muted">工作区：{workspace?.path ?? "—"}</div>
              {packages.length === 0 ? (
                <div className="empty-state">工作区中还没有身份包 —— 创建一个开始</div>
              ) : (
                <ul className="launch__list">
                  {packages.map((p) => (
                    <li key={p.path} className="launch__item">
                      <div className="launch__item-main">
                        <span className="launch__item-name">{p.name}</span>
                        <span className="mono launch__item-meta">
                          format v{p.formatVersion} · {p.currentVersion}
                        </span>
                      </div>
                      <button className="pixel-btn" disabled={busy === p.path} onClick={() => void handleOpen(p.path)}>
                        {busy === p.path ? "打开中…" : "打开"}
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </>
          )}
        </section>

        <section className="launch__panel pixel-panel">
          <h2 className="mono launch__panel-title">CREATE / 创建身份包</h2>
          <hr className="pixel-rule" />
          <form onSubmit={handleCreate} className="col">
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="身份包名称（如 Hero）"
              aria-label="身份包名称"
              maxLength={64}
            />
            <button type="submit" className="pixel-btn pixel-btn--primary" disabled={busy === "create" || !name.trim()}>
              {busy === "create" ? "创建中…" : "创建并进入工作台"}
            </button>
          </form>
        </section>

        <section className="launch__panel pixel-panel">
          <h2 className="mono launch__panel-title">WORKSPACE / 工作区</h2>
          <hr className="pixel-rule" />
          <div className="row">
            <input
              className="grow"
              value={workspacePath ?? workspace?.path ?? ""}
              onChange={(e) => setWorkspacePath(e.target.value)}
              placeholder="工作区目录路径"
              aria-label="工作区目录路径"
            />
            <button className="pixel-btn" disabled={busy === "workspace"} onClick={() => void handleWorkspace()}>
              使用此工作区
            </button>
          </div>
        </section>

        {error && <div className="error-text launch__error">{error}</div>}
      </main>

      <footer className="launch__footer faint mono" data-theme={theme}>
        {appInfo || "…"}
      </footer>
    </div>
  );
}
