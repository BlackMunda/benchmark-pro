// Package spec loads and validates a benchmark spec (YAML).
package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/goccy/go-yaml"
)

const SupportedVersion = 1

var supportedEngines = map[string]bool{"sqlite": true, "postgres": true}
var nameRE = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// SpecError is a validation error in the spec file; the CLI maps it to exit code 2.
type SpecError struct{ msg string }

func (e *SpecError) Error() string { return e.msg }

func errf(format string, a ...any) error {
	return &SpecError{fmt.Sprintf(format, a...)}
}

type Benchmark struct {
	Name     string
	SQL      string
	Params   []any
	Runs     int
	Warmup   int
	Rollback bool
}

type DatabaseConfig struct {
	Engine string
	DSN    string
	Schema string // absolute path, resolved relative to the spec file; "" if unset
}

type Spec struct {
	Version    int
	Database   DatabaseConfig
	Benchmarks []Benchmark
}

// rawSpec/rawDatabase/rawBenchmark mirror the YAML shape for decoding.
type rawSpec struct {
	Version    int            `yaml:"version"`
	Database   rawDatabase    `yaml:"database"`
	Benchmarks []rawBenchmark `yaml:"benchmarks"`
}

type rawDatabase struct {
	Engine string `yaml:"engine"`
	DSN    string `yaml:"dsn"`
	Schema string `yaml:"schema"`
}

type rawBenchmark struct {
	Name     string `yaml:"name"`
	SQL      string `yaml:"sql"`
	Params   []any  `yaml:"params"`
	Runs     *int   `yaml:"runs"`
	Warmup   *int   `yaml:"warmup"`
	Rollback *bool  `yaml:"rollback"`
}

func Load(path string) (*Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errf("spec file not found: %s", path)
		}
		return nil, err
	}

	var raw rawSpec
	// Defaults applied before decode; go-yaml overwrites on the fields it finds.
	raw.Version = SupportedVersion
	raw.Database.Engine = "sqlite"
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, errf("invalid YAML in %s: %v", path, err)
	}

	if raw.Version != SupportedVersion {
		return nil, errf("unsupported spec version: %d", raw.Version)
	}
	if !supportedEngines[raw.Database.Engine] {
		return nil, errf("engine must be one of [sqlite postgres], got %q", raw.Database.Engine)
	}
	if len(raw.Benchmarks) == 0 {
		return nil, errf("'benchmarks' must be a non-empty list")
	}

	database := DatabaseConfig{Engine: raw.Database.Engine, DSN: raw.Database.DSN}
	if raw.Database.Schema != "" {
		database.Schema = filepath.Join(filepath.Dir(path), raw.Database.Schema)
	}

	seen := map[string]bool{}
	benchmarks := make([]Benchmark, 0, len(raw.Benchmarks))
	for i, item := range raw.Benchmarks {
		where := fmt.Sprintf("benchmarks[%d]", i)
		if item.Name == "" || !nameRE.MatchString(item.Name) {
			return nil, errf("%s.name must match [A-Za-z0-9_.-]+", where)
		}
		if seen[item.Name] {
			return nil, errf("duplicate benchmark name: %s", item.Name)
		}
		seen[item.Name] = true
		if item.SQL == "" {
			return nil, errf("%s: 'sql' is required", item.Name)
		}

		runs := 10
		if item.Runs != nil {
			runs = *item.Runs
		}
		if runs < 1 {
			return nil, errf("%s: 'runs' must be an integer >= 1", item.Name)
		}

		warmup := 1
		if item.Warmup != nil {
			warmup = *item.Warmup
		}
		if warmup < 0 {
			return nil, errf("%s: 'warmup' must be an integer >= 0", item.Name)
		}

		rollback := true
		if item.Rollback != nil {
			rollback = *item.Rollback
		}

		benchmarks = append(benchmarks, Benchmark{
			Name: item.Name, SQL: item.SQL, Params: item.Params,
			Runs: runs, Warmup: warmup, Rollback: rollback,
		})
	}

	return &Spec{Version: raw.Version, Database: database, Benchmarks: benchmarks}, nil
}
