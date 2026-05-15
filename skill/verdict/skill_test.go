package verdictskill_test

import (
	"os"
	"testing"

	verdictskill "github.com/KEINOS/go-verdict/skill/verdict"
	"github.com/stretchr/testify/require"
)

func Test_is_embedded(t *testing.T) {
	t.Parallel()

	expectText, err := os.ReadFile("SKILL.md")
	require.NoError(t, err,
		"failed to read expected output from SKILL.md")

	actualText := verdictskill.Text
	require.Equal(t, string(expectText), actualText,
		"unexpected output text")
}
