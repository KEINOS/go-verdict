package complexity

import (
	"go/ast"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnalyzeSourceBytes(t *testing.T) {
	t.Parallel()

	stats, err := Analyze([]Source{
		{
			ImportPath: "example.com/project/other",
			Name:       "other.go",
			Content:    []byte("package other\nfunc Other() {}\n"),
		},
		{
			ImportPath: "example.com/project/sample",
			Name:       "b.go",
			Content:    []byte("package sample\nfunc Value() {}\n"),
		},
		{
			ImportPath: "example.com/project/sample",
			Name:       "a.go",
			Content: []byte(`package sample
type Worker struct{}
type Box[T any] struct{}
type Pair[A, B any] struct{}
func (*Worker) Run() {}
func (Box[T]) Get() {}
func (Pair[A, B]) Key() {}
func init() {}
func init() { if true {} }
`),
		},
	})
	require.NoError(t, err)
	require.Len(t, stats, 7)
	require.True(t, slices.IsSortedFunc(stats, func(left Stat, right Stat) int {
		return strings.Compare(left.Symbol, right.Symbol)
	}))

	index := make(map[string]Stat, len(stats))
	for _, stat := range stats {
		index[stat.Symbol] = stat
	}

	require.Equal(t, "a.go", index["example.com/project/sample.(*Worker).Run"].File)
	require.Contains(t, index, "example.com/project/sample.Box.Get")
	require.Contains(t, index, "example.com/project/sample.Pair.Key")
	require.Contains(t, index, "example.com/project/sample.init.0")
	require.Contains(t, index, "example.com/project/sample.init.1")
	require.Contains(t, index, "example.com/project/sample.Value")
	require.Contains(t, index, "example.com/project/other.Other")
	require.Equal(t, unknownReceiver, receiverName(&ast.BadExpr{From: 0, To: 0}))
}

func TestAnalyzeRejectsBrokenSource(t *testing.T) {
	t.Parallel()

	_, err := Analyze([]Source{{
		ImportPath: "example.com/project/broken",
		Name:       "broken.go",
		Content:    []byte("package broken\nfunc Broken(\n"),
	}})
	require.ErrorContains(t, err, "broken.go")
}

func TestAnalyzeAcceptsEmptyInputAndFilesWithoutFunctions(t *testing.T) {
	t.Parallel()

	stats, err := Analyze(nil)
	require.NoError(t, err)
	require.Empty(t, stats)

	stats, err = Analyze([]Source{{
		ImportPath: "example.com/project/empty",
		Name:       "empty.go",
		Content:    []byte("package empty\nvar Value = 1\n"),
	}})
	require.NoError(t, err)
	require.Empty(t, stats)
}

func TestScoreUsesTheLargerNormalizedComplexity(t *testing.T) {
	t.Parallel()

	require.InDelta(t, 0.0, scoreOf(0, 0), 0)
	require.InDelta(t, 1.0, scoreOf(10, 3), 0)
	require.InDelta(t, 2.0, scoreOf(4, 30), 0)
	require.InDelta(t, 0.1, scoreOf(1, 0), 0)
}

func scoreOf(cyclomatic int, cognitive int) float64 {
	stat := new(Stat)
	stat.Cyclomatic = cyclomatic
	stat.Cognitive = cognitive

	return Score(*stat)
}
