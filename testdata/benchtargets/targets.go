// Package benchtargets contains small benchmark targets for generated fixtures.
package benchtargets

const exampleResult = 1000

func exampleSlow() bool {
	const loopCount = 100000000

	sum := 0
	for range loopCount {
		sum++
	}

	return sum == loopCount
}

func exampleFast() bool {
	return true
}

func exampleOriginal() int {
	sum := 0
	for range exampleResult {
		sum++
	}

	return sum
}

func exampleEnhanced() int {
	return exampleResult
}
