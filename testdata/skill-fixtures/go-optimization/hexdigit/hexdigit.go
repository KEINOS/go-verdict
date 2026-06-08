package hexdigit

import "strings"

// IsHexDigit reports whether b is an ASCII hexadecimal digit.
func IsHexDigit(b byte) bool {
	return strings.ContainsRune("0123456789abcdefABCDEF", rune(b))
}
