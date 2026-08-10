package hotspot

// This file holds the report model and its text and JSON formatting.

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	schemaVersion = 1

	fastCaveat = "CPU shares were measured with allocation profiling enabled (--fast), " +
		"so treat the CPU ranking as approximate."
)

// Result is the JSON-serializable hotspot report.
type Result struct {
	Secondary      *Choice `json:"secondary"`
	Benchmark      string  `json:"benchmark"`
	Caveat         string  `json:"caveat"`
	Classification string  `json:"classification"`
	Function       string  `json:"function"`
	ImportPath     string  `json:"import_path"`
	Next           string  `json:"next"`
	Package        string  `json:"package"`
	Reason         string  `json:"reason"`
	Alloc          Metric  `json:"alloc"`
	CPU            Metric  `json:"cpu"`
	SchemaVersion  int     `json:"schema_version"`
}

// Metric describes one profile contribution.
type Metric struct {
	CumPct    float64 `json:"cum_pct"`
	FlatPct   float64 `json:"flat_pct"`
	CumBytes  float64 `json:"cum_bytes,omitempty"`
	CumMS     float64 `json:"cum_ms,omitempty"`
	FlatBytes float64 `json:"flat_bytes,omitempty"`
	FlatMS    float64 `json:"flat_ms,omitempty"`
}

// Choice describes a primary or secondary hotspot candidate.
type Choice struct {
	Classification string `json:"classification"`
	Function       string `json:"function"`
	Reason         string `json:"reason"`
	Alloc          Metric `json:"alloc"`
	CPU            Metric `json:"cpu"`
}

// appendCaveat joins one more caveat sentence to an existing caveat.
func appendCaveat(existing string, addition string) string {
	if existing == "" {
		return addition
	}

	return existing + " " + addition
}

// withFastCaveat records that --fast lowered the CPU accuracy of the report.
func withFastCaveat(result Result, opts options) Result {
	if !opts.fast {
		return result
	}

	result.Caveat = appendCaveat(result.Caveat, fastCaveat)

	return result
}

func allocMetric(row pprofRow) Metric {
	return Metric{FlatMS: 0, FlatBytes: row.Flat, FlatPct: row.FlatPct, CumMS: 0, CumBytes: row.Cum, CumPct: row.CumPct}
}

func baseResult(opts options, pkgInfo packageInfo) Result {
	result := Result{
		SchemaVersion:  schemaVersion,
		Package:        opts.pkg,
		ImportPath:     pkgInfo.ImportPath,
		Benchmark:      opts.bench,
		Classification: classNoClearHotspot,
		Reason:         classNoClearHotspot,
		Function:       "",
		CPU:            zeroMetric(),
		Alloc:          zeroMetric(),
		Secondary:      nil,
		Caveat:         "",
		Next:           "Optimize a candidate, then judge before/after benchmark results with verdict.",
	}

	if pkgInfo.Module == nil || pkgInfo.Module.Path == "" {
		result.Caveat = "Module path was unavailable; user-code filtering used the package import path."
	}

	return result
}

func cpuMetric(row pprofRow) Metric {
	return Metric{FlatMS: row.Flat, FlatBytes: 0, FlatPct: row.FlatPct, CumMS: row.Cum, CumBytes: 0, CumPct: row.CumPct}
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
	switch result.Reason {
	case classNoBenchmark:
		return withCaveat(result.Package+": no benchmark workload ran; add BenchmarkXxx or pass --bench.\n", result.Caveat)
	case classNoClearHotspot:
		return withCaveat(result.Package+": no clear user-code hotspot found for this benchmark workload.\n", result.Caveat)
	default:
		parts := []string{fmt.Sprintf("%s: inspect %s (%s", result.Package, result.Function, result.Classification)}
		if result.CPU.FlatPct > 0 || result.CPU.CumPct > 0 {
			parts = append(parts, fmt.Sprintf("cpu flat %.1f%%, cpu cum %.1f%%", result.CPU.FlatPct, result.CPU.CumPct))
		}

		if result.Alloc.FlatPct > 0 || result.Alloc.CumPct > 0 {
			parts = append(parts, fmt.Sprintf("alloc flat %.1f%%, alloc cum %.1f%%", result.Alloc.FlatPct, result.Alloc.CumPct))
		}

		text := strings.Join(parts, "; ") + ")\nNext: " + result.Next + "\n"
		if result.Caveat != "" {
			text += "Caveat: " + result.Caveat + "\n"
		}

		return text
	}
}

func withCaveat(text string, caveat string) string {
	if caveat == "" {
		return text
	}

	return text + "Caveat: " + caveat + "\n"
}

func zeroMetric() Metric {
	return Metric{FlatMS: 0, FlatBytes: 0, FlatPct: 0, CumMS: 0, CumBytes: 0, CumPct: 0}
}
