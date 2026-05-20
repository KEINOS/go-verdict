package appver

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	testRevision = "0123456789abcdef"
	testVersion  = "v1.2.3"
)

type versionRunCase struct {
	name      string
	buildInfo *debug.BuildInfo
	ok        bool
	want      string
}

func TestCommandRun(t *testing.T) {
	t.Parallel()

	for _, test := range versionRunCases() {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			cmd := newWithBuildInfo(func() (*debug.BuildInfo, bool) {
				return test.buildInfo, test.ok
			})

			got, err := cmd.Run()
			require.NoError(t, err,
				"version command should not fail")
			require.Equal(t, test.want, got,
				"app version should match build info policy")
		})
	}
}

func versionRunCases() []versionRunCase {
	return []versionRunCase{
		{
			name:      "build info unavailable",
			buildInfo: nil,
			ok:        false,
			want:      "unknown (devel)",
		},
		{
			name:      "uses module version and short revision",
			buildInfo: testBuildInfo(testVersion, testRevision),
			ok:        true,
			want:      "v1.2.3 (0123456)",
		},
		{
			name:      "keeps short revision as-is",
			buildInfo: testBuildInfo(testVersion, "abc123"),
			ok:        true,
			want:      "v1.2.3 (abc123)",
		},
		{
			name:      "uses unknown revision when version exists and revision is missing",
			buildInfo: testBuildInfo("v2.0.0", ""),
			ok:        true,
			want:      "v2.0.0 (unknown)",
		},
		{
			name:      "uses revision with devel fallback when version is missing",
			buildInfo: testBuildInfo("", testRevision),
			ok:        true,
			want:      "0123456 (devel)",
		},
		{
			name:      "treats devel version as missing release version",
			buildInfo: testBuildInfo(defaultVersion, testRevision),
			ok:        true,
			want:      "0123456 (devel)",
		},
		{
			name:      "falls back when version and revision are empty",
			buildInfo: testBuildInfo("", ""),
			ok:        true,
			want:      "unknown (devel)",
		},
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	got, err := New().Run()
	require.NoError(t, err,
		"default version command should not fail")
	require.NotEmpty(t, got,
		"default version output should not be empty")
}

func testBuildInfo(version, revision string) *debug.BuildInfo {
	info := new(debug.BuildInfo)
	info.Main.Version = version

	if revision != "" {
		info.Settings = []debug.BuildSetting{
			{Key: vcsRevisionKey, Value: revision},
		}
	}

	return info
}
