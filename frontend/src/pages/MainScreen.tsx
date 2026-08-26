// 三标签主界面 (workbench-ui spec 10.2): top-level Make / Acceptance / Export
// tabs. All tabs share the same identity package instance (Go-side session
// package + SessionContext) so unfinished work is never lost on switch.
import { useRef, useState } from "react";
import { useSession } from "../state/SessionContext";
import { Tabs } from "../components/Tabs";
import { ThemeToggle } from "../components/ThemeToggle";
import { TaskDrawer } from "../components/TaskDrawer";
import type { TaskDrawerHandle } from "../components/TaskDrawer";
import { SettingsPanel } from "../components/SettingsPanel";
import type { SettingsPanelHandle } from "../components/SettingsPanel";
import { MakeTab } from "./make/MakeTab";
import { AcceptanceTab } from "./AcceptanceTab";
import { ExportTab } from "./ExportTab";
import "./MainScreen.css";

type TopTab = "make" | "acceptance" | "export";

const TOP_TABS = [
  { id: "make", label: "制作", accent: "MAKE" },
  { id: "acceptance", label: "验收", accent: "ACCEPT" },
  { id: "export", label: "导出", accent: "EXPORT" },
] as const;

export function MainScreen() {
  const { pkg, close } = useSession();
  const [tab, setTab] = useState<TopTab>("make");
  const drawerHandle = useRef<TaskDrawerHandle>({ open: () => undefined });
  const settingsHandle = useRef<SettingsPanelHandle>({ open: () => undefined });

  return (
    <div className="main">
      <header className="main__header">
        <div className="main__title">
          <span className="mono main__logo">OFrame</span>
          <span className="main__pkg mono">{pkg?.name ?? "—"}</span>
          <span className="faint mono main__pkg-meta">
            {pkg ? `format v${pkg.formatVersion} · ${pkg.currentVersion}` : ""}
          </span>
        </div>
        <div className="main__header-actions">
          <button className="pixel-btn" onClick={() => drawerHandle.current.open()} aria-label="打开任务抽屉">
            <span className="mono">TASKS</span>
          </button>
          <button className="pixel-btn" onClick={() => settingsHandle.current.open()} aria-label="打开设置" title="设置：provider / 密钥 / 统计">
            <span className="mono">⚙ SETTINGS</span>
          </button>
          <ThemeToggle />
          <button className="pixel-btn" onClick={() => void close()} title="返回启动页">
            返回
          </button>
        </div>
      </header>

      <div className="main__body">
        <Tabs<TopTab> tabs={TOP_TABS} active={tab} onChange={setTab} aria-label="主标签" />
        <div className="main__content">
          {tab === "make" && <MakeTab />}
          {tab === "acceptance" && <AcceptanceTab />}
          {tab === "export" && <ExportTab />}
        </div>
      </div>

      {/* global task drawer — visible from any tab */}
      <TaskDrawer handle={drawerHandle.current} />
      {/* global settings — visible from any tab */}
      <SettingsPanel handle={settingsHandle.current} />
    </div>
  );
}
