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

// Handler is a package-level function literal. The compiler names it after the
// package initializer, so it has no source-level pprof symbol and the analyzer
// skips it.
var Handler = func(value int) int {
	if value > 0 {
		return value
	}

	return -value
}

// Box is a generic receiver fixture.
type Box[T any] struct {
	item T
}

// Get uses a generic receiver, whose type parameters are dropped from the name.
func (b Box[T]) Get() T {
	return b.item
}

// Pair has a receiver with two type parameters.
type Pair[K comparable, V any] struct {
	key   K
	value V
}

// Key exercises a multi-parameter generic receiver.
func (p Pair[K, V]) Key() K {
	return p.key
}

func init() {
	_ = Simple()
}

func init() {
	if Simple() > 1 {
		_ = Simple()
	}
}
