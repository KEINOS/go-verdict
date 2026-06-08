package spacenorm

import "strings"

// CollapseSpace replaces each run of Unicode whitespace with a single ASCII
// space and trims leading and trailing whitespace.
func CollapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
