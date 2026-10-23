// Package sampledata generates realistic, deterministic seed data: more users
// and logs than the weeks 1-2 placeholder, varied names/emails/timestamps,
// and an uneven (Zipf-like) distribution of logs per user, since a flat
// distribution hides index/lock-contention effects that real traffic has.
//
// "Deterministic" means: the same Config (same Seed) always produces byte-
// identical rows, so two runs of the same spec are comparable and a baseline
// saved today still matches what's generated next week.
package sampledata

import (
	"fmt"
	"math/rand"
	"time"
)

// Config controls how much data to generate and with what random seed.
type Config struct {
	Users int
	Logs  int
	Seed  int64
}

// DefaultConfig matches the V1 brief ("insert realistic data, 10K rows"):
// 10,000 users and 50,000 logs, enough for index effects to show up in
// timings without making a full run slow.
func DefaultConfig() Config {
	return Config{Users: 10_000, Logs: 50_000, Seed: 42}
}

type User struct {
	ID        int
	Email     string
	Name      string
	CreatedAt string // "2006-01-02 15:04:05", UTC
}

type Log struct {
	ID        int
	UserID    int
	Message   string
	CreatedAt string
}

// refNow is a fixed point in time, not time.Now() — using the real clock
// would make "deterministic" a lie, since every run would get a different
// date range.
var refNow = time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

var firstNames = []string{
	"Olivia", "Liam", "Emma", "Noah", "Ava", "Oliver", "Sophia", "Elijah",
	"Isabella", "James", "Mia", "Benjamin", "Charlotte", "Lucas", "Amelia",
	"Henry", "Harper", "Alexander", "Evelyn", "Mason", "Abigail", "Michael",
	"Emily", "Ethan", "Elizabeth", "Daniel", "Sofia", "Jacob", "Avery", "Logan",
}

var lastNames = []string{
	"Smith", "Johnson", "Williams", "Brown", "Jones", "Garcia", "Miller",
	"Davis", "Rodriguez", "Martinez", "Hernandez", "Lopez", "Gonzalez",
	"Wilson", "Anderson", "Thomas", "Taylor", "Moore", "Jackson", "Martin",
	"Lee", "Perez", "Thompson", "White", "Harris", "Sanchez", "Clark", "Ramirez",
}

var logTemplates = []string{
	"user logged in", "user logged out", "password reset requested",
	"profile updated", "payment processed", "payment failed",
	"email verified", "subscription renewed", "subscription cancelled",
	"item added to cart", "order placed", "order shipped", "order delivered",
	"support ticket opened", "support ticket closed", "api key generated",
	"session expired", "2fa enabled", "file uploaded", "export requested",
}

// zipfUserID picks a user id for one log row using a Zipf-like distribution
// (via math/rand's built-in Zipf generator): most logs cluster on a small
// number of "active" users rather than spreading evenly, which is closer to
// real traffic and matters for queries like logs_for_user — an even
// distribution would make every user's lookup look the same speed, which
// isn't realistic.
func zipfUserID(rng *rand.Rand, users int) int {
	if users <= 1 {
		return 1
	}
	z := rand.NewZipf(rng, 1.3, 1.0, uint64(users-1))
	return int(z.Uint64()) + 1
}

func randTimeWithin(rng *rand.Rand, start, end time.Time) time.Time {
	span := end.Sub(start)
	offset := time.Duration(rng.Int63n(int64(span)))
	return start.Add(offset)
}

// Generate produces users and logs for cfg. Calling it twice with the same
// cfg always returns identical slices (same seed -> same math/rand sequence
// -> same picks, in the same order).
func Generate(cfg Config) ([]User, []Log) {
	rng := rand.New(rand.NewSource(cfg.Seed))

	userStart := refNow.AddDate(-2, 0, 0) // users signed up over the last 2 years
	users := make([]User, cfg.Users)
	for i := 0; i < cfg.Users; i++ {
		id := i + 1
		first := firstNames[rng.Intn(len(firstNames))]
		last := lastNames[rng.Intn(len(lastNames))]
		created := randTimeWithin(rng, userStart, refNow)
		users[i] = User{
			ID:        id,
			Email:     fmt.Sprintf("%s.%s%d@example.com", lowerASCII(first), lowerASCII(last), id),
			Name:      first + " " + last,
			CreatedAt: created.Format("2006-01-02 15:04:05"),
		}
	}

	logStart := refNow.AddDate(-1, 0, 0) // logs span the last year
	logs := make([]Log, cfg.Logs)
	for i := 0; i < cfg.Logs; i++ {
		created := randTimeWithin(rng, logStart, refNow)
		logs[i] = Log{
			ID:        i + 1,
			UserID:    zipfUserID(rng, cfg.Users),
			Message:   logTemplates[rng.Intn(len(logTemplates))],
			CreatedAt: created.Format("2006-01-02 15:04:05"),
		}
	}

	return users, logs
}

func lowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}
