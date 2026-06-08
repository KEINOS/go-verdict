package csvsum

import "testing"

func TestSumCSVInts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want int
	}{
		{name: "empty", in: "", want: 0},
		{name: "single", in: "42", want: 42},
		{name: "many", in: "1,2,3,4,5", want: 15},
		{name: "large", in: "100,200,3000,40", want: 3340},
		{name: "invalid fields ignored", in: "10,x,20,,30", want: 60},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := SumCSVInts(tt.in); got != tt.want {
				t.Fatalf("SumCSVInts(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

var sink int

func BenchmarkSumCSVInts(b *testing.B) {
	input := "12,34,56,78,90,123,456,789,1001,2020,3030,4040,5050,6060,7070"

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sink = SumCSVInts(input)
	}
}
