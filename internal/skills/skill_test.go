package skills

import (
	"strings"
	"testing"
)

func TestSourceString(t *testing.T) {
	t.Parallel()
	if SourceBundled.String() != "bundled" || SourceUser.String() != "user" || SourceProject.String() != "project" || SourceMCP.String() != "mcp" {
		t.Fatal("unexpected source labels")
	}
}

func TestParseFrontmatter(t *testing.T) {
	t.Parallel()
	raw := `---
name: sample
description: desc
---
body text`
	sf, body, err := parseFrontmatter(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if sf.Name != "sample" || sf.Description != "desc" {
		t.Fatal("bad frontmatter parse")
	}
	if !strings.Contains(body, "body text") {
		t.Fatal("missing body")
	}
}

func TestParseFrontmatterErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "missing opening delimiter",
			raw:  "name: sample\ndescription: desc\n---\nbody",
			want: "missing opening frontmatter delimiter",
		},
		{
			name: "missing name",
			raw:  "---\ndescription: desc\n---\nbody",
			want: "name is required",
		},
		{
			name: "missing description",
			raw:  "---\nname: sample\n---\nbody",
			want: "description is required",
		},
		{
			name: "invalid yaml",
			raw:  "---\nname: [\ndescription: desc\n---\nbody",
			want: "invalid yaml frontmatter",
		},
		{
			name: "missing closing delimiter",
			raw:  "---\nname: sample\ndescription: desc\nbody",
			want: "no closing frontmatter delimiter found",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := parseFrontmatter(strings.NewReader(tc.raw))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestSkillFileLocationHelpers(t *testing.T) {
	t.Parallel()
	if !(SkillFile{Path: "/tmp/a.md"}).IsFilesystem() {
		t.Fatal("expected filesystem skill")
	}
	if (SkillFile{Path: ""}).IsFilesystem() {
		t.Fatal("expected non-filesystem skill")
	}
	if !(SkillFile{EmbedPath: "assets/skills/a.md"}).IsEmbedded() {
		t.Fatal("expected embedded skill")
	}
	if (SkillFile{EmbedPath: ""}).IsEmbedded() {
		t.Fatal("expected non-embedded skill")
	}
}
