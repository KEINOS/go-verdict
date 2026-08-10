package hotspot

// This file parses `go tool pprof -top` output into pprofRow values.

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

const (
	bytesPerKB  = 1_024.0
	microsPerMS = 1_000.0
	msPerSecond = 1_000.0
	nanosPerMS  = 1_000_000.0
)

var (
	inlineMarker = regexp.MustCompile(`\s+\(inline\)$`)
	spacePattern = regexp.MustCompile(`\s+`)

	errNoPprofRows        = errors.New("pprof top output has no rows")
	errUnsupportedProfile = errors.New("unsupported profile kind")
)

type profileKind int

const (
	profileCPU profileKind = iota
	profileAlloc
	profileAllocObjects
	profileInuse
)

type pprofRow struct {
	Function string
	Flat     float64
	FlatPct  float64
	Cum      float64
	CumPct   float64
}

type profileSet struct {
	CPU          map[string]pprofRow
	Alloc        map[string]pprofRow
	AllocObjects map[string]pprofRow
	Inuse        map[string]pprofRow
}

func mergeRows(left pprofRow, right pprofRow) pprofRow {
	return pprofRow{
		Function: right.Function,
		Flat:     left.Flat + right.Flat,
		FlatPct:  left.FlatPct + right.FlatPct,
		Cum:      max(left.Cum, right.Cum),
		CumPct:   max(left.CumPct, right.CumPct),
	}
}

func normalizeSymbol(symbol string) string {
	symbol = inlineMarker.ReplaceAllString(strings.TrimSpace(symbol), "")
	symbol = strings.TrimPrefix(symbol, "vendor/")

	if index := strings.Index(symbol, "/vendor/"); index >= 0 {
		symbol = symbol[index+len("/vendor/"):]
	}

	return spacePattern.ReplaceAllString(symbol, " ")
}

func parseByteValue(value string) (float64, bool) {
	units := []struct {
		suffix string
		scale  float64
	}{
		{suffix: "GB", scale: bytesPerKB * bytesPerKB * bytesPerKB},
		{suffix: "MB", scale: bytesPerKB * bytesPerKB},
		{suffix: "kB", scale: bytesPerKB},
		{suffix: "KB", scale: bytesPerKB},
		{suffix: "B", scale: 1},
	}

	return parseUnitValue(value, units)
}

func parseCPUValue(value string) (float64, bool) {
	units := []struct {
		suffix string
		scale  float64
	}{
		{suffix: "ns", scale: 1.0 / nanosPerMS},
		{suffix: "us", scale: 1.0 / microsPerMS},
		{suffix: "µs", scale: 1.0 / microsPerMS},
		{suffix: "ms", scale: 1},
		{suffix: "s", scale: msPerSecond},
	}

	return parseUnitValue(value, units)
}

func parsePercent(value string) (float64, bool) {
	parsed, err := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 64)

	return parsed, err == nil
}

func parseTop(output []byte, kind profileKind) ([]pprofRow, error) {
	if kind != profileCPU && kind != profileAlloc {
		return nil, errUnsupportedProfile
	}

	lines := strings.Split(string(output), "\n")
	rows := make([]pprofRow, 0)

	for _, line := range lines {
		row, ok := parseTopLine(line, kind)
		if ok {
			rows = append(rows, row)
		}
	}

	if len(rows) == 0 {
		return nil, errNoPprofRows
	}

	return rows, nil
}

func parseTopLine(line string, kind profileKind) (pprofRow, bool) {
	fields := strings.Fields(line)
	if len(fields) < 6 || !strings.HasSuffix(fields[1], "%") || !strings.HasSuffix(fields[4], "%") {
		return zeroPprofRow(), false
	}

	flat, ok := parseValue(fields[0], kind)
	if !ok {
		return zeroPprofRow(), false
	}

	flatPct, ok := parsePercent(fields[1])
	if !ok {
		return zeroPprofRow(), false
	}

	cum, ok := parseValue(fields[3], kind)
	if !ok {
		return zeroPprofRow(), false
	}

	cumPct, ok := parsePercent(fields[4])
	if !ok {
		return zeroPprofRow(), false
	}

	return pprofRow{
		Function: normalizeSymbol(strings.Join(fields[5:], " ")),
		Flat:     flat,
		FlatPct:  flatPct,
		Cum:      cum,
		CumPct:   cumPct,
	}, true
}

func parseUnitValue(value string, units []struct {
	suffix string
	scale  float64
}) (float64, bool) {
	for _, unit := range units {
		if !strings.HasSuffix(value, unit.suffix) {
			continue
		}

		parsed, err := strconv.ParseFloat(strings.TrimSuffix(value, unit.suffix), 64)

		return parsed * unit.scale, err == nil
	}

	parsed, err := strconv.ParseFloat(value, 64)

	return parsed, err == nil
}

func parseValue(value string, kind profileKind) (float64, bool) {
	switch kind {
	case profileCPU:
		return parseCPUValue(value)
	case profileAlloc:
		return parseByteValue(value)
	case profileAllocObjects, profileInuse:
		return 0, false
	default:
		return 0, false
	}
}

func pprofInvocation(binaryPath string, profilePath string, kind profileKind) invocation {
	args := []string{"tool", "pprof", "-top"}
	if kind == profileAlloc {
		args = append(args, "-alloc_space")
	}

	args = append(args, "-nodecount=50", binaryPath, profilePath)

	return invocation{Dir: "", Name: "go", Args: args}
}

func rowsByFunction(rows []pprofRow) map[string]pprofRow {
	result := make(map[string]pprofRow)

	for _, row := range rows {
		result[row.Function] = mergeRows(result[row.Function], row)
	}

	return result
}

func zeroPprofRow() pprofRow {
	return pprofRow{Function: "", Flat: 0, FlatPct: 0, Cum: 0, CumPct: 0}
}
