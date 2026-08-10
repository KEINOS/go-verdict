// Package complexity measures the static code complexity of Go functions.
//
// The analyzer parses each source file once and feeds the same syntax tree to
// gocyclo and gocognit, so one pass yields a cyclomatic score, a cognitive
// score, and the source position of every function. Symbols are named the way
// pprof names them, so a static result can be joined with a profile row.
package complexity

import (
	"fmt"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strings"

	"github.com/fzipp/gocyclo"
	"github.com/uudashr/gocognit"
)

// Stat holds the static complexity of one function.
type Stat struct {
	// ImportPath is the package that declares the function. Callers compare it
	// directly instead of guessing package ownership from the symbol, because
	// an import path may itself contain a dot.
	ImportPath string
	Symbol     string
	File       string
	Line       int
	Cyclomatic int
	Cognitive  int
}

// Package names the Go files of one package to analyze.
type Package struct {
	ImportPath string
	Dir        string
	Files      []string
}

// reading is one analyzer result before the two scores are merged.
type reading struct {
	funcName   string
	pos        token.Position
	cyclomatic int
	cognitive  int
}

/* Helper Functions */

// Analyze returns one Stat per function in the given packages, sorted by
// symbol. A package whose sources fail to parse is a hard error, because a
// partial complexity view would silently understate the code.
func Analyze(packages []Package) ([]Stat, error) {
	stats := make([]Stat, 0, len(packages))

	for _, pkg := range packages {
		analyzed, err := analyzePackage(pkg)
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

func analyzePackage(pkg Package) ([]Stat, error) {
	fileSet := token.NewFileSet()
	merged := make(map[string]Stat, len(pkg.Files))

	for _, name := range pkg.Files {
		file, err := parser.ParseFile(fileSet, filepath.Join(pkg.Dir, name), nil, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", name, err)
		}

		// gocyclo also scores function literals bound to package variables,
		// so the two analyzers do not always report the same set.
		for _, stat := range gocyclo.AnalyzeASTFile(file, fileSet, nil) {
			mergeReading(merged, pkg.ImportPath, reading{
				funcName:   stat.FuncName,
				pos:        stat.Pos,
				cyclomatic: stat.Complexity,
				cognitive:  0,
			})
		}

		for _, stat := range gocognit.ComplexityStats(file, fileSet, nil) {
			mergeReading(merged, pkg.ImportPath, reading{
				funcName:   stat.FuncName,
				pos:        stat.Pos,
				cyclomatic: 0,
				cognitive:  stat.Complexity,
			})
		}
	}

	stats := make([]Stat, 0, len(merged))
	for _, stat := range merged {
		stats = append(stats, stat)
	}

	return stats, nil
}

// mergeReading folds one analyzer result into the symbol it belongs to. A
// symbol that a package declares more than once, such as init, keeps the
// highest score and the position first seen.
func mergeReading(merged map[string]Stat, importPath string, item reading) {
	symbol := importPath + "." + item.funcName

	stat, ok := merged[symbol]
	if !ok {
		stat = Stat{
			ImportPath: importPath,
			Symbol:     symbol,
			File:       filepath.Base(item.pos.Filename),
			Line:       item.pos.Line,
			Cyclomatic: 0,
			Cognitive:  0,
		}
	}

	stat.Cyclomatic = max(stat.Cyclomatic, item.cyclomatic)
	stat.Cognitive = max(stat.Cognitive, item.cognitive)
	merged[symbol] = stat
}
