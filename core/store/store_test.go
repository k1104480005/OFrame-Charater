package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
)

// --- stub driver: a minimal in-memory database/sql driver used to exercise
// the migration framework end-to-end without a real SQLite driver (unavailable
// offline). It records executed DDL and tracks applied migration versions.

type stubDB struct {
	mu      sync.Mutex
	applied map[int]bool
	tables  map[string]bool
	execs   []string
}

func newStubDB() *stubDB {
	return &stubDB{applied: map[int]bool{}, tables: map[string]bool{}}
}

func (d *stubDB) reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.applied = map[int]bool{}
	d.tables = map[string]bool{}
	d.execs = nil
}

type stubDriver struct{ db *stubDB }

func (d *stubDriver) Open(name string) (driver.Conn, error) { return &stubConn{db: d.db}, nil }

type stubConn struct{ db *stubDB }

func (c *stubConn) Prepare(query string) (driver.Stmt, error) { return &stubStmt{query: query}, nil }
func (c *stubConn) Close() error                              { return nil }
func (c *stubConn) Begin() (driver.Tx, error)                 { return &stubTx{}, nil }
func (c *stubConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return &stubTx{}, nil
}

func (c *stubConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.db.mu.Lock()
	defer c.db.mu.Unlock()
	c.db.execs = append(c.db.execs, query)
	q := strings.ToUpper(query)
	switch {
	case strings.Contains(q, "CREATE TABLE IF NOT EXISTS TASKS"):
		c.db.tables["tasks"] = true
	case strings.Contains(q, "CREATE TABLE IF NOT EXISTS SCHEMA_MIGRATIONS"):
		c.db.tables["schema_migrations"] = true
	case strings.Contains(q, "CREATE TABLE IF NOT EXISTS META"):
		c.db.tables["meta"] = true
	case strings.Contains(q, "INSERT INTO SCHEMA_MIGRATIONS"):
		if len(args) > 0 {
			if v, ok := args[0].Value.(int64); ok {
				c.db.applied[int(v)] = true
			}
		}
	}
	return driver.RowsAffected(1), nil
}

func (c *stubConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.db.mu.Lock()
	defer c.db.mu.Unlock()
	q := strings.ToUpper(query)
	if strings.Contains(q, "SELECT MAX(VERSION)") {
		max := 0
		for v := range c.db.applied {
			if v > max {
				max = v
			}
		}
		return &stubRows{maxOnly: true, maxVal: max}, nil
	}
	if strings.Contains(q, "SELECT VERSION FROM SCHEMA_MIGRATIONS") {
		vs := make([]int, 0, len(c.db.applied))
		for v := range c.db.applied {
			vs = append(vs, v)
		}
		sort.Ints(vs)
		return &stubRows{versions: vs}, nil
	}
	return &stubRows{}, nil
}

type stubStmt struct{ query string }

func (s *stubStmt) Close() error                               { return nil }
func (s *stubStmt) NumInput() int                              { return -1 }
func (s *stubStmt) Exec([]driver.Value) (driver.Result, error) { return nil, io.EOF }
func (s *stubStmt) Query([]driver.Value) (driver.Rows, error)  { return nil, io.EOF }

type stubTx struct{}

func (t *stubTx) Commit() error   { return nil }
func (t *stubTx) Rollback() error { return nil }

type stubRows struct {
	versions []int
	idx      int
	maxOnly  bool
	maxVal   int
}

func (r *stubRows) Columns() []string { return []string{"version"} }
func (r *stubRows) Close() error      { return nil }
func (r *stubRows) Next(dest []driver.Value) error {
	if r.maxOnly {
		if r.idx > 0 {
			return io.EOF
		}
		dest[0] = int64(r.maxVal)
		r.idx++
		return nil
	}
	if r.idx >= len(r.versions) {
		return io.EOF
	}
	dest[0] = int64(r.versions[r.idx])
	r.idx++
	return nil
}

var stub = newStubDB()

func TestMain(m *testing.M) {
	sql.Register("stub", &stubDriver{db: stub})
	os.Exit(m.Run())
}

// --- framework unit tests ---

func TestValidateMigrations(t *testing.T) {
	good := []Migration{
		{Version: 1, Name: "a", SQL: "CREATE TABLE a(x);"},
		{Version: 2, Name: "b", SQL: "CREATE TABLE b(x);"},
	}
	if err := ValidateMigrations(good); err != nil {
		t.Fatalf("valid migrations rejected: %v", err)
	}
	dup := []Migration{{Version: 1, Name: "a", SQL: "x;"}, {Version: 1, Name: "b", SQL: "y;"}}
	if err := ValidateMigrations(dup); err == nil {
		t.Error("duplicate versions should fail")
	}
	ooo := []Migration{{Version: 2, Name: "b", SQL: "y;"}, {Version: 1, Name: "a", SQL: "x;"}}
	if err := ValidateMigrations(ooo); err == nil {
		t.Error("out-of-order versions should fail")
	}
	if err := ValidateMigrations([]Migration{{Version: 0, Name: "a", SQL: "x;"}}); err == nil {
		t.Error("zero version should fail")
	}
	if err := ValidateMigrations([]Migration{{Version: 1, Name: " ", SQL: "x;"}}); err == nil {
		t.Error("empty name should fail")
	}
	if err := ValidateMigrations([]Migration{{Version: 1, Name: "a", SQL: "  "}}); err == nil {
		t.Error("empty SQL should fail")
	}
}

func TestPendingMigrations(t *testing.T) {
	all := []Migration{{Version: 1, Name: "a", SQL: "x;"}, {Version: 2, Name: "b", SQL: "y;"}, {Version: 3, Name: "c", SQL: "z;"}}
	got := PendingMigrations(map[int]bool{2: true}, all)
	if len(got) != 2 || got[0].Version != 1 || got[1].Version != 3 {
		t.Fatalf("pending = %+v, want [1 3]", got)
	}
	if got := PendingMigrations(map[int]bool{1: true, 2: true, 3: true}, all); len(got) != 0 {
		t.Fatalf("pending should be empty, got %+v", got)
	}
}

func TestSplitStatements(t *testing.T) {
	got := splitStatements("CREATE TABLE a(x);\nCREATE TABLE b(y);")
	if len(got) != 2 {
		t.Fatalf("split = %d statements, want 2: %v", len(got), got)
	}
	// Quoted semicolons must not split.
	got = splitStatements("INSERT INTO t VALUES ('a;b');")
	if len(got) != 1 || !strings.Contains(got[0], "'a;b'") {
		t.Fatalf("quoted semicolon mishandled: %v", got)
	}
	// Trailing whitespace-only statement ignored.
	got = splitStatements("CREATE TABLE a(x);   ")
	if len(got) != 1 {
		t.Fatalf("split = %v, want 1", got)
	}
}

// --- end-to-end migration apply against the stub driver ---

func TestMigratorApplyWithStub(t *testing.T) {
	stub.reset()
	db, err := sql.Open("stub", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	m := NewMigrator(db)
	if err := m.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	v, err := m.CurrentVersion(ctx)
	if err != nil || v != 1 {
		t.Fatalf("CurrentVersion = %d, %v; want 1", v, err)
	}
	if !stub.tables["tasks"] || !stub.tables["meta"] || !stub.tables["schema_migrations"] {
		t.Fatalf("tables created: %v", stub.tables)
	}
	if !stub.executed("CREATE TABLE IF NOT EXISTS tasks") {
		t.Errorf("tasks DDL not executed; execs: %v", stub.execs)
	}
	applied, err := m.appliedVersions(ctx)
	if err != nil || len(applied) != 1 || !applied[1] {
		t.Fatalf("applied versions = %v, %v", applied, err)
	}

	// Apply is idempotent: no duplicate version records, no re-execution of
	// migration statements. (The schema_migrations guard DDL may re-run; that
	// is intentional and harmless.)
	if err := m.Apply(ctx); err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	applied, err = m.appliedVersions(ctx)
	if err != nil || len(applied) != 1 {
		t.Fatalf("applied after second Apply = %v, %v", applied, err)
	}
	if n := stub.countExecs("INSERT INTO SCHEMA_MIGRATIONS"); n != 1 {
		t.Errorf("schema_migrations inserts = %d, want 1 (idempotency broken)", n)
	}
	if n := stub.countExecs("CREATE TABLE IF NOT EXISTS TASKS"); n != 1 {
		t.Errorf("tasks DDL executions = %d, want 1 (idempotency broken)", n)
	}
}

func (d *stubDB) executed(prefix string) bool {
	return d.countExecs(prefix) > 0
}

func (d *stubDB) countExecs(prefix string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for _, e := range d.execs {
		if strings.Contains(strings.ToUpper(e), strings.ToUpper(prefix)) {
			n++
		}
	}
	return n
}

func TestMigratorRejectsBadMigrations(t *testing.T) {
	stub.reset()
	db, err := sql.Open("stub", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m := &Migrator{DB: db, Migrations: []Migration{{Version: 2, Name: "b", SQL: "x;"}, {Version: 1, Name: "a", SQL: "y;"}}}
	if err := m.Apply(context.Background()); err == nil {
		t.Fatal("Apply should reject out-of-order migrations")
	}
}

// --- real SQLite smoke test (skips when the driver is unavailable offline) ---

func TestMigrateRealSQLite(t *testing.T) {
	if !slices.Contains(sql.Drivers(), "sqlite") {
		t.Skip("sqlite driver is not registered (offline environment); install modernc.org/sqlite to run the real-database smoke test")
	}
	dsn := filepath.Join(t.TempDir(), "app.db")
	db, err := Migrate("sqlite", dsn)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	var table string
	if err := db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='tasks'").Scan(&table); err != nil || table != "tasks" {
		t.Fatalf("tasks table missing: %q, %v", table, err)
	}
	// A task row round-trips through the schema.
	if _, err := db.ExecContext(ctx,
		"INSERT INTO tasks (id, kind, provider_params, expected_call_count, status, progress, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		"t1", "generation", `{"prompt":"x"}`, 1, "queued", 0.0, "now", "now"); err != nil {
		t.Fatalf("insert task: %v", err)
	}
	var status string
	var count int
	if err := db.QueryRowContext(ctx, "SELECT status, expected_call_count FROM tasks WHERE id = ?", "t1").Scan(&status, &count); err != nil {
		t.Fatalf("read task: %v", err)
	}
	if status != "queued" || count != 1 {
		t.Errorf("task row = %q/%d", status, count)
	}
}
