package mentions

import "testing"

func TestClassifyPromptIntentFileStatusWithExplicitFileMention(t *testing.T) {
	parsed := []parsedMention{{Path: "docs/plan.md", Mode: MentionModeAuto}}
	resolved := []resolvedMention{{Path: "docs/plan.md", IsDir: false, Mode: MentionModeAuto}}

	report := ClassifyPromptIntent("what is the current status of @docs/plan.md", parsed, resolved)
	if report.Kind != IntentFileStatus {
		t.Fatalf("intent=%q want %q", report.Kind, IntentFileStatus)
	}
	if report.AttachmentPolicy != AttachContent {
		t.Fatalf("policy=%q want %q", report.AttachmentPolicy, AttachContent)
	}
	if !report.HasMention {
		t.Fatal("expected HasMention=true")
	}
}

func TestClassifyPromptIntentGenericStatusWithoutMentionIsNotFileStatus(t *testing.T) {
	report := ClassifyPromptIntent("what is the current status?", nil, nil)
	if report.Kind == IntentFileStatus {
		t.Fatalf("did not expect %q intent", IntentFileStatus)
	}
	if report.AttachmentPolicy != AttachDefault {
		t.Fatalf("policy=%q want %q", report.AttachmentPolicy, AttachDefault)
	}
}

func TestClassifyPromptIntentStatusWithContinuationDoesNotForceFileStatus(t *testing.T) {
	parsed := []parsedMention{{Path: "docs/plan.md", Mode: MentionModeAuto}}
	resolved := []resolvedMention{{Path: "docs/plan.md", IsDir: false, Mode: MentionModeAuto}}

	report := ClassifyPromptIntent("continue based on previous status of @docs/plan.md", parsed, resolved)
	if report.Kind == IntentFileStatus {
		t.Fatalf("did not expect %q intent for continuation prompt", IntentFileStatus)
	}
}
