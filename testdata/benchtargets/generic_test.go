package benchtargets

import "testing"

// BenchmarkClassifyGeneric drives the generic target so hotspot E2E scenarios
// see an instantiated pprof symbol.
func BenchmarkClassifyGeneric(b *testing.B) {
	values := make([]int, 0, 512)
	for index := range 512 {
		values = append(values, index-256)
	}

	b.ResetTimer()

	for range b.N {
		_ = ClassifyGeneric(values, index0IsStrict)
	}
}

const index0IsStrict = true
