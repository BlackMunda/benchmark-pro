package runner

import (
	"os"
	"path/filepath"
	"testing"

	"benchmarkpro/internal/dbadapter"
	"benchmarkpro/internal/spec"
)

func newSeededAdapter(t *testing.T) *dbadapter.Adapter {
	t.Helper()
	schema, err := os.ReadFile("../../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	a, err := dbadapter.Open("sqlite", filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.ExecuteScript(string(schema)); err != nil {
		t.Fatal(err)
	}
	if err := a.Seed(100, 500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}

func count(t *testing.T, a *dbadapter.Adapter, table string) int64 {
	t.Helper()
	n, err := a.QueryInt("SELECT COUNT(*) FROM " + table)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRunCollectsRequestedSamples(t *testing.T) {
	a := newSeededAdapter(t)
	b := spec.Benchmark{Name: "q", SQL: "SELECT * FROM users WHERE email = %s",
		Params: []any{"user5@example.com"}, Runs: 7, Warmup: 2, Rollback: true}
	r := Run(a, b)
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	if len(r.Timings) != 7 {
		t.Fatalf("want 7 timings, got %d", len(r.Timings))
	}
	for _, ns := range r.Timings {
		if ns <= 0 {
			t.Fatalf("expected positive timing, got %d", ns)
		}
	}
}

func TestWritesAreRolledBack(t *testing.T) {
	a := newSeededAdapter(t)
	b := spec.Benchmark{Name: "del", SQL: "DELETE FROM logs WHERE created_at < %s",
		Params: []any{"2030-01-01"}, Runs: 3, Warmup: 0, Rollback: true}
	if r := Run(a, b); r.Err != nil {
		t.Fatal(r.Err)
	}
	if n := count(t, a, "logs"); n != 500 {
		t.Fatalf("want 500 logs after rolled-back delete, got %d", n)
	}
}

func TestWritesPersistWhenRollbackDisabled(t *testing.T) {
	a := newSeededAdapter(t)
	b := spec.Benchmark{Name: "del", SQL: "DELETE FROM logs WHERE user_id = %s",
		Params: []any{1}, Runs: 1, Warmup: 0, Rollback: false}
	if r := Run(a, b); r.Err != nil {
		t.Fatal(r.Err)
	}
	if n := count(t, a, "logs"); n >= 500 {
		t.Fatalf("want fewer than 500 logs after committed delete, got %d", n)
	}
}

func TestBadQueryIsReportedNotPanicked(t *testing.T) {
	a := newSeededAdapter(t)
	r := Run(a, spec.Benchmark{Name: "bad", SQL: "SELECT * FROM nope", Runs: 2, Warmup: 0, Rollback: true})
	if r.Err == nil {
		t.Fatal("expected error for query against missing table")
	}
	if len(r.Timings) != 0 {
		t.Fatalf("expected no timings on error, got %d", len(r.Timings))
	}
	// connection must still be usable afterwards
	if n := count(t, a, "users"); n != 100 {
		t.Fatalf("want 100 users, adapter possibly broken after error: %d", n)
	}
}

func TestSeedIsDeterministic(t *testing.T) {
	checksum := func() int64 {
		schema, _ := os.ReadFile("../../db/schema.sql")
		a, err := dbadapter.Open("sqlite", filepath.Join(t.TempDir(), "t.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer a.Close()
		if err := a.ExecuteScript(string(schema)); err != nil {
			t.Fatal(err)
		}
		if err := a.Seed(10, 20); err != nil {
			t.Fatal(err)
		}
		// SUM of every log's id * user_id is a cheap deterministic checksum
		// of both row count and content.
		n, err := a.QueryInt("SELECT SUM(id * user_id) FROM logs")
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	a, b := checksum(), checksum()
	if a != b {
		t.Fatalf("seed not deterministic: %d != %d", a, b)
	}
}
