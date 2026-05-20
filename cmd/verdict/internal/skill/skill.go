// Package skill embeds the canonical AI Agent skill for verdict.
package skill

import _ "embed"

//go:embed SKILL.md
var text string

// Command implements the verdict CLI skill subcommand.
type Command struct{}

// New returns the skill subcommand.
func New() Command {
	return Command{}
}

// Run returns the canonical verdict AI Agent skill text.
func (Command) Run() (string, error) {
	return text, nil
}
