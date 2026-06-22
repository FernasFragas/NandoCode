package mentions

import "strings"

type IntentKind string

const (
	IntentUnknown                     IntentKind = "unknown"
	IntentDirectoryListing            IntentKind = "directory_listing"
	IntentDirectoryListingWithContent IntentKind = "directory_listing_with_content"
	IntentFileStatus                  IntentKind = "file_status"
	IntentReview                      IntentKind = "review"
	IntentAnalysis                    IntentKind = "analysis"
)

type AttachmentPolicy string

const (
	AttachDefault         AttachmentPolicy = "default"
	AttachListingTreeOnly AttachmentPolicy = "listing_tree_only"
	AttachContent         AttachmentPolicy = "content"
)

type IntentReport struct {
	Kind             IntentKind
	AttachmentPolicy AttachmentPolicy
	HasMention       bool
	ExplicitMode     MentionMode
	Reasons          []string
}

func (r IntentReport) ListingLike() bool {
	return r.Kind == IntentDirectoryListing || r.Kind == IntentDirectoryListingWithContent
}

func ClassifyPromptIntent(input string, parsed []parsedMention, resolved []resolvedMention) IntentReport {
	report := IntentReport{
		Kind:             IntentUnknown,
		AttachmentPolicy: AttachDefault,
		ExplicitMode:     MentionModeAuto,
	}
	if len(parsed) == 0 {
		return report
	}

	anyDir := false
	anyMention := len(parsed) > 0
	hasExplicitContent := false
	hasExplicitAll := false
	for i := range parsed {
		if parsed[i].Mode == MentionModeContent {
			hasExplicitContent = true
			report.ExplicitMode = MentionModeContent
		}
		if parsed[i].Mode == MentionModeAll {
			hasExplicitAll = true
			report.ExplicitMode = MentionModeAll
		}
		if parsed[i].Mode == MentionModeTree {
			report.ExplicitMode = MentionModeTree
		}
	}
	for i := range resolved {
		if resolved[i].IsDir {
			anyDir = true
			break
		}
	}

	report.HasMention = anyMention
	words := intentWords(input)
	if len(words) == 0 {
		return report
	}
	if anyMention && looksLikeStatusRequest(words) && !looksLikeContinuationRequest(words) {
		report.Kind = IntentFileStatus
		report.AttachmentPolicy = AttachContent
		report.Reasons = append(report.Reasons, "status-or-implementation-request-with-explicit-mentions")
		return report
	}
	if !anyDir {
		return report
	}

	if hasAnyWord(words, map[string]struct{}{
		"review": {}, "summarize": {}, "summary": {}, "analyze": {}, "analysis": {}, "audit": {}, "compare": {},
	}) {
		if hasAnyWord(words, map[string]struct{}{"review": {}, "summarize": {}, "summary": {}}) {
			report.Kind = IntentReview
		} else {
			report.Kind = IntentAnalysis
		}
		report.AttachmentPolicy = AttachContent
		report.Reasons = append(report.Reasons, "analysis-or-review-verbs")
		return report
	}
	if hasPhrase(words, []string{"inspect", "contents"}) || hasPhrase(words, []string{"show", "contents"}) || hasAnyWord(words, map[string]struct{}{"open": {}, "read": {}, "explain": {}}) {
		report.Kind = IntentReview
		report.AttachmentPolicy = AttachContent
		report.Reasons = append(report.Reasons, "content-inspection-verbs")
		return report
	}

	listingVerb := map[string]struct{}{
		"list": {}, "name": {}, "show": {}, "enumerate": {}, "print": {}, "display": {},
	}
	listingNoun := map[string]struct{}{
		"file": {}, "files": {}, "folder": {}, "folders": {}, "directory": {}, "directories": {}, "tree": {}, "project": {},
	}
	filler := map[string]struct{}{
		"all": {}, "the": {}, "every": {}, "each": {}, "a": {}, "an": {}, "of": {}, "in": {},
	}

	listingWording := false
	for i, w := range words {
		if _, ok := listingVerb[w]; !ok {
			continue
		}
		for j := i + 1; j < len(words) && j <= i+6; j++ {
			if _, ok := filler[words[j]]; ok {
				continue
			}
			if _, ok := listingNoun[words[j]]; ok {
				listingWording = true
				break
			}
			if _, ok := listingVerb[words[j]]; ok {
				break
			}
		}
		if listingWording {
			break
		}
	}
	if !listingWording && hasPhrase(words, []string{"what", "files", "are", "in"}) {
		listingWording = true
	}

	if !listingWording {
		return report
	}

	if hasExplicitContent || hasExplicitAll {
		report.Kind = IntentDirectoryListingWithContent
		report.AttachmentPolicy = AttachContent
		report.Reasons = append(report.Reasons, "explicit-content-or-all-mode")
		return report
	}

	report.Kind = IntentDirectoryListing
	report.AttachmentPolicy = AttachListingTreeOnly
	report.Reasons = append(report.Reasons, "listing-words-and-directory-mentions")
	return report
}

func intentPromptIntent(intent IntentKind) string {
	return strings.TrimSpace(string(intent))
}

func looksLikeContinuationRequest(words []string) bool {
	if hasAnyWord(words, map[string]struct{}{
		"continue": {}, "resume": {},
	}) {
		return true
	}
	if hasPhrase(words, []string{"based", "on", "previous"}) {
		return true
	}
	if hasPhrase(words, []string{"continue", "from", "previous"}) {
		return true
	}
	return false
}

func looksLikeStatusRequest(words []string) bool {
	if hasAnyWord(words, map[string]struct{}{"status": {}}) {
		return true
	}
	if hasPhrase(words, []string{"current", "status"}) || hasPhrase(words, []string{"current", "state"}) {
		return true
	}
	if hasPhrase(words, []string{"what", "is", "implemented"}) ||
		hasPhrase(words, []string{"what", "was", "implemented"}) ||
		hasPhrase(words, []string{"review", "what", "was", "implemented"}) {
		return true
	}
	if hasAnyWord(words, map[string]struct{}{"report": {}, "review": {}}) &&
		hasAnyWord(words, map[string]struct{}{"implemented": {}, "implementation": {}, "status": {}, "state": {}}) {
		return true
	}
	return false
}
