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

## Baseline comparison (mid-term addition)

Compare any run against a saved baseline and flag regressions — the local
engine behind "block the merge if a query is more than 10% slower":

```bash
# 1. capture a baseline (e.g. on main, or the last known-good commit)
./benchmark-pro run --init --out baseline.json

# 2. after code/schema changes, compare against it
./benchmark-pro run --out results.json --baseline baseline.json --fail-on-regression
```

Every query gets a verdict — `FASTER`, `NO CHANGE` (within 1% noise),
`SLOWER` (slower but within the threshold), `REGRESSION` (more than the
threshold slower), plus `NEW`/`MISSING` for queries that only exist on one
side. Example output:

```
benchmark                     baseline    current   change  verdict
------------------------------------------------------------------
user_lookup_by_email            45.0us     42.0us    -6.7%  FASTER
batch_delete_logs                5.00ms     5.50ms  +10.0%  SLOWER
create_user                       100us      100us    +0.0%  NO CHANGE
Overall: ✅ PASS (no regressions)
```

With `--fail-on-regression`, a `REGRESSION` verdict makes the CLI exit with
code 3 — that is the hook CI will later use to block the merge. The
`--threshold` flag overrides the default 10% rule, and
`compare.FormatMarkdown` renders the same result as the future PR comment.

## Commands

| Command | What it does |
|---|---|
| `benchmark-pro run` | Run every benchmark in the spec, print a table, optionally write JSON (`--out`) |
| `benchmark-pro init-db` | Reset the schema and load deterministic seed data |

Flags: `--spec` (default `benchmarks.yaml`), `--engine`, `--dsn` (or `$BENCH_DSN`), `--init` (run only), `--out`, `--baseline`, `--threshold` (default 10), `--fail-on-regression`.
Exit codes: `0` ok, `1` a benchmark errored, `2` invalid spec/config, `3` regression detected (only with `--fail-on-regression`).
