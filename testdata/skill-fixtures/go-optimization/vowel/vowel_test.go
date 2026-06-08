package vowel

import "testing"

func TestIsVowelASCII(t *testing.T) {
	tests := map[byte]bool{
		'a':  true,
		'e':  true,
		'i':  true,
		'o':  true,
		'u':  true,
		'A':  true,
		'E':  true,
		'Z':  false,
		'b':  false,
		'0':  false,
		0xe9: false,
	}

	for input, want := range tests {
		if got := IsVowelASCII(input); got != want {
			t.Fatalf("IsVowelASCII(%q) = %v, want %v", input, got, want)
		}
	}
}

var benchSink bool

func BenchmarkIsVowelASCII(b *testing.B) {
	inputs := map[string]byte{
		"lower": 'a',
		"upper": 'E',
		"miss":  'z',
		"digit": '7',
		"high":  0xe9,
	}

	for name, input := range inputs {
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				benchSink = IsVowelASCII(input)
			}
		})
	}
}
