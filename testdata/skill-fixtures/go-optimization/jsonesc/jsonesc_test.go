package jsonesc

import "testing"

func TestNeedsEscape(t *testing.T) {
	tests := map[string]bool{
		"":                  false,
		"plain text":        false,
		`quote " here`:      true,
		`backslash \ here`:  true,
		"line\nbreak":       true,
		"tab\tseparated":    true,
		"carriage\rreturn":  true,
		"unicode snowman ☃": false,
	}

	for input, want := range tests {
		if got := NeedsEscape(input); got != want {
			t.Fatalf("NeedsEscape(%q) = %v, want %v", input, got, want)
		}
	}
}

var benchSink bool

func BenchmarkNeedsEscape(b *testing.B) {
	inputs := map[string]string{
		"plain":     "this is a fairly ordinary ascii string without escapes",
		"early":     "\" starts with quote",
		"late":      "this string has a trailing backslash \\",
		"non_ascii": "unicode snowman ☃ with no json escapes",
	}

	for name, input := range inputs {
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				benchSink = NeedsEscape(input)
			}
		})
	}
}
