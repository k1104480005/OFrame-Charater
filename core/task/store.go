package task

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/oframe/character-workbench/core/store"
)

// DBDriverName is the SQLite driver registered by modernc.org/sqlite.
const DBDriverName = "sqlite"

// Store is the SQLite-backed task queue persistence (tasks spec 6.1: 任务模型
// 与本地持久化; 6.4: 成功结果缓存). Every mutation persists immediately so the
// queue stays continuous across app restarts (task 6.3: 持久化会话).
type Store struct {
	mu       sync.RWMutex
	db       *sql.DB
	log      *slog.Logger
	onChange func()
}

// Open opens (creating if needed) the task queue database at path, applies the
// schema migrations, and returns a ready store. An empty logger selects
// slog.Default().
func Open(path string, log *slog.Logger) (*Store, error) {
	if log == nil {
		log = slog.Default()
	}
	db, err := store.Migrate(DBDriverName, path)
	if err != nil {
		return nil, fmt.Errorf("task: open queue database %s: %w", path, err)
	}
	log.Info("task queue database ready", "path", path)
	return &Store{db: db, log: log}, nil
}

// Close closes the underlying database.
func (s *Store) Close() error {
	return s.db.Close()
}

// SetOnChange installs a callback invoked (outside the lock) after every
// mutation; the GUI uses it to emit the task:changed runtime event.
func (s *Store) SetOnChange(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onChange = fn
}

func (s *Store) fire() {
	s.mu.RLock()
	fn := s.onChange
	s.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

// Create inserts a new task (status queued) and returns it.
func (s *Store) Create(t Task) (Task, error) {
	if t.ID == "" || t.Kind == "" {
		return Task{}, fmt.Errorf("task: id and kind are required")
	}
	if t.Status == "" {
		t.Status = StatusQueued
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	t.CreatedAt, t.UpdatedAt = now, now
	if _, err := s.db.Exec(`
		INSERT INTO tasks (id, kind, provider, provider_params, expected_call_count,
		                   status, progress, error, retry_count, payload, result,
		                   fingerprint, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Kind, t.Provider, t.ProviderParams, t.ExpectedCalls,
		t.Status, t.Progress, t.Error, t.RetryCount, t.Payload, t.Result,
		t.Fingerprint, t.CreatedAt, t.UpdatedAt); err != nil {
		return Task{}, fmt.Errorf("task: insert %s: %w", t.ID, err)
	}
	s.log.Info("task persisted", "id", t.ID, "kind", t.Kind, "status", t.Status)
	s.fire()
	return t, nil
}

// Update applies fn to a copy of the task, persists it, and returns the new
// state. Returns an error when the task is unknown.
func (s *Store) Update(id string, fn func(*Task) error) (Task, error) {
	cur, err := s.Get(id)
	if err != nil {
		return Task{}, err
	}
	if fn != nil {
		if err := fn(&cur); err != nil {
			return Task{}, err
		}
	}
	cur.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.Exec(`
		UPDATE tasks SET kind=?, provider=?, provider_params=?, expected_call_count=?,
		                 status=?, progress=?, error=?, retry_count=?, payload=?,
		                 result=?, fingerprint=?, updated_at=?
		WHERE id = ?`,
		cur.Kind, cur.Provider, cur.ProviderParams, cur.ExpectedCalls,
		cur.Status, cur.Progress, cur.Error, cur.RetryCount, cur.Payload,
		cur.Result, cur.Fingerprint, cur.UpdatedAt, cur.ID); err != nil {
		return Task{}, fmt.Errorf("task: update %s: %w", id, err)
	}
	s.fire()
	return cur, nil
}

// Get returns one task by id.
func (s *Store) Get(id string) (Task, error) {
	row := s.db.QueryRow(`
		SELECT id, kind, provider, provider_params, expected_call_count, status,
		       progress, error, retry_count, payload, result, fingerprint, created_at, updated_at
		FROM tasks WHERE id = ?`, id)
	var t Task
	if err := row.Scan(&t.ID, &t.Kind, &t.Provider, &t.ProviderParams, &t.ExpectedCalls,
		&t.Status, &t.Progress, &t.Error, &t.RetryCount, &t.Payload, &t.Result,
		&t.Fingerprint, &t.CreatedAt, &t.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return Task{}, fmt.Errorf("task: not found: %s", id)
		}
		return Task{}, fmt.Errorf("task: read %s: %w", id, err)
	}
	return t, nil
}

// List returns a copy of all tasks, most recently updated first.
func (s *Store) List() ([]Task, error) {
	rows, err := s.db.Query(`
		SELECT id, kind, provider, provider_params, expected_call_count, status,
		       progress, error, retry_count, payload, result, fingerprint, created_at, updated_at
		FROM tasks`)
	if err != nil {
		return nil, fmt.Errorf("task: list: %w", err)
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.Kind, &t.Provider, &t.ProviderParams, &t.ExpectedCalls,
			&t.Status, &t.Progress, &t.Error, &t.RetryCount, &t.Payload, &t.Result,
			&t.Fingerprint, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("task: scan: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out, nil
}

// Unfinished returns the tasks that were interrupted before finishing (queued
// or running) — the ones a persistent session resumes after a crash/shutdown/
// network failure (task 6.3: 一键续跑).
func (s *Store) Unfinished() ([]Task, error) {
	all, err := s.List()
	if err != nil {
		return nil, err
	}
	var out []Task
	for _, t := range all {
		if t.IsResumable() {
			out = append(out, t)
		}
	}
	return out, nil
}

// Retry re-queues a failed or abandoned task: status → queued, error cleared,
// progress reset, retry count incremented (task 6.5). The generation
// confirmation's maximum-retry agreement is enforced by the caller BEFORE
// calling Retry (the caller knows the plan's MaxTotalAttempts).
func (s *Store) Retry(id string) (Task, error) {
	return s.Update(id, func(t *Task) error {
		if !t.IsRetryable() {
			return fmt.Errorf("task: %s is not retryable in status %s", id, t.Status)
		}
		t.Status = StatusQueued
		t.Error = ""
		t.Progress = 0
		t.RetryCount++
		return nil
	})
}

// Abandon marks a task as abandoned; it is not executed further (task 6.5).
func (s *Store) Abandon(id string) (Task, error) {
	return s.Update(id, func(t *Task) error {
		t.Status = StatusAbandoned
		t.Error = "abandoned by user"
		return nil
	})
}

// --- idempotent success-result cache (tasks 6.4 / 4.8: 幂等去重) ---

// CacheGet returns the cached success result of an identical task, if any.
func (s *Store) CacheGet(fingerprint string) (string, bool, error) {
	if fingerprint == "" {
		return "", false, nil
	}
	var result string
	err := s.db.QueryRow(`SELECT result FROM task_results WHERE fingerprint = ?`, fingerprint).Scan(&result)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("task: cache read %s: %w", fingerprint, err)
	}
	return result, true, nil
}

// CachePut stores the success result of a task keyed by its fingerprint.
func (s *Store) CachePut(fingerprint, result string) error {
	if fingerprint == "" {
		return nil
	}
	if _, err := s.db.Exec(`
		INSERT INTO task_results (fingerprint, result, created_at) VALUES (?, ?, ?)
		ON CONFLICT(fingerprint) DO UPDATE SET result = excluded.result`,
		fingerprint, result, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("task: cache write %s: %w", fingerprint, err)
	}
	return nil
}

// MarshalPayload encodes an arbitrary payload for the tasks table.
func MarshalPayload(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("task: encode payload: %w", err)
	}
	return string(data), nil
}
