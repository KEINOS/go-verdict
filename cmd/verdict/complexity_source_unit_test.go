package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var errFakeComplexityGit = errors.New("fake Git failure")

func TestNewComplexityResolverUsesTheCurrentDirectory(t *testing.T) {
	t.Parallel()

	resolver := newComplexityResolver()
	require.NotEmpty(t, resolver.workingDir)
	require.NotNil(t, resolver.git)
}

func TestResolveFilesystemComplexitySources(t *testing.T) {
	t.Parallel()

	worktree := createComplexityModule(t, "example.com/worktree", simpleComplexitySource())
	directory := createComplexityModule(t, "example.com/directory", branchyComplexitySource())
	resolver := complexityResolver{workingDir: filepath.Join(worktree, "pkg"), git: nil}

	worktreeResult, err := resolver.resolve(sourceMappingOf(
		sourceKindWorktree,
		"",
		"",
		"pkg/work.go",
		"example.com/worktree/pkg.Work",
	))
	require.NoError(t, err)
	require.Equal(t, 1, worktreeResult.Cyclomatic)

	relativeDirectory, err := filepath.Rel(resolver.workingDir, directory)
	require.NoError(t, err)

	directoryResult, err := resolver.resolve(sourceMappingOf(
		sourceKindDirectory,
		relativeDirectory,
		"",
		"pkg/work.go",
		"example.com/directory/pkg.Work",
	))
	require.NoError(t, err)
	require.Greater(t, directoryResult.Score, worktreeResult.Score)
	require.Equal(t, sourceKindDirectory, directoryResult.Kind)
}

func TestResolveGitComplexitySourceReadsCommittedBlobsWithoutCheckout(t *testing.T) {
	t.Parallel()

	repository := createComplexityModule(t, "example.com/project", simpleComplexitySource())
	runTestGit(t, repository, "init")
	runTestGit(t, repository, "config", "user.email", "verdict@example.com")
	runTestGit(t, repository, "config", "user.name", "Verdict Test")
	runTestGit(t, repository, "add", ".")
	runTestGit(t, repository, "commit", "-m", "baseline")
	writeTestFile(t, filepath.Join(repository, "pkg", "work.go"), branchyComplexitySource())

	resolver := complexityResolver{workingDir: filepath.Join(repository, "pkg"), git: runGitCommand}
	got, err := resolver.resolve(sourceMappingOf(
		sourceKindGit,
		"",
		"HEAD",
		"pkg/work.go",
		"example.com/project/pkg.Work",
	))
	require.NoError(t, err)
	require.Equal(t, 1, got.Cyclomatic, "the uncommitted worktree change must not affect the Git blob")

	_, err = resolver.resolve(sourceMappingOf(
		sourceKindGit,
		"",
		"missing",
		"pkg/work.go",
		"example.com/project/pkg.Work",
	))
	require.ErrorContains(t, err, "reading go.mod")

	_, err = runGitCommand(repository, "not-a-command")
	require.ErrorContains(t, err, "git not-a-command")
}

func TestResolveGitComplexitySourceUsesTheSelectedModuleAtTheRef(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	moduleRoot := filepath.Join(repository, "nested")
	writeTestFile(t, filepath.Join(moduleRoot, "go.mod"), "module example.com/nested\n")
	writeTestFile(t, filepath.Join(moduleRoot, "pkg", "work.go"), branchyComplexitySource())

	calls := make([][]string, 0)
	resolver := complexityResolver{
		workingDir: filepath.Join(moduleRoot, "pkg"),
		git: func(_ string, args ...string) ([]byte, error) {
			calls = append(calls, slices.Clone(args))

			switch strings.Join(args, " ") {
			case "rev-parse --show-toplevel":
				return []byte(repository + "\n"), nil
			case "cat-file blob baseline:nested/go.mod":
				return []byte("module example.com/nested\n"), nil
			case "cat-file blob baseline:nested/pkg/work.go":
				return []byte(simpleComplexitySource()), nil
			default:
				return nil, errFakeComplexityGit
			}
		},
	}

	got, err := resolver.resolve(sourceMappingOf(
		sourceKindGit,
		"",
		"baseline",
		"pkg/work.go",
		"example.com/nested/pkg.Work",
	))
	require.NoError(t, err)
	require.Equal(t, 1, got.Cyclomatic)
	require.Equal(t, "baseline", got.Ref)
	require.Len(t, calls, 3)
}

func TestResolveComplexitySourceRejectsUnsafeOrMissingInputs(t *testing.T) {
	t.Parallel()

	moduleRoot := createComplexityModule(t, "example.com/project", simpleComplexitySource())
	resolver := complexityResolver{workingDir: filepath.Join(moduleRoot, "pkg"), git: nil}
	tests := map[string]struct {
		source sourceMapping
		want   string
	}{
		"path escape": {
			source: sourceMappingOf(
				sourceKindWorktree,
				"",
				"",
				"../outside.go",
				"example.com/project.Work",
			),
			want: "module-relative",
		},
		"non Go file": {
			source: sourceMappingOf(
				sourceKindWorktree,
				"",
				"",
				"go.mod",
				"example.com/project.Work",
			),
			want: ".go file",
		},
		"unsupported kind": {
			source: sourceMappingOf("archive", "", "", "pkg/work.go", "example.com/project/pkg.Work"),
			want:   "unsupported kind",
		},
		"missing symbol": {
			source: sourceMappingOf(
				sourceKindWorktree,
				"",
				"",
				"pkg/work.go",
				"example.com/project/pkg.Missing",
			),
			want: "exactly one function",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := resolver.resolve(test.source)
			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestResolveComplexitySourceReportsFilesystemErrors(t *testing.T) {
	t.Parallel()

	moduleRoot := createComplexityModule(t, "example.com/project", simpleComplexitySource())
	resolver := complexityResolver{workingDir: filepath.Join(moduleRoot, "pkg"), git: nil}

	brokenRoot := filepath.Join(t.TempDir(), "missing")
	_, err := resolver.resolve(sourceMappingOf(
		sourceKindDirectory,
		brokenRoot,
		"",
		"pkg/work.go",
		"example.com/project/pkg.Work",
	))
	require.ErrorContains(t, err, "resolving module root")

	noModule := t.TempDir()
	_, err = resolver.resolve(sourceMappingOf(
		sourceKindDirectory,
		noModule,
		"",
		"pkg/work.go",
		"example.com/project/pkg.Work",
	))
	require.ErrorContains(t, err, "reading go.mod")

	writeTestFile(t, filepath.Join(noModule, "go.mod"), "go 1.26\n")
	_, err = resolver.resolve(sourceMappingOf(
		sourceKindDirectory,
		noModule,
		"",
		"pkg/work.go",
		"example.com/project/pkg.Work",
	))
	require.ErrorContains(t, err, "go.mod has no module path")

	writeTestFile(t, filepath.Join(moduleRoot, "pkg", "broken.go"), "package pkg\nfunc Broken(\n")

	_, err = resolver.resolve(sourceMappingOf(
		sourceKindWorktree,
		"",
		"",
		"pkg/broken.go",
		"example.com/project/pkg.Broken",
	))
	require.ErrorContains(t, err, "analyzing pkg")
}

func TestResolveContainedComplexitySourceErrors(t *testing.T) {
	t.Parallel()

	moduleRoot := createComplexityModule(t, "example.com/project", simpleComplexitySource())
	resolver := complexityResolver{workingDir: filepath.Join(moduleRoot, "pkg"), git: nil}

	_, err := resolver.resolve(sourceMappingOf(
		sourceKindWorktree,
		"",
		"",
		"pkg/missing.go",
		"example.com/project/pkg.Missing",
	))
	require.ErrorContains(t, err, "resolving pkg")

	directorySource := filepath.Join(moduleRoot, "pkg", "directory.go")
	require.NoError(t, os.Mkdir(directorySource, 0o750))

	_, err = resolver.resolve(sourceMappingOf(
		sourceKindWorktree,
		"",
		"",
		"pkg/directory.go",
		"example.com/project/pkg.Directory",
	))
	require.ErrorContains(t, err, "reading pkg")

	outside := filepath.Join(t.TempDir(), "outside.go")
	writeTestFile(t, outside, simpleComplexitySource())

	link := filepath.Join(moduleRoot, "pkg", "outside.go")

	err = os.Symlink(outside, link)
	if err == nil {
		_, err = resolver.resolve(sourceMappingOf(
			sourceKindWorktree,
			"",
			"",
			"pkg/outside.go",
			"example.com/project/pkg.Work",
		))
		require.ErrorContains(t, err, "escapes module root")
	}

	rootFile := filepath.Join(t.TempDir(), "module")
	writeTestFile(t, rootFile, "module example.com/project\n")
	_, _, err = openModuleRoot(rootFile)
	require.ErrorContains(t, err, "opening module root")
}

func TestResolveGitComplexitySourceReportsResolutionErrors(t *testing.T) {
	t.Parallel()

	moduleRoot := createComplexityModule(t, "example.com/project", simpleComplexitySource())
	source := sourceMappingOf(
		sourceKindGit,
		"",
		"HEAD",
		"pkg/work.go",
		"example.com/project/pkg.Work",
	)

	resolver := complexityResolver{workingDir: filepath.Join(moduleRoot, "pkg"), git: nil}
	_, err := resolver.resolve(source)
	require.ErrorContains(t, err, "Git runner is unavailable")

	resolver.git = func(_ string, _ ...string) ([]byte, error) {
		return nil, errFakeComplexityGit
	}
	_, err = resolver.resolve(source)
	require.ErrorContains(t, err, "finding Git root")

	unsafe := source
	unsafe.Ref = "-HEAD"
	_, err = resolver.resolve(unsafe)
	require.ErrorContains(t, err, "unsafe Git ref")

	invalidFile := source
	invalidFile.File = "../work.go"
	_, err = resolver.resolve(invalidFile)
	require.ErrorContains(t, err, "module-relative")

	withoutModule := complexityResolver{workingDir: t.TempDir(), git: resolver.git}
	_, err = withoutModule.resolve(source)
	require.ErrorContains(t, err, "go.mod")

	_, err = withoutModule.resolve(sourceMappingOf(
		sourceKindWorktree,
		"",
		"",
		"pkg/work.go",
		"example.com/project/pkg.Work",
	))
	require.ErrorContains(t, err, "go.mod")
}

func TestResolveGitComplexitySourceReportsBlobAndLocationErrors(t *testing.T) {
	t.Parallel()

	moduleRoot := createComplexityModule(t, "example.com/project", simpleComplexitySource())
	source := sourceMappingOf(
		sourceKindGit,
		"",
		"HEAD",
		"pkg/work.go",
		"example.com/project/pkg.Work",
	)

	repository := moduleRoot
	resolver := complexityResolver{
		workingDir: filepath.Join(moduleRoot, "pkg"),
		git:        stagedGitRunner(repository, []byte("module example.com/project\n"), nil),
	}
	_, err := resolver.resolve(source)
	require.ErrorContains(t, err, "reading pkg/work.go")

	resolver.git = stagedGitRunner(repository, []byte("go 1.26\n"), []byte(simpleComplexitySource()))
	_, err = resolver.resolve(source)
	require.ErrorContains(t, err, "go.mod has no module path")

	resolver.git = func(_ string, _ ...string) ([]byte, error) {
		return []byte(filepath.Join(t.TempDir(), "missing")), nil
	}
	_, _, err = resolver.gitModuleLocation(moduleRoot)
	require.ErrorContains(t, err, "resolving Git root")

	resolver.git = func(_ string, _ ...string) ([]byte, error) {
		return []byte(repository), nil
	}
	_, _, err = resolver.gitModuleLocation(filepath.Join(t.TempDir(), "missing"))
	require.ErrorContains(t, err, "resolving module root")

	otherModule := createComplexityModule(t, "example.com/other", simpleComplexitySource())
	_, _, err = resolver.gitModuleLocation(otherModule)
	require.ErrorContains(t, err, "module outside Git root")
}

func TestFindModuleRootRejectsDirectoryWithoutGoMod(t *testing.T) {
	t.Parallel()

	_, err := findModuleRoot(t.TempDir())
	require.ErrorContains(t, err, "go.mod")
}

func TestPathHelpersReportPlatformPathErrors(t *testing.T) {
	t.Parallel()

	pathError := func(string) (string, error) {
		return "", errFakeComplexityGit
	}
	_, err := findModuleRootWith(".", pathError)
	require.ErrorContains(t, err, "resolving start directory")

	relError := func(string, string) (string, error) {
		return "", errFakeComplexityGit
	}
	_, err = relativeWithinWith(".", "target", relError)
	require.ErrorContains(t, err, "computing relative path")
}

func createComplexityModule(t *testing.T, modulePath string, source string) string {
	t.Helper()

	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "go.mod"), "module "+modulePath+"\n")
	writeTestFile(t, filepath.Join(root, "pkg", "work.go"), source)

	return root
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func sourceMappingOf(kind string, root string, ref string, file string, symbol string) sourceMapping {
	return sourceMapping{Kind: kind, Root: root, Ref: ref, File: file, Symbol: symbol}
}

func simpleComplexitySource() string {
	return "package pkg\nfunc Work() {}\n"
}

func branchyComplexitySource() string {
	return "package pkg\nfunc Work(value int) int {\n" +
		"if value > 0 { return value }; if value < 0 { return -value }; return 0\n}\n"
}

func runTestGit(t *testing.T, directory string, args ...string) {
	t.Helper()

	_, err := runGitCommand(directory, args...)
	require.NoError(t, err)
}

func stagedGitRunner(repository string, moduleData []byte, sourceData []byte) gitRunner {
	return func(_ string, args ...string) ([]byte, error) {
		switch strings.Join(args, " ") {
		case "rev-parse --show-toplevel":
			return []byte(repository), nil
		case "cat-file blob HEAD:go.mod":
			return moduleData, nil
		case "cat-file blob HEAD:pkg/work.go":
			if sourceData == nil {
				return nil, errFakeComplexityGit
			}

			return sourceData, nil
		default:
			return nil, errFakeComplexityGit
		}
	}
}
