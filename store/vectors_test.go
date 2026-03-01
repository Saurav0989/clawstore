package store

import "testing"

func TestNormalizeFTSRank(t *testing.T) {
	minRank := -5.0
	maxRank := -1.0

	cases := []struct {
		rank float64
		want float64
	}{
		{rank: -5.0, want: 1.0},
		{rank: -3.0, want: 0.5},
		{rank: -1.0, want: 0.0},
	}

	for _, tc := range cases {
		got := normalizeFTSRank(tc.rank, minRank, maxRank)
		if diff := got - tc.want; diff < -0.00001 || diff > 0.00001 {
			t.Fatalf("normalizeFTSRank(%v) = %v, want %v", tc.rank, got, tc.want)
		}
	}
}

func TestNormalizeVecDistance(t *testing.T) {
	cases := []struct {
		distance float64
		max      float64
		want     float64
	}{
		{distance: 0, max: 2, want: 1},
		{distance: 1, max: 2, want: 0.5},
		{distance: 2, max: 2, want: 0},
		{distance: 0.7, max: 0.7, want: 0},
		{distance: 0.2, max: 0, want: 1},
	}

	for _, tc := range cases {
		got := normalizeVecDistance(tc.distance, tc.max)
		if diff := got - tc.want; diff < -0.00001 || diff > 0.00001 {
			t.Fatalf("normalizeVecDistance(%v, %v) = %v, want %v", tc.distance, tc.max, got, tc.want)
		}
	}
}
