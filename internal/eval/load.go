package eval

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

func DiscoverFixtureRoots(root string) ([]string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve eval root: %w", err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, fmt.Errorf("stat eval root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("eval root is not a directory: %s", absRoot)
	}

	var roots []string
	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() {
			return nil
		}
		scoringPath := filepath.Join(path, "scoring.yaml")
		if _, err := os.Stat(scoringPath); err == nil {
			roots = append(roots, path)
			if path != absRoot {
				return filepath.SkipDir
			}
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discover fixtures: %w", err)
	}
	sort.Strings(roots)
	return roots, nil
}

func LoadFixtures(root string) ([]Fixture, error) {
	roots, err := DiscoverFixtureRoots(root)
	if err != nil {
		return nil, err
	}
	fixtures := make([]Fixture, 0, len(roots))
	for _, fixtureRoot := range roots {
		fixture, err := LoadFixture(fixtureRoot)
		if err != nil {
			return nil, err
		}
		fixtures = append(fixtures, fixture)
	}
	return fixtures, nil
}

func LoadAndValidate(root string) ([]Fixture, error) {
	fixtures, err := LoadFixtures(root)
	if err != nil {
		return nil, err
	}
	if err := ValidateFixtures(fixtures); err != nil {
		return nil, err
	}
	return fixtures, nil
}

func LoadFixture(root string) (Fixture, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Fixture{}, fmt.Errorf("resolve fixture root %q: %w", root, err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return Fixture{}, fmt.Errorf("stat fixture root %q: %w", absRoot, err)
	}
	if !info.IsDir() {
		return Fixture{}, fmt.Errorf("fixture root is not a directory: %s", absRoot)
	}

	taskPath, err := resolveFixturePath(absRoot, "task.md", false)
	if err != nil {
		return Fixture{}, fmt.Errorf("fixture %s task.md: %w", absRoot, err)
	}
	repoDir, err := resolveFixturePath(absRoot, "repo", true)
	if err != nil {
		return Fixture{}, fmt.Errorf("fixture %s repo: %w", absRoot, err)
	}
	expectedDir, err := resolveFixturePath(absRoot, "expected", true)
	if err != nil {
		return Fixture{}, fmt.Errorf("fixture %s expected: %w", absRoot, err)
	}
	scoringPath, err := resolveFixturePath(absRoot, "scoring.yaml", false)
	if err != nil {
		return Fixture{}, fmt.Errorf("fixture %s scoring.yaml: %w", absRoot, err)
	}

	// Read fixture files through os.Root so they cannot resolve outside the
	// fixture directory (resolveFixturePath above reports the friendlier error).
	fixtureRoot, err := os.OpenRoot(absRoot)
	if err != nil {
		return Fixture{}, fmt.Errorf("open fixture root %q: %w", absRoot, err)
	}
	defer fixtureRoot.Close()

	taskBytes, err := fixtureRoot.ReadFile(filepath.Base(taskPath))
	if err != nil {
		return Fixture{}, fmt.Errorf("read task.md: %w", err)
	}
	taskText := strings.ReplaceAll(string(taskBytes), "\r\n", "\n")

	scoringBytes, err := fixtureRoot.ReadFile(filepath.Base(scoringPath))
	if err != nil {
		return Fixture{}, fmt.Errorf("read scoring.yaml: %w", err)
	}
	var cfg ScoringConfig
	dec := yaml.NewDecoder(bytes.NewReader(scoringBytes))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return Fixture{}, fmt.Errorf("parse scoring.yaml: %w", err)
	}
	applyDefaults(&cfg)

	recordingsDir := filepath.Join(absRoot, "recordings")
	if stat, err := os.Stat(recordingsDir); err == nil && stat.IsDir() {
		recordingsDir, err = resolveFixturePath(absRoot, "recordings", true)
		if err != nil {
			return Fixture{}, fmt.Errorf("fixture %s recordings: %w", absRoot, err)
		}
	} else {
		recordingsDir = ""
	}

	return Fixture{
		ID:            cfg.ID,
		Root:          absRoot,
		Task:          taskText,
		TaskPath:      taskPath,
		RepoDir:       repoDir,
		ExpectedDir:   expectedDir,
		RecordingsDir: recordingsDir,
		Config:        cfg,
	}, nil
}

// LoadRecording reads recordings/<name>.json from a fixture. The name must be a
// slug, and the file is opened through os.Root, so neither a crafted name
// (e.g. from --recording) nor a symlink can read outside recordings/.
func LoadRecording(fixtureRoot, name string) (Recording, error) {
	if !slugPattern.MatchString(name) {
		return Recording{}, fmt.Errorf("recording name %q must match %s", name, slugPattern.String())
	}
	recordings, err := os.OpenRoot(filepath.Join(fixtureRoot, "recordings"))
	if err != nil {
		return Recording{}, err
	}
	defer recordings.Close()
	file, err := recordings.Open(name + ".json")
	if err != nil {
		return Recording{}, err
	}
	defer file.Close()

	var recording Recording
	dec := json.NewDecoder(file)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&recording); err != nil {
		return Recording{}, err
	}
	if err := dec.Decode(&struct{}{}); err == nil {
		return Recording{}, errors.New("unexpected trailing JSON content")
	} else if !errors.Is(err, io.EOF) {
		return Recording{}, err
	}
	return recording, nil
}

func applyDefaults(cfg *ScoringConfig) {
	if cfg.Execution.PermissionMode == "" {
		cfg.Execution.PermissionMode = "default"
	}
	if cfg.Execution.ApprovalStrategy == "" {
		cfg.Execution.ApprovalStrategy = ApprovalStrategyAllow
	}
	if cfg.Golden.Compare == "" {
		cfg.Golden.Compare = CompareExact
	}
	if strings.TrimSpace(cfg.Model.Recorded.Recording) == "" {
		cfg.Model.Recorded.Recording = "default"
	}
}

func resolveFixturePath(root, rel string, wantDir bool) (string, error) {
	full := filepath.Join(root, rel)
	info, err := os.Lstat(full)
	if err != nil {
		return "", err
	}
	if wantDir && !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", rel)
	}
	if !wantDir && info.IsDir() {
		return "", fmt.Errorf("%s is a directory", rel)
	}

	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		resolvedRoot = root
	}
	resolvedPath, err := filepath.EvalSymlinks(full)
	if err != nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", err
		}
		resolvedPath = full
	}
	if !pathWithin(resolvedRoot, resolvedPath) {
		return "", fmt.Errorf("%s resolves outside fixture root", rel)
	}
	return full, nil
}

func pathWithin(root, target string) bool {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	if root == target {
		return true
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	if rel == "." || rel == ".." {
		return rel == "."
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
