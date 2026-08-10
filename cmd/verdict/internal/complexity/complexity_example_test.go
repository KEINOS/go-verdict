package complexity_test

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/KEINOS/go-verdict/cmd/verdict/internal/complexity"
)

func ExampleAnalyze() {
	stats, err := complexity.Analyze([]complexity.Package{{
		ImportPath: "example.com/project/sample",
		Dir:        filepath.Join("testdata", "sample"),
		Files:      []string{"sample.go"},
	}})
	if err != nil {
		log.Fatal(err)
	}

	for _, stat := range stats {
		fmt.Println(stat.Symbol)
	}

	// Output:
	// example.com/project/sample.(*Worker).Run
	// example.com/project/sample.Box.Get
	// example.com/project/sample.Map
	// example.com/project/sample.Pair.Key
	// example.com/project/sample.Simple
	// example.com/project/sample.Worker.Name
	// example.com/project/sample.init.0
	// example.com/project/sample.init.1
}
