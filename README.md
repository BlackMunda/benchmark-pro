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
