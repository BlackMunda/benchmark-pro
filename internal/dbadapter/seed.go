package dbadapter

import (
	"fmt"
	"time"
)

var seedBase = time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

// Seed inserts deterministic rows (weeks 5-6 replace this with the real test
// data generator). The same inputs always produce identical rows.
func (a *Adapter) Seed(users, logs int) error {
	userRows := make([][]any, users)
	for i := 1; i <= users; i++ {
		userRows[i-1] = []any{
			i, fmt.Sprintf("user%d@example.com", i), fmt.Sprintf("User %d", i),
			seedBase.Add(time.Duration(i) * time.Hour).Format("2006-01-02 15:04:05"),
		}
	}
	if err := a.ExecuteMany(
		"INSERT INTO users (id, email, name, created_at) VALUES (%s, %s, %s, %s)", userRows,
	); err != nil {
		return err
	}

	logRows := make([][]any, logs)
	for i := 1; i <= logs; i++ {
		logRows[i-1] = []any{
			i, (i % users) + 1, fmt.Sprintf("event %d", i),
			seedBase.Add(time.Duration(i*4) * time.Hour).Format("2006-01-02 15:04:05"),
		}
	}
	return a.ExecuteMany(
		"INSERT INTO logs (id, user_id, message, created_at) VALUES (%s, %s, %s, %s)", logRows,
	)
}
