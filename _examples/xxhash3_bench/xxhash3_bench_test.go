package main

import (
	"encoding/binary"
	"testing"

	cespare "github.com/cespare/xxhash/v2"
	zeebo "github.com/zeebo/xxh3"
)

// Shared test seed.
const testSeed uint64 = 123456789

// ============================================================================
// 1. Benchmark counter mode with a small 8-byte input.
// ============================================================================

func BenchmarkCounter(b *testing.B) {
	b.Run("cespare", func(b *testing.B) {
		var (
			buf     [8]byte
			seedBuf [8]byte
		)
		binary.LittleEndian.PutUint64(seedBuf[:], testSeed)

		b.ResetTimer()

		for i := range b.N {
			binary.LittleEndian.PutUint64(buf[:], uint64(i))

			// Hash the seed bytes and input bytes in sequence.
			h := cespare.New()
			_, _ = h.Write(seedBuf[:])
			_, _ = h.Write(buf[:])
			_ = h.Sum64()
		}
	})
	b.Run("zeebo", func(b *testing.B) {
		var buf [8]byte

		b.ResetTimer()

		for i := range b.N {
			binary.LittleEndian.PutUint64(buf[:], uint64(i))

			// Calculate a 64-bit hash with the seed.
			_ = zeebo.HashSeed(buf[:], testSeed)
		}
	})
}

func BenchmarkCounter_Zeebo_XXH3_128(b *testing.B) {
	var buf [8]byte

	b.ResetTimer()

	for i := range b.N {
		binary.LittleEndian.PutUint64(buf[:], uint64(i))

		// Calculate a 128-bit hash with the seed.
		_ = zeebo.Hash128Seed(buf[:], testSeed)
	}
}

// ============================================================================
// 2. Benchmark hashing a large (1 KB) input in one call.
// ============================================================================

func BenchmarkLargeChunk(b *testing.B) {
	data := make([]byte, 1024)

	b.Run("cespare", func(b *testing.B) {
		b.SetBytes(int64(len(data)))
		b.ResetTimer()

		for b.Loop() {
			// Use the fastest assembly path without a seed.
			_ = cespare.Sum64(data)
		}
	})
	b.Run("zeebo", func(b *testing.B) {
		b.SetBytes(int64(len(data)))
		b.ResetTimer()

		for b.Loop() {
			// Use the default hash function without a seed.
			_ = zeebo.Hash(data)
		}
	})
}
