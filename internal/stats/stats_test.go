package stats

import "testing"

func TestSummarizeConvertsNsToUs(t *testing.T) {
	s, err := Summarize([]int64{1000, 2000, 3000, 4000})
	if err != nil {
		t.Fatal(err)
	}
	if s.AvgUS != 2.5 || s.MinUS != 1.0 || s.MaxUS != 4.0 {
		t.Fatalf("unexpected summary: %+v", s)
	}
}

func TestPercentileNearestRank(t *testing.T) {
	vals := make([]float64, 100)
	for i := range vals {
		vals[i] = float64(i + 1)
	}
	p, _ := Percentile(vals, 99)
	if p != 99 {
		t.Fatalf("want 99, got %v", p)
	}
	p2, _ := Percentile([]float64{5, 1, 3}, 99) // few samples -> max
	if p2 != 5 {
		t.Fatalf("want 5, got %v", p2)
	}
}

func TestSummarizeRejectsEmpty(t *testing.T) {
	if _, err := Summarize(nil); err != ErrNoValues {
		t.Fatalf("want ErrNoValues, got %v", err)
	}
}
