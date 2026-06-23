package tui

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/permissions"
	tea "github.com/charmbracelet/bubbletea"
)

func TestPermissionBrokerAllow(t *testing.T) {
	var mu sync.Mutex
	var sentMsgs []tea.Msg
	broker := NewPermissionBroker(func(msg tea.Msg) {
		mu.Lock()
		sentMsgs = append(sentMsgs, msg)
		mu.Unlock()
	})

	promptFunc := broker.PromptFunc()

	// Start a goroutine to submit a decision after a delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		// Find the prompt ID from sent messages
		mu.Lock()
		msgs := sentMsgs
		mu.Unlock()

		if len(msgs) > 0 {
			if pmsg, ok := msgs[0].(permissionPromptMsg); ok {
				broker.Resolve(pmsg.Request.ID, decisionAllow)
			}
		}
	}()

	// Call the prompt function
	decision, reason, err := promptFunc(context.Background(), permissions.Prompt{
		ToolName: "Bash",
		Target:   "echo hello",
		Reason:   "user approved",
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if decision != permissions.DecisionAllow {
		t.Errorf("Expected DecisionAllow, got %v", decision)
	}
	if reason == "" {
		t.Error("Expected non-empty reason")
	}
}

func TestPermissionBrokerDeny(t *testing.T) {
	var mu sync.Mutex
	var sentMsgs []tea.Msg
	broker := NewPermissionBroker(func(msg tea.Msg) {
		mu.Lock()
		sentMsgs = append(sentMsgs, msg)
		mu.Unlock()
	})

	promptFunc := broker.PromptFunc()

	// Start a goroutine to deny after a delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		mu.Lock()
		msgs := sentMsgs
		mu.Unlock()

		if len(msgs) > 0 {
			if pmsg, ok := msgs[0].(permissionPromptMsg); ok {
				broker.Resolve(pmsg.Request.ID, decisionDeny)
			}
		}
	}()

	decision, _, err := promptFunc(context.Background(), permissions.Prompt{
		ToolName: "Bash",
		Target:   "rm -rf /",
		Reason:   "dangerous",
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if decision != permissions.DecisionDeny {
		t.Errorf("Expected DecisionDeny, got %v", decision)
	}
}

func TestPermissionBrokerCancelation(t *testing.T) {
	var mu sync.Mutex
	var sent []tea.Msg
	broker := NewPermissionBroker(func(msg tea.Msg) {
		mu.Lock()
		sent = append(sent, msg)
		mu.Unlock()
	})
	promptFunc := broker.PromptFunc()

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	decision, _, err := promptFunc(ctx, permissions.Prompt{
		ToolName: "Bash",
		Target:   "echo",
		Reason:   "test",
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if decision != permissions.DecisionDeny {
		t.Errorf("Expected DecisionDeny on cancellation, got %v", decision)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(sent) != 2 {
		t.Fatalf("expected prompt and cancellation messages, got %#v", sent)
	}
	if _, ok := sent[1].(permissionCancelledMsg); !ok {
		t.Fatalf("expected permissionCancelledMsg, got %T", sent[1])
	}
}

func TestPermissionCancelledMsgClearsMatchingPrompt(t *testing.T) {
	model := newTestModel(t)
	model.Update(permissionPromptMsg{Request: permissionRequest{ID: "p1", ToolName: "Bash"}})
	if model.store.Get().PermissionPrompt == nil {
		t.Fatal("expected active permission prompt")
	}
	model.Update(permissionCancelledMsg{ID: "p1"})
	if model.store.Get().PermissionPrompt != nil {
		t.Fatalf("expected prompt to clear, got %#v", model.store.Get().PermissionPrompt)
	}
}

func TestPermissionBrokerCancelAll(t *testing.T) {
	var mu sync.Mutex
	var sentMsgs []tea.Msg

	broker := NewPermissionBroker(func(msg tea.Msg) {
		mu.Lock()
		sentMsgs = append(sentMsgs, msg)
		mu.Unlock()
	})

	promptFunc := broker.PromptFunc()

	ctx := context.Background()

	// Start multiple prompt calls
	results := make(chan permissions.Decision, 2)

	go func() {
		decision, _, _ := promptFunc(ctx, permissions.Prompt{
			ToolName: "Bash",
			Target:   "cmd1",
			Reason:   "test1",
		})
		results <- decision
	}()

	go func() {
		decision, _, _ := promptFunc(ctx, permissions.Prompt{
			ToolName: "FileWrite",
			Target:   "file.txt",
			Reason:   "test2",
		})
		results <- decision
	}()

	// Give prompts time to register
	time.Sleep(100 * time.Millisecond)

	// Cancel all pending prompts
	broker.CancelAll()

	// Collect results
	d1 := <-results
	d2 := <-results

	if d1 != permissions.DecisionDeny || d2 != permissions.DecisionDeny {
		t.Errorf("Expected both decisions to be Deny after CancelAll, got %v and %v", d1, d2)
	}
}

func TestPermissionBrokerUnknownID(t *testing.T) {
	broker := NewPermissionBroker(func(msg tea.Msg) {})

	// Resolve with an unknown ID should not panic
	broker.Resolve("unknown-id", decisionAllow)
	// Test passes if no panic
}
