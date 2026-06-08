package hasdigit

import (
	"regexp"
	"testing"
)

func TestHasASCIIDigit(t *testing.T) {
	tests := map[string]bool{
		"":                    false,
		"abc":                 false,
		"abc123":              true,
		"123abc":              true,
		"abc-xyz-999":         true,
		"unicode digit １２３": false,
		"symbols_only_!@#":    false,
	}

	for input, want := range tests {
		if got := HasASCIIDigit(input); got != want {
			t.Fatalf("HasASCIIDigit(%q) = %v, want %v", input, got, want)
		}
	}
}

var benchResult bool

var baselineDigitRE = regexp.MustCompile(`[0-9]`)

func baselineHasASCIIDigit(s string) bool {
	return baselineDigitRE.MatchString(s)
}

func candidateHasASCIIDigit(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' {
			return true
		}
	}
	return false
}

func BenchmarkHasASCIIDigit(b *testing.B) {
	inputs := map[string]string{
		"none":  "this string has no ascii decimal digits at all",
		"early": "1 starts with a digit and then has words",
		"late":  "this string has words and then a digit 7",
	}

	for name, input := range inputs {
		b.Run(name, func(b *testing.B) {
			b.Run("baseline", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					benchResult = baselineHasASCIIDigit(input)
				}
			})

			b.Run("candidate", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					benchResult = candidateHasASCIIDigit(input)
				}
			})
		})
	}
}
