package skill

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCommandRunReturnsEmbeddedSkill(t *testing.T) {
	t.Parallel()

	want, err := os.ReadFile("SKILL.md")
	require.NoError(t, err,
		"failed to read expected output from SKILL.md")

	got, err := New().Run()
	require.NoError(t, err,
		"skill command should not fail")
	require.Equal(t, string(want), got,
		"unexpected skill command output")
}
