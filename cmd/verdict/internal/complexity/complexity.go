// Package complexity measures the static code complexity of Go functions.
//
// The analyzer parses each source file once and scores every declaration with
// both gocyclo and gocognit, so one pass yields a cyclomatic score, a cognitive
// score, and the source position of every function. Symbols are named the way
// pprof names them, so a static result can be joined with a profile row.
package complexity

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strings"

	"github.com/fzipp/gocyclo"
	"github.com/uudashr/gocognit"
)

// unknownReceiver stands in for a receiver the parser could not name.
const unknownReceiver = "?"

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

/* Helper Functions */

// Analyze returns one Stat per function in the given packages, sorted by
// symbol. A package whose sources fail to parse is a hard error, because a
// partial complexity view would silently understate the code.
//
// Only declared functions and methods are scored. A function literal bound to
// a package variable is skipped, because the compiler names it after the
// package initializer and no source-level symbol would ever match it.
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

		for _, decl := range file.Decls {
			function, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}

			mergeStat(merged, statOf(pkg.ImportPath, function, fileSet.Position(function.Pos())))
		}
	}

	stats := make([]Stat, 0, len(merged))
	for _, stat := range merged {
		stats = append(stats, stat)
	}

	return stats, nil
}

func statOf(importPath string, function *ast.FuncDecl, pos token.Position) Stat {
	return Stat{
		ImportPath: importPath,
		Symbol:     importPath + "." + funcName(function),
		File:       filepath.Base(pos.Filename),
		Line:       pos.Line,
		Cyclomatic: gocyclo.Complexity(function),
		Cognitive:  gocognit.Complexity(function),
	}
}

// mergeStat folds one declaration into the symbol it belongs to. A symbol that
// a package declares more than once, such as init, keeps the highest score and
// the position first seen.
func mergeStat(merged map[string]Stat, stat Stat) {
	current, ok := merged[stat.Symbol]
	if !ok {
		merged[stat.Symbol] = stat

		return
	}

	current.Cyclomatic = max(current.Cyclomatic, stat.Cyclomatic)
	current.Cognitive = max(current.Cognitive, stat.Cognitive)
	merged[stat.Symbol] = current
}

// funcName renders a declaration the way pprof names it: "(*T).Name" for a
// pointer method, "(T).Name" for a value method, and "Name" otherwise.
func funcName(function *ast.FuncDecl) string {
	if function.Recv == nil || function.Recv.NumFields() == 0 {
		return function.Name.Name
	}

	return "(" + receiverName(function.Recv.List[0].Type) + ")." + function.Name.Name
}

// receiverName renders a receiver type. A generic receiver drops its type
// parameters, matching the symbol left after pprof shapes are stripped.
func receiverName(expr ast.Expr) string {
	switch typed := expr.(type) {
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
