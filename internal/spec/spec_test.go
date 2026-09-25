package spec

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "spec.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSampleSpecLoads(t *testing.T) {
	s, err := Load("../../benchmarks.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Benchmarks) != 4 {
		t.Fatalf("want 4 benchmarks, got %d", len(s.Benchmarks))
	}
	if s.Benchmarks[0].Runs != 10 || !s.Benchmarks[0].Rollback {
		t.Fatalf("unexpected defaults: %+v", s.Benchmarks[0])
	}
}

func TestInvalidSpecs(t *testing.T) {
	cases := []string{
		"benchmarks: []",
		"benchmarks:\n  - name: \"a b\"\n    sql: SELECT 1",
		"benchmarks:\n  - name: a\n    sql: SELECT 1\n  - name: a\n    sql: SELECT 2",
		"benchmarks:\n  - name: a\n    sql: SELECT 1\n    runs: 0",
		"benchmarks:\n  - name: a",
		"version: 9\nbenchmarks:\n  - name: a\n    sql: SELECT 1",
	}
	for _, body := range cases {
		if _, err := Load(write(t, body)); err == nil {
			t.Errorf("expected error for spec:\n%s", body)
		} else if _, ok := err.(*SpecError); !ok {
			t.Errorf("expected *SpecError, got %T: %v", err, err)
		}
	}
}

func TestMissingSpecFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("expected error for missing file")
	}
}
