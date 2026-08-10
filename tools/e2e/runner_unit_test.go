//go:build e2e

package e2e_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stdinReaderCase struct {
	name     string
	testCase Case
	want     string
	wantErr  string
}

func Test_readerForStdin(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	pathFixture := filepath.Join(repoRoot, "testdata", "fixture.txt")
	require.NoError(t, os.MkdirAll(filepath.Dir(pathFixture), 0o750))
	require.NoError(t, os.WriteFile(pathFixture, []byte("from file"), 0o600))

	for _, test := range stdinReaderCases(pathFixture) {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			reader, err := readerForStdin(repoRoot, test.testCase)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)

				return
			}

			require.NoError(t, err)

			got, err := io.ReadAll(reader)
			require.NoError(t, err)
			assert.Equal(t, test.want, string(got))
		})
	}
}

func Test_runTestScenarioWithFakeBinary(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	pathBin := "/bin/sh"
	pathScript := writeFakeScript(t, repoRoot)
	pathInput := filepath.Join(repoRoot, "input.txt")
	require.NoError(t, os.WriteFile(pathInput, []byte("fixture input"), 0o600))

	var emptyStderr string

	suite := &Suite{
		Name:        "fake binary",
		Timeout:     ScenarioDuration{Duration: 0},
		ScenarioDir: "",
		RepoRoot:    repoRoot,
		Cases: []Case{
			{
				Name:      "reads stdin file",
				Args:      []string{pathScript},
				Stdin:     "",
				StdinFile: "input.txt",
				Env:       nil,
				Want: Want{
					ExitCode: 0,
					Stdout: TextAssert{
						Equals:   nil,
						Contains: []string{"args:", "stdin:fixture input"}, NotContains: nil,
						Matches:  nil,
					},
					Stderr: TextAssert{Equals: &emptyStderr, Contains: nil, NotContains: nil, Matches: nil},
				},
			},
			{
				Name:      "captures stderr and exit code",
				Args:      []string{pathScript, "--stderr", "warning", "--exit", "7"},
				Stdin:     "",
				StdinFile: "",
				Env:       nil,
				Want: Want{
					ExitCode: 7,
					Stdout:   TextAssert{Equals: nil, Contains: nil, NotContains: nil, Matches: nil},
					Stderr:   TextAssert{Equals: nil, Contains: []string{"warning"}, NotContains: nil, Matches: nil},
				},
			},
		},
	}

	runTestScenario(t, pathBin, suite)
}

func stdinReaderCases(pathFixture string) []stdinReaderCase {
	const inlineStdin = "inline"

	return []stdinReaderCase{
		{
			name:     "inline stdin",
			testCase: testCaseWithStdin(inlineStdin),
			want:     inlineStdin,
			wantErr:  "",
		},
		{
			name:     "repo relative stdin file",
			testCase: testCaseWithStdinFile("testdata/fixture.txt"),
			want:     "from file",
			wantErr:  "",
		},
		{
			name:     "absolute stdin file",
			testCase: testCaseWithStdinFile(pathFixture),
			want:     "from file",
			wantErr:  "",
		},
		{
			name:     "stdin and stdin file",
			testCase: testCaseWithBothStdinSources(inlineStdin, "testdata/fixture.txt"),
			want:     "",
			wantErr:  "must not define both stdin and stdin_file",
		},
		{
			name:     "missing stdin file",
			testCase: testCaseWithStdinFile("testdata/missing.txt"),
			want:     "",
			wantErr:  "read stdin_file",
		},
	}
}

func testCaseWithBothStdinSources(stdin string, stdinFile string) Case {
	testCase := testCaseWithStdin(stdin)
	testCase.StdinFile = stdinFile

	return testCase
}

func testCaseWithStdin(stdin string) Case {
	return Case{
		Name:      "stdin test case",
		Args:      nil,
		Stdin:     stdin,
		StdinFile: "",
		Env:       nil,
		Want:      Want{ExitCode: 0, Stdout: emptyTextAssert(), Stderr: emptyTextAssert()},
	}
}

func testCaseWithStdinFile(stdinFile string) Case {
	return Case{
		Name:      "stdin file test case",
		Args:      nil,
		Stdin:     "",
		StdinFile: stdinFile,
		Env:       nil,
		Want:      Want{ExitCode: 0, Stdout: emptyTextAssert(), Stderr: emptyTextAssert()},
	}
}

func writeFakeScript(t *testing.T, repoRoot string) string {
	t.Helper()

	pathBin := filepath.Join(repoRoot, "fake-verdict.sh")
	script := `#!/bin/sh
all_args="$*"
exit_code=0
stderr_text=""
while [ "$#" -gt 0 ]; do
	case "$1" in
		--exit)
			exit_code="$2"
			shift 2
			;;
		--stderr)
			stderr_text="$2"
			shift 2
			;;
		*)
			shift
			;;
	esac
done
stdin=$(cat)
printf 'args:%s\n' "$all_args"
printf 'stdin:%s\n' "$stdin"
if [ "$stderr_text" != "" ]; then
	printf '%s\n' "$stderr_text" >&2
fi
exit "$exit_code"
`

	require.NoError(t, os.WriteFile(pathBin, []byte(script), 0o600))

	return pathBin
}
