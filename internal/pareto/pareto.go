// Package pareto aggregates normalized metric comparisons.
package pareto

// Metric describes whether a candidate metric is better, worse, or effectively the same.
type Metric string

const (
	// Better means the candidate metric improved.
	Better Metric = "better"
	// Worse means the candidate metric regressed.
	Worse Metric = "worse"
	// Same means the candidate metric has no meaningful change.
	Same Metric = "same"
)

// Relation describes the aggregate relation between a baseline and candidate.
type Relation string

const (
	// CandidateWins means at least one metric improved and no metric regressed.
	CandidateWins Relation = "candidate-wins"
	// BaselineWins means at least one metric regressed and no metric improved.
	BaselineWins Relation = "baseline-wins"
	// Tie means no metric meaningfully changed.
	Tie Relation = "tie"
	// TradeOff means improvements and regressions coexist.
	TradeOff Relation = "trade-off"
	// Inconclusive means there are no comparable metrics.
	Inconclusive Relation = "inconclusive"
)

// Compare aggregates normalized metrics using Pareto-style rules.
func Compare(metrics ...Metric) Relation {
	if len(metrics) == 0 {
		return Inconclusive
	}

	better, worse := false, false

	for _, metric := range metrics {
		switch metric {
		case Better:
			better = true
		case Worse:
			worse = true
		case Same:
			continue
		}
	}

	return comparePresence(better, worse)
}

func comparePresence(better, worse bool) Relation {
	switch {
	case better && !worse:
		return CandidateWins
	case worse && !better:
		return BaselineWins
	case !better && !worse:
		return Tie
	default:
		return TradeOff
	}
}
