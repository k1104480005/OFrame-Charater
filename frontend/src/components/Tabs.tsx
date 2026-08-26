// Pixel-styled tab strip used for the top-level tabs (制作/验收/导出) and the
// Make sub-tabs (身份/动作/编辑).
import type { ReactNode } from "react";
import "./Tabs.css";

export interface TabDef<T extends string> {
  id: T;
  label: string;
  /** optional pixel-font accent label (title-case) */
  accent?: string;
}

interface TabsProps<T extends string> {
  tabs: readonly TabDef<T>[];
  active: T;
  onChange: (id: T) => void;
  /** extra content rendered on the right of the strip (e.g. status) */
  trailing?: ReactNode;
  "aria-label"?: string;
}

export function Tabs<T extends string>({ tabs, active, onChange, trailing, "aria-label": ariaLabel }: TabsProps<T>) {
  return (
    <div className="pixel-tabs" role="tablist" aria-label={ariaLabel}>
      {tabs.map((t) => (
        <button
          key={t.id}
          role="tab"
          aria-selected={active === t.id}
          className={`pixel-tab${active === t.id ? " pixel-tab--active" : ""}`}
          onClick={() => onChange(t.id)}
        >
          {t.accent && <span className="pixel-tab__accent">{t.accent}</span>}
          {t.label}
        </button>
      ))}
      {trailing && <div className="pixel-tabs__trailing">{trailing}</div>}
    </div>
  );
}
