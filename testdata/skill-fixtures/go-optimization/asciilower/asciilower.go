package asciilower

// LowerASCII returns s with ASCII uppercase letters converted to lowercase.
// It intentionally leaves non-ASCII bytes unchanged.
func LowerASCII(s string) string {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			return lowerASCIIWithCopy(s, i)
		}
	}
	return s
}

func lowerASCIIWithCopy(s string, firstUpper int) string {
	b := []byte(s)
	for i := firstUpper; i < len(b); i++ {
		c := b[i]
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}
