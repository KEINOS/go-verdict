package dedupe

import (
	"fmt"
	"slices"
	"testing"
)

func TestUniqueStable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "keeps first occurrence order",
			input: []string{"b", "a", "b", "c", "a", "d"},
			want:  []string{"b", "a", "c", "d"},
		},
		{
			name:  "handles empty strings",
			input: []string{"", "a", "", "b"},
			want:  []string{"", "a", "b"},
		},
		{
			name:  "nil input returns empty result",
			input: nil,
			want:  []string{},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := UniqueStable(test.input)
			if !slices.Equal(got, test.want) {
				t.Fatalf("UniqueStable() = %v, want %v", got, test.want)
			}
		})
	}
}

var benchmarkSink []string

func BenchmarkUniqueStable(b *testing.B) {
	input := make([]string, 0, 4096)
	for i := range 4096 {
		input = append(input, fmt.Sprintf("item-%03d", i%512))
	}

	b.Run("original", func(b *testing.B) {
		for b.Loop() {
			benchmarkSink = uniqueStableOriginal(input)
		}
	})

	b.Run("enhanced", func(b *testing.B) {
		for b.Loop() {
			benchmarkSink = UniqueStable(input)
		}
	})
}

func uniqueStableOriginal(items []string) []string {
	var result []string
	for _, item := range items {
		seen := false
		for _, existing := range result {
			if existing == item {
				seen = true
				break
			}
		}
		if !seen {
			result = append(result, item)
		}
	}
	return result
}
