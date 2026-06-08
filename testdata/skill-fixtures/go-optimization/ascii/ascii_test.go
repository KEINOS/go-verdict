package ascii

import "testing"

func TestIsUpperASCII(t *testing.T) {
	tests := map[byte]bool{
		'A': true,
		'Z': true,
		'M': true,
		'a': false,
		'z': false,
		'0': false,
		'_': false,
		0xc0: false,
	}

	for input, want := range tests {
		if got := IsUpperASCII(input); got != want {
			t.Fatalf("IsUpperASCII(%q) = %v, want %v", input, got, want)
		}
	}
}

var benchSink bool

func BenchmarkIsUpperASCII(b *testing.B) {
	inputs := map[string]byte{
		"upper": 'Q',
		"lower": 'q',
		"digit": '7',
		"high":  0xc0,
	}

	for name, input := range inputs {
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				benchSink = IsUpperASCII(input)
			}
		})
	}
}
