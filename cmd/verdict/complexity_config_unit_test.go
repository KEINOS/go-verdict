package main

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	testComplexityBenchmark = "BenchmarkWork-8"
	testBaselineSymbol      = "example.com/project/pkg.Original"
	testCandidateSymbol     = "example.com/project/pkg.Enhanced"
)

func TestParseComplexityOptionsMergesInlineOverConfig(t *testing.T) {
	t.Parallel()

	configPath := writeComplexityConfig(t, `{
  "version": 1,
  "benchmarks": [{
    "benchmark": "BenchmarkWork-8",
    "baseline": {"kind":"git","ref":"HEAD~1","file":"pkg/work.go","symbol":"example.com/project/pkg.Original"},
    "candidate": {"kind":"git","ref":"HEAD","file":"pkg/work.go","symbol":"example.com/project/pkg.Enhanced"}
  }]
}`)
	inline := `{
  "benchmark": "BenchmarkWork-8",
  "baseline": {"kind":"worktree","file":"pkg/work.go","symbol":"example.com/project/pkg.Original"},
  "candidate": {
    "kind":"directory","root":"../candidate","file":"pkg/work.go",
    "symbol":"example.com/project/pkg.Enhanced"
  }
}`

	got, err := parseComplexityOptions([]string{inline}, configPath, true)
	require.NoError(t, err)
	require.True(t, got.requested)
	require.Len(t, got.mappings, 1)
	require.Equal(t, sourceKindWorktree, got.mappings[testComplexityBenchmark].Baseline.Kind)
	require.Equal(t, sourceKindDirectory, got.mappings[testComplexityBenchmark].Candidate.Kind)
}

func TestParseComplexityOptionsRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	for name, test := range invalidComplexityCases() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			configPath := ""
			if test.configSeen {
				configPath = writeComplexityConfig(t, test.config)
			}

			_, err := parseComplexityOptions(test.inline, configPath, test.configSeen)
			require.ErrorContains(t, err, test.want)
		})
	}
}

type invalidComplexityCase struct {
	inline     []string
	config     string
	configSeen bool
	want       string
}

func invalidComplexityCases() map[string]invalidComplexityCase {
	cases := invalidMappingCases()
	maps.Copy(cases, invalidSourceCases())
	maps.Copy(cases, invalidJSONConfigCases())

	return cases
}

func invalidMappingCases() map[string]invalidComplexityCase {
	valid := `{
  "benchmark":"BenchmarkWork-8",
  "baseline":{"kind":"worktree","file":"pkg/work.go","symbol":"example.com/project/pkg.Original"},
  "candidate":{"kind":"worktree","file":"pkg/work.go","symbol":"example.com/project/pkg.Enhanced"}
}`

	return map[string]invalidComplexityCase{
		"empty benchmark": {
			inline:     []string{strings.Replace(valid, testComplexityBenchmark, "", 1)},
			config:     "",
			configSeen: false,
			want:       "benchmark is required",
		},
		"unknown inline field": {
			inline:     []string{`{"benchmark":"BenchmarkWork-8","unknown":true}`},
			config:     "",
			configSeen: false,
			want:       "unknown field",
		},
		"duplicate inline benchmark": {
			inline:     []string{valid, valid},
			config:     "",
			configSeen: false,
			want:       "duplicate inline complexity benchmark",
		},
		"missing mapping field": {
			inline:     []string{`{"benchmark":"BenchmarkWork-8"}`},
			config:     "",
			configSeen: false,
			want:       testBaselineLabel,
		},
		"missing candidate field": {
			inline: []string{`{
  "benchmark":"BenchmarkWork-8",
  "baseline":{"kind":"worktree","file":"pkg/work.go","symbol":"example.com/project/pkg.Original"}
}`},
			config:     "",
			configSeen: false,
			want:       testCandidateLabel,
		},
	}
}

func invalidSourceCases() map[string]invalidComplexityCase {
	return map[string]invalidComplexityCase{
		"unsupported source kind": {
			inline: []string{`{
  "benchmark":"BenchmarkWork-8",
  "baseline":{"kind":"archive","file":"pkg/work.go","symbol":"example.com/project/pkg.Original"},
  "candidate":{"kind":"worktree","file":"pkg/work.go","symbol":"example.com/project/pkg.Enhanced"}
}`},
			config:     "",
			configSeen: false,
			want:       "unsupported source kind",
		},
		"conflicting worktree field": {
			inline: []string{`{
  "benchmark":"BenchmarkWork-8",
  "baseline":{"kind":"worktree","ref":"HEAD","file":"pkg/work.go","symbol":"example.com/project/pkg.Original"},
  "candidate":{"kind":"worktree","file":"pkg/work.go","symbol":"example.com/project/pkg.Enhanced"}
}`},
			config:     "",
			configSeen: false,
			want:       "does not accept root or ref",
		},
		"Git source without ref": {
			inline: []string{`{
  "benchmark":"BenchmarkWork-8",
  "baseline":{"kind":"git","file":"pkg/work.go","symbol":"example.com/project/pkg.Original"},
  "candidate":{"kind":"worktree","file":"pkg/work.go","symbol":"example.com/project/pkg.Enhanced"}
}`},
			config:     "",
			configSeen: false,
			want:       "git requires ref",
		},
		"directory source without root": {
			inline: []string{`{
  "benchmark":"BenchmarkWork-8",
  "baseline":{"kind":"directory","file":"pkg/work.go","symbol":"example.com/project/pkg.Original"},
  "candidate":{"kind":"worktree","file":"pkg/work.go","symbol":"example.com/project/pkg.Enhanced"}
}`},
			config:     "",
			configSeen: false,
			want:       "directory requires root",
		},
	}
}

func invalidJSONConfigCases() map[string]invalidComplexityCase {
	valid := `{
  "benchmark":"BenchmarkWork-8",
  "baseline":{"kind":"worktree","file":"pkg/work.go","symbol":"example.com/project/pkg.Original"},
  "candidate":{"kind":"worktree","file":"pkg/work.go","symbol":"example.com/project/pkg.Enhanced"}
}`

	return map[string]invalidComplexityCase{
		"multiple JSON values": {
			inline:     []string{valid + " {}"},
			config:     "",
			configSeen: false,
			want:       "multiple JSON values",
		},
		"malformed trailing JSON": {
			inline:     []string{valid + " {"},
			config:     "",
			configSeen: false,
			want:       "decoding trailing JSON",
		},
		"unsupported version": {
			inline:     nil,
			config:     `{"version":2,"benchmarks":[]}`,
			configSeen: true,
			want:       "version must be 1",
		},
		"duplicate config benchmark": {
			inline:     nil,
			config:     `{"version":1,"benchmarks":[` + valid + "," + valid + `]}`,
			configSeen: true,
			want:       "duplicate config complexity benchmark",
		},
		"invalid config mapping": {
			inline:     nil,
			config:     `{"version":1,"benchmarks":[{"benchmark":"Foo-8"}]}`,
			configSeen: true,
			want:       testBaselineLabel,
		},
	}
}

func TestParseComplexityOptionsLeavesTheFeatureDisabledWithoutFlags(t *testing.T) {
	t.Parallel()

	got, err := parseComplexityOptions(nil, "", false)
	require.NoError(t, err)
	require.False(t, got.requested)
	require.Empty(t, got.mappings)
}

func TestParseComplexityOptionsReportsConfigFileErrors(t *testing.T) {
	t.Parallel()

	_, err := parseComplexityOptions(nil, filepath.Join(t.TempDir(), "missing.json"), true)
	require.ErrorContains(t, err, "reading --complexity-config")

	path := writeComplexityConfig(t, "{")
	_, err = parseComplexityOptions(nil, path, true)
	require.ErrorContains(t, err, "parsing --complexity-config")
}

func TestInitializeRejectsRepeatedComplexityConfig(t *testing.T) {
	t.Parallel()

	_, _, err := initialize([]string{"--complexity-config", "a.json", "--complexity-config", "b.json"})
	require.ErrorContains(t, err, "complexity-config may be specified only once")
}

func TestInitializeReportsInvalidComplexityJSON(t *testing.T) {
	t.Parallel()

	_, _, err := initialize([]string{"--complexity", "{"})
	require.ErrorIs(t, err, errInvalidComplexityConfig)
}

func writeComplexityConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "complexity.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}
