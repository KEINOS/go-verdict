package main

func ExampleSlow() bool {
	const loopCount = 100000000

	sum := 0
	for range loopCount {
		sum++
	}

	return sum == loopCount
}

func ExampleFast() bool {
	return true
}

func ExampleOriginal() int {
	const loopCount = 1000

	sum := 0
	for range loopCount {
		sum++
	}

	return sum
}

func ExampleEnhanced() int {
	return 1000
}
