package dbadapter

import "benchmarkpro/internal/sampledata"

// Seed inserts deterministic, realistic test data (weeks 5-6's generator —
// see internal/sampledata). Kept as a thin wrapper so callers that only care
// about "give me N users and M logs" don't need to know about sampledata.Config.
func (a *Adapter) Seed(users, logs int) error {
	return a.SeedWith(sampledata.Config{Users: users, Logs: logs, Seed: sampledata.DefaultConfig().Seed})
}

// SeedWith generates data with sampledata.Generate and inserts it. Exposed
// separately from Seed so callers can override the random seed (e.g. the CLI's
// --seed flag) without changing Seed's simple two-int signature.
func (a *Adapter) SeedWith(cfg sampledata.Config) error {
	users, logs := sampledata.Generate(cfg)

	userRows := make([][]any, len(users))
	for i, u := range users {
		userRows[i] = []any{u.ID, u.Email, u.Name, u.CreatedAt}
	}
	if err := a.ExecuteMany(
		"INSERT INTO users (id, email, name, created_at) VALUES (%s, %s, %s, %s)", userRows,
	); err != nil {
		return err
	}

	logRows := make([][]any, len(logs))
	for i, l := range logs {
		logRows[i] = []any{l.ID, l.UserID, l.Message, l.CreatedAt}
	}
	return a.ExecuteMany(
		"INSERT INTO logs (id, user_id, message, created_at) VALUES (%s, %s, %s, %s)", logRows,
	)
}
