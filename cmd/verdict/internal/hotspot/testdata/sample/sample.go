// Package sample is a static-analysis fixture for the hotspot command. It is
// under testdata, so the Go toolchain never builds it.
package sample

const (
	bigNegative   = -100
	smallNegative = -10
	busyTotal     = 1000
)

// Work is deliberately complex so the static analyzer has a clear target.
func Work(values []int, flag bool) int {
	total := 0

	for _, value := range values {
		if value > 0 {
			if value%2 == 0 && total < busyTotal {
				if flag {
					total += value * 2
				} else {
					total += value
				}
			} else if value%3 == 0 {
				total -= value
			} else {
				total++
			}
		} else if value < 0 {
			switch {
			case value < bigNegative:
				total += bigNegative
			case value < smallNegative:
				total += smallNegative
			default:
				total--
			}
		}
	}

	return total
}

// Simple stays at the base complexity of one.
func Simple() int {
	return 0
}
