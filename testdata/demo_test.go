package main

import "testing"

func TestExampleSlow(t *testing.T) {
	if !ExampleSlow() {
		t.Error("ExampleSlow() should return true")
	}
}

func TestExampleFast(t *testing.T) {
	if !ExampleFast() {
		t.Error("ExampleFast() should return true")
	}
}

func TestExampleOriginal(t *testing.T) {
	if ExampleOriginal() != ExampleEnhanced() {
		t.Error("ExampleOriginal() should match ExampleEnhanced()")
	}
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
