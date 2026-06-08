package htmlesc

import (
	"strings"
	"testing"
)

func TestEscapeHTMLLite(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain", input: "hello world", want: "hello world"},
		{name: "ampersand", input: "a & b", want: "a &amp; b"},
		{name: "tags", input: "<b>hi</b>", want: "&lt;b&gt;hi&lt;/b&gt;"},
		{name: "quotes", input: `"it's ok"`, want: "&#34;it&#39;s ok&#34;"},
		{name: "mixed", input: `<a href="x&y">it's</a>`, want: "&lt;a href=&#34;x&amp;y&#34;&gt;it&#39;s&lt;/a&gt;"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := EscapeHTMLLite(test.input)
			if got != test.want {
				t.Fatalf("EscapeHTMLLite(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

var benchmarkSink string

func BenchmarkEscapeHTMLLite(b *testing.B) {
	noEscape := strings.Repeat("hello world ", 32)
	mixed := strings.Repeat(`<a href="x&y">it's ok</a> `, 16)

	b.Run("noescape", func(b *testing.B) {
		b.Run("original", func(b *testing.B) {
			for b.Loop() {
				benchmarkSink = escapeHTMLLiteOriginal(noEscape)
			}
		})
		b.Run("enhanced", func(b *testing.B) {
			for b.Loop() {
				benchmarkSink = EscapeHTMLLite(noEscape)
			}
		})
	})

	b.Run("mixed", func(b *testing.B) {
		b.Run("original", func(b *testing.B) {
			for b.Loop() {
				benchmarkSink = escapeHTMLLiteOriginal(mixed)
			}
		})
		b.Run("enhanced", func(b *testing.B) {
			for b.Loop() {
				benchmarkSink = EscapeHTMLLite(mixed)
			}
		})
	})
}

func escapeHTMLLiteOriginal(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&#34;",
		"'", "&#39;",
	)
	return replacer.Replace(s)
}
