package pathnorm

import "testing"

func TestNormalizeSlashPath(t *testing.T) {
	tests := map[string]string{
		"":               "",
		"/":              "/",
		"///":            "/",
		"a/b/c":          "a/b/c",
		"a//b///c":       "a/b/c",
		"/leading/path":  "leading/path",
		"trailing/path/": "trailing/path",
		"//both//sides/": "both/sides",
	}

	for input, want := range tests {
		if got := NormalizeSlashPath(input); got != want {
			t.Fatalf("NormalizeSlashPath(%q) = %q, want %q", input, got, want)
		}
	}
}

var benchSink string

func BenchmarkNormalizeSlashPath(b *testing.B) {
	inputs := []string{
		"a/b/c/d/e",
		"a//b///c////d/e",
		"////",
		"/leading/path/with/trailing/",
	}

	for _, input := range inputs {
		b.Run(input, func(b *testing.B) {
			b.Run("original", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					benchSink = normalizeSlashPathOriginal(input)
				}
			})
			b.Run("enhanced", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					benchSink = normalizeSlashPathEnhanced(input)
				}
			})
		})
	}
}
