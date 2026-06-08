package prefix

import "testing"

func TestHasHTTPPrefix(t *testing.T) {
	tests := map[string]bool{
		"":                         false,
		"http://example.com":       true,
		"https://example.com":      true,
		"HTTP://example.com":       false,
		"ftp://example.com":        false,
		"mailto:test@example.com":  false,
		"https:/missing-one-slash": false,
	}

	for input, want := range tests {
		if got := HasHTTPPrefix(input); got != want {
			t.Fatalf("HasHTTPPrefix(%q) = %v, want %v", input, got, want)
		}
	}
}

var benchSink bool

func BenchmarkHasHTTPPrefix(b *testing.B) {
	inputs := map[string]string{
		"http":    "http://example.com/path/to/resource",
		"https":   "https://example.com/path/to/resource",
		"missing": "ftp://example.com/path/to/resource",
		"short":   "ht",
	}

	for name, input := range inputs {
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				benchSink = HasHTTPPrefix(input)
			}
		})
	}
}
