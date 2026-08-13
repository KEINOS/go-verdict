// Package helptopic embeds on-demand workflow help topics for the verdict CLI.
//
// Each topic is one Markdown file under topics/ that explains a single step of
// the benchmark-guided optimization workflow. The CLI prints a topic with
// "verdict help <topic>" so detailed guidance stays out of the default help
// and out of the AI Agent skill text.
package helptopic

import (
	"embed"
	"errors"
	"fmt"
	"strings"
)

//go:embed topics/*.md
var topicFiles embed.FS

// fileReader abstracts file reading for testability.
type fileReader interface {
	ReadFile(name string) ([]byte, error)
}

// defaultFileReader wraps embed.FS to implement the fileReader interface.
type defaultFileReader struct {
	fs embed.FS
}

func (r defaultFileReader) ReadFile(name string) ([]byte, error) {
	data, err := r.fs.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("reading file %q: %w", name, err)
	}

	return data, nil
}

//nolint:gochecknoglobals // reader is an injection point for testing
var reader fileReader = defaultFileReader{fs: topicFiles}

// ErrUnknownTopic reports a help topic that does not exist.
var ErrUnknownTopic = errors.New("unknown help topic")

type topicEntry struct {
	name    string
	summary string
}

// topicEntries lists the topics in the order shown by the help index.
// The order follows the optimization workflow: bootstrap evidence, find the
// target, judge the change, then read the result.
func topicEntries() []topicEntry {
	return []topicEntry{
		{name: "bootstrap", summary: "Create a representative benchmark when none exists."},
		{name: "hotspot", summary: "Find the function to optimize from benchmark profiles."},
		{name: "benchstat", summary: "Judge a before/after edit of one implementation."},
		{name: "gotestbench", summary: "Judge two implementations compared in one benchmark run."},
		{name: "results", summary: "Interpret outcomes, the Pareto rule, and inconclusive results."},
	}
}

// Topics returns the available topic names in help index order.
func Topics() []string {
	entries := topicEntries()
	names := make([]string, 0, len(entries))

	for _, entry := range entries {
		names = append(names, entry.name)
	}

	return names
}

// Text returns the embedded Markdown text for one topic.
func Text(topic string) (string, error) {
	return TextWithReader(topic, reader)
}

// TextWithReader is the internal implementation that accepts a fileReader for testing.
func TextWithReader(topic string, r fileReader) (string, error) {
	if !isKnownTopic(topic) {
		return "", fmt.Errorf("%w: %q (available: %s)", ErrUnknownTopic, topic, strings.Join(Topics(), ", "))
	}

	data, err := r.ReadFile("topics/" + topic + ".md")
	if err != nil {
		return "", fmt.Errorf("reading help topic %q: %w", topic, err)
	}

	return string(data), nil
}

// IndexText returns the topic list printed by a plain "verdict help".
func IndexText() string {
	var builder strings.Builder

	builder.WriteString("Workflow help topics:\n\n")

	for _, entry := range topicEntries() {
		fmt.Fprintf(&builder, "  %-12s %s\n", entry.name, entry.summary)
	}

	builder.WriteString("\nUsage:\n  verdict help <topic>\n\nFor command flags, run: verdict --help\n")

	return builder.String()
}

func isKnownTopic(topic string) bool {
	for _, entry := range topicEntries() {
		if entry.name == topic {
			return true
		}
	}

	return false
}
