// Package dbadapter runs SQL against SQLite or Postgres through database/sql,
// timing each query inside a transaction that is rolled back by default.
package dbadapter

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"           // postgres driver
	_ "github.com/mattn/go-sqlite3" // sqlite driver (cgo)
)

type Adapter struct {
	db     *sql.DB
	engine string
}

func Open(engine, dsn string) (*Adapter, error) {
	driver := map[string]string{"sqlite": "sqlite3", "postgres": "postgres"}[engine]
	if driver == "" {
		return nil, fmt.Errorf("unknown engine: %s", engine)
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	if engine == "sqlite" {
		db.SetMaxOpenConns(1) // avoid "database is locked" from concurrent connections
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return &Adapter{db: db, engine: engine}, nil
}

func (a *Adapter) Close() error { return a.db.Close() }

// QueryInt runs a scalar query outside of any benchmark transaction — used by
// tests and tooling to check row counts, not part of the timed path.
func (a *Adapter) QueryInt(sqlText string, params ...any) (int64, error) {
	var n int64
	err := a.db.QueryRow(a.placeholders(sqlText), params...).Scan(&n)
	return n, err
}

func splitStatements(script string) []string {
	var out []string
	for _, s := range strings.Split(script, ";") {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func (a *Adapter) ExecuteScript(script string) error {
	for _, stmt := range splitStatements(script) {
		if _, err := a.db.Exec(stmt); err != nil {
			return fmt.Errorf("executing %q: %w", stmt, err)
		}
	}
	return nil
}

// placeholders rewrites the spec's engine-neutral %s into what each driver expects:
// sqlite3 accepts plain "?"; lib/pq requires numbered "$1, $2, ...".
func (a *Adapter) placeholders(sqlText string) string {
	if a.engine == "sqlite" {
		return strings.ReplaceAll(sqlText, "%s", "?")
	}
	n := 0
	var b strings.Builder
	for i := 0; i < len(sqlText); i++ {
		if sqlText[i] == '%' && i+1 < len(sqlText) && sqlText[i+1] == 's' {
			n++
			fmt.Fprintf(&b, "$%d", n)
			i++
		} else {
			b.WriteByte(sqlText[i])
		}
	}
	return b.String()
}

func (a *Adapter) ExecuteMany(sqlText string, rows [][]any) error {
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(a.placeholders(sqlText))
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, row := range rows {
		if _, err := stmt.Exec(row...); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// TimeQuery runs one query inside its own transaction and returns elapsed
// nanoseconds. The timer covers execute + fetching all rows, excluding
// transaction setup and the commit/rollback afterwards — same boundary as
// the Python adapters.
func (a *Adapter) TimeQuery(sqlText string, params []any, rollback bool) (int64, error) {
	tx, err := a.db.Begin()
	if err != nil {
		return 0, err
	}

	start := time.Now()
	rows, qErr := tx.Query(a.placeholders(sqlText), params...)
	if qErr == nil {
		for rows.Next() {
			// fetch all rows, matching the Python adapters' fetchall()
		}
		qErr = rows.Err()
		rows.Close()
	}
	elapsed := time.Since(start)

	if qErr != nil {
		tx.Rollback()
		return 0, qErr
	}
	if rollback {
		err = tx.Rollback()
	} else {
		err = tx.Commit()
	}
	if err != nil {
		return 0, err
	}
	return elapsed.Nanoseconds(), nil
}
