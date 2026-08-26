package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/oframe/character-workbench/core/task"
)

func jsonResp(code int, v any) *http.Response {
	data, _ := json.Marshal(v)
	return &http.Response{
		StatusCode: code,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(data)),
	}
}

// configureDoubao sets a fake key so execution can reach the fake transport.
func configureDoubao(app *App) error {
	cfg, err := app.ProviderConfigGet("doubao")
	if err != nil {
		return err
	}
	cfg.APIKey = "test-key"
	return app.ProviderConfigSave("doubao", *cfg)
}

// TestTaskDrawerLifecycle verifies the queue-backed drawer bindings
// (workbench-ui spec 10.7 / tasks spec 6.2/6.5): empty initially, a confirmed
// generation persists a task row that the drawer lists, failed tasks can be
// retried (the retry re-executes and increments the retry count) or abandoned,
// and unknown ids error.
func TestTaskDrawerLifecycle(t *testing.T) {
	app, _ := newTestAppSimple(t)

	// Empty initially (fresh queue database).
	tasks, err := app.TaskList()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected empty task list, got %d", len(tasks))
	}

	// A failing provider makes the confirmed generation fail → the drawer row
	// carries the reason and supports retry/abandon (task 6.5).
	app, _ = newTestApp(t, fakeClient(func(r *http.Request) (*http.Response, error) {
		return jsonResp(500, map[string]any{"error": map[string]any{"message": "boom"}}), nil
	}))
	if _, err := app.PackageCreate("Hero", ""); err != nil {
		t.Fatal(err)
	}
	if err := app.IdentitySetCanvas(32, 32); err != nil {
		t.Fatal(err)
	}
	if err := configureDoubao(app); err != nil {
		t.Fatal(err)
	}
	plan, err := app.GenerationPlanPrepare(GenerationRequestView{Directions: 1, FrameCount: 4})
	if err != nil {
		t.Fatal(err)
	}
	res, err := app.GenerationPlanConfirm(plan.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "failed" {
		t.Fatalf("generation should fail with the fake provider: %+v", res)
	}
	tasks, err = app.TaskList()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Status != TaskStatusFailed || tasks[0].Error == "" {
		t.Fatalf("failed task not in drawer: %+v", tasks)
	}
	if tasks[0].Kind != "generate" {
		t.Fatalf("task kind = %q", tasks[0].Kind)
	}

	// Retry re-executes (still failing) and increments the retry count.
	if err := app.TaskRetry(tasks[0].ID); err != nil {
		t.Fatalf("TaskRetry: %v", err)
	}
	after, _ := app.TaskList()
	if after[0].RetryCount != 1 || after[0].Status != TaskStatusFailed {
		t.Fatalf("after retry: %+v", after[0])
	}

	// Abandon: not executed further.
	if err := app.TaskAbandon(tasks[0].ID); err != nil {
		t.Fatalf("TaskAbandon: %v", err)
	}
	after, _ = app.TaskList()
	if after[0].Status != TaskStatusAbandoned {
		t.Fatalf("task should be abandoned: %+v", after[0])
	}

	// Unknown ids error.
	if err := app.TaskRetry("nope"); err == nil {
		t.Fatal("expected error retrying unknown task")
	}
	if err := app.TaskAbandon("nope"); err == nil {
		t.Fatal("expected error abandoning unknown task")
	}
}

// TestTaskResumeAllBinding verifies the one-click resume binding (task 6.3):
// an unfinished (interrupted) task is resumed with TaskResumeAll.
func TestTaskResumeAllBinding(t *testing.T) {
	app, _ := newTestApp(t, fakeClient(func(r *http.Request) (*http.Response, error) {
		return filmstripPNGResp(t), nil
	}))
	if _, err := app.PackageCreate("Hero", ""); err != nil {
		t.Fatal(err)
	}
	if err := app.IdentitySetCanvas(32, 32); err != nil {
		t.Fatal(err)
	}
	if err := configureDoubao(app); err != nil {
		t.Fatal(err)
	}
	plan, err := app.GenerationPlanPrepare(GenerationRequestView{Directions: 1, FrameCount: 4})
	if err != nil {
		t.Fatal(err)
	}
	res, err := app.GenerationPlanConfirm(plan.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "executed" {
		t.Fatalf("generation should succeed: %+v", res)
	}

	// Simulate an interruption: force the task row back to "running" (a crash
	// would leave it mid-flight) — the queue row is the source of truth.
	svc, err := app.service()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.QueueStore().Update(plan.ID, func(tt *task.Task) error {
		tt.Status = task.StatusRunning
		tt.Progress = 0.4
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// One-click resume completes the task.
	n, err := app.TaskResumeAll()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("resumed = %d, want 1", n)
	}
	tasks, _ := app.TaskList()
	if tasks[0].Status != TaskStatusSucceeded || tasks[0].Progress != 1 {
		t.Fatalf("task not resumed: %+v", tasks[0])
	}
}
