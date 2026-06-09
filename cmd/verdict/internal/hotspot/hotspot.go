// Package hotspot implements the verdict hotspot Scout command.
package hotspot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBench     = "."
	defaultBenchtime = "1s"
	defaultCount     = 1
	defaultFormat    = "text"
	formatJSON       = "json"
	schemaVersion    = 1

	allocCumThreshold  = 10.0
	allocFlatThreshold = 5.0
	bytesPerKB         = 1_024.0
	cpuCumThreshold    = 10.0
	cpuFlatThreshold   = 5.0
	defaultPrefixCap   = 2
	microsPerMS        = 1_000.0
	msPerSecond        = 1_000.0
	nanosPerMS         = 1_000_000.0
	rankAlloc          = 2
	rankCPU            = 1
	rankMixed          = 0
	rankOther          = 3

	classAllocHotspot   = "alloc-hotspot"
	classCPUHotspot     = "cpu-hotspot"
	classMixedHotspot   = "mixed-hotspot"
	classNoBenchmark    = "no-benchmark"
	classNoClearHotspot = "no-clear-hotspot"
)

var (
	benchmarkRowPattern = regexp.MustCompile(`(?m)^Benchmark\S+\s+\d+`)
	benchtimeCount      = regexp.MustCompile(`^[1-9][0-9]*x$`)
	inlineMarker        = regexp.MustCompile(`\s+\(inline\)$`)
	spacePattern        = regexp.MustCompile(`\s+`)

	errInvalidBenchtime   = errors.New("benchtime must be a Go benchmark duration or iteration count")
	errInvalidCount       = errors.New("count must be at least 1")
	errInvalidFormat      = errors.New("format must be text or json")
	errMissingPackage     = errors.New("hotspot requires exactly one package")
	errMultiplePackages   = errors.New("hotspot supports exactly one package, not a multi-package pattern")
	errNilOutput          = errors.New("writing output: nil output writer")
	errNoPprofRows        = errors.New("pprof top output has no rows")
	errUnsupportedProfile = errors.New("unsupported profile kind")
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

// Command runs the hotspot Scout command.
type Command struct {
	runner commandRunner
}

type commandRunner interface {
	Run(command invocation) ([]byte, error)
}

type execRunner struct{}

type invocation struct {
	Dir  string
	Name string
	Args []string
}

type options struct {
	bench     string
	benchtime string
	format    string
	pkg       string
	count     int
}

type packageInfo struct {
	Module *struct {
		Path string `json:"Path"`
	} `json:"Module"`
	ImportPath string `json:"ImportPath"`
	Dir        string `json:"Dir"`
}

type profileKind int

const (
	profileCPU profileKind = iota
	profileAlloc
)

type pprofRow struct {
	Function string
	Flat     float64
	FlatPct  float64
	Cum      float64
	CumPct   float64
}

type profileSet struct {
	CPU   map[string]pprofRow
	Alloc map[string]pprofRow
}

/* Constructors and Methods */

// Command

// New returns a hotspot command with the default process runner.
func New() Command {
	return Command{runner: execRunner{}}
}

// Run executes the hotspot Scout command with the provided args.
func (command Command) Run(args []string, output io.Writer) error {
	if output == nil {
		return errNilOutput
	}

	if isHelpRequest(args) {
		return writeText(output, HelpText())
	}

	opts, err := parseArgs(args)
	if err != nil {
		return err
	}

	if command.runner == nil {
		command.runner = execRunner{}
	}

	result, err := command.scout(opts)
	if err != nil {
		return err
	}

	text, err := formatResult(result, opts.format)
	if err != nil {
		return err
	}

	return writeText(output, text)
}

func (command Command) compileBenchmark(binaryPath string, pkg string) error {
	_, err := command.runner.Run(invocation{Dir: "", Name: "go", Args: []string{"test", "-c", "-o", binaryPath, pkg}})
	if err != nil {
		return fmt.Errorf("compiling benchmark binary: %w", err)
	}

	return nil
}

func (command Command) runBenchmark(
	pkgDir string,
	binaryPath string,
	cpuPath string,
	memPath string,
	opts options,
) ([]byte, error) {
	output, err := command.runner.Run(invocation{
		Dir:  pkgDir,
		Name: binaryPath,
		Args: []string{
			"-test.run=^$",
			"-test.bench=" + opts.bench,
			"-test.benchtime=" + opts.benchtime,
			"-test.count=" + strconv.Itoa(opts.count),
			"-test.cpuprofile=" + cpuPath,
			"-test.memprofile=" + memPath,
			"-test.memprofilerate=1",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("running benchmark workload: %w", err)
	}

	return output, nil
}

func (command Command) readProfiles(binaryPath string, cpuPath string, memPath string) (profileSet, error) {
	cpuOutput, err := command.runner.Run(pprofInvocation(binaryPath, cpuPath, profileCPU))
	if err != nil {
		return profileSet{}, fmt.Errorf("reading CPU profile: %w", err)
	}

	allocOutput, err := command.runner.Run(pprofInvocation(binaryPath, memPath, profileAlloc))
	if err != nil {
		return profileSet{}, fmt.Errorf("reading allocation profile: %w", err)
	}

	cpuRows, err := parseTop(cpuOutput, profileCPU)
	if err != nil {
		return profileSet{}, fmt.Errorf("parsing CPU profile: %w", err)
	}

	allocRows, err := parseTop(allocOutput, profileAlloc)
	if err != nil {
		return profileSet{}, fmt.Errorf("parsing allocation profile: %w", err)
	}

	return profileSet{CPU: rowsByFunction(cpuRows), Alloc: rowsByFunction(allocRows)}, nil
}

func (command Command) resolvePackage(pkg string) (packageInfo, error) {
	output, err := command.runner.Run(invocation{Dir: "", Name: "go", Args: []string{"list", "-json", pkg}})
	if err != nil {
		return packageInfo{}, fmt.Errorf("resolving package: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(output))

	var packages []packageInfo

	for decoder.More() {
		var item packageInfo

		err = decoder.Decode(&item)
		if err != nil {
			return packageInfo{}, fmt.Errorf("decoding go list output: %w", err)
		}

		packages = append(packages, item)
	}

	switch len(packages) {
	case 0:
		return packageInfo{}, errMissingPackage
	case 1:
		return packages[0], nil
	default:
		return packageInfo{}, errMultiplePackages
	}
}

func (command Command) scout(opts options) (Result, error) {
	pkgInfo, err := command.resolvePackage(opts.pkg)
	if err != nil {
		return Result{}, err
	}

	result := baseResult(opts, pkgInfo)

	tmpDir, err := os.MkdirTemp("", "verdict-hotspot-*")
	if err != nil {
		return Result{}, fmt.Errorf("creating hotspot temp dir: %w", err)
	}

	defer func() { _ = os.RemoveAll(tmpDir) }()

	binaryPath := filepath.Join(tmpDir, "hotspot.test")
	cpuPath := filepath.Join(tmpDir, "cpu.out")
	memPath := filepath.Join(tmpDir, "mem.out")

	err = command.compileBenchmark(binaryPath, opts.pkg)
	if err != nil {
		return Result{}, err
	}

	benchOutput, err := command.runBenchmark(pkgInfo.Dir, binaryPath, cpuPath, memPath, opts)
	if err != nil {
		return Result{}, err
	}

	if !benchmarkRowPattern.Match(benchOutput) {
		result.Classification = classNoBenchmark
		result.Reason = classNoBenchmark
		result.Caveat = "No benchmark workload ran. Add BenchmarkXxx or pass --bench."

		return result, nil
	}

	profiles, err := command.readProfiles(binaryPath, cpuPath, memPath)
	if err != nil {
		return Result{}, err
	}

	return classify(result, profileSet{
		CPU:   userRows(profiles.CPU, pkgInfo.userPrefixes()),
		Alloc: userRows(profiles.Alloc, pkgInfo.userPrefixes()),
	}), nil
}

// execRunner

func (execRunner) Run(command invocation) ([]byte, error) {
	//nolint:gosec // The CLI intentionally executes go and generated test binaries from parsed user input.
	cmd := exec.CommandContext(context.Background(), command.Name, command.Args...)
	cmd.Dir = command.Dir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("%s %s: %w\n%s", command.Name, strings.Join(command.Args, " "), err, string(output))
	}

	return output, nil
}

// packageInfo

func (pkgInfo packageInfo) userPrefixes() []string {
	prefixes := make([]string, 0, defaultPrefixCap)

	if pkgInfo.Module != nil && pkgInfo.Module.Path != "" {
		prefixes = append(prefixes, pkgInfo.Module.Path)
	}

	if pkgInfo.ImportPath != "" && !contains(prefixes, pkgInfo.ImportPath) {
		prefixes = append(prefixes, pkgInfo.ImportPath)
	}

	return prefixes
}

/* Helper Functions */

// HelpText returns hotspot-specific help text.
func HelpText() string {
	return `Usage:
  verdict hotspot [options] <package>

Suggest the first function to inspect from benchmark CPU and allocation profiles.

Options:
  --bench regexp
      Benchmark regexp. Default: .
  --benchtime duration|Nx
      Benchmark time or iteration count. Default: 1s.
  --count n
      Benchmark run count. Default: 1.
  --format text|json
      Output format. Default: text.
`
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

func choiceScore(choice Choice) float64 {
	return max(
		metricScore(choice.CPU, cpuFlatThreshold, cpuCumThreshold),
		metricScore(choice.Alloc, allocFlatThreshold, allocCumThreshold),
	)
}

func classificationRank(classification string) int {
	switch classification {
	case classMixedHotspot:
		return rankMixed
	case classCPUHotspot:
		return rankCPU
	case classAllocHotspot:
		return rankAlloc
	default:
		return rankOther
	}
}

func classify(base Result, profiles profileSet) Result {
	cpuCandidates := profileCandidates(profiles.CPU, profileCPU)
	allocCandidates := profileCandidates(profiles.Alloc, profileAlloc)
	choices := make([]Choice, 0)

	for function, cpu := range cpuCandidates {
		if alloc, ok := allocCandidates[function]; ok {
			choices = append(choices, makeChoice(classMixedHotspot, classMixedHotspot, function, cpu, alloc))
		}
	}

	if len(choices) == 0 {
		for function, cpu := range cpuCandidates {
			choices = append(choices, makeChoice(classCPUHotspot, classCPUHotspot, function, cpu, zeroPprofRow()))
		}

		for function, alloc := range allocCandidates {
			choices = append(choices, makeChoice(classAllocHotspot, classAllocHotspot, function, zeroPprofRow(), alloc))
		}
	}

	if len(choices) == 0 {
		base.Classification = classNoClearHotspot

		base.Reason = classNoClearHotspot
		if base.Caveat == "" {
			base.Caveat = "No clear user-code hotspot found for this benchmark workload."
		}

		return base
	}

	sort.SliceStable(choices, func(left int, right int) bool {
		return compareChoice(choices[left], choices[right]) < 0
	})

	primary := choices[0]
	base.Classification = primary.Classification
	base.Reason = primary.Reason
	base.Function = primary.Function
	base.CPU = primary.CPU
	base.Alloc = primary.Alloc

	if len(choices) > 1 {
		secondary := choices[1]
		base.Secondary = &secondary
	}

	return base
}

func compareChoice(left Choice, right Choice) int {
	leftRank := classificationRank(left.Classification)
	rightRank := classificationRank(right.Classification)

	if leftRank != rightRank {
		return leftRank - rightRank
	}

	leftScore := choiceScore(left)

	rightScore := choiceScore(right)
	if leftScore > rightScore {
		return -1
	}

	if leftScore < rightScore {
		return 1
	}

	if left.Function < right.Function {
		return -1
	}

	if left.Function > right.Function {
		return 1
	}

	return 0
}

func contains(items []string, want string) bool {
	return slices.Contains(items, want)
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

func isBenchmarkFunction(function string) bool {
	lastDot := strings.LastIndex(function, ".")
	if lastDot < 0 || lastDot == len(function)-1 {
		return false
	}

	return strings.HasPrefix(function[lastDot+1:], "Benchmark")
}

func isHelpRequest(args []string) bool {
	return len(args) == 1 && (args[0] == "-h" || args[0] == "--help")
}

func isProfileNoise(function string) bool {
	return strings.HasPrefix(function, "runtime.") ||
		strings.HasPrefix(function, "runtime/") ||
		strings.HasPrefix(function, "testing.") ||
		strings.HasPrefix(function, "testing/") ||
		strings.HasPrefix(function, "sync/atomic.") ||
		isBenchmarkFunction(function)
}

func isUserFunction(function string, prefixes []string) bool {
	if isProfileNoise(function) {
		return false
	}

	for _, prefix := range prefixes {
		if function == prefix || strings.HasPrefix(function, prefix+".") || strings.HasPrefix(function, prefix+"/") {
			return true
		}
	}

	return false
}

func makeChoice(classification string, reason string, function string, cpu pprofRow, alloc pprofRow) Choice {
	return Choice{
		Classification: classification,
		Reason:         reason,
		Function:       function,
		CPU:            cpuMetric(cpu),
		Alloc:          allocMetric(alloc),
	}
}

func meetsThreshold(row pprofRow, kind profileKind) bool {
	switch kind {
	case profileCPU:
		return row.FlatPct >= cpuFlatThreshold || row.CumPct >= cpuCumThreshold
	case profileAlloc:
		return row.FlatPct >= allocFlatThreshold || row.CumPct >= allocCumThreshold
	default:
		return false
	}
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

func metricScore(metric Metric, flatThreshold float64, cumThreshold float64) float64 {
	return max(metric.FlatPct/flatThreshold, metric.CumPct/cumThreshold)
}

func normalizeSymbol(symbol string) string {
	symbol = inlineMarker.ReplaceAllString(strings.TrimSpace(symbol), "")
	symbol = strings.TrimPrefix(symbol, "vendor/")

	if index := strings.Index(symbol, "/vendor/"); index >= 0 {
		symbol = symbol[index+len("/vendor/"):]
	}

	return spacePattern.ReplaceAllString(symbol, " ")
}

// Start Parser Section

func parseArgs(args []string) (options, error) {
	opts := options{bench: defaultBench, benchtime: defaultBenchtime, count: defaultCount, format: defaultFormat, pkg: ""}
	flags := flag.NewFlagSet("verdict hotspot", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.bench, "bench", opts.bench, "benchmark regexp")
	flags.StringVar(&opts.benchtime, "benchtime", opts.benchtime, "benchmark duration or Nx count")
	flags.IntVar(&opts.count, "count", opts.count, "benchmark run count")
	flags.StringVar(&opts.format, "format", opts.format, "output format: text or json")

	err := flags.Parse(args)
	if err != nil {
		return options{}, fmt.Errorf("parsing hotspot flags: %w", err)
	}

	if flags.NArg() != 1 {
		return options{}, errMissingPackage
	}

	opts.pkg = flags.Arg(0)

	if opts.count < 1 {
		return options{}, errInvalidCount
	}

	if !validBenchtime(opts.benchtime) {
		return options{}, errInvalidBenchtime
	}

	if opts.format != defaultFormat && opts.format != formatJSON {
		return options{}, errInvalidFormat
	}

	return opts, nil
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
	default:
		return 0, false
	}
}

// /END Parser Section

func pprofInvocation(binaryPath string, profilePath string, kind profileKind) invocation {
	args := []string{"tool", "pprof", "-top"}
	if kind == profileAlloc {
		args = append(args, "-alloc_space")
	}

	args = append(args, "-nodecount=50", binaryPath, profilePath)

	return invocation{Dir: "", Name: "go", Args: args}
}

func profileCandidates(rows map[string]pprofRow, kind profileKind) map[string]pprofRow {
	result := make(map[string]pprofRow)

	for function, row := range rows {
		if meetsThreshold(row, kind) {
			result[function] = row
		}
	}

	return result
}

func rowsByFunction(rows []pprofRow) map[string]pprofRow {
	result := make(map[string]pprofRow)

	for _, row := range rows {
		result[row.Function] = mergeRows(result[row.Function], row)
	}

	return result
}

func userRows(rows map[string]pprofRow, prefixes []string) map[string]pprofRow {
	result := make(map[string]pprofRow)

	for _, row := range rows {
		if !isUserFunction(row.Function, prefixes) {
			continue
		}

		result[row.Function] = row
	}

	return result
}

func validBenchtime(value string) bool {
	if benchtimeCount.MatchString(value) {
		return true
	}

	duration, err := time.ParseDuration(value)

	return err == nil && duration > 0
}

func withCaveat(text string, caveat string) string {
	if caveat == "" {
		return text
	}

	return text + "Caveat: " + caveat + "\n"
}

func writeText(output io.Writer, text string) error {
	_, err := fmt.Fprint(output, text)
	if err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	return nil
}

func zeroMetric() Metric {
	return Metric{FlatMS: 0, FlatBytes: 0, FlatPct: 0, CumMS: 0, CumBytes: 0, CumPct: 0}
}

func zeroPprofRow() pprofRow {
	return pprofRow{Function: "", Flat: 0, FlatPct: 0, Cum: 0, CumPct: 0}
}
