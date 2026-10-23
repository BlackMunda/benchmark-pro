package sampledata

import (
	"reflect"
	"testing"
)

func TestGenerateCountsMatchConfig(t *testing.T) {
	users, logs := Generate(Config{Users: 100, Logs: 500, Seed: 1})
	if len(users) != 100 {
		t.Fatalf("want 100 users, got %d", len(users))
	}
	if len(logs) != 500 {
		t.Fatalf("want 500 logs, got %d", len(logs))
	}
}

func TestGenerateIsDeterministic(t *testing.T) {
	cfg := Config{Users: 50, Logs: 200, Seed: 7}
	u1, l1 := Generate(cfg)
	u2, l2 := Generate(cfg)
	if !reflect.DeepEqual(u1, u2) {
		t.Fatal("users differ between two generations with the same seed")
	}
	if !reflect.DeepEqual(l1, l2) {
		t.Fatal("logs differ between two generations with the same seed")
	}
}

func TestDifferentSeedsDiffer(t *testing.T) {
	u1, _ := Generate(Config{Users: 50, Logs: 10, Seed: 1})
	u2, _ := Generate(Config{Users: 50, Logs: 10, Seed: 2})
	if reflect.DeepEqual(u1, u2) {
		t.Fatal("different seeds produced identical users — Seed isn't wired into the RNG")
	}
}

func TestEmailsAreUnique(t *testing.T) {
	users, _ := Generate(Config{Users: 2000, Logs: 1, Seed: 42})
	seen := make(map[string]bool, len(users))
	for _, u := range users {
		if seen[u.Email] {
			t.Fatalf("duplicate email: %s", u.Email)
		}
		seen[u.Email] = true
	}
}

func TestLogsReferenceValidUsers(t *testing.T) {
	users, logs := Generate(Config{Users: 30, Logs: 1000, Seed: 3})
	for _, l := range logs {
		if l.UserID < 1 || l.UserID > len(users) {
			t.Fatalf("log %d references out-of-range user_id %d (have %d users)", l.ID, l.UserID, len(users))
		}
	}
}

func TestLogDistributionIsUneven(t *testing.T) {
	// A Zipf-like distribution should NOT spread logs evenly across users —
	// this is the whole point of using Zipf instead of i % users.
	_, logs := Generate(Config{Users: 100, Logs: 5000, Seed: 9})
	counts := map[int]int{}
	for _, l := range logs {
		counts[l.UserID]++
	}
	max := 0
	for _, c := range counts {
		if c > max {
			max = c
		}
	}
	avg := float64(len(logs)) / float64(len(counts))
	if float64(max) < avg*2 {
		t.Fatalf("distribution looks too flat: busiest user has %d logs, average is %.1f", max, avg)
	}
}

func TestDefaultConfigMatchesBrief(t *testing.T) {
	d := DefaultConfig()
	if d.Users != 10_000 || d.Logs != 50_000 {
		t.Fatalf("want 10,000 users / 50,000 logs per the V1 brief, got %d/%d", d.Users, d.Logs)
	}
}
