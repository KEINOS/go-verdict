package benchtargets

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExampleSlow(t *testing.T) {
	t.Parallel()

	require.True(t, exampleSlow(),
		"exampleSlow() should return true")
}

func TestExampleFast(t *testing.T) {
	t.Parallel()

	require.True(t, exampleFast(),
		"exampleFast() should return true")
}

func TestExampleOriginal(t *testing.T) {
	t.Parallel()

	require.Equal(t, exampleOriginal(), exampleEnhanced(),
		"exampleOriginal() should match exampleEnhanced()")
}

func BenchmarkExampleSlow(b *testing.B) {
	for b.Loop() {
		exampleSlow()
	}
}

func BenchmarkExampleFast(b *testing.B) {
	for b.Loop() {
		exampleFast()
	}
}

func BenchmarkEnhance(b *testing.B) {
	b.Run("original", func(b *testing.B) {
		for b.Loop() {
			exampleOriginal()
		}
	})

	b.Run("enhanced", func(b *testing.B) {
		for b.Loop() {
			exampleEnhanced()
		}
	})
}
