package ascii

// IsUpperASCII reports whether b is an ASCII uppercase letter.
func IsUpperASCII(b byte) bool {
	return b >= 'A' && b <= 'Z'
}
