package hotspot

// This file holds the report model and its text and JSON formatting.

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	schemaVersion = 2

	unitBytes   = "bytes"
	unitMS      = "ms"
	unitObjects = "objects"

	fastCaveat = "CPU shares were measured with allocation profiling enabled (--fast), " +
		"so treat the CPU ranking as approximate."

	caveatNoBenchmark = "No benchmark workload ran. Add BenchmarkXxx or pass --bench. See: verdict help bootstrap."

	caveatNoBenchmarkStatic = "No benchmark workload ran, so this is a static estimate, not measured cost. " +
		"Add a benchmark to raise accuracy. See: verdict help bootstrap."

	caveatNoClearHotspot = "No clear user-code hotspot found for this benchmark workload."

	caveatNoClearHotspotStatic = "The profiles show no clear user-code hotspot, so this is a static estimate, " +
		"not measured cost. Widen the benchmark workload to raise accuracy. See: verdict help bootstrap."
)

// Result is the JSON-serializable hotspot report. The top-level fields describe
// the suggested function, and Candidates lists the runners-up in rank order.
type Result struct {
	Benchmark      string     `json:"benchmark"`
	Caveat         string     `json:"caveat"`
	Classification string     `json:"classification"`
	File           string     `json:"file"`
	Function       string     `json:"function"`
	ImportPath     string     `json:"import_path"`
	Next           string     `json:"next"`
	Package        string     `json:"package"`
	Candidates     []Choice   `json:"candidates"`
	Signals        []string   `json:"signals"`
	AllocBytes     Metric     `json:"alloc_bytes"`
	AllocObjects   Metric     `json:"alloc_objects"`
	Complexity     Complexity `json:"complexity"`
	CPU            Metric     `json:"cpu"`
	Retained       Metric     `json:"retained"`
	Line           int        `json:"line"`
	SchemaVersion  int        `json:"schema_version"`
}

// Metric describes one signal's contribution for one function. Flat counts the
// function itself and Cum counts everything it calls.
type Metric struct {
	Unit    string  `json:"unit"`
	Cum     float64 `json:"cum"`
	CumPct  float64 `json:"cum_pct"`
	Flat    float64 `json:"flat"`
	FlatPct float64 `json:"flat_pct"`
}

// Complexity describes the static complexity of one function.
type Complexity struct {
	Cyclomatic int `json:"cyclomatic"`
	Cognitive  int `json:"cognitive"`
}

// Choice describes one hotspot candidate.
type Choice struct {
	Classification string     `json:"classification"`
	File           string     `json:"file"`
	Function       string     `json:"function"`
	Signals        []string   `json:"signals"`
	AllocBytes     Metric     `json:"alloc_bytes"`
	AllocObjects   Metric     `json:"alloc_objects"`
	Complexity     Complexity `json:"complexity"`
	CPU            Metric     `json:"cpu"`
	Retained       Metric     `json:"retained"`
	Line           int        `json:"line"`
}

/* Helper Functions */

// appendCaveat joins one more caveat sentence to an existing caveat.
func appendCaveat(existing string, addition string) string {
	if existing == "" {
		return addition
	}

	return existing + " " + addition
}

func baseResult(opts options, pkgInfo packageInfo) Result {
	result := Result{
		SchemaVersion:  schemaVersion,
		Package:        opts.pkg,
		ImportPath:     pkgInfo.ImportPath,
		Benchmark:      opts.bench,
		Classification: classNoClearHotspot,
		Function:       "",
		File:           "",
		Line:           0,
		Signals:        []string{},
		CPU:            zeroMetric(unitMS),
		AllocBytes:     zeroMetric(unitBytes),
		AllocObjects:   zeroMetric(unitObjects),
		Retained:       zeroMetric(unitBytes),
		Complexity:     Complexity{Cyclomatic: 0, Cognitive: 0},
		Candidates:     []Choice{},
		Caveat:         "",
		Next:           "Optimize a candidate, then judge before/after benchmark results with verdict.",
	}

	if pkgInfo.Module == nil || pkgInfo.Module.Path == "" {
		result.Caveat = "Module path was unavailable; user-code filtering used the package import path."
	}

	return result
}

func formatResult(result Result, format string) (string, error) {
	switch format {
	case defaultFormat:
		return formatText(result), nil
	case formatJSON:
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return "", fmt.Errorf("formatting hotspot json: %w", err)
		}

		return string(data) + "\n", nil
	default:
		return "", errInvalidFormat
	}
}

func formatText(result Result) string {
	switch result.Classification {
	case classNoBenchmark:
		return withCaveat(result.Package+": no benchmark workload ran; add BenchmarkXxx or pass --bench.\n", result.Caveat)
	case classNoClearHotspot:
		return withCaveat(result.Package+": no clear user-code hotspot found for this benchmark workload.\n", result.Caveat)
	default:
		return formatSuggestion(result)
	}
}

func formatSuggestion(result Result) string {
	text := fmt.Sprintf(
		"%s: inspect %s%s (%s)\n",
		result.Package, result.Function, sourcePosition(result.File, result.Line), strings.Join(signalParts(result), "; "),
	)

	if line := alsoLine(result.Candidates); line != "" {
		text += line
	}

	text += "Next: " + result.Next + "\n"

	if result.Caveat != "" {
		text += "Caveat: " + result.Caveat + "\n"
	}

	return text
}

// alsoLine lists the runners-up on one line, so the suggestion stays first.
func alsoLine(candidates []Choice) string {
	if len(candidates) == 0 {
		return ""
	}

	parts := make([]string, 0, len(candidates))

	for _, choice := range candidates {
		parts = append(parts, fmt.Sprintf(
			"%s%s (%s)", choice.Function, sourcePosition(choice.File, choice.Line), choice.Classification,
		))
	}

	return "Also: " + strings.Join(parts, ", ") + "\n"
}

// signalParts lists the classification and every signal that has a value.
func signalParts(result Result) []string {
	parts := []string{result.Classification}
	parts = appendMetricPart(parts, "cpu", result.CPU)
	parts = appendMetricPart(parts, "alloc bytes", result.AllocBytes)
	parts = appendMetricPart(parts, "alloc objects", result.AllocObjects)
	parts = appendMetricPart(parts, "retained", result.Retained)

	if result.Complexity.Cyclomatic > 0 || result.Complexity.Cognitive > 0 {
		parts = append(parts, fmt.Sprintf(
			"cyclomatic %d, cognitive %d", result.Complexity.Cyclomatic, result.Complexity.Cognitive,
		))
	}

	return parts
}

func appendMetricPart(parts []string, label string, metric Metric) []string {
	if metric.FlatPct <= 0 && metric.CumPct <= 0 {
		return parts
	}

	return append(parts, fmt.Sprintf("%s flat %.1f%%, %s cum %.1f%%", label, metric.FlatPct, label, metric.CumPct))
}

// sourcePosition renders " at file:line" when the static pass located the
// function, and nothing otherwise.
func sourcePosition(file string, line int) string {
	if file == "" || line <= 0 {
		return ""
	}

	return fmt.Sprintf(" at %s:%d", file, line)
}

func withCaveat(text string, caveat string) string {
	if caveat == "" {
		return text
	}

	return text + "Caveat: " + caveat + "\n"
}

// withFastCaveat records that --fast lowered the CPU accuracy of the report.
func withFastCaveat(result Result, opts options) Result {
	if !opts.fast {
		return result
	}

	result.Caveat = appendCaveat(result.Caveat, fastCaveat)

	return result
}

func metricOf(row pprofRow, unit string) Metric {
	return Metric{Unit: unit, Flat: row.Flat, FlatPct: row.FlatPct, Cum: row.Cum, CumPct: row.CumPct}
}

func zeroMetric(unit string) Metric {
	return Metric{Unit: unit, Flat: 0, FlatPct: 0, Cum: 0, CumPct: 0}
}
