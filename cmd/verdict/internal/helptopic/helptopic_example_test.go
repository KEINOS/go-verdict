package helptopic_test

import (
	"fmt"
	"strings"

	"github.com/KEINOS/go-verdict/cmd/verdict/internal/helptopic"
)

func ExampleTopics() {
	fmt.Println(strings.Join(helptopic.Topics(), ", "))
	// Output:
	// bootstrap, hotspot, benchstat, gotestbench, results
}

func ExampleText() {
	text, err := helptopic.Text("results")
	if err != nil {
		fmt.Println("error:", err)

		return
	}

	fmt.Println(strings.SplitN(text, "\n", 2)[0])
	// Output:
	// # Results: Read Verdict Output And Decide
}
