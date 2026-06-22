package picker

import "testing"

func TestDetectFileTrigger(t *testing.T) {
	t.Parallel()
	ctx := Detect("read @internal/tui/app.go now", len("read @internal/t"))
	if !ctx.Active || ctx.Kind != TriggerFile {
		t.Fatalf("ctx=%#v", ctx)
	}
	if ctx.Query != "internal/tui/app.go" {
		t.Fatalf("query=%q", ctx.Query)
	}
}

func TestDetectCommandOnlyAtLineStart(t *testing.T) {
	t.Parallel()
	ctx := Detect("please /help", len("please /he"))
	if ctx.Active {
		t.Fatalf("unexpected active ctx=%#v", ctx)
	}

	active := Detect("   /help me", len("   /he"))
	if !active.Active || active.Kind != TriggerCommand {
		t.Fatalf("ctx=%#v", active)
	}
}

func TestDetectBacktickSuppression(t *testing.T) {
	t.Parallel()
	ctx := Detect("`@main.go` and text", len("`@mai"))
	if ctx.Active {
		t.Fatalf("expected inactive in backtick span: %#v", ctx)
	}
}

func TestDetectAtStartMidAndEndOfLine(t *testing.T) {
	t.Parallel()
	start := Detect("@main.go", len("@ma"))
	if !start.Active || start.Kind != TriggerFile {
		t.Fatalf("start=%#v", start)
	}

	mid := Detect("look @internal/tui/app.go now", len("look @internal/tui/app"))
	if !mid.Active || mid.Kind != TriggerFile {
		t.Fatalf("mid=%#v", mid)
	}

	end := Detect("read @note.txt", len("read @note.txt"))
	if !end.Active || end.Kind != TriggerFile {
		t.Fatalf("end=%#v", end)
	}
}

func TestDetectInactiveOutsideToken(t *testing.T) {
	t.Parallel()
	ctx := Detect("read @note.txt now", len("read")-1)
	if ctx.Active {
		t.Fatalf("expected inactive, got %#v", ctx)
	}
}
