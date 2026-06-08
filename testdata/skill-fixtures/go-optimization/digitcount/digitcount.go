package digitcount

// CountDigits returns the number of ASCII decimal digits in s.
func CountDigits(s string) int {
	count := 0
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			count++
		}
	}
	return count
}
