package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"strings"
)

const (
	sourceKindDirectory = "directory"
	sourceKindGit       = "git"
	sourceKindWorktree  = "worktree"
)

var (
	errInvalidComplexityConfig = errors.New("invalid complexity configuration")
	errMultipleJSONValues      = errors.New("multiple JSON values")
	errRepeatedComplexityFile  = errors.New("complexity-config may be specified only once")
)

type complexityOptions struct {
	mappings  map[string]complexityMapping
	requested bool
}

type complexityConfig struct {
	Version    int                 `json:"version"`
	Benchmarks []complexityMapping `json:"benchmarks"`
}

type complexityMapping struct {
	Benchmark string        `json:"benchmark"`
	Baseline  sourceMapping `json:"baseline"`
	Candidate sourceMapping `json:"candidate"`
}

type sourceMapping struct {
	Kind   string `json:"kind"`
	Root   string `json:"root,omitempty"`
	Ref    string `json:"ref,omitempty"`
	File   string `json:"file"`
	Symbol string `json:"symbol"`
}

type stringListFlag []string

func (values *stringListFlag) String() string {
	return strings.Join(*values, ",")
}

func (values *stringListFlag) Set(value string) error {
	*values = append(*values, value)

	return nil
}

type singlePathFlag struct {
	value string
	seen  bool
}

func (option *singlePathFlag) String() string {
	return option.value
}

func (option *singlePathFlag) Set(value string) error {
	if option.seen {
		return errRepeatedComplexityFile
	}

	option.value = value
	option.seen = true

	return nil
}

func parseComplexityOptions(inline []string, configPath string, configSeen bool) (complexityOptions, error) {
	result := complexityOptions{
		mappings:  make(map[string]complexityMapping),
		requested: len(inline) > 0 || configSeen,
	}
	if !result.requested {
		return result, nil
	}

	err := addConfigMappings(result.mappings, configPath, configSeen)
	if err != nil {
		return complexityOptions{}, err
	}

	inlineMappings, err := parseInlineMappings(inline)
	if err != nil {
		return complexityOptions{}, err
	}

	maps.Copy(result.mappings, inlineMappings)

	return result, nil
}

func addConfigMappings(target map[string]complexityMapping, path string, seen bool) error {
	if !seen {
		return nil
	}

	config, err := readComplexityConfig(path)
	if err != nil {
		return err
	}

	return addMappings(target, config.Benchmarks, "config")
}

func parseInlineMappings(inputs []string) (map[string]complexityMapping, error) {
	mappings := make(map[string]complexityMapping)

	for index, input := range inputs {
		var mapping complexityMapping

		err := decodeStrictJSON(strings.NewReader(input), &mapping)
		if err != nil {
			return nil, fmt.Errorf("%w: parsing --complexity %d: %w",
				errInvalidComplexityConfig, index+1, err)
		}

		err = validateComplexityMapping(mapping)
		if err != nil {
			return nil, err
		}

		if _, exists := mappings[mapping.Benchmark]; exists {
			return nil, fmt.Errorf(
				"%w: duplicate inline complexity benchmark %q",
				errInvalidComplexityConfig,
				mapping.Benchmark,
			)
		}

		mappings[mapping.Benchmark] = mapping
	}

	return mappings, nil
}

func readComplexityConfig(path string) (complexityConfig, error) {
	//nolint:gosec // The user explicitly selects the configuration file.
	file, err := os.Open(path)
	if err != nil {
		return complexityConfig{}, fmt.Errorf("%w: reading --complexity-config: %w",
			errInvalidComplexityConfig, err)
	}
	defer func() { _ = file.Close() }()

	var config complexityConfig

	err = decodeStrictJSON(file, &config)
	if err != nil {
		return complexityConfig{}, fmt.Errorf("%w: parsing --complexity-config: %w",
			errInvalidComplexityConfig, err)
	}

	if config.Version != 1 {
		return complexityConfig{}, fmt.Errorf("%w: config version must be 1",
			errInvalidComplexityConfig)
	}

	return config, nil
}

func addMappings(target map[string]complexityMapping, mappings []complexityMapping, source string) error {
	for _, mapping := range mappings {
		err := validateComplexityMapping(mapping)
		if err != nil {
			return err
		}

		if _, exists := target[mapping.Benchmark]; exists {
			return fmt.Errorf(
				"%w: duplicate %s complexity benchmark %q",
				errInvalidComplexityConfig,
				source,
				mapping.Benchmark,
			)
		}

		target[mapping.Benchmark] = mapping
	}

	return nil
}

func validateComplexityMapping(mapping complexityMapping) error {
	if mapping.Benchmark == "" {
		return fmt.Errorf("%w: benchmark is required", errInvalidComplexityConfig)
	}

	err := validateSourceMapping("baseline", mapping.Baseline)
	if err != nil {
		return err
	}

	return validateSourceMapping("candidate", mapping.Candidate)
}

func validateSourceMapping(side string, source sourceMapping) error {
	if source.File == "" || source.Symbol == "" {
		return fmt.Errorf("%w: %s file and symbol are required", errInvalidComplexityConfig, side)
	}

	switch source.Kind {
	case sourceKindWorktree:
		return validateWorktreeSource(side, source)
	case sourceKindGit:
		return validateGitSource(side, source)
	case sourceKindDirectory:
		return validateDirectorySource(side, source)
	default:
		return fmt.Errorf("%w: %s has unsupported source kind %q",
			errInvalidComplexityConfig, side, source.Kind)
	}
}

func validateWorktreeSource(side string, source sourceMapping) error {
	if source.Root != "" || source.Ref != "" {
		return fmt.Errorf("%w: %s worktree does not accept root or ref",
			errInvalidComplexityConfig, side)
	}

	return nil
}

func validateGitSource(side string, source sourceMapping) error {
	if source.Root != "" || source.Ref == "" {
		return fmt.Errorf("%w: %s git requires ref and does not accept root",
			errInvalidComplexityConfig, side)
	}

	return nil
}

func validateDirectorySource(side string, source sourceMapping) error {
	if source.Root == "" || source.Ref != "" {
		return fmt.Errorf("%w: %s directory requires root and does not accept ref",
			errInvalidComplexityConfig, side)
	}

	return nil
}

func decodeStrictJSON(reader io.Reader, target any) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()

	err := decoder.Decode(target)
	if err != nil {
		return fmt.Errorf("decoding JSON: %w", err)
	}

	var extra any

	err = decoder.Decode(&extra)
	if !errors.Is(err, io.EOF) {
		if err == nil {
			return errMultipleJSONValues
		}

		return fmt.Errorf("decoding trailing JSON: %w", err)
	}

	return nil
}
