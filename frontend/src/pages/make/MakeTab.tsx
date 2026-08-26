// 制作标签 (workbench-ui spec 10.3): sub-page switching between
// 身份 (Identity) / 动作 (Motion) / 编辑 (Edit).
import { useState } from "react";
import { Tabs } from "../../components/Tabs";
import { IdentityPage } from "./IdentityPage";
import { MotionPage } from "./MotionPage";
import { EditPage } from "./EditPage";

type MakeSubTab = "identity" | "motion" | "edit";

const SUB_TABS = [
  { id: "identity", label: "身份", accent: "ID" },
  { id: "motion", label: "动作", accent: "MOVE" },
  { id: "edit", label: "编辑", accent: "EDIT" },
] as const;

export function MakeTab() {
  const [sub, setSub] = useState<MakeSubTab>("identity");
  return (
    <div>
      <Tabs<MakeSubTab> tabs={SUB_TABS} active={sub} onChange={setSub} aria-label="制作子页" />
      {sub === "identity" && <IdentityPage />}
      {sub === "motion" && <MotionPage />}
      {sub === "edit" && <EditPage />}
    </div>
  );
}
