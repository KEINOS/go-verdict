package csvsum

import (
	"strconv"
	"strings"
)

// SumCSVInts returns the sum of comma-separated non-negative base-10 integers.
func SumCSVInts(s string) int {
	sum := 0
	for _, part := range strings.Split(s, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err == nil {
			sum += n
		}
	}
	return sum
}
