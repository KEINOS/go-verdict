package pareto

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompare(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		metrics []Metric
		want    Relation
	}{
		{name: "empty metrics", metrics: nil, want: Inconclusive},
		{name: "all same", metrics: []Metric{Same, Same}, want: Tie},
		{name: "candidate wins", metrics: []Metric{Better, Same}, want: CandidateWins},
		{name: "baseline wins", metrics: []Metric{Worse, Same}, want: BaselineWins},
		{name: "trade-off", metrics: []Metric{Better, Worse}, want: TradeOff},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, test.want, Compare(test.metrics...))
		})
	}
}
