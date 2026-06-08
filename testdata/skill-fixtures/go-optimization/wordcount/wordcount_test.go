package wordcount

import "testing"

func TestCountASCIIWords(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want int
	}{
		{name: "empty", in: "", want: 0},
		{name: "spaces", in: "   \t\n\r\v\f   ", want: 0},
		{name: "single", in: "alpha", want: 1},
		{name: "simple", in: "alpha beta gamma", want: 3},
		{name: "mixed whitespace", in: "\talpha\nbeta\r\ngamma\vdelta\fepsilon ", want: 5},
		{name: "punctuation stays in word", in: "alpha,beta gamma!", want: 2},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := CountASCIIWords([]byte(tt.in)); got != tt.want {
				t.Fatalf("CountASCIIWords(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

var sink int

func BenchmarkCountASCIIWords(b *testing.B) {
	input := []byte("alpha beta\tgamma\n" +
		"delta epsilon zeta eta theta iota kappa lambda mu\n" +
		"nu xi omicron pi rho sigma tau upsilon phi chi psi omega\r\n")

	b.ReportAllocs()
	b.Run("original", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			sink = CountASCIIWords(input)
		}
	})
	b.Run("enhanced", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			sink = CountASCIIWords(input)
		}
	})
}
