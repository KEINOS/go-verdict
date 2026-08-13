package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KEINOS/go-verdict/internal/pareto"
	"github.com/KEINOS/go-verdict/verdict"
	"github.com/stretchr/testify/require"
)

const tieBenchstatInput = "name          old time/op  new time/op  delta\n" +
	"Foo-8         10.0ns ± 1%   10.0ns ± 1%  ~ (p=0.744 n=12)\n"

const (
	flagComplexity     = "--complexity"
	testBaselineLabel  = "baseline"
	testCandidateLabel = "candidate"
	testFooBenchmark   = "Foo-8"
)

func TestRunCLIComplexityTurnsABenchmarkTieIntoANewWin(t *testing.T) {
	t.Parallel()

	baseline := createComplexityModule(t, "example.com/project", branchyComplexitySource())
	candidate := createComplexityModule(t, "example.com/project", simpleComplexitySource())
	mapping := directoryMappingJSON(testFooBenchmark, baseline, candidate)

	var output strings.Builder

	err := runCLI(
		[]string{flagComplexity, mapping, "--verbose"},
		strings.NewReader(tieBenchstatInput),
		&output,
	)
	require.NoError(t, err)
	require.Contains(t, output.String(), "Foo-8: new wins")
	require.Contains(t, output.String(), "complexity improved")
	require.Contains(t, output.String(), "baseline directory")
	require.Contains(t, output.String(), "candidate directory")
}

func TestRunCLIComplexityCanTurnANewWinIntoATradeOffAndAffectRequire(t *testing.T) {
	t.Parallel()

	baseline := createComplexityModule(t, "example.com/project", simpleComplexitySource())
	candidate := createComplexityModule(t, "example.com/project", branchyComplexitySource())
	mapping := directoryMappingJSON(testFooBenchmark, baseline, candidate)

	var output strings.Builder

	err := runCLI(
		[]string{flagComplexity, mapping, "--require", string(verdict.NewWins)},
		strings.NewReader(winningInput),
		&output,
	)
	require.ErrorIs(t, err, errRequiredOutcome)
	require.Contains(t, output.String(), "Foo-8: trade-off")
}

func TestRunCLIComplexityJSONIncludesComparedAndNotMappedDetails(t *testing.T) {
	t.Parallel()

	baseline := createComplexityModule(t, "example.com/project", branchyComplexitySource())
	candidate := createComplexityModule(t, "example.com/project", simpleComplexitySource())
	mapping := directoryMappingJSON(testFooBenchmark, baseline, candidate)
	input := tieBenchstatInput +
		"Bar-8         10.0ns ± 1%   10.0ns ± 1%  ~ (p=0.744 n=12)\n"

	var output strings.Builder

	err := runCLI(
		[]string{flagComplexity, mapping, "--format", formatJSON},
		strings.NewReader(input),
		&output,
	)
	require.NoError(t, err)
	require.Contains(t, output.String(), `"status": "compared"`)
	require.Contains(t, output.String(), `"status": "not-mapped"`)
	require.Contains(t, output.String(), `"direction": "improved"`)
	require.Contains(t, output.String(), `"score":`)
}

func TestRunCLIComplexityUsesTheSynthesizedRawFileBenchmarkName(t *testing.T) {
	t.Parallel()

	baselineSource := createComplexityModule(t, "example.com/project", branchyComplexitySource())
	candidateSource := createComplexityModule(t, "example.com/project", simpleComplexitySource())
	baselineBench := writeRawComplexityBenchmark(t, "BenchmarkOriginal-8")
	candidateBench := writeRawComplexityBenchmark(t, "BenchmarkEnhanced-8")
	mapping := directoryMappingJSON(
		"BenchmarkOriginal_vs_BenchmarkEnhanced",
		baselineSource,
		candidateSource,
	)

	var output strings.Builder

	err := runCLI(
		[]string{"-a", baselineBench, "-b", candidateBench, flagComplexity, mapping},
		strings.NewReader(""),
		&output,
	)
	require.NoError(t, err)
	require.Contains(
		t,
		output.String(),
		"BenchmarkOriginal_vs_BenchmarkEnhanced: BenchmarkEnhanced wins",
	)
}

func TestEnrichComplexityKeepsInconclusiveOutcome(t *testing.T) {
	t.Parallel()

	baseline := createComplexityModule(t, "example.com/project", branchyComplexitySource())
	candidate := createComplexityModule(t, "example.com/project", simpleComplexitySource())
	mapping := directoryMapping(baseline, candidate)
	report := verdict.Report{Verdicts: []verdict.BenchmarkVerdict{inconclusiveVerdict(testFooBenchmark)}}

	resolver := newComplexityResolver()

	got, err := enrichComplexityReport(report, map[string]complexityMapping{testFooBenchmark: mapping}, resolver)
	require.NoError(t, err)
	require.Equal(t, verdict.Inconclusive, got.Report.Verdicts[0].Outcome)
	require.Equal(t, complexityStatusCompared, got.Details[testFooBenchmark].Status)
}

func TestEnrichComplexityRejectsMappingAbsentFromReport(t *testing.T) {
	t.Parallel()

	resolver := newComplexityResolver()

	_, err := enrichComplexityReport(
		verdict.Report{Verdicts: []verdict.BenchmarkVerdict{inconclusiveVerdict(testFooBenchmark)}},
		map[string]complexityMapping{"Missing-8": emptyComplexityMapping()},
		resolver,
	)
	require.ErrorContains(t, err, "does not exist in benchmark report")
}

func TestResolveComplexityDetailDirectionsAndErrors(t *testing.T) {
	t.Parallel()

	simple := createComplexityModule(t, "example.com/project", simpleComplexitySource())
	branchy := createComplexityModule(t, "example.com/project", branchyComplexitySource())
	resolver := newComplexityResolver()

	same, err := resolveComplexityDetail(directoryMapping(simple, simple), resolver)
	require.NoError(t, err)
	require.Equal(t, verdict.Same, same.Direction)

	missing := directoryMapping(filepathMissing(t), simple)
	_, err = resolveComplexityDetail(missing, resolver)
	require.ErrorContains(t, err, "resolving baseline")

	missing = directoryMapping(simple, filepathMissing(t))
	_, err = resolveComplexityDetail(missing, resolver)
	require.ErrorContains(t, err, "resolving candidate")

	badReport := verdict.Report{Verdicts: []verdict.BenchmarkVerdict{tieVerdict(testFooBenchmark)}}
	_, err = enrichComplexityReport(
		badReport,
		map[string]complexityMapping{
			testFooBenchmark: directoryMapping(branchy, filepathMissing(t)),
		},
		resolver,
	)
	require.ErrorIs(t, err, errComplexityMapping)
}

func TestComplexityParetoDecisionAndWinnerBranches(t *testing.T) {
	t.Parallel()

	decisions := map[pareto.Relation]verdict.Outcome{
		pareto.CandidateWins: verdict.NewWins,
		pareto.BaselineWins:  verdict.OldWins,
		pareto.Tie:           verdict.Tie,
		pareto.TradeOff:      verdict.TradeOff,
		pareto.Inconclusive:  verdict.Inconclusive,
		pareto.Relation("?"): verdict.Inconclusive,
	}
	for relation, want := range decisions {
		got, reason := complexityDecision(relation)
		require.Equal(t, want, got)
		require.NotEmpty(t, reason)
	}

	require.Equal(t, pareto.Same, paretoMetric(verdict.Direction("?")))
	require.Equal(t, "candidate", complexityWinner(labeledVerdict(verdict.NewWins)))
	require.Equal(t, "baseline", complexityWinner(labeledVerdict(verdict.OldWins)))
	require.Equal(t, "new", complexityWinner(unlabeledVerdict(verdict.NewWins)))
	require.Equal(t, "old", complexityWinner(unlabeledVerdict(verdict.OldWins)))
	require.Empty(t, complexityWinner(unlabeledVerdict(verdict.Tie)))
	require.Empty(t, complexityWinner(unlabeledVerdict(verdict.Outcome("?"))))
}

func TestWriteComplexityReportBranchesAndErrors(t *testing.T) {
	t.Parallel()

	report := outputComplexityReport()
	options := new(cliOptions)
	options.outputFormat = formatDefault

	err := writeComplexityReport(report, *options, failingWriter{})
	require.ErrorIs(t, err, errWritingOutput)

	options.outputFormat = "xml"
	err = writeComplexityReport(report, *options, io.Discard)
	require.ErrorIs(t, err, errUnknownFormat)

	options.outputFormat = formatJSON
	err = writeComplexityReport(report, *options, failingWriter{})
	require.ErrorIs(t, err, errWritingOutput)

	options.outputFormat = formatDefault
	options.verbose = true
	err = writeComplexityReport(report, *options, failingWriter{})
	require.ErrorIs(t, err, errWritingOutput)

	var output strings.Builder

	err = writeComplexityReport(report, *options, &output)
	require.NoError(t, err)
	require.Contains(t, output.String(), "complexity: not-mapped")

	err = writeComplexityMeasurement(
		failingWriter{},
		testBaselineLabel,
		report.Details[testFooBenchmark].Baseline,
	)
	require.ErrorIs(t, err, errWritingOutput)
	require.NoError(t, writeComplexityMeasurement(io.Discard, testBaselineLabel, nil))
}

func TestWriteComplexityVerboseTextReportsEveryWriteFailure(t *testing.T) {
	t.Parallel()

	report := outputComplexityReport()
	options := new(cliOptions)
	options.outputFormat = formatDefault
	options.verbose = true

	for allowedWrites := 1; allowedWrites <= 30; allowedWrites++ {
		writer := &countedFailingWriter{remaining: allowedWrites}

		err := writeComplexityReport(report, *options, writer)
		if err != nil {
			require.ErrorIs(t, err, errWritingOutput)
		}
	}
}

func TestRunComplexityCLIReportsEnrichmentAndOutputErrors(t *testing.T) {
	t.Parallel()

	options := new(cliOptions)
	options.complexity.requested = true
	options.complexity.mappings = map[string]complexityMapping{
		"Missing-8": emptyComplexityMapping(),
	}
	err := runComplexityCLI(
		verdict.Report{Verdicts: []verdict.BenchmarkVerdict{tieVerdict(testFooBenchmark)}},
		*options,
		io.Discard,
	)
	require.ErrorIs(t, err, errComplexityMapping)

	options.complexity.mappings = nil
	options.outputFormat = formatDefault
	err = runComplexityCLI(
		verdict.Report{Verdicts: []verdict.BenchmarkVerdict{tieVerdict(testFooBenchmark)}},
		*options,
		failingWriter{},
	)
	require.ErrorIs(t, err, errWritingOutput)
}

func directoryMappingJSON(benchmark string, baseline string, candidate string) string {
	return fmt.Sprintf(
		`{"benchmark":%q,"baseline":%s,"candidate":%s}`,
		benchmark,
		directorySourceJSON(baseline),
		directorySourceJSON(candidate),
	)
}

func directorySourceJSON(root string) string {
	return fmt.Sprintf(
		`{"kind":"directory","root":%q,"file":"pkg/work.go","symbol":"example.com/project/pkg.Work"}`,
		root,
	)
}

func directoryMapping(baseline string, candidate string) complexityMapping {
	return complexityMapping{
		Benchmark: testFooBenchmark,
		Baseline: sourceMappingOf(
			sourceKindDirectory,
			baseline,
			"",
			"pkg/work.go",
			"example.com/project/pkg.Work",
		),
		Candidate: sourceMappingOf(
			sourceKindDirectory,
			candidate,
			"",
			"pkg/work.go",
			"example.com/project/pkg.Work",
		),
	}
}

func inconclusiveVerdict(benchmark string) verdict.BenchmarkVerdict {
	item := new(verdict.BenchmarkVerdict)
	item.Benchmark = benchmark
	item.Outcome = verdict.Inconclusive
	item.Reason = complexityReasonNoMetrics

	return *item
}

func filepathMissing(t *testing.T) string {
	t.Helper()

	return t.TempDir() + "/missing"
}

func writeRawComplexityBenchmark(t *testing.T, benchmark string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), benchmark+".txt")
	content := benchmark + " 100 10 ns/op 8 B/op 1 allocs/op\n" +
		benchmark + " 100 10 ns/op 8 B/op 1 allocs/op\n" +
		benchmark + " 100 10 ns/op 8 B/op 1 allocs/op\n"
	writeTestFile(t, path, content)

	return path
}

func tieVerdict(benchmark string) verdict.BenchmarkVerdict {
	item := inconclusiveVerdict(benchmark)
	item.Outcome = verdict.Tie
	item.Metrics = []verdict.Comparison{{
		BaselineLabel:  testBaselineLabel,
		Benchmark:      benchmark,
		CandidateLabel: testCandidateLabel,
		Metric:         "sec/op",
		Direction:      verdict.Same,
		DeltaPct:       0,
		PValue:         1,
		Significant:    false,
	}}

	return item
}

func labeledVerdict(outcome verdict.Outcome) verdict.BenchmarkVerdict {
	item := unlabeledVerdict(outcome)
	item.BaselineLabel = testBaselineLabel
	item.CandidateLabel = testCandidateLabel

	return item
}

func unlabeledVerdict(outcome verdict.Outcome) verdict.BenchmarkVerdict {
	item := new(verdict.BenchmarkVerdict)
	item.Outcome = outcome

	return *item
}

func outputComplexityReport() complexityReport {
	baseline := new(complexityMeasurement)
	baseline.Kind = sourceKindGit
	baseline.Ref = "HEAD~1"
	baseline.File = "pkg/work.go"
	baseline.Symbol = "example.com/project/pkg.Work"

	return complexityReport{
		Details: map[string]complexityDetail{
			"Bar-8": {
				Baseline:  nil,
				Candidate: nil,
				Status:    complexityStatusNotMapped,
				Direction: "",
			},
			testFooBenchmark: {
				Baseline:  baseline,
				Candidate: baseline,
				Status:    complexityStatusCompared,
				Direction: verdict.Same,
			},
		},
		Report: verdict.Report{Verdicts: []verdict.BenchmarkVerdict{
			tieVerdict("Bar-8"),
			tieVerdict(testFooBenchmark),
		}},
	}
}

type countedFailingWriter struct {
	remaining int
}

func (writer *countedFailingWriter) Write(data []byte) (int, error) {
	if writer.remaining == 0 {
		return 0, errTestWrite
	}

	writer.remaining--

	return len(data), nil
}

func emptyComplexityMapping() complexityMapping {
	return complexityMapping{
		Benchmark: "",
		Baseline:  sourceMappingOf("", "", "", "", ""),
		Candidate: sourceMappingOf("", "", "", "", ""),
	}
}
