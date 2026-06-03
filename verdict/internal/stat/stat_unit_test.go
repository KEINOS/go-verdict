package stat_test

import (
	"testing"

	"github.com/KEINOS/go-verdict/verdict/internal/stat"
	"github.com/stretchr/testify/require"
)

const statisticalMinSamples = 2

func TestMean(t *testing.T) {
	t.Parallel()

	got := stat.Mean([]float64{1, 2, 3})

	require.InDelta(t, 2.0, got, 1e-9)
}

func TestVariance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []float64
		mean float64
		want float64
	}{
		{name: "single sample", in: []float64{1}, mean: 1, want: 0},
		{name: "sample variance uses n minus one", in: []float64{1, 2, 3}, mean: 2, want: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := stat.Variance(test.in, test.mean, statisticalMinSamples)

			require.InDelta(t, test.want, got, 1e-9)
		})
	}
}

func TestDeltaPercent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		baselineMean  float64
		candidateMean float64
		want          float64
	}{
		{name: "zero baseline", baselineMean: 0, candidateMean: 10, want: 0},
		{name: "candidate lower", baselineMean: 100, candidateMean: 90, want: -10},
		{name: "candidate higher", baselineMean: 100, candidateMean: 125, want: 25},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := stat.DeltaPercent(test.baselineMean, test.candidateMean)

			require.InDelta(t, test.want, got, 1e-9)
		})
	}
}

func TestPValueApproximation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		baseline  []float64
		candidate []float64
		want      float64
	}{
		{name: "identical zero variance", baseline: []float64{1, 1, 1}, candidate: []float64{1, 1, 1}, want: 1},
		{name: "different zero variance", baseline: []float64{1, 1, 1}, candidate: []float64{2, 2, 2}, want: 0},
		{
			name:      "variable samples",
			baseline:  []float64{10, 12, 11},
			candidate: []float64{8, 9, 8.5},
			want:      0.00010751117672950157,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := stat.PValueApproximation(test.baseline, test.candidate, statisticalMinSamples)

			require.InDelta(t, test.want, got, 1e-15)
		})
	}
}

func TestPValueApproximationIsSymmetric(t *testing.T) {
	t.Parallel()

	baseline := []float64{10, 12, 11}
	candidate := []float64{8, 9, 8.5}

	forward := stat.PValueApproximation(baseline, candidate, statisticalMinSamples)
	reverse := stat.PValueApproximation(candidate, baseline, statisticalMinSamples)

	require.InDelta(t, forward, reverse, 1e-15)
}
