// Package hotspot implements the verdict hotspot Scout command.
//
// The command compiles the target package's test binary, runs its benchmarks
// with CPU and allocation profiling, and suggests the first function to
// inspect. The package is split by responsibility: this file owns the command
// lifecycle and process execution, pprof.go parses profile output, classify.go
// selects hotspot candidates, and result.go formats the report.
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
	defaultPrefixCap = 2

	// profileSignalCount is the number of pprof views read from one run.
	profileSignalCount = 4
)

var (
	benchmarkRowPattern = regexp.MustCompile(`(?m)^Benchmark\S+\s+\d+`)
	benchtimeCount      = regexp.MustCompile(`^[1-9][0-9]*x$`)

	errInvalidBenchtime = errors.New("benchtime must be a Go benchmark duration or iteration count")
	errInvalidCount     = errors.New("count must be at least 1")
	errInvalidFormat    = errors.New("format must be text or json")
	errMissingPackage   = errors.New("hotspot requires exactly one package")
	errMultiplePackages = errors.New("hotspot supports exactly one package, not a multi-package pattern")
	errNilOutput        = errors.New("writing output: nil output writer")
)

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
	fast      bool
}

type packageInfo struct {
	Module *struct {
		Path string `json:"Path"`
	} `json:"Module"`
	ImportPath string `json:"ImportPath"`
	Dir        string `json:"Dir"`
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

	result = withFastCaveat(result, opts)

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

// runBenchmark runs the compiled benchmark binary once. An empty cpuPath or
// memPath leaves that profile out of the run.
func (command Command) runBenchmark(
	pkgDir string,
	binaryPath string,
	cpuPath string,
	memPath string,
	opts options,
) ([]byte, error) {
	args := []string{
		"-test.run=^$",
		"-test.bench=" + opts.bench,
		"-test.benchtime=" + opts.benchtime,
		"-test.count=" + strconv.Itoa(opts.count),
	}

	if cpuPath != "" {
		args = append(args, "-test.cpuprofile="+cpuPath)
	}

	if memPath != "" {
		args = append(args, "-test.memprofile="+memPath, "-test.memprofilerate=1")
	}

	output, err := command.runner.Run(invocation{Dir: pkgDir, Name: binaryPath, Args: args})
	if err != nil {
		return nil, fmt.Errorf("running benchmark workload: %w", err)
	}

	return output, nil
}

// runProfilingPasses collects the profiles and reports whether a benchmark
// workload actually ran. The default separates the CPU pass from the memory
// pass because "-test.memprofilerate=1" biases CPU samples toward allocation
// paths. The memory pass is skipped when the CPU pass ran no benchmark.
func (command Command) runProfilingPasses(
	pkgDir string,
	binaryPath string,
	cpuPath string,
	memPath string,
	opts options,
) (bool, error) {
	if opts.fast {
		output, err := command.runBenchmark(pkgDir, binaryPath, cpuPath, memPath, opts)
		if err != nil {
			return false, err
		}

		return benchmarkRowPattern.Match(output), nil
	}

	output, err := command.runBenchmark(pkgDir, binaryPath, cpuPath, "", opts)
	if err != nil {
		return false, err
	}

	if !benchmarkRowPattern.Match(output) {
		return false, nil
	}

	_, err = command.runBenchmark(pkgDir, binaryPath, "", memPath, opts)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (command Command) readProfiles(binaryPath string, cpuPath string, memPath string) (profileSet, error) {
	rows := make(map[profileKind]map[string]pprofRow, profileSignalCount)

	for _, spec := range profileSpecs(cpuPath, memPath) {
		parsed, err := command.readProfile(binaryPath, spec)
		if err != nil {
			return profileSet{}, err
		}

		rows[spec.kind] = parsed
	}

	return profileSet{
		CPU:          rows[profileCPU],
		Alloc:        rows[profileAlloc],
		AllocObjects: rows[profileAllocObjects],
		Inuse:        rows[profileInuse],
	}, nil
}

func (command Command) readProfile(binaryPath string, spec profileSpec) (map[string]pprofRow, error) {
	output, err := command.runner.Run(pprofInvocation(binaryPath, spec.path, spec.kind))
	if err != nil {
		return nil, fmt.Errorf("reading %s profile: %w", spec.label, err)
	}

	parsed, err := parseTop(output, spec.kind)
	if err != nil {
		return nil, fmt.Errorf("parsing %s profile: %w", spec.label, err)
	}

	return rowsByFunction(parsed), nil
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

	benchmarked, err := command.runProfilingPasses(pkgInfo.Dir, binaryPath, cpuPath, memPath, opts)
	if err != nil {
		return Result{}, err
	}

	if !benchmarked {
		result.Classification = classNoBenchmark
		result.Reason = classNoBenchmark
		result.Caveat = "No benchmark workload ran. Add BenchmarkXxx or pass --bench. See: verdict help bootstrap."

		return result, nil
	}

	profiles, err := command.readProfiles(binaryPath, cpuPath, memPath)
	if err != nil {
		return Result{}, err
	}

	return classify(result, profileSet{
		CPU:          userRows(profiles.CPU, pkgInfo.userPrefixes()),
		Alloc:        userRows(profiles.Alloc, pkgInfo.userPrefixes()),
		AllocObjects: userRows(profiles.AllocObjects, pkgInfo.userPrefixes()),
		Inuse:        userRows(profiles.Inuse, pkgInfo.userPrefixes()),
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
  --fast
      Profile CPU and memory in one benchmark pass instead of two.
      Halves the run time and lowers CPU accuracy.

Workflow guidance: verdict help hotspot
`
}

func contains(items []string, want string) bool {
	return slices.Contains(items, want)
}

func isHelpRequest(args []string) bool {
	return len(args) == 1 && (args[0] == "-h" || args[0] == "--help")
}

func parseArgs(args []string) (options, error) {
	opts := options{
		bench:     defaultBench,
		benchtime: defaultBenchtime,
		count:     defaultCount,
		format:    defaultFormat,
		pkg:       "",
		fast:      false,
	}
	flags := flag.NewFlagSet("verdict hotspot", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.bench, "bench", opts.bench, "benchmark regexp")
	flags.StringVar(&opts.benchtime, "benchtime", opts.benchtime, "benchmark duration or Nx count")
	flags.IntVar(&opts.count, "count", opts.count, "benchmark run count")
	flags.StringVar(&opts.format, "format", opts.format, "output format: text or json")
	flags.BoolVar(&opts.fast, "fast", opts.fast, "profile CPU and memory in one pass instead of two")

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

func validBenchtime(value string) bool {
	if benchtimeCount.MatchString(value) {
		return true
	}

	duration, err := time.ParseDuration(value)

	return err == nil && duration > 0
}

func writeText(output io.Writer, text string) error {
	_, err := fmt.Fprint(output, text)
	if err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	return nil
}
