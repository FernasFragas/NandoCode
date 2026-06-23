package sendmessage

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/tasks"
	"github.com/FernasFragas/Nandocode/internal/tools"
	"github.com/FernasFragas/Nandocode/internal/types"
)

func TestSendMessageQueuesByTaskID(t *testing.T) {
	t.Parallel()
	sup := tasks.NewSupervisor(filepath.Join(t.TempDir(), "tasks"), nil)
	id, err := sup.Start(context.Background(), types.KindAgent, "agent", func(ctx context.Context, out *tasks.OutputWriter) (int, error) {
		<-ctx.Done()
		return 137, ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	tool := New(sup, nil)
	_, err = tool.Call(tools.Context{}, Input{To: id, Message: "hello", MessageID: "m1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := sup.DrainMessages(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Content != "hello" {
		t.Fatalf("unexpected drained messages: %#v", msgs)
	}
	_ = sup.Stop(id)
}

func TestSendMessageQueuesByName(t *testing.T) {
	t.Parallel()
	sup := tasks.NewSupervisor(filepath.Join(t.TempDir(), "tasks"), nil)
	id, err := sup.Start(context.Background(), types.KindAgent, "agent", func(ctx context.Context, out *tasks.OutputWriter) (int, error) {
		<-ctx.Done()
		return 137, ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sup.RegisterName("worker-a", id); err != nil {
		t.Fatal(err)
	}
	tool := New(sup, nil)
	_, err = tool.Call(tools.Context{}, Input{To: "worker-a", Message: "hello", MessageID: "m1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = sup.Stop(id)
}

func TestSendMessageWaitForReplyUnsupported(t *testing.T) {
	t.Parallel()
	tool := New(tasks.NewSupervisor(filepath.Join(t.TempDir(), "tasks"), nil), nil)
	_, err := tool.Call(tools.Context{}, Input{To: "bridge:x", Message: "x", WaitForReply: true}, nil)
	if err == nil || err != ErrNotSupported {
		t.Fatalf("expected ErrNotSupported, got %v", err)
	}
}

func TestSendMessageRejectsUnknownTarget(t *testing.T) {
	t.Parallel()
	tool := New(tasks.NewSupervisor(filepath.Join(t.TempDir(), "tasks"), nil), nil)
	_, err := tool.Call(tools.Context{}, Input{To: "worker-z", Message: "x"}, nil)
	if err == nil || !strings.Contains(err.Error(), "unknown agent target") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSendMessageUDS(t *testing.T) {
	stateRoot, err := os.MkdirTemp("/tmp", "sm-state-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(stateRoot) })
	t.Setenv("NANDOCODEGO_STATE_HOME", stateRoot)
	socketDir := filepath.Join(stateRoot, "sockets")
	if err := os.MkdirAll(socketDir, 0o700); err != nil {
		t.Fatal(err)
	}
	socketPath := filepath.Join(socketDir, "peer.sock")
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		var msg tasks.PendingMessage
		_ = json.NewDecoder(conn).Decode(&msg)
	}()

	tool := New(tasks.NewSupervisor(filepath.Join(t.TempDir(), "tasks"), nil), nil)
	_, err = tool.Call(tools.Context{}, Input{To: "uds:" + socketPath, Message: "x"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	<-done
}

func TestSendMessageTerminalTaskCallsResume(t *testing.T) {
	t.Parallel()
	sup := tasks.NewSupervisor(filepath.Join(t.TempDir(), "tasks"), nil)
	id, err := sup.Start(context.Background(), types.KindAgent, "agent", func(ctx context.Context, out *tasks.OutputWriter) (int, error) {
		return 0, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		st, ok := sup.Get(id)
		if !ok {
			t.Fatalf("missing task %s", id)
		}
		status := st.ToSummary().Status
		if status == types.StatusCompleted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("task %s did not complete; status=%s", id, status)
		}
		time.Sleep(10 * time.Millisecond)
	}
	called := false
	tool := New(sup, nil, WithResumeFunc(func(ctx tools.Context, taskID string, msg tasks.PendingMessage) (string, error) {
		called = true
		if taskID != id {
			return "", fmt.Errorf("unexpected task id %s", taskID)
		}
		if msg.Content != "hello" {
			return "", fmt.Errorf("unexpected message %q", msg.Content)
		}
		return "a-ffffffffffff", nil
	}))
	res, err := tool.Call(tools.Context{}, Input{To: id, Message: "hello", MessageID: "m1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("expected resume function to be called")
	}
	if !strings.Contains(res.Display, "resumed as a-ffffffffffff") {
		t.Fatalf("unexpected display: %q", res.Display)
	}
}
