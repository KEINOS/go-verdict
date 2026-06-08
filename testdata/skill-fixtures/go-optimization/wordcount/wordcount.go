package wordcount

// CountASCIIWords returns the number of ASCII whitespace-separated words in b.
func CountASCIIWords(b []byte) int {
	count := 0
	inWord := false

	for _, c := range b {
		switch c {
		case ' ', '\t', '\n', '\r', '\v', '\f':
			inWord = false
		default:
			if !inWord {
				count++
				inWord = true
			}
		}
	}

	return count
}
