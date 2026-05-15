// Package verdictskill embeds the canonical AI Agent skill for verdict.
package verdictskill

import _ "embed"

// Text is the canonical verdict AI Agent skill.
//
//go:embed SKILL.md
var Text string
