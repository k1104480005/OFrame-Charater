// 制作 > 编辑 sub-page (workbench-ui spec 10.3): switching works; lightweight
// editing (frame/sequence/anchor/batch) lands with the editing capability
// (tasks 7.x). The PixiJS canvas skeleton + matting/alpha-check view render
// here (magenta appears only in the technical matting view — spec 10.8).
import { useState } from "react";
import { PixelCanvas } from "../../components/PixelCanvas";

export function EditPage() {
  const [showMatting, setShowMatting] = useState(false);
  const [showGrid, setShowGrid] = useState(true);

  return (
    <div className="col">
      <div className="pixel-panel col">
        <h3 className="mono panel-heading">轻量编辑 / EDIT</h3>
        <hr className="pixel-rule" />
        <div className="empty-state">
          帧级 / 序列级 / 锚点级 / 批量编辑将在 editing 能力阶段实现（任务 7.1–7.5）
        </div>
      </div>

      <div className="pixel-panel col">
        <h3 className="mono panel-heading">PixelPerfect 预览骨架 / PREVIEW</h3>
        <hr className="pixel-rule" />
        <div className="row">
          <label className="row">
            <input type="checkbox" checked={showGrid} onChange={(e) => setShowGrid(e.target.checked)} />
            网格叠加
          </label>
          <label className="row">
            <input type="checkbox" checked={showMatting} onChange={(e) => setShowMatting(e.target.checked)} />
            Alpha 检查（抠图技术背景）
          </label>
        </div>
        <PixelCanvas unitWidth={16} unitHeight={16} scale={16} showMatting={showMatting} showGrid={showGrid} label="PREVIEW" />
      </div>
    </div>
  );
}
