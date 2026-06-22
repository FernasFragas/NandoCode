package skills

import (
	"embed"
	"io/fs"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed assets/skills/*.md
var BundledFS embed.FS

func loadBundledSkills(fsys fs.FS) ([]SkillFile, error) {
	var out []SkillFile
	err := fs.WalkDir(fsys, "assets/skills", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			slog.Warn("skills: failed while walking bundled skills", "path", path, "error", err)
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}
		b, readErr := fs.ReadFile(fsys, path)
		if readErr != nil {
			slog.Warn("skills: failed to read bundled skill", "path", path, "error", readErr)
			return nil
		}
		sf, body, parseErr := parseFrontmatter(strings.NewReader(string(b)))
		if parseErr != nil {
			slog.Warn("skills: failed to parse bundled skill frontmatter", "path", path, "error", parseErr)
			return nil
		}
		sf.Source = SourceBundled
		sf.EmbedPath = filepath.ToSlash(path)
		sf.Body = body
		out = append(out, sf)
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, err
}
