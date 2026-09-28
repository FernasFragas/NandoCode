# Phase 14 Exit Gate Validation Guide

## Overview

Phase 14 implements a unified task supervisor that manages long-running background operations (bash commands and sub-agents) without blocking the REPL. This document guides manual testing of the implementation.

## Implementation Summary

### Completed Components

1. **Task Core Infrastructure** ✅
   - `internal/types/task.go` - TaskKind, TaskStatus, TaskSummary types
   - `internal/ids/ids.go` - Kind-prefixed ID generation
   - `internal/tasks/supervisor.go` - Task lifecycle management
   - `internal/tasks/state.go` - Sealed TaskState interface and variants
   - `internal/tasks/output.go` - JSONL output writer and tail reader

2. **Task Tools** ✅
   - TaskCreate (bash and agent kinds)
   - TaskList (with optional filtering)
   - TaskGet (with output tail)
   - TaskOutput (read JSONL lines)
   - TaskStop (cancel running tasks)

3. **State Integration** ✅
   - `state.App.Tasks` field added
   - Publish-subscribe updates via `store.Set`
   - Copy-on-write semantics for task map

4. **Agent Task Support** ✅
   - KindAgent tasks can spawn sub-agents
   - Agent events serialized to JSONL
   - Terminal reason mapped to exit codes

5. **UI/UX Enhancements** ✅
   - `/agents list` slash command shows agent tasks in table format
   - Status bar displays "[N tasks running]" badge
   - TaskGet returns richer metadata including output tail

## Exit Gate Test Procedure

### Prerequisites

- Ollama running and accessible at `http://localhost:11434` (or configured endpoint)
- A model pulled in Ollama (e.g., `qwen2` or similar)
- Terminal session in the repository directory

### Test Steps

#### 1. Start the REPL

```bash
./bin/nandocodego --model qwen2 --no-alt-screen
```

Replace `qwen2` with your available model name. The `--no-alt-screen` flag keeps scrollback history visible.

#### 2. Test Bash Background Task (Non-Blocking Return)

Ask the agent to run a long-lived bash command:

```
Please run this command in the background: sleep 10 && echo "Task complete"
```

**Expected behavior:**
- Task ID returned immediately (e.g., `b-1a2b3c4d`)
- Output file path displayed
- REPL prompt returns immediately (non-blocking)
- Task continues running in background

**Verify:**
```
/agents list
```
Should show the task if it's an agent task, or:
```
/clear
```
Then ask: "What tasks are running?" and let the agent call TaskList.

#### 3. Test Task Listing

Ask the agent:
```
List all running tasks and show their status
```

**Expected behavior:**
- Agent calls TaskList tool
- Shows task ID, status, description, output file path
- Agent can see the task is still running

#### 4. Test Task Status Check

Ask the agent:
```
Check the status of task [ID from step 2] and show me the last few lines of output
```

**Expected behavior:**
- Agent calls TaskGet with tail_lines parameter
- Returns full summary + last 20 lines of JSONL output
- Shows task is still running

#### 5. Test Task Stop

Ask the agent:
```
Stop task [ID] and verify it's killed
```

**Expected behavior:**
- Agent calls TaskStop
- Task transitions from "running" to "killed" within 200ms
- Subsequent TaskGet shows status="killed"

#### 6. Verify JSONL Output File

Ask the agent:
```
Read and show me the output file that was created for the task [ID]. The path is [path from step 2]
```

**Expected behavior:**
- File exists and is readable
- Contains JSONL lines with timestamps and content
- Last line is exit sentinel: `{"kind":"exit","code":...}`
- Can read while task was running or after completion

#### 7. Test Session Isolation

1. Stop the current REPL (Ctrl+C)
2. Start a new REPL session:
   ```bash
   ./bin/nandocodego --model qwen2 --no-alt-screen
   ```
3. Ask: "List all running tasks"

**Expected behavior:**
- Task list is empty (or only shows tasks from THIS session)
- Previous session's tasks are NOT visible
- Output files still exist on disk but state doesn't reload

#### 8. Test Agent Background Task (Optional)

Ask the agent:
```
Create a background agent task to summarize this conversation
```

**Expected behavior:**
- Task created with kind=agent
- ID starts with 'a-'
- Status shows "running"
- Output contains JSONL-formatted agent events

## Acceptance Criteria Checklist

- [ ] TaskCreate returns task ID immediately (timing < 50ms)
- [ ] REPL remains responsive while background task runs
- [ ] TaskList shows all tasks sorted by creation time
- [ ] TaskGet returns full task summary with output tail
- [ ] TaskStop cancels task within 200ms and transitions to killed state
- [ ] JSONL output file exists and is readable during execution
- [ ] JSONL file ends with `{"kind":"exit","code":N}` sentinel
- [ ] Status bar shows "[N tasks running]" when tasks are active
- [ ] `/agents list` shows only agent tasks (kind='a')
- [ ] Session isolation: new REPL doesn't inherit previous session's tasks
- [ ] All tests pass: `go test ./...`
- [ ] Race detector passes: `go test -race ./internal/tasks/...`
- [ ] Build succeeds: `go build ./cmd/nandocodego`

## Known Limitations (Phase 14)

- Task output files grow unbounded (rotation deferred to Phase 16)
- No automatic retry logic (deferred to Phase 15+)
- No task dependency graphs or pipelines
- Cross-session task persistence not implemented (by design)
- Output files require 600ms-2s to appear on disk (OS buffering)

## Troubleshooting

### "task supervisor unavailable" error
- Confirm REPL is running with TaskCreate tool registered
- Check that repl.go was updated to use NewWithAgent

### Task appears stuck running
- Confirm Ollama is responsive: `curl http://localhost:11434/api/tags`
- Check task output file exists and is being written

### JSONL file not found
- Verify path returned from TaskCreate
- Check session directory exists: `~/.nandocodego/sessions/<session-id>/`

### Agent task not appearing in /agents list
- Confirm kind is "agent" (not "bash")
- Check that task was created with TaskCreate, not Agent tool

## Exit Gate Status

After completing the test steps above and verifying all acceptance criteria:

1. Record test results and timestamp in Phase log
2. Commit test documentation
3. Mark Phase 14 complete in PHASE-LOG.md

Phase 14 is complete when all acceptance criteria are met and manual exit gate passes with a real Ollama model.
