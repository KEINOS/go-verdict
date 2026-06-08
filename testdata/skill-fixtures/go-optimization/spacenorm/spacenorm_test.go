package spacenorm

import (
	"strings"
	"testing"
)

func TestCollapseSpace(t *testing.T) {
	tests := map[string]string{
		"":                         "",
		"already clean":            "already clean",
		"  leading and trailing  ": "leading and trailing",
		"tabs\tand\nnewlines":      "tabs and newlines",
		"multi   space":            "multi space",
		"unicode\u00a0space":       "unicode space",
		"em\u2003space":            "em space",
		"\u3000wide trim\u3000":    "wide trim",
	}

	for input, want := range tests {
		if got := CollapseSpace(input); got != want {
			t.Fatalf("CollapseSpace(%q) = %q, want %q", input, got, want)
		}
	}
}

var benchSink string

func baselineCollapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func asciiOnlyCandidate(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inSpace := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		isSpace := c == ' ' || c == '\t' || c == '\n' || c == '\r'
		if isSpace {
			if !inSpace {
				b.WriteByte(' ')
				inSpace = true
			}
			continue
		}
		b.WriteByte(c)
		inSpace = false
	}
	out := b.String()
	return strings.TrimSuffix(out, " ")
}

func BenchmarkCollapseSpace(b *testing.B) {
	inputs := map[string]string{
		"ascii":   "  many\tascii\nspaces   across   words  ",
		"clean":   "already clean words only",
		"unicode": "unicode\u00a0space and em\u2003space with wide\u3000trim",
	}

	for name, input := range inputs {
		b.Run(name, func(b *testing.B) {
			b.Run("baseline", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					benchSink = baselineCollapseSpace(input)
				}
			})

			b.Run("candidate", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					benchSink = asciiOnlyCandidate(input)
				}
			})
		})
	}
}
