package vowel

import "strings"

// IsVowelASCII reports whether b is an ASCII vowel.
func IsVowelASCII(b byte) bool {
	return strings.ContainsRune("aeiouAEIOU", rune(b))
}
