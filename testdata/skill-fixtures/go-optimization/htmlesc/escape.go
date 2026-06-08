package htmlesc

import "strings"

var htmlReplacer = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	"\"", "&#34;",
	"'", "&#39;",
)

func EscapeHTMLLite(s string) string {
	return htmlReplacer.Replace(s)
}
