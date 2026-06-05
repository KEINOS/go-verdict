//go:build e2e

package e2e_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type invalidScenarioCase struct {
	name    string
	data    string
	wantErr string
}

func Test_decodeTestScenarioRejectsMultipleDocuments(t *testing.T) {
	t.Parallel()

	data := []byte(`
name: help
cases:
  - name: show help
    want:
      exit_code: 0
---
name: another
cases:
  - name: show another
    want:
      exit_code: 0
`)

	_, err := decodeTestScenario(data)

	require.ErrorContains(t, err, "exactly one YAML document")
}

func Test_decodeTestScenarioRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	data := []byte(`
name: help
unknown_key: unexpected
cases:
  - name: show help
    want:
      exit_code: 0
`)

	_, err := decodeTestScenario(data)

	require.ErrorContains(t, err, "field unknown_key not found")
}

func Test_decodeTestScenarioRejectsInvalidSemantics(t *testing.T) {
	t.Parallel()

	for _, test := range invalidScenarioCases() {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := decodeTestScenario([]byte(test.data))

			require.ErrorContains(t, err, test.wantErr)
		})
	}
}

func invalidScenarioCases() []invalidScenarioCase {
	tests := invalidScenarioStructureCases()
	tests = append(tests, invalidScenarioRegexCases()...)

	return tests
}

func invalidScenarioRegexCases() []invalidScenarioCase {
	return []invalidScenarioCase{
		{
			name: "invalid stdout regex",
			data: `
name: help
cases:
  - name: bad stdout regex
    want:
      exit_code: 0
      stdout:
        matches:
          - "["
`,
			wantErr: "invalid stdout regex",
		},
		{
			name: "invalid stderr regex",
			data: `
name: help
cases:
  - name: bad stderr regex
    want:
      exit_code: 0
      stderr:
        matches:
          - "["
`,
			wantErr: "invalid stderr regex",
		},
	}
}

func invalidScenarioStructureCases() []invalidScenarioCase {
	return []invalidScenarioCase{
		{
			name: "invalid timeout",
			data: `
name: help
timeout: nope
cases:
  - name: show help
    want:
      exit_code: 0
`,
			wantErr: "parse timeout duration",
		},
		{
			name: "missing suite name",
			data: `
cases:
  - name: show help
    want:
      exit_code: 0
`,
			wantErr: "suite name is required",
		},
		{
			name: "empty cases",
			data: `
name: help
cases: []
`,
			wantErr: "suite cases are required",
		},
		{
			name: "missing case name",
			data: `
name: help
cases:
  - want:
      exit_code: 0
`,
			wantErr: "case name is required",
		},
		{
			name: "stdin and stdin_file",
			data: `
name: help
cases:
  - name: ambiguous stdin
    stdin: inline
    stdin_file: testdata/input.txt
    want:
      exit_code: 0
`,
			wantErr: "must not define both stdin and stdin_file",
		},
	}
}
