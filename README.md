# BenchmarkPro (Go)

Auto-run database query benchmarks on every PR, show results, and (once weeks 9-11 land)
block merges on regressions. Go port of the Python `benchmark-pro` project — same spec
format, same JSON result schema.

**Status: weeks 1-6 of 12.**

| Weeks | Deliverable | Status |
|---|---|---|
| 1-2 | Benchmark framework (spec, runner, stats, CLI) | ✅ Done |
| 3-4 | GitHub webhook service | ✅ Done |
| 5-6 | Realistic test data generation | ✅ Done |
| 7-8 | Query execution + timing | ✅ Already covered by weeks 1-2's runner |
| 9-10 | Baseline comparison (10% regression threshold) | ⬜ Not built — see note below |
| 11 | Merge-blocking GitHub status check | ⬜ Not built — see note below |

**Important:** the webhook server posts a comment and a status today, but it is **not yet a
regression check** — there's no saved baseline to compare against. It reports "did these
queries run, and how fast", not "did they get slower." The PR comment says this explicitly.
Don't point a real branch-protection rule at this yet; it'll always report success/failure
based only on whether queries executed without error.

## Dependencies

- `github.com/mattn/go-sqlite3` — SQLite driver (cgo; needs gcc)
- `github.com/lib/pq` — Postgres driver (pure Go)
- `github.com/goccy/go-yaml` — YAML parsing
- Everything else (webhook signature verification, the GitHub API client, the HTTP server,
  git checkout) is Go standard library only — no new dependencies for weeks 3-6.

## Part 1: the CLI (local / weeks 1-2)

```bash
go build -o benchmark-pro ./cmd/benchmark-pro
./benchmark-pro run --init --out results.json
```

New flags from weeks 5-6 (defaults match the V1 brief's "10K rows"):

| Flag | Default | Meaning |
|---|---|---|
| `--users` | `10000` | Users to seed |
| `--logs` | `50000` | Logs to seed |
| `--seed` | `42` | Random seed — same seed always produces the same data |

```bash
./benchmark-pro run --init --users 500 --logs 2000 --seed 1 --out results.json  # smaller/faster for quick checks
```

### Postgres

```bash
docker compose up -d
./benchmark-pro run --init --engine postgres \
  --dsn "postgres://bench:bench@localhost:5433/bench?sslmode=disable" --out results.json
```

## Part 2: the webhook server (weeks 3-4)

```bash
go build -o benchmark-pro-server ./cmd/benchmark-pro-server
WEBHOOK_SECRET=your-webhook-secret GITHUB_TOKEN=ghp_xxx ./benchmark-pro-server
```

Environment variables:

| Variable | Required | Meaning |
|---|---|---|
| `WEBHOOK_SECRET` | yes | Must match the secret configured on the GitHub webhook |
| `GITHUB_TOKEN` | yes | Token with `repo:status` + comment permissions |
| `PORT` | no (default `8080`) | HTTP port |
| `WORKDIR` | no (default OS temp dir) | Base directory for per-PR checkouts |

### What it does, end to end

1. GitHub POSTs a `pull_request` event to `/webhook`.
2. The signature is verified against `WEBHOOK_SECRET` (HMAC-SHA256) before the body is trusted.
3. Only `opened`, `synchronize`, `reopened` actions trigger a run; everything else is ignored (200 OK, no-op).
4. Responds `202 Accepted` immediately (GitHub expects a response within ~10s), then runs the rest in the background.
5. Sets a `pending` commit status.
6. `git clone`s the repo and fetches `refs/pull/<N>/head` — the ref GitHub always exposes for a PR's head commit, including PRs from forks.
7. Loads that commit's `benchmarks.yaml`, applies its `db/schema.sql` to a fresh **in-memory SQLite database** (isolated per run — concurrent webhook deliveries never share state), seeds it, and runs every benchmark.
8. Posts a Markdown comment with the results table and sets the final `success`/`failure` commit status.

### Setting it up against a real repo

1. **Deploy it somewhere reachable from the internet.** For local testing, use [smee.io](https://smee.io) or `ngrok http 8080` to forward a public URL to your machine.
2. **Add a webhook** on the GitHub repo: Settings → Webhooks → Add webhook. Payload URL = your server's `/webhook` endpoint, content type = `application/json`, secret = the same value as `WEBHOOK_SECRET`, events = "Pull requests" only.
3. **Generate a token** with `repo:status` scope (classic PAT) or, for a GitHub App, an installation token with Checks/Statuses + Pull requests (write) permissions.
4. **Branch protection** (optional, and not meaningful yet — see the status note above): Settings → Branches → require the `benchmark-pro` status check to pass before merging.

### Known limitations (documented, not silently ignored)

- **Postgres isn't supported by the server yet** — it refuses specs with `engine: postgres`
  with a clear error. Running an isolated Postgres database per concurrent PR needs more
  infrastructure than an in-memory SQLite file; see `docs/DESIGN.md`.
- **No regression detection yet.** The comment and status reflect only whether queries ran
  without error, not whether they got slower. That's weeks 9-11.
- **The commit-status API, not the newer Checks API**, is used for the ✅/❌ — works with a
  plain personal access token; a GitHub App could use Checks for richer UI later without
  changing anything outside `internal/ghclient`.

## Writing benchmarks

See `benchmarks.yaml` — same format as the Python version. `%s` placeholders work for every
engine; the adapter rewrites them per engine (`?` for SQLite, `$1, $2, ...` for Postgres).

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
internal/spec/        YAML spec loader + validation
internal/dbadapter/    database/sql wrapper (sqlite3 + postgres), timed queries
internal/sampledata/   realistic, deterministic seed data generator (weeks 5-6)
internal/runner/       runs benchmarks, collects timings
internal/stats/        avg/min/max/p99/stdev
internal/report/       JSON result (same schema as the Python version)
internal/webhook/      GitHub webhook signature verification + payload parsing (weeks 3-4)
internal/checkout/     fetches a PR's code via git (weeks 3-4)
internal/ghclient/     posts commit statuses + PR comments to the GitHub API (weeks 3-4)
internal/comment/      formats a report as a PR comment (weeks 3-4)
cmd/benchmark-pro/      the local CLI
cmd/benchmark-pro-server/  the webhook service
db/schema.sql, docker-compose.yml, benchmarks.yaml   shared config/data
```

(`internal/testdata` isn't used as a package name — Go's tooling always ignores directories
named `testdata`, so the generator lives in `internal/sampledata` instead.)

## Testing

```bash
go test ./...
```

42 tests total, including one true end-to-end test (`TestEndToEndWebhookRun`) that sends a
real signed HTTP request, clones a real local git fixture repo's PR ref, runs real SQL
against a real in-memory SQLite database, and checks the resulting comment/status — all
without touching the network or the real GitHub API.
