package task

import (
	"path/filepath"
	"testing"
)

// newTestStore opens a store on a temp database file.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "queue.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func sampleTask(id string) Task {
	return Task{
		ID:             id,
		Kind:           "generate",
		Provider:       "doubao",
		ProviderParams: `{"prompt":"hero walk","model":"doubao-seedream-4-0"}`,
		ExpectedCalls:  3,
		Payload:        `{"planId":"p1"}`,
		Fingerprint:    "fp-" + id,
	}
}

// TestTaskPersistsAcrossRestart verifies task 6.1: a task created in one store
// instance (one app run) is still readable from a fresh store over the same
// database file (restart).
func TestTaskPersistsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.db")
	s1, err := Open(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	created, err := s1.Create(sampleTask("t1"))
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != StatusQueued || created.CreatedAt == "" {
		t.Fatalf("created task: %+v", created)
	}
	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}

	// "restart": a fresh store over the same database file.
	s2, err := Open(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got, err := s2.Get("t1")
	if err != nil {
		t.Fatalf("task lost after restart: %v", err)
	}
	if got.Kind != "generate" || got.Provider != "doubao" ||
		got.ProviderParams == "" || got.ExpectedCalls != 3 ||
		got.Status != StatusQueued || got.Fingerprint != "fp-t1" || got.Payload == "" {
		t.Fatalf("task fields not preserved: %+v", got)
	}
}

// TestUpdateRetryAbandon verifies the state machine: running/progress updates
// persist, Retry re-queues a failed task and increments the retry count, and
// Abandon marks the task abandoned.
func TestUpdateRetryAbandon(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Create(sampleTask("t1")); err != nil {
		t.Fatal(err)
	}
	// progress update persists.
	up, err := s.Update("t1", func(t *Task) error {
		t.Status = StatusRunning
		t.Progress = 0.5
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if up.Status != StatusRunning || up.Progress != 0.5 {
		t.Fatalf("updated task: %+v", up)
	}
	if _, err := s.Update("t1", func(t *Task) error {
		t.Status = StatusFailed
		t.Error = "provider 500"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Retry re-queues and increments.
	rt, err := s.Retry("t1")
	if err != nil {
		t.Fatal(err)
	}
	if rt.Status != StatusQueued || rt.RetryCount != 1 || rt.Error != "" || rt.Progress != 0 {
		t.Fatalf("retried task: %+v", rt)
	}
	// Abandon marks abandoned.
	ab, err := s.Abandon("t1")
	if err != nil {
		t.Fatal(err)
	}
	if ab.Status != StatusAbandoned {
		t.Fatalf("abandoned task: %+v", ab)
	}
	// A running task is not retryable.
	if _, err := s.Create(sampleTask("t2")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update("t2", func(t *Task) error { t.Status = StatusRunning; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Retry("t2"); err == nil {
		t.Fatal("retrying a running task must fail")
	}
}

// TestUnfinishedAndDedupCache verifies task 6.3 (interrupted tasks are listed
// for one-click resume) and task 6.4 (identical tasks reuse the cached result).
func TestUnfinishedAndDedupCache(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Create(sampleTask("t1")); err != nil { // queued → resumable
		t.Fatal(err)
	}
	if _, err := s.Create(sampleTask("t2")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update("t2", func(t *Task) error { t.Status = StatusRunning; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(sampleTask("t3")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update("t3", func(t *Task) error { t.Status = StatusSucceeded; return nil }); err != nil {
		t.Fatal(err)
	}
	unf, err := s.Unfinished()
	if err != nil {
		t.Fatal(err)
	}
	if len(unf) != 2 {
		t.Fatalf("unfinished = %d, want 2 (queued + running)", len(unf))
	}
	// Dedup cache: get miss → put → get hit.
	if _, hit, err := s.CacheGet("fp-x"); err != nil || hit {
		t.Fatalf("cache miss expected: hit=%v err=%v", hit, err)
	}
	if err := s.CachePut("fp-x", `{"status":"executed"}`); err != nil {
		t.Fatal(err)
	}
	res, hit, err := s.CacheGet("fp-x")
	if err != nil || !hit || res != `{"status":"executed"}` {
		t.Fatalf("cache hit expected: %q hit=%v err=%v", res, hit, err)
	}
}

// TestListOrdering verifies List returns most-recently-updated first.
func TestListOrdering(t *testing.T) {
	s := newTestStore(t)
	for _, id := range []string{"a", "b", "c"} {
		if _, err := s.Create(sampleTask(id)); err != nil {
			t.Fatal(err)
		}
	}
	// Update "b" last so it sorts first.
	if _, err := s.Update("b", func(t *Task) error { t.Progress = 0.25; return nil }); err != nil {
		t.Fatal(err)
	}
	all, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 || all[0].ID != "b" {
		t.Fatalf("list order: %+v", all)
	}
}
