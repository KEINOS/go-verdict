//go:build e2e

package e2e_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

const defaultScenarioTimeout = 30 * time.Second

var (
	errBothStdinSources      = errors.New("must not define both stdin and stdin_file")
	errCaseNameRequired      = errors.New("case name is required")
	errMultipleYAMLDocuments = errors.New("scenario file must contain exactly one YAML document")
	errSuiteCasesRequired    = errors.New("suite cases are required")
	errSuiteNameRequired     = errors.New("suite name is required")
	errSuiteRequired         = errors.New("suite is required")
)

// Suite represents a test suite containing multiple test cases.
type Suite struct {
	Name        string           `yaml:"name"`
	ScenarioDir string           `yaml:"-"`
	RepoRoot    string           `yaml:"-"`
	Cases       []Case           `yaml:"cases"`
	Timeout     ScenarioDuration `yaml:"timeout"`
}

// Case represents a single test case with its configuration and expected outcomes.
type Case struct {
	Name      string            `yaml:"name"`
	Args      []string          `yaml:"args"`
	Stdin     string            `yaml:"stdin"`
	StdinFile string            `yaml:"stdin_file"`
	Env       map[string]string `yaml:"env"`
	Want      Want              `yaml:"want"`
}

// Want represents the expected outcomes of a test case, including exit code and assertions for stdout and stderr.
type Want struct {
	Stderr   TextAssert `yaml:"stderr"`
	Stdout   TextAssert `yaml:"stdout"`
	ExitCode int        `yaml:"exit_code"`
}

// TextAssert represents assertions for text output.
type TextAssert struct {
	Equals      *string  `yaml:"equals"`
	Contains    []string `yaml:"contains"`
	NotContains []string `yaml:"not_contains"`
	Matches     []string `yaml:"matches"`
}

// ScenarioDuration decodes YAML duration strings such as "30s".
type ScenarioDuration struct {
	time.Duration
}

func decodeTestScenario(data []byte) (*Suite, error) {
	var suite Suite

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	err := decoder.Decode(&suite)
	if err != nil {
		return nil, fmt.Errorf("decode scenario YAML: %w", err)
	}

	var extra yaml.Node

	err = decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		errValidate := validateSuite(&suite)
		if errValidate != nil {
			return nil, errValidate
		}

		return &suite, nil
	} else if err != nil {
		return nil, fmt.Errorf("decode extra YAML document: %w", err)
	}

	return nil, errMultipleYAMLDocuments
}

func emptyTextAssert() TextAssert {
	return TextAssert{Equals: nil, Contains: nil, NotContains: nil, Matches: nil}
}

func (duration *ScenarioDuration) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode && value.Value == "" {
		return nil
	}

	parsed, err := time.ParseDuration(value.Value)
	if err != nil {
		return fmt.Errorf("parse timeout duration: %w", err)
	}

	duration.Duration = parsed

	return nil
}

func validateCase(suiteName string, index int, testCase Case) error {
	caseID := fmt.Sprintf("%s cases[%d]", suiteName, index)

	if testCase.Name == "" {
		return fmt.Errorf("%s: %w", caseID, errCaseNameRequired)
	}

	if testCase.Stdin != "" && testCase.StdinFile != "" {
		return fmt.Errorf("%s: %w", caseID, errBothStdinSources)
	}

	err := validateTextAssertRegex(caseID, "stdout", testCase.Want.Stdout)
	if err != nil {
		return err
	}

	err = validateTextAssertRegex(caseID, "stderr", testCase.Want.Stderr)
	if err != nil {
		return err
	}

	return nil
}

func validateSuite(suite *Suite) error {
	if suite == nil {
		return errSuiteRequired
	}

	if suite.Name == "" {
		return errSuiteNameRequired
	}

	if len(suite.Cases) == 0 {
		return fmt.Errorf("suite %q: %w", suite.Name, errSuiteCasesRequired)
	}

	for index, testCase := range suite.Cases {
		err := validateCase(suite.Name, index, testCase)
		if err != nil {
			return err
		}
	}

	return nil
}

func validateTextAssertRegex(caseID string, streamName string, want TextAssert) error {
	for _, pattern := range want.Matches {
		_, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("%s has invalid %s regex %q: %w", caseID, streamName, pattern, err)
		}
	}

	return nil
}
