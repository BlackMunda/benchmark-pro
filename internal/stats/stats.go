// Package stats computes timing statistics. Inputs are nanoseconds ([]int64);
// outputs are microseconds (float64), matching the Python version's units.
package stats

import (
	"errors"
	"math"
	"sort"
)

var ErrNoValues = errors.New("no timings to summarize")

type Summary struct {
	AvgUS   float64 `json:"avg_us"`
	MinUS   float64 `json:"min_us"`
	MaxUS   float64 `json:"max_us"`
	P99US   float64 `json:"p99_us"`
	StdevUS float64 `json:"stdev_us"`
}

func round3(v float64) float64 {
	return math.Round(v*1000) / 1000
}

// Percentile uses nearest-rank. With few samples, p99 equals the max, which
// is expected — raise `runs` in the spec for a meaningful p99.
func Percentile(valuesUS []float64, p float64) (float64, error) {
	if len(valuesUS) == 0 {
		return 0, ErrNoValues
	}
	ordered := append([]float64(nil), valuesUS...)
	sort.Float64s(ordered)
	rank := int(math.Ceil(p / 100 * float64(len(ordered))))
	if rank < 1 {
		rank = 1
	}
	return ordered[rank-1], nil
}

func SamplesUS(timingsNS []int64) []float64 {
	out := make([]float64, len(timingsNS))
	for i, t := range timingsNS {
		out[i] = round3(float64(t) / 1000)
	}
	return out
}

func Summarize(timingsNS []int64) (Summary, error) {
	if len(timingsNS) == 0 {
		return Summary{}, ErrNoValues
	}
	us := SamplesUS(timingsNS)

	var sum, min, max float64
	min, max = us[0], us[0]
	for _, v := range us {
		sum += v
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	avg := sum / float64(len(us))

	var sqDiff float64
	for _, v := range us {
		d := v - avg
		sqDiff += d * d
	}
	stdev := math.Sqrt(sqDiff / float64(len(us))) // population stdev, matches Python's statistics.pstdev

	p99, err := Percentile(us, 99)
	if err != nil {
		return Summary{}, err
	}

	return Summary{
		AvgUS:   round3(avg),
		MinUS:   round3(min),
		MaxUS:   round3(max),
		P99US:   round3(p99),
		StdevUS: round3(stdev),
	}, nil
}
