package digitcount

import "testing"

func TestCountDigits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want int
	}{
		{name: "empty", in: "", want: 0},
		{name: "none", in: "abc-XYZ", want: 0},
		{name: "only digits", in: "0123456789", want: 10},
		{name: "mixed", in: "id=42, retry=007, hex=0xff", want: 6},
		{name: "unicode digits ignored", in: "１２3٤5", want: 2},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := CountDigits(tt.in); got != tt.want {
				t.Fatalf("CountDigits(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

var sink int

func BenchmarkCountDigits(b *testing.B) {
	input := "trace id=781239 route=/v1/orders/4567 status=200 retry=003 latency_ms=29 payload=abcdef"

	b.ReportAllocs()
	b.Run("original", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			sink = CountDigits(input)
		}
	})
	b.Run("enhanced", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			sink = CountDigits(input)
		}
	})
}
