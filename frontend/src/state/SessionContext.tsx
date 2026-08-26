// Session state shared by every tab (workbench-ui spec 10.2: 标签间共享同一
// 身份包实例). The Go side holds the single *identity.Package instance; React
// only mirrors its summary and re-reads views through bindings. The
// session:changed runtime event keeps all tabs in sync.
import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import type { PackageSummary, WorkspaceInfo } from "../api/client";
import {
  closePackage,
  createPackage,
  currentPackage,
  ensureDefaultWorkspace,
  listPackages,
  migrateWorkspace as migrateWorkspaceBinding,
  onSessionChanged,
  openPackage,
  openWorkspace as openWorkspaceBinding,
} from "../api/client";

export interface SessionState {
  /** current identity package summary, or null on the launch page */
  pkg: PackageSummary | null;
  workspace: WorkspaceInfo | null;
  /** identity packages found in the workspace (launch page list) */
  packages: PackageSummary[];
  loading: boolean;
  error: string | null;
  /** open an existing package (launch page → workbench) */
  open: (path: string) => Promise<void>;
  /** create and open a new package */
  create: (name: string, category?: string) => Promise<void>;
  /** close the package and return to the launch page */
  close: () => Promise<void>;
  refreshList: () => Promise<void>;
  setError: (msg: string | null) => void;
  /** switch the active workspace directory (opens & persists the choice) */
  switchWorkspace: (path: string) => Promise<WorkspaceInfo>;
  /** migrate current workspace packages to a new location (copy or move) */
  migrateWorkspace: (dst: string, move: boolean) => Promise<WorkspaceInfo>;
}

const SessionContext = createContext<SessionState | null>(null);

export function SessionProvider({ children }: { children: ReactNode }) {
  const [pkg, setPkg] = useState<PackageSummary | null>(null);
  const [workspace, setWorkspace] = useState<WorkspaceInfo | null>(null);
  const [packages, setPackages] = useState<PackageSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // On boot: ensure the default workspace, list packages, and restore the
  // session package if one is already open (persistent session).
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const ws = await ensureDefaultWorkspace();
        if (cancelled) return;
        setWorkspace(ws);
        const [pkgs, cur] = await Promise.all([listPackages(), currentPackage()]);
        if (cancelled) return;
        setPackages(pkgs);
        setPkg(cur);
      } catch (e) {
        if (!cancelled) setError(String(e));
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  // Live sync: the Go side emits session:changed on every package open/close
  // and every manifest mutation, so all tabs see the same instance state.
  useEffect(() => {
    const off = onSessionChanged((p) => {
      setPkg(p);
      setError(null);
    });
    return off;
  }, []);

  const refreshList = useCallback(async () => {
    const pkgs = await listPackages();
    setPackages(pkgs);
  }, []);

  const open = useCallback(async (path: string) => {
    const summary = await openPackage(path);
    setPkg(summary);
    setError(null);
  }, []);

  const create = useCallback(async (name: string, category = "") => {
    const summary = await createPackage(name, category);
    setPkg(summary);
    setError(null);
  }, []);

  const close = useCallback(async () => {
    await closePackage();
    setPkg(null);
    await refreshList();
  }, [refreshList]);

  // Switch the active workspace: open it on the Go side (which persists the
  // choice), mirror it into session state, and re-list its packages.
  const switchWorkspace = useCallback(async (path: string) => {
    const info = await openWorkspaceBinding(path);
    setWorkspace(info);
    await refreshList();
    setError(null);
    return info;
  }, [refreshList]);

  // Migrate the current workspace's packages to a new location, switch to it,
  // and drop any open package (its directory may have moved).
  const migrateWorkspace = useCallback(async (dst: string, move: boolean) => {
    const info = await migrateWorkspaceBinding(dst, move);
    setWorkspace(info);
    setPkg(null);
    await refreshList();
    setError(null);
    return info;
  }, [refreshList]);

  const value = useMemo<SessionState>(
    () => ({ pkg, workspace, packages, loading, error, open, create, close, refreshList, setError, switchWorkspace, migrateWorkspace }),
    [pkg, workspace, packages, loading, error, open, create, close, refreshList, switchWorkspace, migrateWorkspace],
  );

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): SessionState {
  const ctx = useContext(SessionContext);
  if (!ctx) throw new Error("useSession must be used inside SessionProvider");
  return ctx;
}
