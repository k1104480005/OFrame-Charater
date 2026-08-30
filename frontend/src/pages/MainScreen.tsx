// 三标签主界面 (workbench-ui spec 10.2): top-level Make / Acceptance / Export
// tabs. All tabs share the same identity package instance (Go-side session
// package + SessionContext) so unfinished work is never lost on switch.
import { useRef, useState } from "react";
import { useSession } from "../state/SessionContext";
import { WorkbenchProvider } from "../state/WorkContext";
import { Tabs } from "../components/Tabs";
import { TaskDrawer } from "../components/TaskDrawer";
import type { TaskDrawerHandle } from "../components/TaskDrawer";
import { SettingsPanel } from "../components/SettingsPanel";
import type { SettingsPanelHandle } from "../components/SettingsPanel";
import { IdentityPage } from "./make/IdentityPage";
import { MotionPage } from "./make/MotionPage";
import { AcceptanceTab } from "./AcceptanceTab";
import { ExportTab } from "./ExportTab";
import "./MainScreen.css";

type TopTab = "identity" | "motion" | "acceptance" | "export";

const TOP_TABS = [
  { id: "identity", label: "身份", icon: "🌱" },
  { id: "motion", label: "动作", icon: "🏃" },
  { id: "acceptance", label: "验收", icon: "✅" },
  { id: "export", label: "导出", icon: "📦" },
] as const;

export function MainScreen() {
  const { pkg, close } = useSession();
  return (
    <WorkbenchProvider>
      <WorkbenchContent pkg={pkg} close={close} />
    </WorkbenchProvider>
  );
}

function WorkbenchContent({ pkg, close }: { pkg: ReturnType<typeof useSession>["pkg"]; close: () => Promise<void> }) {
  const [tab, setTab] = useState<TopTab>("identity");
  const drawerHandle = useRef<TaskDrawerHandle>({ open: () => undefined });
  const settingsHandle = useRef<SettingsPanelHandle>({ open: () => undefined });

  return (
    <div className="main">
      <header className="main__header">
        <div className="main__title">
          <span className="main__logo">工作台</span>
          <span className="main__pkg">{pkg?.name ?? "—"}</span>
          <span className="faint main__pkg-meta">
            {pkg ? `版本 ${pkg.currentVersion} · 格式 v${pkg.formatVersion}` : ""}
          </span>
        </div>
        <div className="main__header-actions">
          <button className="pixel-btn" onClick={() => drawerHandle.current.open()} aria-label="打开任务抽屉">
            任务
          </button>
          <button className="pixel-btn" onClick={() => settingsHandle.current.open()} aria-label="打开设置" title="设置：provider / 密钥 / 统计">
            设置
          </button>
          <button className="pixel-btn pixel-btn--warn" onClick={() => void close()} title="返回启动页">
            返回
          </button>
        </div>
      </header>

      <div className="main__body">
        <main className="main__workspace">
          <Tabs<TopTab> tabs={TOP_TABS} active={tab} onChange={setTab} aria-label="主标签" />
          <div className="main__content">
            {tab === "identity" && <IdentityPage />}
            {tab === "motion" && <MotionPage />}
            {tab === "acceptance" && <AcceptanceTab />}
            {tab === "export" && <ExportTab />}
          </div>
        </main>
      </div>

      <TaskDrawer handle={drawerHandle.current} />
      <SettingsPanel handle={settingsHandle.current} />
    </div>
  );
}
