//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assertText(t *testing.T, streamName string, got string, want TextAssert) {
	t.Helper()

	if want.Equals != nil {
		assert.Equal(t, *want.Equals, got, "%s should exactly match", streamName)
	}

	for _, expected := range want.Contains {
		assert.Contains(t, got, expected, "%s should contain expected text", streamName)
	}

	for _, unexpected := range want.NotContains {
		assert.NotContains(t, got, unexpected, "%s should not contain text", streamName)
	}

	for _, pattern := range want.Matches {
		compiledPattern, err := regexp.Compile(pattern)
		require.NoError(t, err, "%s has invalid regex pattern: %s", streamName, pattern)

		assert.Regexp(t, compiledPattern, got, "%s should match regex pattern", streamName)
	}
}

func readerForStdin(repoRoot string, testCase Case) (io.Reader, error) {
	if testCase.Stdin != "" && testCase.StdinFile != "" {
		return nil, fmt.Errorf("case %q: %w", testCase.Name, errBothStdinSources)
	}

	if testCase.StdinFile == "" {
		return strings.NewReader(testCase.Stdin), nil
	}

	pathStdinFile := testCase.StdinFile
	if !filepath.IsAbs(pathStdinFile) {
		pathStdinFile = filepath.Join(repoRoot, pathStdinFile)
	}

	data, err := os.ReadFile(filepath.Clean(pathStdinFile))
	if err != nil {
		return nil, fmt.Errorf("read stdin_file %q: %w", testCase.StdinFile, err)
	}

	return bytes.NewReader(data), nil
}

func runTestCase(t *testing.T, pathVerdictBin string, repoRoot string, timeout time.Duration, testCase Case) {
	t.Helper()

	require.NotEmpty(t, testCase.Name, "test case should have a name")

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	//nolint:gosec // Subprocess execution is intentional due to the E2E nature of the test.
	cmd := exec.CommandContext(ctx, pathVerdictBin, testCase.Args...)

	cmd.Dir = repoRoot
	stdin, err := readerForStdin(repoRoot, testCase)
	require.NoError(t, err, "failed to prepare stdin")

	cmd.Stdin = stdin
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = os.Environ()

	for name, value := range testCase.Env {
		cmd.Env = append(cmd.Env, name+"="+value)
	}

	err = cmd.Run()

	require.NoError(t, ctx.Err(), "test case timed out after %s", timeout)

	exitCode := 0

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			require.NoError(t, err, "failed to run command")
		}
	}

	assert.Equal(t, testCase.Want.ExitCode, exitCode, "exit code mismatch")
	assertText(t, "stdout", stdout.String(), testCase.Want.Stdout)
	assertText(t, "stderr", stderr.String(), testCase.Want.Stderr)
}

func runTestScenario(t *testing.T, pathVerdictBin string, suite *Suite) {
	t.Helper()

	require.NotNil(t, suite, "test suite should not be nil")
	require.NoError(t, validateSuite(suite), "test suite should be valid")
	require.DirExists(t, suite.RepoRoot, "repository root should exist")

	t.Run(suite.Name, func(t *testing.T) {
		t.Parallel()

		timeout := suite.Timeout.Duration
		if timeout == 0 {
			timeout = defaultScenarioTimeout
		}

		for _, testCase := range suite.Cases {
			t.Run(testCase.Name, func(t *testing.T) {
				t.Parallel()

				runTestCase(t, pathVerdictBin, suite.RepoRoot, timeout, testCase)
			})
		}
	})
}
