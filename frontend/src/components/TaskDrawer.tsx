// 全局任务抽屉 (workbench-ui spec 10.7): visible from any tab, shows
// running/queued/failed tasks with progress; failed tasks show their reason and
// support retry or abandon. Task data comes from the persisted recoverable
// queue (tasks 6.1–6.5) and stays live via the task:changed runtime event.
// Interrupted (queued/running) tasks can be resumed with ONE action after an
// app restart (task 6.3: 一键续跑).
import { useCallback, useEffect, useState } from "react";
import type { TaskSummary } from "../api/client";
import { abandonTask, deleteFinishedTasks, deleteTask, fetchTasks, onTasksChanged, resumeAllTasks, retryTask } from "../api/client";
import "./TaskDrawer.css";

export interface TaskDrawerHandle {
  open: () => void;
}

const STATUS_LABEL: Record<string, string> = {
  queued: "排队",
  running: "进行中",
  failed: "失败",
  succeeded: "成功",
  abandoned: "放弃",
};

function groupTasks(tasks: TaskSummary[]): { running: TaskSummary[]; queued: TaskSummary[]; failed: TaskSummary[]; completed: TaskSummary[] } {
  const running: TaskSummary[] = [];
  const queued: TaskSummary[] = [];
  const failed: TaskSummary[] = [];
  const completed: TaskSummary[] = [];
  for (const t of tasks) {
    if (t.status === "running") running.push(t);
    else if (t.status === "queued") queued.push(t);
    else if (t.status === "failed") failed.push(t);
    else if (t.status === "succeeded" || t.status === "abandoned") completed.push(t);
  }
  return { running, queued, failed, completed };
}

function TaskRow({ task, onAction }: { task: TaskSummary; onAction: () => void }) {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const run = async (fn: () => Promise<void>) => {
    setBusy(true);
    setErr(null);
    try {
      await fn();
      onAction();
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(false);
    }
  };

  const isFailed = task.status === "failed";
  const isClearable = task.status === "succeeded" || task.status === "abandoned";
  const pct = Math.round(task.progress * 100);

  return (
    <li className={`task-row${isFailed ? " task-row--failed" : ""}`}>
      <div className="task-row__head">
        <span className={`status-badge status-${isFailed ? "warn" : task.status === "running" ? "ok" : "muted"}`}>
          {STATUS_LABEL[task.status] ?? task.status}
        </span>
        {(task.status === "queued" || task.status === "running") && (
          <span className={`mono ${task.live ? "status-ok" : "status-warn"}`}>
            {task.live ? "本会话执行中" : "中断遗留"}
          </span>
        )}
        <span className="mono task-row__kind">{task.kind}</span>
        <span className="mono task-row__id">{task.id.slice(0, 8)}</span>
        {task.status === "running" && <span className="mono task-row__pct">{pct}%</span>}
        {isFailed && task.retryCount > 0 && <span className="faint mono">重试 {task.retryCount} 次</span>}
      </div>

      {(task.status === "running" || task.status === "queued") && (
        <div className="task-row__progress" role="progressbar" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100}>
          <div className="task-row__progress-fill" style={{ width: `${pct}%` }} />
        </div>
      )}

      {isFailed && (
        <div className="task-row__error error-text">
          <span className="mono">原因</span> {task.error || "未知错误"}
        </div>
      )}

      {(isClearable || isFailed || ((task.status === "queued" || task.status === "running") && !task.live)) && (
        <div className="task-row__actions">
          {isFailed && (
            <button className="pixel-btn pixel-btn--ok" disabled={busy} onClick={() => run(() => retryTask(task.id))}>
              重试
            </button>
          )}
          {(task.status === "queued" || task.status === "running") && !task.live && (
            <span className="faint mono">上次会话的中断遗留 —— 可一键续跑，或</span>
          )}
          {(isFailed || ((task.status === "queued" || task.status === "running") && !task.live)) && (
            <button className="pixel-btn" disabled={busy} onClick={() => run(() => abandonTask(task.id))} title={isFailed ? "放弃此任务" : "放弃此中断遗留任务（不再续跑）"}>
              放弃
            </button>
          )}
          {isClearable && (
            <button className="pixel-btn" disabled={busy} onClick={() => run(() => deleteTask(task.id))} title="从任务历史中移除此记录，不影响已生成的候选结果">
              清理
            </button>
          )}
        </div>
      )}
      {err && <div className="error-text">{err}</div>}
    </li>
  );
}

export function TaskDrawer({ handle }: { handle: TaskDrawerHandle }) {
  const [open, setOpen] = useState(false);
  const [tasks, setTasks] = useState<TaskSummary[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [resuming, setResuming] = useState(false);
  const [clearing, setClearing] = useState(false);

  useEffect(() => {
    handle.open = () => setOpen(true);
  }, [handle]);

  const refresh = useCallback(async () => {
    try {
      setTasks(await fetchTasks());
      setError(null);
    } catch (e) {
      setError(String(e));
    }
  }, []);

  // Load once on mount and keep live via the task:changed event.
  useEffect(() => {
    void refresh();
    const off = onTasksChanged((ts) => setTasks(ts));
    return off;
  }, [refresh]);

  // Events cover mutations, while polling keeps progress moving when a runtime
  // event is missed or the drawer was opened after a task started.
  useEffect(() => {
    if (!open) return;
    const iv = window.setInterval(() => { void refresh(); }, 1000);
    return () => window.clearInterval(iv);
  }, [open, refresh]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open]);

  if (!open) return null;

  const groups = groupTasks(tasks);
  // 一键续跑只针对上次会话的中断遗留：本会话正在执行的任务（live）不算。
  const staleUnfinished = tasks.filter((t) => (t.status === "queued" || t.status === "running") && !t.live).length;
  const sections: Array<[string, TaskSummary[]]> = [
    ["进行中", groups.running],
    ["排队", groups.queued],
    ["失败", groups.failed],
    ["已完成", groups.completed],
  ];

  const handleResumeAll = async () => {
    setResuming(true);
    setError(null);
    try {
      const n = await resumeAllTasks();
      setError(n === 0 ? "没有需要续跑的未完成任务" : `已续跑 ${n} 个任务`);
      await refresh();
    } catch (e) {
      setError(String(e));
    } finally {
      setResuming(false);
    }
  };

  const handleClearFinished = async () => {
    setClearing(true);
    setError(null);
    try {
      const n = await deleteFinishedTasks();
      setError(n === 0 ? "没有可清理的已完成任务" : `已清理 ${n} 个已完成任务`);
      await refresh();
    } catch (e) {
      setError(String(e));
    } finally {
      setClearing(false);
    }
  };

  return (
    <div className="drawer-scrim" onClick={() => setOpen(false)}>
      <aside className="task-drawer pixel-panel pixel-panel--elevated" onClick={(e) => e.stopPropagation()}>
        <div className="task-drawer__head">
          <h2 className="mono">TASKS / 任务</h2>
          <div className="task-drawer__head-actions">
            {groups.completed.length > 0 && (
              <button className="pixel-btn" disabled={clearing} onClick={() => void handleClearFinished()} title="一次清理所有已完成和已放弃任务，不影响候选图">
                {clearing ? "清理中…" : `一键清理（${groups.completed.length}）`}
              </button>
            )}
            <button className="pixel-btn" onClick={() => setOpen(false)} aria-label="关闭任务抽屉">
              ✕
            </button>
          </div>
        </div>
        <hr className="pixel-rule" />
        {staleUnfinished > 0 && (
          <div className="row task-drawer__resume">
            <span className="faint mono">中断遗留的未完成任务：{staleUnfinished} 个</span>
            <button className="pixel-btn pixel-btn--primary" disabled={resuming} onClick={() => void handleResumeAll()}>
              {resuming ? "续跑中…" : "一键续跑"}
            </button>
          </div>
        )}
        {error && <div className="error-text">{error}</div>}
        {tasks.length === 0 ? (
          <div className="empty-state">暂无任务 —— 生成/校正/导出任务会显示在这里</div>
        ) : (
          sections.map(([title, items]) =>
            items.length === 0 ? null : (
              <section key={title} className="task-drawer__group">
                <h3 className="mono task-drawer__group-title">
                  {title} <span className="faint">({items.length})</span>
                </h3>
                <ul className="task-drawer__list">
                  {items.map((t) => (
                    <TaskRow key={t.id} task={t} onAction={() => void refresh()} />
                  ))}
                </ul>
              </section>
            ),
          )
        )}
      </aside>
    </div>
  );
}
