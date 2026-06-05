//go:build e2e

package e2e_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"unicode"

	"github.com/stretchr/testify/require"
)

const (
	envNameVerdictBin      = "VERDICT_BIN"
	envNameE2EScenariosDir = "VERDICT_E2E_SCENARIOS_DIR"
)

func findPathsTestScenarios(t *testing.T, pathTestScenariosDir string) []string {
	t.Helper()

	pathsYML, err := filepath.Glob(filepath.Join(pathTestScenariosDir, "*.yml"))
	require.NoError(t, err, "failed to glob .yml scenario files")

	pathsYAML, err := filepath.Glob(filepath.Join(pathTestScenariosDir, "*.yaml"))
	require.NoError(t, err, "failed to glob .yaml scenario files")

	pathsYML = append(pathsYML, pathsYAML...)
	slices.Sort(pathsYML)

	require.NotEmpty(t, pathsYML,
		"test scenarios directory should contain at least one .yml or .yaml file: %s", pathTestScenariosDir)

	return pathsYML
}

func getPathDirTestScenarios(t *testing.T) string {
	t.Helper()

	pathDirTestScenarios, ok := os.LookupEnv(envNameE2EScenariosDir)
	require.True(t, ok,
		"environment variable not set: %s", envNameE2EScenariosDir)

	return resolvedPath(t, pathDirTestScenarios)
}

func getPathTargetBin(t *testing.T) string {
	t.Helper()

	verdictBinEnv, ok := os.LookupEnv(envNameVerdictBin)
	require.True(t, ok,
		"environment variable %s is not set; please set it to the path of the verdict binary to test", envNameVerdictBin)

	return resolvedPath(t, verdictBinEnv)
}

func loadTestScenario(t *testing.T, path string) *Suite {
	t.Helper()

	path = filepath.Clean(path)

	data, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read scenario file %q", path)

	suite, err := decodeTestScenario(data)
	require.NoError(t, err, "failed to decode scenario file %q", path)
	suite.ScenarioDir = filepath.Dir(path)
	suite.RepoRoot = filepath.Clean(filepath.Join(suite.ScenarioDir, "..", ".."))

	return suite
}

func resolvedPath(t *testing.T, path string) string {
	t.Helper()

	for _, char := range path {
		if unicode.IsControl(char) {
			t.Fatalf("path contains control characters: %q", path)
		}
	}

	pathAbsClean, err := filepath.Abs(path)
	require.NoError(t, err, "failed to resolve path during test: %v", err)

	return pathAbsClean
}
