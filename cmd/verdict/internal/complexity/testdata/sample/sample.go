// Package sample is a parse fixture for the complexity analyzer. It is under
// testdata, so the Go toolchain never builds it.
package sample

const (
	maxTotal = 100
	minTotal = -100
)

// Worker is a receiver fixture for method symbol names.
type Worker struct {
	name string
}

// Simple stays at the base complexity of one.
func Simple() int {
	return 1
}

// Run is deliberately complex: branches, nesting, boolean operators, and a
// switch push both the cyclomatic and the cognitive score well past the
// hotspot thresholds.
func (w *Worker) Run(values []int) int {
	total := 0

	for _, value := range values {
		if value > 0 {
			if value%2 == 0 && value < maxTotal {
				total += value
			} else {
				total -= value
			}
		} else if value < -10 || value == -5 {
			total++
		}
	}

	switch {
	case total > maxTotal:
		return maxTotal
	case total < minTotal:
		return minTotal
	case total == 0:
		return len(w.name)
	}

	return total
}

// Name uses a value receiver.
func (w Worker) Name() string {
	return w.name
}

// Map is generic, so its pprof symbol carries a shape suffix.
func Map[T any](items []T, transform func(T) T) []T {
	out := make([]T, 0, len(items))

	for _, item := range items {
		out = append(out, transform(item))
	}

	return out
}
