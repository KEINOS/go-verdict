//go:build e2e

/*
Package e2e_test contains the E2E test harness for the `verdict` binary.

Note: VS Code users need to pre-build the `verdict` binary before running each
test via CodeLens. Press `command+shift+B` to build the binary in `dist`.
*/
package e2e_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVerdict(t *testing.T) {
	t.Parallel()

	pathVerdictBin := getPathTargetBin(t)
	pathTestScenariosDir := getPathDirTestScenarios(t)

	require.FileExists(t, pathVerdictBin,
		"verdict binary does not exist at path: %s", pathVerdictBin)
	require.DirExists(t, pathTestScenariosDir,
		"test scenarios directory does not exist at path: %s", pathTestScenariosDir)

	pathsTestScenarios := findPathsTestScenarios(t, pathTestScenariosDir)

	for _, pathTestScenario := range pathsTestScenarios {
		testScenario := loadTestScenario(t, pathTestScenario)
		t.Logf("Scenario: %s", pathTestScenario)

		runTestScenario(t, pathVerdictBin, testScenario)
	}
}
