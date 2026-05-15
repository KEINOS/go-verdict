package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExampleSlow(t *testing.T) {
	require.True(t, ExampleSlow(),
		"ExampleSlow() should return true")
}

func TestExampleFast(t *testing.T) {
	require.True(t, ExampleFast(),
		"ExampleFast() should return true")
}

func TestExampleOriginal(t *testing.T) {
	require.Equal(t, ExampleOriginal(), ExampleEnhanced(),
		"ExampleOriginal() should match ExampleEnhanced()")
}

func BenchmarkExampleSlow(b *testing.B) {
	for b.Loop() {
		ExampleSlow()
	}
}

func BenchmarkExampleFast(b *testing.B) {
	for b.Loop() {
		ExampleFast()
	}
}

func BenchmarkEnhance(b *testing.B) {
	b.Run("original", func(b *testing.B) {
		for b.Loop() {
			ExampleOriginal()
		}
	})

	b.Run("enhanced", func(b *testing.B) {
		for b.Loop() {
			ExampleEnhanced()
		}
	})
}
