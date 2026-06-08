package jsonesc

// NeedsEscape reports whether s contains a byte that must be escaped in a
// simple JSON string encoder.
func NeedsEscape(s string) bool {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"', '\\', '\n', '\r', '\t':
			return true
		}
	}
	return false
}
