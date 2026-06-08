package hexdigit

import "testing"

func TestIsHexDigit(t *testing.T) {
	tests := map[byte]bool{
		'0': true,
		'9': true,
		'a': true,
		'f': true,
		'A': true,
		'F': true,
		'g': false,
		'G': false,
		'x': false,
		'/': false,
		0xff: false,
	}

	for input, want := range tests {
		if got := IsHexDigit(input); got != want {
			t.Fatalf("IsHexDigit(%q) = %v, want %v", input, got, want)
		}
	}
}

var benchSink bool

func BenchmarkIsHexDigit(b *testing.B) {
	inputs := map[string]byte{
		"digit": '7',
		"lower": 'c',
		"upper": 'D',
		"miss":  'x',
		"high":  0xff,
	}

	for name, input := range inputs {
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				benchSink = IsHexDigit(input)
			}
		})
	}
}
