// Package stat provides internal statistical helpers for raw benchmark comparisons.
package stat

import "math"

const percentScale = 100

// Mean returns the arithmetic mean of values.
func Mean(values []float64) float64 {
	// Callers enforce non-empty sample sets before statistical comparison.
	total := 0.0
	for _, value := range values {
		total += value
	}

	return total / float64(len(values))
}

// Variance returns the sample variance for values.
func Variance(values []float64, sampleMean float64, minSamples int) float64 {
	if len(values) < minSamples {
		return 0
	}

	sum := 0.0

	for _, value := range values {
		diff := value - sampleMean
		sum += diff * diff
	}

	return sum / float64(len(values)-1)
}

// DeltaPercent returns the candidate change relative to the baseline mean.
func DeltaPercent(baselineMean, candidateMean float64) float64 {
	if baselineMean == 0 {
		return 0
	}

	return ((candidateMean - baselineMean) / baselineMean) * percentScale
}

// PValueApproximation uses a pragmatic normal approximation from repeated
// samples instead of a heavier statistics dependency. It is most useful with
// the recommended raw sample count or more.
func PValueApproximation(baseline, candidate []float64, minSamples int) float64 {
	baselineMean := Mean(baseline)
	candidateMean := Mean(candidate)
	baselineVariance := Variance(baseline, baselineMean, minSamples)
	candidateVariance := Variance(candidate, candidateMean, minSamples)
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
