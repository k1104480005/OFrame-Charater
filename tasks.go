// Task bindings over the persisted, recoverable task queue (tasks spec
// 6.1–6.5): the queue lives in the shared application service (SQLite, next to
// the settings), survives app restarts, and every generation/replacement/
// regeneration runs as a task row with status, progress, error, and retry
// count. The global task drawer renders TaskList; failed tasks show their
// reason and support retry or abandon (10.7); interrupted tasks resume with
// one action (6.3).
//
// The task:changed runtime event fires on every queue mutation so the drawer
// stays live across tabs (the App installs the hook when the service is
// created).
package main

import "context"

// TaskStatus is the lifecycle status shown in the drawer (mirrors the queue
// task statuses).
type TaskStatus string

// Task lifecycle statuses (tasks spec 6.2: 进行中/排队/失败; 6.5: 放弃).
const (
	TaskStatusQueued    TaskStatus = "queued"    // 排队
	TaskStatusRunning   TaskStatus = "running"   // 进行中
	TaskStatusFailed    TaskStatus = "failed"    // 失败
	TaskStatusSucceeded TaskStatus = "succeeded" // 成功
	TaskStatusAbandoned TaskStatus = "abandoned" // 放弃
)

// TaskSummary is the typed drawer row.
type TaskSummary struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	Status     TaskStatus `json:"status"`
	Progress   float64    `json:"progress"` // 0..1
	Error      string     `json:"error"`
	RetryCount int        `json:"retryCount"`
	CreatedAt  string     `json:"createdAt"`
	UpdatedAt  string     `json:"updatedAt"`
}

// bgCtx returns the app context, falling back to context.Background() so
// bindings stay unit-testable headlessly.
func (a *App) bgCtx() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

// TaskList returns all persisted tasks for the global task drawer
// (tasks spec 6.2: 进行中/排队/失败 with progress).
func (a *App) TaskList() ([]TaskSummary, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	views, err := svc.TaskList()
	if err != nil {
		return nil, err
	}
	out := make([]TaskSummary, 0, len(views))
	for _, v := range views {
		out = append(out, TaskSummary{
			ID: v.ID, Kind: v.Kind, Status: TaskStatus(v.Status), Progress: v.Progress,
			Error: v.Error, RetryCount: v.RetryCount, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
		})
	}
	return out, nil
}

// TaskGet returns one persisted task (失败可查看原因).
func (a *App) TaskGet(id string) (TaskSummary, error) {
	svc, err := a.service()
	if err != nil {
		return TaskSummary{}, err
	}
	v, err := svc.TaskGet(id)
	if err != nil {
		return TaskSummary{}, err
	}
	return TaskSummary{
		ID: v.ID, Kind: v.Kind, Status: TaskStatus(v.Status), Progress: v.Progress,
		Error: v.Error, RetryCount: v.RetryCount, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}, nil
}

// TaskRetry re-queues and re-executes a failed/abandoned task (task 6.5:
// 重试遵循生成确认上限 — the retry cap agreed in the confirmation is enforced).
func (a *App) TaskRetry(id string) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	_, err = svc.TaskRetry(a.bgCtx(), id)
	return err
}

// TaskAbandon abandons a failed task; it is not executed further (task 6.5).
func (a *App) TaskAbandon(id string) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	_, err = svc.TaskAbandon(id)
	return err
}

// TaskResumeAll resumes every unfinished task with one action (task 6.3: 中断后
// 一键续跑). Returns the number of resumed tasks.
func (a *App) TaskResumeAll() (int, error) {
	svc, err := a.service()
	if err != nil {
		return 0, err
	}
	return svc.TaskResumeAll(a.bgCtx())
}
