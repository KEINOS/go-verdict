package asciilower

import "testing"

func TestLowerASCII(t *testing.T) {
	tests := map[string]string{
		"":                       "",
		"Already lower":          "already lower",
		"ASCII-ONLY 123":         "ascii-only 123",
		"Keep Café Δ UNCHANGED":  "keep café Δ unchanged",
		"MiXeD_with_SYMBOLS!?":   "mixed_with_symbols!?",
		"no uppercase here 1234": "no uppercase here 1234",
	}

	for input, want := range tests {
		if got := LowerASCII(input); got != want {
			t.Fatalf("LowerASCII(%q) = %q, want %q", input, got, want)
		}
	}
}

func originalLowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

func enhancedLowerASCII(s string) string {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			return enhancedLowerASCIIWithCopy(s, i)
		}
	}
	return s
}

func enhancedLowerASCIIWithCopy(s string, firstUpper int) string {
	b := []byte(s)
	for i := firstUpper; i < len(b); i++ {
		c := b[i]
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

var benchSink string

func BenchmarkLowerASCII(b *testing.B) {
	inputs := map[string]string{
		"no_upper":  "no uppercase here 1234 with repeated text",
		"all_upper": "ASCII-ONLY 123 WITH A FEW WORDS",
		"mixed":     "MixedCase with Some UPPER and lower Words",
	}

	for name, input := range inputs {
		b.Run(name, func(b *testing.B) {
			b.Run("original", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					benchSink = originalLowerASCII(input)
				}
			})

			b.Run("enhanced", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					benchSink = enhancedLowerASCII(input)
				}
			})
		})
	}
}
