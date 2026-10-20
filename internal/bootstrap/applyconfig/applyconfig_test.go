package applyconfig

import (
	"reflect"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/bootstrap"
	"github.com/FernasFragas/Nandocode/internal/config"
	"github.com/FernasFragas/Nandocode/internal/permissions"
)

// landsIn maps each config.Config field to the Initial field(s) it must reach.
var landsIn = map[string][]string{
	"DefaultModel":                       {"DefaultModel"},
	"OllamaBaseURL":                      {"OllamaBaseURL", "LLMBaseURL"},
	"OllamaCloudEnabled":                 {"OllamaCloudEnabled"},
	"ChatKeepAlive":                      {"KeepAlive"},
	"LLMStreamIdleTimeout":               {"LLMStreamIdleTimeout"},
	"CloudLLMStreamIdleTimeout":          {"CloudLLMStreamIdleTimeout"},
	"PermissionMode":                     {"PermissionMode"},
	"MaxTurns":                           {"MaxTurns"},
	"MaxConcurrentTools":                 {"MaxConcurrentTools"},
	"BashTimeout":                        {"BashTimeout"},
	"MaxReadChars":                       {"MaxReadChars"},
	"MaxResultChars":                     {"MaxResultChars"},
	"MaxDirFiles":                        {"MaxDirFiles"},
	"MaxPromptFiles":                     {"MaxPromptFiles"},
	"MaxDirBytes":                        {"MaxDirBytes"},
	"MaxPromptBytes":                     {"MaxPromptBytes"},
	"MaxDirDepth":                        {"MaxDirDepth"},
	"MentionDirectorySource":             {"MentionDirectorySource"},
	"MentionIncludeGitignoredOnExplicit": {"MentionIncludeGitignoredOnExplicit"},
	"PromptDumpMode":                     {"PromptDumpMode"},
	"PromptDumpKeep":                     {"PromptDumpKeep"},
	"PromptPreviewChars":                 {"PromptPreviewChars"},
	"LogLevel":                           {"LogLevel"},
	"LogFormat":                          {"LogFormat"},
	"SlowStageNoticeThreshold":           {"SlowStageNoticeThreshold"},
	"ContextMode":                        {"ContextMode"},
	"MemoryRecallMode":                   {"MemoryRecallMode"},
}

// exempt fields are read straight from config.Config and never enter Initial.
var exempt = map[string]bool{
	"MemoryEnabled":    true,
	"SkillsDir":        true,
	"ProjectSkillsDir": true,
	"SemanticIndex":    true,
}

func TestEveryConfigFieldIsMappedOrExempt(t *testing.T) {
	ct := reflect.TypeOf(config.Config{})
	for i := 0; i < ct.NumField(); i++ {
		name := ct.Field(i).Name
		_, mapped := landsIn[name]
		if !mapped && !exempt[name] {
			t.Errorf("config.Config.%s is neither mapped by ApplyConfig nor exempt", name)
		}
		if mapped && exempt[name] {
			t.Errorf("config.Config.%s is both mapped and exempt", name)
		}
	}
}

// TestApplyConfigEveryFieldLands gives each config field its own distinct
// sentinel, so a wrong-but-consistent mapping (MaxDirFiles into MaxPromptFiles)
// fails.
func TestApplyConfigEveryFieldLands(t *testing.T) {
	var cfg config.Config
	cv := reflect.ValueOf(&cfg).Elem()
	n := int64(1000)
	for name := range landsIn {
		f := cv.FieldByName(name)
		n++
		switch {
		case f.Type() == reflect.TypeOf(permissions.Mode("")):
			f.SetString(string(permissions.ModeBypass))
		case f.Type() == reflect.TypeOf(time.Duration(0)):
			f.SetInt(n * int64(time.Second))
		case f.Kind() == reflect.String:
			f.SetString("sentinel-" + name)
		case f.Kind() == reflect.Bool:
			f.SetBool(true)
		case f.Kind() == reflect.Int, f.Kind() == reflect.Int64:
			f.SetInt(n)
		default:
			t.Fatalf("unsupported kind for %s", name)
		}
	}

	initial := bootstrap.DefaultInitial(t.TempDir())
	// Start every bool opposite to the config value so defaults cannot hide a gap.
	initial.OllamaCloudEnabled = false
	initial.MentionIncludeGitignoredOnExplicit = false
	ApplyConfig(&initial, cfg, "file")

	iv := reflect.ValueOf(initial)
	for cfgField, initFields := range landsIn {
		for _, f := range initFields {
			got := iv.FieldByName(f)
			if !got.IsValid() {
				t.Fatalf("Initial has no field %s", f)
			}
			want := cv.FieldByName(cfgField)
			if !reflect.DeepEqual(got.Interface(), want.Interface()) {
				t.Errorf("config.%s -> Initial.%s = %v, want %v", cfgField, f, got.Interface(), want.Interface())
			}
		}
	}
	if initial.SlowStageNoticeThresholdSource != "file" {
		t.Errorf("slow stage source = %q, want file", initial.SlowStageNoticeThresholdSource)
	}
}
