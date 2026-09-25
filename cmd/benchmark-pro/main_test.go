package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestCLIRunEndToEnd builds the binary and runs it against a temp SQLite DB,
// exercising spec loading, DB init, seeding, benchmarking, and the JSON report.
func TestCLIRunEndToEnd(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "benchmark-pro")
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Env = os.Environ()
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	outJSON := filepath.Join(dir, "results.json")
	dsn := filepath.Join(dir, "t.db")
	cmd := exec.Command(binPath, "run", "--spec", "../../benchmarks.yaml",
		"--dsn", dsn, "--init", "--out", outJSON)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run failed: %v\n%s", err, out)
	}

	data, err := os.ReadFile(outJSON)
	if err != nil {
		t.Fatal(err)
	}
	var rep struct {
		SchemaVersion int `json:"schema_version"`
		Benchmarks    []struct {
			Name      string    `json:"name"`
			SamplesUS []float64 `json:"samples_us"`
			MinUS     float64   `json:"min_us"`
			AvgUS     float64   `json:"avg_us"`
			MaxUS     float64   `json:"max_us"`
		} `json:"benchmarks"`
	}
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatal(err)
	}
	if rep.SchemaVersion != 1 {
		t.Fatalf("want schema_version 1, got %d", rep.SchemaVersion)
	}
	want := map[string]bool{"user_lookup_by_email": true, "logs_for_user": true,
		"batch_delete_logs": true, "create_user": true}
	if len(rep.Benchmarks) != len(want) {
		t.Fatalf("want %d benchmarks, got %d", len(want), len(rep.Benchmarks))
	}
	for _, b := range rep.Benchmarks {
		if !want[b.Name] {
			t.Errorf("unexpected benchmark: %s", b.Name)
		}
		if len(b.SamplesUS) != 10 {
			t.Errorf("%s: want 10 samples, got %d", b.Name, len(b.SamplesUS))
		}
		if !(b.MinUS <= b.AvgUS && b.AvgUS <= b.MaxUS) {
			t.Errorf("%s: min/avg/max out of order: %v/%v/%v", b.Name, b.MinUS, b.AvgUS, b.MaxUS)
		}
	}
}

func TestCLIMissingSpecExits2(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "benchmark-pro")
	build := exec.Command("go", "build", "-o", binPath, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	cmd := exec.Command(binPath, "run", "--spec", filepath.Join(dir, "missing.yaml"))
	err := cmd.Run()
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 2 {
		t.Fatalf("want exit code 2, got err=%v", err)
	}
}
