package prefix

// HasHTTPPrefix reports whether s starts with http:// or https://.
func HasHTTPPrefix(s string) bool {
	return (len(s) >= 7 && s[:7] == "http://") || (len(s) >= 8 && s[:8] == "https://")
}
