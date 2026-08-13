package complexity_test

import (
	"fmt"
	"log"

	"github.com/KEINOS/go-verdict/complexity"
)

func ExampleAnalyze() {
	stats, err := complexity.Analyze([]complexity.Source{{
		ImportPath: "example.com/project/sample",
		Name:       "sample.go",
		Content: []byte(`package sample

func Simple() {}

func Branchy(value int) int {
	if value > 0 {
		return value
	}

	return -value
}
`),
	}})
	if err != nil {
		log.Fatal(err)
	}

	for _, stat := range stats {
		fmt.Printf("%s %.1f\n", stat.Symbol, complexity.Score(stat))
	}

	// Output:
	// example.com/project/sample.Branchy 0.2
	// example.com/project/sample.Simple 0.1
}
