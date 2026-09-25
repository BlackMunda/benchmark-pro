# BenchmarkPro (Go)

Auto-run database query benchmarks, compare them to a baseline, and block slow PRs.
Go port of the Python `benchmark-pro` project — same spec format, same JSON result
schema, so either can produce a report the other's baseline comparison could read.

**Status: V1, weeks 1-2 (benchmark framework).** GitHub integration (weeks 3-4),
data generation, baseline comparison and status checks come next. See `docs/DESIGN.md`.

## Dependencies

Three GitHub-hosted modules, no Go-proxy or gopkg.in access needed:

- `github.com/mattn/go-sqlite3` — SQLite driver (cgo; needs gcc)
- `github.com/lib/pq` — Postgres driver (pure Go)
- `github.com/goccy/go-yaml` — YAML parsing

## Quick start (SQLite, no Docker)

```bash
go build -o benchmark-pro ./cmd/benchmark-pro
./benchmark-pro run --init --out results.json
go test ./...
```

## Postgres

```bash
docker compose up -d
./benchmark-pro run --init --engine postgres \
  --dsn "postgres://bench:bench@localhost:5433/bench?sslmode=disable" --out results.json
```

## Commands

| Command | What it does |
|---|---|
| `benchmark-pro run` | Run every benchmark in the spec, print a table, optionally write JSON (`--out`) |
| `benchmark-pro init-db` | Reset the schema and load deterministic seed data |

Flags: `--spec` (default `benchmarks.yaml`), `--engine`, `--dsn` (or `$BENCH_DSN`), `--init` (run only).
Exit codes: `0` ok, `1` a benchmark errored, `2` invalid spec/config.

## Writing benchmarks

See `benchmarks.yaml` — identical format to the Python version. Use `%s` placeholders for
every engine; the adapter rewrites them to `?` (SQLite) or `$1, $2, ...` (Postgres).

| Field | Default | Meaning |
|---|---|---|
| `name` | required | Unique id, `[A-Za-z0-9_.-]+`. Used to match against the baseline |
| `sql` | required | Query text |
| `params` | `[]` | Values for the placeholders |
| `runs` | `10` | Measured runs |
| `warmup` | `1` | Discarded runs before measuring |
| `rollback` | `true` | Roll back after every run so INSERT/DELETE never change the data |

## Layout

```
internal/spec/       YAML spec loader + validation
internal/dbadapter/  database/sql wrapper (sqlite3 + postgres), seeding, timed queries
internal/runner/     runs benchmarks, collects timings
internal/stats/      avg/min/max/p99/stdev
internal/report/     JSON result (same schema as the Python version)
cmd/benchmark-pro/   CLI
db/schema.sql, docker-compose.yml, benchmarks.yaml   shared with the Python version
```

## Differences from the Python version

- `"go_version"` in the report instead of `"python"` — everything else in the schema is identical.
- Cross-compiling needs `CGO_ENABLED=1` and a C toolchain for the target, because the SQLite
  driver uses cgo. Postgres-only builds can set `CGO_ENABLED=0`.
