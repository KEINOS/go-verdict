// Package complexity measures the static code complexity of Go functions.
package complexity

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"slices"
	"strings"

	"github.com/fzipp/gocyclo"
	"github.com/uudashr/gocognit"
)

const (
	cyclomaticThreshold = 10.0
	cognitiveThreshold  = 15.0
	unknownReceiver     = "?"
)

// Source is one Go source file supplied to Analyze.
type Source struct {
	ImportPath string
	Name       string
	Content    []byte
}

// Stat holds the static complexity of one function.
type Stat struct {
	ImportPath string
	Symbol     string
	File       string
	Line       int
	Cyclomatic int
	Cognitive  int
}

// Analyze returns one Stat per declared function, sorted by symbol.
func Analyze(sources []Source) ([]Stat, error) {
	ordered := slices.Clone(sources)
	slices.SortFunc(ordered, compareSources)

	stats := make([]Stat, 0, len(ordered))
	initIndexes := make(map[string]int)

	for _, source := range ordered {
		analyzed, err := analyzeSource(source, initIndexes)
		if err != nil {
			return nil, err
		}

		stats = append(stats, analyzed...)
	}

	slices.SortFunc(stats, func(left Stat, right Stat) int {
		return strings.Compare(left.Symbol, right.Symbol)
	})

	return stats, nil
}

// Score normalizes a function's complexity against the project thresholds.
func Score(stat Stat) float64 {
	return math.Max(
		float64(stat.Cyclomatic)/cyclomaticThreshold,
		float64(stat.Cognitive)/cognitiveThreshold,
	)
}

func compareSources(left Source, right Source) int {
	if compared := strings.Compare(left.ImportPath, right.ImportPath); compared != 0 {
		return compared
	}

	return strings.Compare(left.Name, right.Name)
}

func analyzeSource(source Source, initIndexes map[string]int) ([]Stat, error) {
	fileSet := token.NewFileSet()

	file, err := parser.ParseFile(fileSet, source.Name, source.Content, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", source.Name, err)
	}

	stats := make([]Stat, 0)

	for _, decl := range file.Decls {
		function, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		name := funcName(function)
		if name == "init" {
			name = fmt.Sprintf("init.%d", initIndexes[source.ImportPath])
			initIndexes[source.ImportPath]++
		}

		stats = append(stats, statOf(source, name, function, fileSet.Position(function.Pos())))
	}

	return stats, nil
}

func statOf(source Source, name string, function *ast.FuncDecl, position token.Position) Stat {
	return Stat{
		ImportPath: source.ImportPath,
		Symbol:     source.ImportPath + "." + name,
		File:       source.Name,
		Line:       position.Line,
		Cyclomatic: gocyclo.Complexity(function),
		Cognitive:  gocognit.Complexity(function),
	}
}

func funcName(function *ast.FuncDecl) string {
	if function.Recv == nil || function.Recv.NumFields() == 0 {
		return function.Name.Name
	}

	receiver := receiverName(function.Recv.List[0].Type)
	if strings.HasPrefix(receiver, "*") {
		return "(" + receiver + ")." + function.Name.Name
	}

	return receiver + "." + function.Name.Name
}

func receiverName(expression ast.Expr) string {
	switch typed := expression.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.StarExpr:
		return "*" + receiverName(typed.X)
	case *ast.IndexExpr:
		return receiverName(typed.X)
	case *ast.IndexListExpr:
		return receiverName(typed.X)
	default:
		return unknownReceiver
	}
}
