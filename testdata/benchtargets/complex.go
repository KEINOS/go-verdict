package benchtargets

// This file holds a deliberately complex function with no benchmark, so E2E
// scenarios can exercise the static complexity signal of verdict hotspot.

const (
	classifyBig   = 1000
	classifySmall = 10
	classifyStep  = 2
)

// ClassifyValues sums a slice under several rules. It is intentionally
// branchy: it has no benchmark, so only the static analyzer ever sees it.
func ClassifyValues(values []int, strict bool) int {
	total := 0

	for _, value := range values {
		switch {
		case value > classifyBig:
			total += classifyBig
		case value > classifySmall:
			if strict && value%classifyStep == 0 {
				total += value / classifyStep
			} else if value%classifyStep == 0 {
				total += value
			} else {
				total++
			}
		case value < -classifyBig:
			total -= classifyBig
		case value < 0:
			if strict {
				total--
			} else {
				total -= value
			}
		default:
			total += classifyStep
		}
	}

	if strict && total > classifyBig {
		return classifyBig
	}

	return total
}
