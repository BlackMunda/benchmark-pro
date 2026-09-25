// Command benchmark-pro runs database query benchmarks defined in a YAML spec.
package main

import (
	"flag"
	"fmt"
	"os"

	"benchmarkpro/internal/dbadapter"
	"benchmarkpro/internal/report"
	"benchmarkpro/internal/runner"
	"benchmarkpro/internal/spec"
)

type config struct {
	specPath string
	engine   string
	dsn      string
	out      string
	initDB   bool
}

func flagSet(c *config, withRunFlags bool) *flag.FlagSet {
	fs := flag.NewFlagSet("benchmark-pro", flag.ContinueOnError)
	fs.StringVar(&c.specPath, "spec", "benchmarks.yaml", "path to spec YAML")
	fs.StringVar(&c.engine, "engine", "", "override spec engine (sqlite|postgres)")
	fs.StringVar(&c.dsn, "dsn", "", "database DSN (or $BENCH_DSN)")
	if withRunFlags {
		fs.StringVar(&c.out, "out", "", "write JSON report to this path")
		fs.BoolVar(&c.initDB, "init", false, "reset + seed the database first")
	}
	return fs
}

func fmtUS(v float64) string {
	if v >= 1000 {
		return fmt.Sprintf("%.2fms", v/1000)
	}
	return fmt.Sprintf("%.1fus", v)
}

func formatTable(rep report.Report) string {
	header := fmt.Sprintf("%-26s%5s%11s%11s%11s%11s", "benchmark", "runs", "avg", "min", "max", "p99")
	out := header + "\n" + stringsRepeat("-", len(header))
	for _, b := range rep.Benchmarks {
		if b.Error != "" {
			out += fmt.Sprintf("\n%-26s  ERROR: %s", b.Name, b.Error)
			continue
		}
		out += fmt.Sprintf("\n%-26s%5d%11s%11s%11s%11s",
			b.Name, b.Runs, fmtUS(b.AvgUS), fmtUS(b.MinUS), fmtUS(b.MaxUS), fmtUS(b.P99US))
	}
	return out
}

func stringsRepeat(s string, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = s[0]
	}
	return string(b)
}

func connect(c config, sp *spec.Spec) (string, *dbadapter.Adapter, error) {
	engine := c.engine
	if engine == "" {
		engine = sp.Database.Engine
	}
	dsn := c.dsn
	if dsn == "" {
		dsn = os.Getenv("BENCH_DSN")
	}
	if dsn == "" {
		dsn = sp.Database.DSN
	}
	if dsn == "" {
		return "", nil, fmt.Errorf("no database DSN: set database.dsn, --dsn or $BENCH_DSN")
	}
	a, err := dbadapter.Open(engine, dsn)
	return engine, a, err
}

func initDB(a *dbadapter.Adapter, sp *spec.Spec) error {
	if sp.Database.Schema == "" {
		return fmt.Errorf("database.schema is required to initialise the database")
	}
	script, err := os.ReadFile(sp.Database.Schema)
	if err != nil {
		return err
	}
	if err := a.ExecuteScript(string(script)); err != nil {
		return err
	}
	return a.Seed(1000, 5000)
}

func run(c config) int {
	sp, err := spec.Load(c.specPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}
	engine, a, err := connect(c, sp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}
	defer a.Close()

	if c.initDB {
		if err := initDB(a, sp); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 2
		}
	}

	fmt.Fprintf(os.Stderr, "Running %d benchmark(s) on %s...\n", len(sp.Benchmarks), engine)
	results := runner.RunAll(a, sp, func(b spec.Benchmark) {
		fmt.Fprintf(os.Stderr, "  %s ...\n", b.Name)
	})

	rep := report.Build(results, engine)
	fmt.Println(formatTable(rep))

	if c.out != "" {
		if err := report.Write(rep, c.out); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 2
		}
		fmt.Printf("\nResults written to %s\n", c.out)
	}

	for _, b := range rep.Benchmarks {
		if b.Error != "" {
			return 1
		}
	}
	return 0
}

func runInitDB(c config) int {
	sp, err := spec.Load(c.specPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}
	_, a, err := connect(c, sp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}
	defer a.Close()
	if err := initDB(a, sp); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}
	fmt.Println("Database reset and seeded (1000 users, 5000 logs).")
	return 0
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: benchmark-pro <run|init-db> [flags]")
		os.Exit(2)
	}
	cmd, rest := os.Args[1], os.Args[2:]

	var c config
	fs := flagSet(&c, cmd == "run")
	if err := fs.Parse(rest); err != nil {
		os.Exit(2)
	}

	switch cmd {
	case "run":
		os.Exit(run(c))
	case "init-db":
		os.Exit(runInitDB(c))
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		os.Exit(2)
	}
}
