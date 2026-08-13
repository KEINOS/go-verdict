package helptopic

import (
	"embed"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTopicsMatchEmbeddedFiles(t *testing.T) {
	t.Parallel()

	entries, err := topicFiles.ReadDir("topics")
	require.NoError(t, err,
		"failed to read embedded topics directory")

	embedded := make([]string, 0, len(entries))
	for _, entry := range entries {
		embedded = append(embedded, strings.TrimSuffix(entry.Name(), ".md"))
	}

	require.ElementsMatch(t, embedded, Topics(),
		"topic index should list exactly the embedded topic files")
}

func TestTextReturnsEachTopic(t *testing.T) {
	t.Parallel()

	for _, topic := range Topics() {
		text, err := Text(topic)
		require.NoError(t, err,
			"failed to read topic %q", topic)
		require.True(t, strings.HasPrefix(text, "# "),
			"topic %q should start with a Markdown heading", topic)
		require.NotEmpty(t, strings.TrimSpace(text),
			"topic %q should not be empty", topic)
	}
}

func TestTextUnknownTopicListsAvailableTopics(t *testing.T) {
	t.Parallel()

	_, err := Text("no-such-topic")
	require.ErrorIs(t, err, ErrUnknownTopic,
		"unknown topic should return ErrUnknownTopic")

	for _, topic := range Topics() {
		require.ErrorContains(t, err, topic,
			"unknown topic error should list available topic %q", topic)
	}
}

// TestTextReportsReadFileError verifies that ReadFile errors are properly handled.
//
//nolint:paralleltest // Not parallel because it tests error path with mock
func TestTextReportsReadFileError(t *testing.T) {
	//nolint:err113,exhaustruct // test errors are fine to create dynamically; data field intentionally zero
	mockReader := &mockFileReader{
		err: errors.New("test read error"),
	}

	text, err := TextWithReader("hotspot", mockReader)
	require.Error(t, err)
	require.Empty(t, text)
	require.ErrorContains(t, err, "reading help topic")
	require.ErrorContains(t, err, "test read error")
}

// TestDefaultFileReaderReportsReadFileError verifies defaultFileReader error handling.
func TestDefaultFileReaderReportsReadFileError(t *testing.T) {
	t.Parallel()

	// Use an empty embed.FS that will fail to read any file
	var emptyFS embed.FS

	reader := defaultFileReader{fs: emptyFS}

	data, err := reader.ReadFile("topics/nonexistent.md")
	require.Error(t, err)
	require.Empty(t, data)
	require.ErrorContains(t, err, "reading file")
	require.ErrorContains(t, err, "nonexistent.md")
}

// mockFileReader is a test helper that implements fileReader.
type mockFileReader struct {
	data []byte
	err  error
}

func (m *mockFileReader) ReadFile(_ string) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}

	return m.data, nil
}

func TestIndexTextListsAllTopics(t *testing.T) {
	t.Parallel()

	index := IndexText()

	for _, topic := range Topics() {
		require.Contains(t, index, topic,
			"index should list topic %q", topic)
	}

	require.Contains(t, index, "verdict help <topic>",
		"index should explain how to open a topic")
	require.Contains(t, index, "verdict --help",
		"index should point to the flag help")
}

func TestTopicsGuideToNextStep(t *testing.T) {
	t.Parallel()

	// Each workflow topic should chain to the next step so an agent that only
	// fetched one topic still discovers the rest of the loop.
	nextReferences := map[string]string{
		"bootstrap":   "verdict help hotspot",
		"hotspot":     "verdict help benchstat",
		"benchstat":   "verdict help results",
		"gotestbench": "verdict help results",
	}

	for topic, want := range nextReferences {
		text, err := Text(topic)
		require.NoError(t, err,
			"failed to read topic %q", topic)
		require.Contains(t, text, want,
			"topic %q should reference the next workflow step", topic)
	}
}
