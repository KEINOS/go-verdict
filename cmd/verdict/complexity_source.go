package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/KEINOS/go-verdict/complexity"
	"golang.org/x/mod/modfile"
)

var (
	errComplexityModule  = errors.New("resolving complexity module")
	errComplexitySource  = errors.New("resolving complexity source")
	errInvalidSourcePath = errors.New("source file must be a safe module-relative .go file")
)

type complexityMeasurement struct {
	Kind       string  `json:"kind"`
	Root       string  `json:"root,omitempty"`
	Ref        string  `json:"ref,omitempty"`
	File       string  `json:"file"`
	Symbol     string  `json:"symbol"`
	Cyclomatic int     `json:"cyclomatic"`
	Cognitive  int     `json:"cognitive"`
	Score      float64 `json:"score"`
}

type gitRunner func(string, ...string) ([]byte, error)

type complexityResolver struct {
	workingDir string
	git        gitRunner
}

func newComplexityResolver() complexityResolver {
	return complexityResolver{workingDir: ".", git: runGitCommand}
}

func (resolver complexityResolver) resolve(source sourceMapping) (complexityMeasurement, error) {
	switch source.Kind {
	case sourceKindWorktree:
		moduleRoot, err := findModuleRoot(resolver.workingDir)
		if err != nil {
			return complexityMeasurement{}, err
		}

		return resolveFilesystemSource(moduleRoot, source)
	case sourceKindDirectory:
		root := source.Root
		if !filepath.IsAbs(root) {
			root = filepath.Join(resolver.workingDir, root)
		}

		return resolveFilesystemSource(root, source)
	case sourceKindGit:
		return resolver.resolveGitSource(source)
	default:
		return complexityMeasurement{}, fmt.Errorf("%w: unsupported kind %q",
			errComplexitySource, source.Kind)
	}
}

func resolveFilesystemSource(root string, source sourceMapping) (complexityMeasurement, error) {
	canonicalRoot, moduleData, err := openModuleRoot(root)
	if err != nil {
		return complexityMeasurement{}, err
	}

	cleanFile, err := validateModuleFile(source.File)
	if err != nil {
		return complexityMeasurement{}, err
	}

	content, err := readContainedFile(canonicalRoot, cleanFile)
	if err != nil {
		return complexityMeasurement{}, err
	}

	return analyzeMappedSource(source, moduleData, cleanFile, content)
}

func (resolver complexityResolver) resolveGitSource(source sourceMapping) (complexityMeasurement, error) {
	if resolver.git == nil {
		return complexityMeasurement{}, fmt.Errorf("%w: Git runner is unavailable", errComplexitySource)
	}

	if strings.HasPrefix(source.Ref, "-") || strings.ContainsAny(source.Ref, ":\r\n") {
		return complexityMeasurement{}, fmt.Errorf("%w: unsafe Git ref %q", errComplexitySource, source.Ref)
	}

	moduleRoot, err := findModuleRoot(resolver.workingDir)
	if err != nil {
		return complexityMeasurement{}, err
	}

	cleanFile, err := validateModuleFile(source.File)
	if err != nil {
		return complexityMeasurement{}, err
	}

	repositoryRoot, moduleRelative, err := resolver.gitModuleLocation(moduleRoot)
	if err != nil {
		return complexityMeasurement{}, err
	}

	moduleFile := gitPath(moduleRelative, "go.mod")
	sourceFile := gitPath(moduleRelative, cleanFile)

	moduleData, err := resolver.git(repositoryRoot, "cat-file", "blob", source.Ref+":"+moduleFile)
	if err != nil {
		return complexityMeasurement{}, fmt.Errorf("%w: reading go.mod at Git ref %q: %w",
			errComplexitySource, source.Ref, err)
	}

	content, err := resolver.git(repositoryRoot, "cat-file", "blob", source.Ref+":"+sourceFile)
	if err != nil {
		return complexityMeasurement{}, fmt.Errorf("%w: reading %s at Git ref %q: %w",
			errComplexitySource, cleanFile, source.Ref, err)
	}

	return analyzeMappedSource(source, moduleData, cleanFile, content)
}

func (resolver complexityResolver) gitModuleLocation(moduleRoot string) (string, string, error) {
	repositoryOutput, err := resolver.git(moduleRoot, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", fmt.Errorf("%w: finding Git root: %w", errComplexitySource, err)
	}

	repositoryRoot, err := filepath.EvalSymlinks(strings.TrimSpace(string(repositoryOutput)))
	if err != nil {
		return "", "", fmt.Errorf("%w: resolving Git root: %w", errComplexitySource, err)
	}

	canonicalModuleRoot, err := filepath.EvalSymlinks(moduleRoot)
	if err != nil {
		return "", "", fmt.Errorf("%w: resolving module root: %w", errComplexitySource, err)
	}

	moduleRelative, err := relativeWithin(repositoryRoot, canonicalModuleRoot)
	if err != nil {
		return "", "", fmt.Errorf("%w: module outside Git root: %w", errComplexitySource, err)
	}

	return repositoryRoot, moduleRelative, nil
}

func findModuleRoot(start string) (string, error) {
	return findModuleRootWith(start, filepath.Abs)
}

func findModuleRootWith(start string, absolute func(string) (string, error)) (string, error) {
	current, err := absolute(start)
	if err != nil {
		return "", fmt.Errorf("%w: resolving start directory: %w", errComplexityModule, err)
	}

	for {
		info, statErr := os.Stat(filepath.Join(current, "go.mod"))
		if statErr == nil && info.Mode().IsRegular() {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("%w: no go.mod found from %s", errComplexityModule, start)
		}

		current = parent
	}
}

func openModuleRoot(root string) (string, []byte, error) {
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", nil, fmt.Errorf("%w: resolving module root: %w", errComplexityModule, err)
	}

	module, err := os.OpenRoot(canonicalRoot)
	if err != nil {
		return "", nil, fmt.Errorf("%w: opening module root: %w", errComplexityModule, err)
	}
	defer func() { _ = module.Close() }()

	data, err := module.ReadFile("go.mod")
	if err != nil {
		return "", nil, fmt.Errorf("%w: reading go.mod: %w", errComplexityModule, err)
	}

	if modfile.ModulePath(data) == "" {
		return "", nil, fmt.Errorf("%w: go.mod has no module path", errComplexityModule)
	}

	return canonicalRoot, data, nil
}

func validateModuleFile(name string) (string, error) {
	clean := filepath.Clean(name)
	if !filepath.IsLocal(clean) || filepath.Ext(clean) != ".go" {
		return "", fmt.Errorf("%w: %q", errInvalidSourcePath, name)
	}

	return clean, nil
}

func readContainedFile(root string, name string) ([]byte, error) {
	target, err := filepath.EvalSymlinks(filepath.Join(root, name))
	if err != nil {
		return nil, fmt.Errorf("%w: resolving %s: %w", errComplexitySource, name, err)
	}

	_, err = relativeWithin(root, target)
	if err != nil {
		return nil, fmt.Errorf("%w: %s escapes module root: %w", errComplexitySource, name, err)
	}

	content, err := os.ReadFile(target)
	if err != nil {
		return nil, fmt.Errorf("%w: reading %s: %w", errComplexitySource, name, err)
	}

	return content, nil
}

func relativeWithin(root string, target string) (string, error) {
	return relativeWithinWith(root, target, filepath.Rel)
}

func relativeWithinWith(
	root string,
	target string,
	relativePath func(string, string) (string, error),
) (string, error) {
	relative, err := relativePath(root, target)
	if err != nil {
		return "", fmt.Errorf("computing relative path: %w", err)
	}

	if relative != "." && !filepath.IsLocal(relative) {
		return "", errInvalidSourcePath
	}

	return relative, nil
}

func gitPath(moduleRelative string, name string) string {
	if moduleRelative == "." {
		return filepath.ToSlash(name)
	}

	return filepath.ToSlash(filepath.Join(moduleRelative, name))
}

func analyzeMappedSource(
	source sourceMapping,
	moduleData []byte,
	cleanFile string,
	content []byte,
) (complexityMeasurement, error) {
	modulePath := modfile.ModulePath(moduleData)
	if modulePath == "" {
		return complexityMeasurement{}, fmt.Errorf("%w: go.mod has no module path", errComplexityModule)
	}

	importPath := modulePath

	directory := filepath.ToSlash(filepath.Dir(cleanFile))
	if directory != "." {
		importPath += "/" + directory
	}

	stats, err := complexity.Analyze([]complexity.Source{{
		ImportPath: importPath,
		Name:       filepath.ToSlash(cleanFile),
		Content:    content,
	}})
	if err != nil {
		return complexityMeasurement{}, fmt.Errorf("%w: analyzing %s: %w",
			errComplexitySource, cleanFile, err)
	}

	matches := make([]complexity.Stat, 0, 1)

	for _, stat := range stats {
		if stat.Symbol == source.Symbol {
			matches = append(matches, stat)
		}
	}

	if len(matches) != 1 {
		return complexityMeasurement{}, fmt.Errorf(
			"%w: symbol %q must match exactly one function in %s; matched %d",
			errComplexitySource,
			source.Symbol,
			cleanFile,
			len(matches),
		)
	}

	stat := matches[0]

	return complexityMeasurement{
		Kind:       source.Kind,
		Root:       source.Root,
		Ref:        source.Ref,
		File:       source.File,
		Symbol:     source.Symbol,
		Cyclomatic: stat.Cyclomatic,
		Cognitive:  stat.Cognitive,
		Score:      complexity.Score(stat),
	}, nil
}

func runGitCommand(directory string, args ...string) ([]byte, error) {
	//nolint:gosec // Git arguments come from validated source mappings and fixed subcommands.
	command := exec.CommandContext(context.Background(), "git", args...)
	command.Dir = directory

	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}

	return output, nil
}
