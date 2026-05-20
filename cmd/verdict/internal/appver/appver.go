// Package appver provides the verdict CLI version subcommand.
package appver

import (
	"fmt"
	"runtime/debug"
)

const (
	defaultRevision = "unknown"
	defaultVersion  = "(devel)"
	displayDevel    = "devel"
	revisionLen     = 7
	vcsRevisionKey  = "vcs.revision"
)

// Command implements the verdict CLI version subcommand.
type Command struct {
	readBuildInfo func() (*debug.BuildInfo, bool)
}

// New returns the version subcommand.
func New() Command {
	return newWithBuildInfo(debug.ReadBuildInfo)
}

func newWithBuildInfo(readBuildInfo func() (*debug.BuildInfo, bool)) Command {
	return Command{readBuildInfo: readBuildInfo}
}

// Run returns the CLI version text without a trailing newline.
func (cmd Command) Run() (string, error) {
	buildInfo, ok := cmd.readBuildInfo()
	if !ok || buildInfo == nil {
		return formatAppVersion("", ""), nil
	}

	return formatAppVersion(buildInfo.Main.Version, vcsRevision(buildInfo.Settings)), nil
}

func formatAppVersion(version, revision string) string {
	if version == defaultVersion {
		version = ""
	}

	revision = shortRevision(revision)
	if version != "" {
		if revision == "" {
			revision = defaultRevision
		}

		return fmt.Sprintf("%s (%s)", version, revision)
	}

	if revision == "" {
		revision = defaultRevision
	}

	return fmt.Sprintf("%s (%s)", revision, displayDevel)
}

func vcsRevision(settings []debug.BuildSetting) string {
	for _, setting := range settings {
		if setting.Key == vcsRevisionKey {
			return setting.Value
		}
	}

	return ""
}

func shortRevision(revision string) string {
	if len(revision) <= revisionLen {
		return revision
	}

	return revision[:revisionLen]
}
