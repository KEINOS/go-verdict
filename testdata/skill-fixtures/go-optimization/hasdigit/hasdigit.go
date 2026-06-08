package hasdigit

// HasASCIIDigit reports whether s contains an ASCII decimal digit.
func HasASCIIDigit(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' {
			return true
		}
	}
	return false
}
