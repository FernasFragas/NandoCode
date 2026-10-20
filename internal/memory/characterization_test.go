package memory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/tools"
)

// scriptedClient answers every Chat call with a fixed body.
type scriptedClient struct {
	body string
	reqs []*llm.ChatRequest
}

func (c *scriptedClient) Chat(_ context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	c.reqs = append(c.reqs, req)
	ch := make(chan llm.StreamEvent, 2)
	ch <- llm.StreamEvent{Message: llm.Message{Content: c.body}}
	ch <- llm.StreamEvent{Done: true, DoneReason: "stop"}
	close(ch)
	return ch, nil
}

func (c *scriptedClient) Embed(context.Context, string, []string) ([][]float32, error) {
	return nil, nil
}
func (c *scriptedClient) ListModels(context.Context) ([]llm.ModelInfo, error) { return nil, nil }
func (c *scriptedClient) ShowModel(context.Context, string) (llm.ModelDetails, error) {
	return llm.ModelDetails{}, nil
}

func (c *scriptedClient) PullModel(context.Context, string, chan<- llm.PullProgress) error {
	return nil
}

func memFile(name, desc, typ string) string {
	return "---\nname: " + name + "\ndescription: " + desc + "\ntype: " + typ + "\n---\nbody of " + name + "\n"
}

func writeMem(t *testing.T, dir, file, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestScanCharacterization(t *testing.T) {
	dir := t.TempDir()
	writeMem(t, dir, "b_second.md", memFile("B", "second", "project"))
	writeMem(t, dir, "a_first.md", memFile("A", "first", "user"))
	writeMem(t, dir, "MEMORY.md", "- index\n")
	writeMem(t, dir, ".hidden.md", memFile("H", "hidden", "user"))
	writeMem(t, dir, "notes.txt", "not markdown")
	writeMem(t, dir, "bad.md", "no frontmatter here\n")
	if err := os.MkdirAll(filepath.Join(dir, "pending"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeMem(t, filepath.Join(dir, "pending"), "p.md", memFile("P", "pending", "user"))

	res, err := Scan(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entries) != 2 || res.Entries[0].Filename != "a_first.md" || res.Entries[1].Filename != "b_second.md" {
		t.Fatalf("entries not sorted/filtered as expected: %+v", res.Entries)
	}
	if res.FileCount != 2 {
		t.Fatalf("FileCount = %d", res.FileCount)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "bad.md") {
		t.Fatalf("warnings = %v", res.Warnings)
	}
}

func TestScanMissingDirAndCancelled(t *testing.T) {
	res, err := Scan(context.Background(), filepath.Join(t.TempDir(), "nope"))
	if err != nil || len(res.Entries) != 0 {
		t.Fatalf("missing dir: %v %+v", err, res)
	}
	dir := t.TempDir()
	writeMem(t, dir, "a.md", memFile("A", "d", "user"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Scan(ctx, dir); err == nil {
		t.Fatal("expected context error")
	}
}

func TestParseFrontmatterEdgeCases(t *testing.T) {
	parse := func(src string) error {
		_, err := ParseFrontmatter("x.md", strings.NewReader(src), time.Now(), int64(len(src)))
		return err
	}
	if err := parse("name: x\n"); err == nil || !strings.Contains(err.Error(), "missing YAML frontmatter") {
		t.Fatalf("no fence: %v", err)
	}
	if err := parse("---\nname: x\ndescription: y\ntype: user\n"); err == nil || !strings.Contains(err.Error(), "unterminated") {
		t.Fatalf("unterminated: %v", err)
	}
	if err := parse("---\nname: x\ndescription: y\ntype: bogus\n---\n"); err == nil || !strings.Contains(err.Error(), "invalid memory type") {
		t.Fatalf("bad type: %v", err)
	}
	if err := parse("---\nname: [unclosed\ndescription: y\ntype: user\n---\n"); err == nil || !strings.Contains(err.Error(), "invalid YAML") {
		t.Fatalf("bad yaml: %v", err)
	}
	// Only the first 30 lines are read, so a late closing fence is unterminated.
	long := "---\nname: x\ndescription: y\ntype: user\n" + strings.Repeat("# c\n", 40) + "---\n"
	if err := parse(long); err == nil || !strings.Contains(err.Error(), "unterminated") {
		t.Fatalf("30-line cap: %v", err)
	}
	for _, typ := range []string{"user", "feedback", "project", "reference"} {
		if err := parse(memFile("n", "d", typ)); err != nil {
			t.Fatalf("type %s: %v", typ, err)
		}
	}
}

func TestLoadIndexCharacterization(t *testing.T) {
	dir := t.TempDir()
	missing, err := LoadIndex(filepath.Join(dir, "MEMORY.md"), 10, 100)
	if err != nil || missing.Content != "" || missing.Capped {
		t.Fatalf("missing index: %v %+v", err, missing)
	}
	p := filepath.Join(dir, "MEMORY.md")
	if err := os.WriteFile(p, []byte("0123456789\nabc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	byBytes, _ := LoadIndex(p, 0, 5)
	if !byBytes.Capped || byBytes.Content != "01234" || byBytes.Warning == "" {
		t.Fatalf("byte cap: %+v", byBytes)
	}
	exact, _ := LoadIndex(p, 2, 100)
	if exact.Capped {
		t.Fatalf("exactly maxLines must not be capped: %+v", exact)
	}
	uncapped, _ := LoadIndex(p, 0, 0)
	if uncapped.Content != "0123456789\nabc" || uncapped.Capped {
		t.Fatalf("no limits: %+v", uncapped)
	}
}

func TestStalenessCharacterization(t *testing.T) {
	now := time.Date(2026, 3, 10, 15, 0, 0, 0, time.UTC)
	cases := []struct {
		age  time.Duration
		want string
		warn bool
	}{
		{0, "today", false},
		{-time.Hour, "today", false},
		{24 * time.Hour, "yesterday", false},
		{48 * time.Hour, "2 days ago", true},
		{10 * 24 * time.Hour, "10 days ago", true},
	}
	for _, c := range cases {
		if got := AgeDescription(now, now.Add(-c.age)); got != c.want {
			t.Errorf("AgeDescription(%v) = %q, want %q", c.age, got, c.want)
		}
		w := StalenessWarning(now, now.Add(-c.age))
		if (w != "") != c.warn {
			t.Errorf("StalenessWarning(%v) = %q", c.age, w)
		}
		if c.warn && !strings.Contains(w, c.want) {
			t.Errorf("warning %q lacks %q", w, c.want)
		}
	}
}

func TestScopeRootCharacterization(t *testing.T) {
	if got, _ := ScopeRoot("/ignored", "/repo/../repo"); got != filepath.FromSlash("/repo") {
		t.Fatalf("gitRoot not cleaned: %q", got)
	}
	if _, err := ScopeRoot("  ", ""); err == nil {
		t.Fatal("expected error for empty working dir")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	if got, _ := ScopeRoot(sub, ""); got != root {
		t.Fatalf("walk-up: got %q want %q", got, root)
	}
}

func TestStoreFilenameGuards(t *testing.T) {
	s := newStore(t.TempDir())
	for _, bad := range []string{"", "  ", "../x.md", "a/b.md", `a\b.md`, "notmd.txt"} {
		if _, err := s.activePath(bad); err == nil {
			t.Errorf("activePath(%q) should fail", bad)
		}
	}
	if p, err := s.activePath("OK.MD"); err != nil || filepath.Base(p) != "OK.MD" {
		t.Fatalf("activePath uppercase ext: %q %v", p, err)
	}
	ctx := context.Background()
	if _, err := s.writePending(ctx, Draft{Filename: "../evil.md", Content: "x"}); err == nil {
		t.Fatal("writePending must reject traversal")
	}
	if err := s.ensure(ctx); err != nil {
		t.Fatal(err)
	}
	target, err := s.writePending(ctx, Draft{Filename: "note", Content: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(target) != "note.md" || filepath.Base(filepath.Dir(target)) != "pending" {
		t.Fatalf("target = %s", target)
	}
	if b, _ := os.ReadFile(target); string(b) != "hi" {
		t.Fatalf("content = %q", b)
	}
	if _, err := os.Stat(target + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("tmp file left behind")
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.writePending(cctx, Draft{Filename: "c.md"}); err == nil {
		t.Fatal("cancelled ctx must fail")
	}
}

func TestRecallCharacterization(t *testing.T) {
	entries := []Entry{
		{Filename: "a.md", Name: "A", Type: TypeUser, UpdatedAt: time.Unix(0, 0)},
		{Filename: "b.md", Name: "B", Type: TypeUser, UpdatedAt: time.Unix(0, 0)},
		{Filename: "c.md", Name: "C", Type: TypeUser, UpdatedAt: time.Unix(0, 0)},
	}
	cfg := DefaultConfig("m")
	cfg.MaxSelected = 2
	client := &scriptedClient{body: `{"selected":["b.md","b.md","ghost.md","c.md","a.md"]}`}
	res, err := Recall(context.Background(), client, cfg, Query{LatestUser: "q"}, entries, map[string]bool{"a.md": true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Selected) != 2 || res.Selected[0].Filename != "b.md" || res.Selected[1].Filename != "c.md" {
		t.Fatalf("selected = %+v", res.Selected)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "ghost.md") {
		t.Fatalf("warnings = %v", res.Warnings)
	}
	if strings.Contains(client.reqs[0].Messages[1].Content, "a.md |") {
		t.Fatal("already-loaded entry leaked into manifest")
	}

	bad := &scriptedClient{body: "not json"}
	res, err = Recall(context.Background(), bad, cfg, Query{}, entries, nil)
	if err != nil || len(res.Selected) != 0 || len(res.Warnings) != 1 {
		t.Fatalf("parse failure: %v %+v", err, res)
	}
	if res, _ := Recall(context.Background(), nil, cfg, Query{}, entries, nil); len(res.Selected) != 0 {
		t.Fatal("nil client must yield empty result")
	}
	allLoaded := map[string]bool{"a.md": true, "b.md": true, "c.md": true}
	if res, _ := Recall(context.Background(), client, cfg, Query{}, entries, allLoaded); len(res.Selected) != 0 {
		t.Fatal("all loaded must yield empty result")
	}
}

func TestExtractDraftsCharacterization(t *testing.T) {
	client := &scriptedClient{body: `{"drafts":[
		{"filename":"good","content":"  body  "},
		{"filename":"../escape.md","content":"x"},
		{"filename":"empty.md","content":"   "}]}`}
	convo := []llm.Message{{Role: llm.RoleUser, Content: "remember"}}
	drafts, err := ExtractDrafts(context.Background(), client, DefaultConfig("m"), convo, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(drafts) != 2 {
		t.Fatalf("drafts = %+v", drafts)
	}
	if drafts[0].Filename != "good.md" || drafts[0].Content != "body\n" {
		t.Fatalf("first = %+v", drafts[0])
	}
	if strings.Contains(drafts[1].Filename, "..") || !strings.HasSuffix(drafts[1].Filename, "-memory.md") {
		t.Fatalf("traversal name not replaced: %q", drafts[1].Filename)
	}
	if d, err := ExtractDrafts(context.Background(), nil, DefaultConfig("m"), convo, nil); d != nil || err != nil {
		t.Fatal("nil client")
	}
	garbage := &scriptedClient{body: "nope"}
	if d, err := ExtractDrafts(context.Background(), garbage, DefaultConfig("m"), convo, nil); d != nil || err != nil {
		t.Fatalf("unparseable reply: %v %v", d, err)
	}
	blank := []llm.Message{{Role: llm.RoleUser, Content: "  "}}
	if d, _ := ExtractDrafts(context.Background(), client, DefaultConfig("m"), blank, nil); d != nil {
		t.Fatal("blank conversation")
	}
}

func TestBuildSectionCharacterization(t *testing.T) {
	out := BuildSection(SectionInput{
		MemoryDir: "/m",
		Index:     Index{Content: "- idx", Warning: "too big"},
		Recalled: []LoadedEntry{
			{Entry: Entry{Filename: "a.md"}, Content: "no newline", StalenessWarning: "stale!"},
			{Entry: Entry{Filename: "b.md"}, Content: "with newline\n"},
		},
	})
	for _, want := range []string{
		sectionHeader, sectionFooter, "Memory directory: /m", "Index warning: too big",
		"MEMORY.md index:\n- idx", "[a.md]\nstale!\nno newline\n", "[b.md]\nwith newline\n" + sectionFooter,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("section missing %q in:\n%s", want, out)
		}
	}
	plain := BuildSection(SectionInput{MemoryDir: "/m"})
	if strings.Contains(plain, "MEMORY.md index:") || strings.Contains(plain, "Recalled memory files") {
		t.Fatal("empty sections should be omitted")
	}
}

func TestScoreMemoryEntryAndListingQueries(t *testing.T) {
	e := Entry{Filename: "feedback_testing.md", Name: "Testing Policy", Description: "use real database", Type: TypeFeedback}
	if got := scoreMemoryEntry("what is the testing database policy", e); got != 6 {
		t.Fatalf("score = %d, want 6", got)
	}
	if got := scoreMemoryEntry("list project files", e); got != 0 {
		t.Fatalf("stop words must not score: %d", got)
	}
	if got := scoreMemoryEntry("", e); got != 0 {
		t.Fatalf("empty query: %d", got)
	}
	if !looksLikeListingQuery("please list the files") || looksLikeListingQuery("list") || looksLikeListingQuery("") {
		t.Fatal("looksLikeListingQuery mismatch")
	}
	if !looksLikeListingQuery("Directory tree data: x") {
		t.Fatal("tree marker")
	}
}

func runnerFor(t *testing.T, mode string) (*Runner, string) {
	t.Helper()
	t.Setenv("NANDOCODEGO_DATA_HOME", t.TempDir())
	cfg := DefaultConfig("m")
	cfg.RecallMode = mode
	r := NewRunner(fakeMemoryRunner{}, &scriptedClient{body: `{"selected":[]}`}, cfg)
	r.nowFn = func() time.Time { return time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC) }
	work := t.TempDir()
	return r, work
}

func TestBuildAugmentedPromptFastMode(t *testing.T) {
	r, work := runnerFor(t, "fast")
	memDir := DirForScope(work)
	writeMem(t, memDir, "feedback_testing.md", memFile("Testing Policy", "use real database", "feedback"))
	writeMem(t, memDir, "user_role.md", memFile("Role", "senior engineer", "user"))
	writeMem(t, memDir, "MEMORY.md", "- [testing](feedback_testing.md)\n")

	in := agent.Input{
		SystemPrompt: "BASE",
		Messages:     []llm.Message{{Role: llm.RoleUser, Content: "what is the testing policy for the database"}},
		ToolContext:  tools.Context{WorkingDir: work},
	}
	got := r.buildAugmentedPrompt(context.Background(), in, memDir)
	if !strings.HasPrefix(got, "BASE\n\n"+sectionHeader) {
		t.Fatalf("prompt = %q", got)
	}
	if !strings.Contains(got, "body of Testing Policy") || strings.Contains(got, "body of Role") {
		t.Fatalf("recall selection wrong:\n%s", got)
	}
	if !strings.Contains(got, "- [testing](feedback_testing.md)") {
		t.Fatal("index missing")
	}

	in.Messages = []llm.Message{{Role: llm.RoleUser, Content: "zzz unrelated"}}
	if got := r.buildAugmentedPrompt(context.Background(), in, memDir); got != "BASE" {
		t.Fatalf("no match must leave prompt untouched: %q", got)
	}
	in.Messages = []llm.Message{{Role: llm.RoleUser, Content: "list the testing files"}}
	if got := r.buildAugmentedPrompt(context.Background(), in, memDir); got != "BASE" {
		t.Fatalf("listing query must skip memory: %q", got)
	}
	in.PromptIntent = "directory_listing"
	in.Messages = []llm.Message{{Role: llm.RoleUser, Content: "testing database policy"}}
	if got := r.buildAugmentedPrompt(context.Background(), in, memDir); got != "BASE" {
		t.Fatalf("directory_listing intent must skip memory: %q", got)
	}
}

func TestBuildAugmentedPromptOffMode(t *testing.T) {
	r, work := runnerFor(t, "off")
	memDir := DirForScope(work)
	writeMem(t, memDir, "feedback_testing.md", memFile("Testing Policy", "use real database", "feedback"))
	in := agent.Input{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "testing database policy"}},
	}
	if got := r.buildAugmentedPrompt(context.Background(), in, memDir); got != "" {
		t.Fatalf("off mode: %q", got)
	}
}

func TestExtractPendingWritesTimestampedDrafts(t *testing.T) {
	r, work := runnerFor(t, "fast")
	r.client = &scriptedClient{body: `{"drafts":[{"filename":"lesson.md","content":"c"}]}`}
	memDir := DirForScope(work)
	n := r.extractPending(context.Background(), memDir, "m", []llm.Message{{Role: llm.RoleUser, Content: "x"}})
	if n != 1 {
		t.Fatalf("written = %d", n)
	}
	ents, _ := os.ReadDir(filepath.Join(memDir, "pending"))
	if len(ents) != 1 || !strings.HasSuffix(ents[0].Name(), "-lesson.md") {
		t.Fatalf("pending = %v", ents)
	}
	// Pending drafts must not be scanned as active memories.
	res, _ := Scan(context.Background(), memDir)
	if len(res.Entries) != 0 {
		t.Fatalf("pending leaked into active scan: %+v", res.Entries)
	}
}

func TestRunnerDisabledAndNoExtractPassThrough(t *testing.T) {
	t.Setenv("NANDOCODEGO_DATA_HOME", t.TempDir())
	term := agent.Terminal{Reason: agent.TerminalCompleted, Conversation: []llm.Message{{Role: llm.RoleUser, Content: "x"}}}
	for _, cfg := range []Config{
		{Enabled: false},
		func() Config { c := DefaultConfig("m"); c.NoExtract = true; return c }(),
	} {
		client := &scriptedClient{body: `{"drafts":[]}`}
		r := NewRunner(fakeMemoryRunner{events: []agent.Event{term}}, client, cfg)
		var got []agent.Event
		for evt := range r.Run(context.Background(), agent.Input{ToolContext: tools.Context{WorkingDir: t.TempDir()}}) {
			got = append(got, evt)
		}
		sawTerm := false
		for _, e := range got {
			if _, ok := e.(agent.Terminal); ok {
				sawTerm = true
			}
		}
		if !sawTerm {
			t.Fatalf("terminal not forwarded for cfg %+v", cfg)
		}
		if len(client.reqs) != 0 {
			t.Fatalf("no LLM call expected (extraction off), got %d", len(client.reqs))
		}
	}
}
