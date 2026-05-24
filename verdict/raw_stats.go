package verdict

import "math"

func mean(values []float64) float64 {
	// Callers enforce non-empty sample sets before statistical comparison.
	total := 0.0
	for _, value := range values {
		total += value
	}

	return total / float64(len(values))
}

func variance(values []float64, sampleMean float64) float64 {
	if len(values) < StatisticalMinSamples {
		return 0
	}

	sum := 0.0

	for _, value := range values {
		diff := value - sampleMean
		sum += diff * diff
	}

	return sum / float64(len(values)-1)
}

func deltaPercent(baselineMean, candidateMean float64) float64 {
	if baselineMean == 0 {
		return 0
	}

	return ((candidateMean - baselineMean) / baselineMean) * percentScale
}

// pValueApproximation uses a pragmatic normal approximation from repeated
// samples instead of a heavier statistics dependency. It is most useful with
// the recommended raw sample count or more.
func pValueApproximation(baseline, candidate []float64) float64 {
	baselineMean := mean(baseline)
	candidateMean := mean(candidate)
	baselineVariance := variance(baseline, baselineMean)
	candidateVariance := variance(candidate, candidateMean)
	standardError := math.Sqrt(
		baselineVariance/float64(len(baseline)) +
			candidateVariance/float64(len(candidate)),
	)

	if standardError == 0 {
		// Exact zero represents zero variance in both sample sets.
		if baselineMean == candidateMean {
			return 1
		}

		return 0
	}

	zScore := math.Abs((candidateMean - baselineMean) / standardError)

	return math.Erfc(zScore / math.Sqrt2)
}
