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
