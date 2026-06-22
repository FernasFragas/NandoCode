package bootstrap

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/FernasFragas/nandocodego/internal/permissions"
)

func TestDefaultInitial(t *testing.T) {
	initial := DefaultInitial("")

	if initial.WorkingDir == "" {
		t.Error("expected non-empty working directory")
	}
	if initial.SessionID == "" {
		t.Error("expected non-empty session ID")
	}
	if initial.ConfigDir == "" {
		t.Error("expected non-empty config dir")
	}
	if initial.DataDir == "" {
		t.Error("expected non-empty data dir")
	}
	if initial.PermissionMode != permissions.ModeDefault {
		t.Errorf("expected default permission mode, got %s", initial.PermissionMode)
	}
	if initial.MaxOutputTokens != 8192 {
		t.Errorf("expected max output tokens 8192, got %d", initial.MaxOutputTokens)
	}
	if initial.LengthRetryTokens != 65536 {
		t.Errorf("expected length retry tokens 65536, got %d", initial.LengthRetryTokens)
	}
}

func TestDefaultInitialWithWorkingDir(t *testing.T) {
	wd := "/some/working/dir"
	initial := DefaultInitial(wd)

	if initial.WorkingDir != wd {
		t.Errorf("expected working dir %s, got %s", wd, initial.WorkingDir)
	}
}

func TestNewNormalizesPermissionMode(t *testing.T) {
	initial := DefaultInitial("")
	initial.PermissionMode = permissions.Mode("invalid")

	state := New(initial)
	snapshot := state.Snapshot()

	if snapshot.PermissionMode != permissions.ModeDefault {
		t.Errorf("expected normalized permission mode to be %s, got %s", permissions.ModeDefault, snapshot.PermissionMode)
	}
}

func TestSnapshotReturnsACopy(t *testing.T) {
	initial := DefaultInitial("")
	initial.PermissionRules = permissions.Rules{
		AlwaysAllow: []permissions.Rule{{Pattern: "bash(safe-cmd)", Source: permissions.SourceUser}},
	}

	state := New(initial)
	snap1 := state.Snapshot()
	snap1.DefaultModel = "modified"

	snap2 := state.Snapshot()

	if snap2.DefaultModel == "modified" {
		t.Error("modifying snapshot should not affect subsequent snapshots")
	}
}

func TestSnapshotCopiesRules(t *testing.T) {
	initial := DefaultInitial("")
	initial.PermissionRules = permissions.Rules{
		AlwaysAllow: []permissions.Rule{{Pattern: "bash(safe-cmd)", Source: permissions.SourceUser}},
		AlwaysDeny:  []permissions.Rule{{Pattern: "bash(rm -rf /)", Source: permissions.SourcePolicy}},
	}

	state := New(initial)
	snap1 := state.Snapshot()

	// Modify the snapshot's rules (if possible)
	snap1.PermissionRules.AlwaysAllow = append(snap1.PermissionRules.AlwaysAllow,
		permissions.Rule{Pattern: "modified", Source: permissions.SourceSession})

	snap2 := state.Snapshot()

	if len(snap2.PermissionRules.AlwaysAllow) != len(initial.PermissionRules.AlwaysAllow) {
		t.Errorf("expected %d allow rules, got %d", len(initial.PermissionRules.AlwaysAllow), len(snap2.PermissionRules.AlwaysAllow))
	}
}

func TestUpdateModifiesState(t *testing.T) {
	initial := DefaultInitial("")
	state := New(initial)

	newModel := "new-model"
	state.Update(func(s *Snapshot) {
		s.DefaultModel = newModel
	})

	snap := state.Snapshot()
	if snap.DefaultModel != newModel {
		t.Errorf("expected model %s, got %s", newModel, snap.DefaultModel)
	}
}

func TestUpdateRefreshesTimestamp(t *testing.T) {
	initial := DefaultInitial("")
	state := New(initial)

	firstUpdate := state.Snapshot().UpdatedAt
	time.Sleep(10 * time.Millisecond)

	state.Update(func(s *Snapshot) {
		s.DefaultModel = "changed"
	})

	secondUpdate := state.Snapshot().UpdatedAt

	if secondUpdate.Before(firstUpdate) || secondUpdate.Equal(firstUpdate) {
		t.Error("expected UpdatedAt to increase after Update")
	}
}

func TestUpdateNormalizesPermissionMode(t *testing.T) {
	initial := DefaultInitial("")
	state := New(initial)

	state.Update(func(s *Snapshot) {
		s.PermissionMode = permissions.Mode("invalid")
	})

	snap := state.Snapshot()
	if snap.PermissionMode != permissions.ModeDefault {
		t.Errorf("expected normalized permission mode, got %s", snap.PermissionMode)
	}
}

func TestUpdateCopiesRules(t *testing.T) {
	initial := DefaultInitial("")
	state := New(initial)

	rules := permissions.Rules{
		AlwaysAllow: []permissions.Rule{{Pattern: "bash(safe)", Source: permissions.SourceUser}},
	}

	state.Update(func(s *Snapshot) {
		s.PermissionRules = rules
	})

	// Modify the original rules slice
	rules.AlwaysAllow = append(rules.AlwaysAllow, permissions.Rule{Pattern: "modified", Source: permissions.SourceSession})

	snap := state.Snapshot()
	if len(snap.PermissionRules.AlwaysAllow) != 1 {
		t.Errorf("expected 1 allow rule, got %d", len(snap.PermissionRules.AlwaysAllow))
	}
}

func TestConcurrentSnapshotAndUpdate(t *testing.T) {
	initial := DefaultInitial("")
	state := New(initial)

	var wg sync.WaitGroup
	errors := make(chan string, 100)

	// Concurrent readers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				snap := state.Snapshot()
				if snap.WorkingDir == "" {
					errors <- "snapshot has empty working dir"
				}
			}
		}()
	}

	// Concurrent writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				state.Update(func(s *Snapshot) {
					s.DefaultModel = string(rune('a' + (idx % 26)))
				})
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for errMsg := range errors {
		t.Error(errMsg)
	}
}

func TestResetGlobalForTest(t *testing.T) {
	// First reset to ensure clean state
	initial1 := DefaultInitial("")
	initial1.DefaultModel = "model1"
	ResetGlobalForTest(initial1)

	snap1 := Global().Snapshot()
	if snap1.DefaultModel != "model1" {
		t.Errorf("after first reset: expected model1, got %s", snap1.DefaultModel)
	}

	// Reset again
	initial2 := DefaultInitial("")
	initial2.DefaultModel = "model2"
	ResetGlobalForTest(initial2)

	snap2 := Global().Snapshot()
	if snap2.DefaultModel != "model2" {
		t.Errorf("after second reset: expected model2, got %s", snap2.DefaultModel)
	}
}

func TestGlobalSingleton(t *testing.T) {
	initial := DefaultInitial("")
	initial.DefaultModel = "test-model"
	ResetGlobalForTest(initial)

	state1 := Global()
	state1.Update(func(s *Snapshot) {
		s.OllamaBaseURL = "http://localhost:11434"
	})

	state2 := Global()
	snap := state2.Snapshot()

	if snap.OllamaBaseURL != "http://localhost:11434" {
		t.Errorf("expected global singleton to persist changes, got %s", snap.OllamaBaseURL)
	}
}

func TestEmptyWorkingDirFallsBackToGetwd(t *testing.T) {
	initial := DefaultInitial("")
	if initial.WorkingDir == "" {
		t.Error("expected DefaultInitial to fill in working dir")
	}

	// Verify it's a real path
	if _, err := os.Stat(initial.WorkingDir); err != nil {
		t.Errorf("working dir should be accessible: %v", err)
	}
}
