package task

// Task lifecycle statuses (tasks spec 6.2: 进行中/排队/失败; 6.5: 放弃).
// The values are shared with the GUI task drawer (main.TaskSummary).
const (
	StatusQueued    = "queued"    // 排队: created, not yet started
	StatusRunning   = "running"   // 进行中: executing with visible progress
	StatusSucceeded = "succeeded" // 成功: result cached for idempotent dedup
	StatusFailed    = "failed"    // 失败: reason recorded, retry/abandon available
	StatusAbandoned = "abandoned" // 放弃: not executed further
)

// Task is one trackable, locally-persisted task (tasks spec 6.1): every
// generation / correction / export in the workflow. It carries the provider
// parameters (provider id + a JSON snapshot of the plan), the expected call
// count agreed in the generation confirmation, the lifecycle status, progress,
// the recorded error, the retry count, the opaque execution payload needed to
// re-run it after a restart, and the success result used for idempotent
// deduplication (tasks spec 6.4: 成功结果缓存).
type Task struct {
	ID             string  `json:"id"`
	Kind           string  `json:"kind"`           // generate | replace | regenerate | export | ...
	Provider       string  `json:"provider"`       // provider 参数: provider id
	ProviderParams string  `json:"providerParams"` // provider 参数: JSON plan snapshot
	ExpectedCalls  int     `json:"expectedCalls"`  // 预计调用量 (生成确认约定)
	Status         string  `json:"status"`
	Progress       float64 `json:"progress"` // 0..1
	Error          string  `json:"error"`
	RetryCount     int     `json:"retryCount"`
	Payload        string  `json:"payload"`     // opaque JSON payload for re-execution (persistent session)
	Result         string  `json:"result"`      // success result JSON (idempotent dedup cache value)
	Fingerprint    string  `json:"fingerprint"` // dedup key; "" = no dedup
	CreatedAt      string  `json:"createdAt"`
	UpdatedAt      string  `json:"updatedAt"`
}

// IsFinished reports whether the task reached a terminal status.
func (t *Task) IsFinished() bool {
	return t.Status == StatusSucceeded || t.Status == StatusFailed || t.Status == StatusAbandoned
}

// IsResumable reports whether the task was interrupted before finishing
// (queued or running) and can be resumed after an app restart (task 6.3).
func (t *Task) IsResumable() bool {
	return t.Status == StatusQueued || t.Status == StatusRunning
}

// IsRetryable reports whether the task can be retried from the drawer
// (failed or abandoned, task 6.5).
func (t *Task) IsRetryable() bool {
	return t.Status == StatusFailed || t.Status == StatusAbandoned
}
