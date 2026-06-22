# Nandocodego User Manual

## 1. What Nandocodego Is

**Nandocodego** is a local-first AI coding assistant for your terminal. It connects a local Ollama model to your project directory by default, can optionally switch to direct Ollama Cloud models with explicit credential consent, and gives the model controlled access to tools for reading files, editing files, searching code, running shell commands, tracking tasks, using skills, loading memory, and calling configured MCP servers.

The core workflow is simple: you describe a development task, the agent reasons about the work, requests tool access when needed, applies changes, and reports the result. The important difference from a plain chat interface is that Nandocodego can inspect and modify the real working tree while enforcing permission checks.

### What It Enables

| Feature | What it enables |
| :--- | :--- |
| Interactive TUI | Work with the agent in a live terminal session with streaming responses, tool status, permission prompts, and slash commands. |
| Non-interactive `--print` mode | Run one prompt from scripts, CI helpers, or shell pipelines and print the final answer. |
| Local Ollama models | Keep prompts and code local by default by using an Ollama server such as `http://localhost:11434`. |
| Ollama Cloud API models | Opt into cloud-only Ollama models with `OLLAMA_API_KEY` or a saved keychain credential. |
| Built-in tools | Let the agent read files, edit files, search code, run shell commands, fetch web pages, and manage session todos. |
| Permission modes and rules | Control which tools run automatically, which require confirmation, and which are denied. |
| `@path` mentions | Attach concrete files or directories to a prompt without pasting their content manually. |
| Persistent memory | Store project/user notes that can be recalled in later sessions. |
| Skills | Load reusable behavioral instructions such as code-review, debugging, or test-writing procedures. |
| Hooks | Run project or user automation at lifecycle events such as prompt submit, pre-tool use, post-tool use, and stop. |
| MCP integration | Add tools from trusted Model Context Protocol servers, such as repository, database, or issue-tracker tools. |
| Background tasks and sub-agents | Ask the agent to start long-running shell or agent tasks while the main session remains usable. |
| Semantic workspace index | Build a local embedding index and use bounded semantic evidence during prompt assembly. |
| Observability | Inspect token/tool usage and optionally emit telemetry through environment configuration. |

---

## 2. Installation and First Run

### Prerequisites

- Go matching `go.mod` (`go 1.26.2` at the time of this document).
- Ollama running locally for local model interaction.
- At least one model available in Ollama.
- Optional for semantic indexing: an embedding model such as `qwen3-embedding:8b`.

Example Ollama setup:

```bash
ollama serve
ollama pull qwen3.6:35b
ollama pull qwen3-embedding:8b
```

### Install from Go

```bash
go install github.com/FernasFragas/nandocodego@latest
```

This installs the `nandocodego` binary into your Go binary directory, usually `~/go/bin`.

### Build from Source

```bash
git clone https://github.com/FernasFragas/nandocodego.git
cd nandocodego
make build
./bin/nandocodego version
```

### Initialize Configuration

```bash
nandocodego init
```

This creates a default user config file at:

```text
~/.nandocodego/config.toml
```

If `XDG_CONFIG_HOME` or `NANDOCODEGO_CONFIG_HOME` is set, the config directory follows those variables instead.

### Check the Environment

```bash
nandocodego doctor
```

`doctor` prints version details, runtime information, config/data/cache/state paths, MCP status, telemetry status, and security baseline checks. Use it when the app starts incorrectly or cannot find expected config files.

---

## 3. Command-Line Usage

### Start the Interactive TUI

```bash
nandocodego --model qwen3.6:35b
```

What this enables:

- A long-running terminal session.
- Streaming assistant text.
- Visible tool execution events.
- Permission prompts for operations that are not automatically allowed.
- Slash commands such as `/help`, `/model`, `/permissions show`, and `/cost`.
- Skills, memory, hooks, MCP tools, task tools, and the sub-agent tool.

Example prompts:

```text
Explain the architecture of this repository and point me to the main entry points.
```

```text
Find the code that loads config.toml, explain the precedence rules, and suggest tests for edge cases.
```

```text
Add validation for empty project names, update the relevant tests, then run the package tests.
```

### Use a Specific Ollama Endpoint

```bash
nandocodego --ollama-url http://localhost:11434 --model qwen3.6:35b
```

This is useful when Ollama is listening on a different host or port.

### Set a Larger Ollama Context Window

```bash
nandocodego --model qwen3.6:35b --num-ctx 200000
```

`--num-ctx` sets the Ollama context window size in tokens for the REPL. Use it when working with larger projects or prompts that include many `@directory` references.

### Disable Alternate Screen

```bash
nandocodego --no-alt-screen
```

This keeps output in the normal terminal buffer. It is useful for debugging, recording terminal output, or running under simple terminal test harnesses.

### Run One Prompt with `--print`

```bash
nandocodego --model qwen3.6:35b --print "Summarize the public API of this project"
```

What this enables:

- A single non-interactive agent run.
- Shell-script friendly behavior.
- A final answer written to stdout.
- Built-in file/search/shell/web tools during the run.

Example scripting use:

```bash
nandocodego --print "Read @README.md and output the three most important setup steps"
```

### JSON Output from `--print`

```bash
nandocodego --print "Explain @internal/cli/root.go" --json
```

Use JSON output when another program needs to consume the response. The JSON includes assistant content and observed tool-use records.

### Ollama Cloud API Models

Nandocodego supports direct Ollama Cloud API access while keeping local models as default.

Model listing:

- `/models` shows local models only.
- `/models --cloud` shows direct cloud catalog models.
- `/models --all` shows both groups.

Model switching:

- `/model <name>` resolves local-first.
- If the model is cloud-only, TUI asks for an Ollama API key before switching.
- `OLLAMA_API_KEY` or a saved keychain credential skips the prompt.
- `/pull <model>` always targets the local Ollama daemon, even when the active provider is cloud.

Credential storage:

- `Use once` keeps key in process memory only.
- `Save to keychain` persists to OS keychain with:
  - service: `nandocodego`
  - account: `ollama.com`

Non-interactive behavior:

- `--print` does not prompt for credentials. Cloud-only models require `OLLAMA_API_KEY` or keychain beforehand.
- Server mode does not prompt for credentials. Missing cloud credentials return a structured `requires_credential` response.

Stream watchdog behavior:

- Local/default streams use `llm_stream_idle_timeout` (`90s` by default); direct Ollama Cloud streams use `cloud_llm_stream_idle_timeout` (`5m` by default).
- When a model stream is idle past the internal warning threshold, TUI shows `[Still waiting for model stream (...); idle ...]`.
- `--print` writes idle warnings to stderr, so stdout and `--json` output stay parseable.
- Server mode emits an SSE event named `llm_idle_warning` with `provider`, `timeout_ms`, and `timeout_str`.
- If the stream stays idle until the configured idle timeout, the run still ends with `llm stream watchdog timeout`.
- Watchdog settings do not change Ollama Cloud `403 requires a subscription` or other provider entitlement errors.

### Print Version Information

```bash
nandocodego version
nandocodego --version
```

---

## 4. Configuration

Nandocodego loads configuration from two layers:

| Layer | Path | Purpose |
| :--- | :--- | :--- |
| User config | `~/.nandocodego/config.toml` | Defaults that apply across projects. |
| Project config | `.nandocodego/config.toml` in the current project | Project-specific overrides and MCP server definitions. |

CLI flags override config values for the current run.

### Complete Example

```toml
# ~/.nandocodego/config.toml

default_model = "qwen3.6:35b"
ollama_base_url = "http://localhost:11434"
llm_stream_idle_timeout = "90s"
cloud_llm_stream_idle_timeout = "5m"
permission_mode = "default"
max_turns = 200
bash_timeout = "5m"
max_read_chars = 65536
max_result_chars = 8192
max_dir_files = 200
max_prompt_files = 400
max_dir_bytes = 524288
max_prompt_bytes = 2097152
max_dir_depth = 8
log_level = "info"
log_format = "text"
memory_enabled = true

[concurrency]
max_batch_size = 10

[semantic_index]
enabled = true
mode = "auto" # off | explicit | auto
auto_build = false
model = "qwen3-embedding:8b"
dimensions = 1024
max_chunk_tokens = 700
chunk_overlap_tokens = 120
max_file_bytes = 1048576
max_records = 200000
batch_size = 32
light_top_k_records = 4
full_top_k_records = 12
deep_top_k_records = 40
prompt_refresh_max_files = 8
prompt_refresh_timeout_ms = 1500
```

### Important Settings

| Setting | Enables or controls | Example |
| :--- | :--- | :--- |
| `default_model` | The model used when `--model` is not provided. | `default_model = "qwen3.6:35b"` |
| `ollama_base_url` | The Ollama endpoint. | `ollama_base_url = "http://localhost:11434"` |
| `llm_stream_idle_timeout` | Idle watchdog timeout for local/default model streams. | `llm_stream_idle_timeout = "120s"` |
| `cloud_llm_stream_idle_timeout` | Idle watchdog timeout for Ollama Cloud API model streams. | `cloud_llm_stream_idle_timeout = "5m"` |
| `permission_mode` | Default safety behavior for tool calls. | `permission_mode = "default"` |
| `max_turns` | Maximum agent thought/action turns before stopping. | `max_turns = 200` |
| `concurrency.max_batch_size` | Maximum number of safe concurrent tool calls. | `max_batch_size = 10` |
| `bash_timeout` | Maximum shell command runtime unless a smaller tool timeout is requested. | `bash_timeout = "2m"` |
| `max_read_chars` | File read cap for file content included in tool results. | `max_read_chars = 100000` |
| `max_result_chars` | Display cap for tool results. | `max_result_chars = 12000` |
| `max_prompt_files` | Maximum files attached through `@directory` mentions. | `max_prompt_files = 500` |
| `max_prompt_bytes` | Maximum total bytes attached through `@path` mentions. | `max_prompt_bytes = 4194304` |
| `max_dir_depth` | Maximum directory traversal depth for directory mentions. | `max_dir_depth = 10` |
| `log_level` | Runtime log verbosity. | `log_level = "debug"` |
| `log_format` | Log output style. | `log_format = "json"` |
| `memory_enabled` | Enables project memory recall and extraction. | `memory_enabled = true` |
| `semantic_index.enabled` | Enables semantic retrieval support. | `enabled = true` |
| `semantic_index.mode` | Retrieval mode: `off`, `explicit`, or `auto`. | `mode = "auto"` |
| `semantic_index.model` | Ollama embedding model for index build and query. | `model = "qwen3-embedding:8b"` |
| `semantic_index.light_top_k_records` | Small-prompt retrieval size for the response-time fast path. | `light_top_k_records = 4` |
| `semantic_index.prompt_refresh_max_files` | Max files opportunistically refreshed around a prompt. | `prompt_refresh_max_files = 8` |

### Environment-Controlled Paths

| Variable | Overrides |
| :--- | :--- |
| `NANDOCODEGO_CONFIG_HOME` | Config directory. |
| `NANDOCODEGO_DATA_HOME` | Data directory for sessions, tasks, and memory. |
| `NANDOCODEGO_CACHE_HOME` | Cache directory. |
| `NANDOCODEGO_STATE_HOME` | State directory. |
| `XDG_CONFIG_HOME` | Base config directory when app-specific override is absent. |
| `XDG_DATA_HOME` | Base data directory when app-specific override is absent. |
| `XDG_CACHE_HOME` | Base cache directory when app-specific override is absent. |
| `XDG_STATE_HOME` | Base state directory when app-specific override is absent. |

Example isolated test run:

```bash
NANDOCODEGO_CONFIG_HOME=/tmp/ncg-config \
NANDOCODEGO_DATA_HOME=/tmp/ncg-data \
nandocodego --model qwen3.6:35b
```

Watchdog timeout override examples:

```bash
nandocodego --model kimi-k2.6:cloud --cloud-llm-stream-idle-timeout 5m

nandocodego --print "Generate a docs plan in @docs" --model kimi-k2.6:cloud --cloud-llm-stream-idle-timeout 5m

nandocodego server --model kimi-k2.6:cloud --cloud-llm-stream-idle-timeout 5m --write-timeout 0
```

---

## 5. Core Workflow

Nandocodego runs an agent loop:

1. It receives your prompt.
2. It expands any `@path` mentions into concrete file context.
3. It can attach bounded semantic index evidence when retrieval is enabled and useful.
4. It asks the model what to do next.
5. The model may answer directly or request one or more tools.
6. Permission resolution decides whether each tool call is allowed, denied, or sent to the TUI prompt.
7. Tool results are returned to the model.
8. The loop repeats until the task is complete, stopped, denied, or reaches `max_turns`.

### Example: Ask for an Explanation

```text
Explain how config loading works. Include the user config path, project config path, and flag override behavior.
```

Likely enabled features:

- `Grep` to find config-loading code.
- `FileRead` to inspect implementation files.
- Direct assistant explanation with file references.

### Example: Ask for a Code Change

```text
Add a regression test for unknown config keys and run the relevant package tests.
```

Likely enabled features:

- `Grep` and `FileRead` to locate tests.
- `FileEdit` or `FileWrite` to update test files.
- `Bash` to run `go test`.
- Permission prompt before mutating files or running non-read-only shell commands when in `default` mode.

---

## 6. `@path` Mentions

Use `@` mentions to attach file or directory content to your prompt.

### Attach One File

```text
Explain @internal/cli/root.go and list every supported top-level CLI flag.
```

You can also request an explicit line slice:

```text
Review @docs/PHASE-LOG.md#L3000-L3300 and summarize blockers.
```

Supported range syntax is only `@file#Lstart-Lend`.

What this enables:

- The file content is appended to the prompt as structured context.
- The agent does not need to discover the file first.
- File snapshots are recorded so later edits can detect stale reads.

### Attach Multiple Files

```text
Compare @internal/config/config.go and @internal/config/loader.go. Explain how defaults and overrides interact.
```

### Attach a Directory

```text
Review @internal/permissions for edge cases in rule matching.
```

What this enables:

- Nandocodego walks the directory and appends a tree plus selected text files.
- Directory traversal respects caps such as `max_prompt_files`, `max_prompt_bytes`, `max_dir_files`, `max_dir_bytes`, and `max_dir_depth`.
- Large directories are truncated rather than pasted without bound.

### Directory Mention Modes

Use suffixes on directory mentions to control how context is attached:

```text
@docs?tree
@docs?content
@docs?all
```

Behavior:

- `?tree`: attach only the directory tree (no file bodies). Best for listing prompts.
- `?content`: attach tree + bounded file bodies.
- `?all`: force filesystem-backed expansion for explicit directory mentions (still respects hard excludes and caps).
- Line ranges cannot be combined with mode suffixes. `@file?content#L10-L20` is rejected; use only `@file#L10-L20`.

Default listing-intent behavior:

- Prompts such as `name the all the files and folders in @docs` automatically expand directories as tree-only context.
- Analysis prompts such as `review @docs` keep content mode.

Transparency:

- Expansion summary now reports included/discovered counts and warnings when gitignore omits files in git-backed mode.
- Omitted entries carry explicit reasons (`gitignored`, `binary`, `permission`, caps).

### Prompt Fidelity And Packing

Normal prompts are never silently rewritten into project-wide analysis.

- `@file` and `@dir` evidence is packed against context budget using a manifest + raw/excerpt/omission parts.
- Large explicit files are packed as line-numbered partial ranges (metadata + head/tail/match windows, or explicit `#Lx-Ly` range).
- Packed directory prompts include a directory tree first, then selected file chunks when deterministic selection has enough signal.
- If directory selection is low-confidence, the prompt includes a visible omission notice instead of arbitrary file bodies.
- The original user request remains authoritative in packed prompts.
- If only a tiny packet fits, the model is still called with a partial-file notice and follow-up read guidance.
- The run stops before model call only when no useful source evidence can be included or the source is invalid/unreadable.
- `/analyze-project` remains explicit and separate.
- `--num-ctx` applies to both REPL and `--print` and influences current-turn packing budget.

### Inspect Final Prompt

Use `/prompt` to inspect what was sent to the model:

- `/prompt last`: metadata + per-message previews for the latest final request.
- `/prompt save last`: persist latest dump under state dir (`prompt-dumps/latest.json`).
- `/prompt show last full`: print full content if full dump mode is enabled.

Prompt dump config:

```toml
prompt_dump_mode = "off"      # off | metadata | full
prompt_dump_keep = 10
prompt_preview_chars = 600
```

Notes:

- If `prompt_dump_mode = "off"`, `/prompt last` shows metadata only and prints a hint explaining how to enable previews.
- `/prompt last` also shows evidence-pack metadata (budget, raw/excerpt/omitted bytes, and largest omitted paths).
- For one-off debug sessions, you can override mode with environment variable:

```bash
NANDOCODEGO_PROMPT_DUMP=metadata
```

### Practical Examples

```text
Using @internal/tools and @internal/permissions, explain which built-in tools are read-only and which modify state.
```

```text
Read @README.md and @USER_MANUAL.md, then suggest documentation gaps without editing files.
```

```text
Update @USER_MANUAL.md to include examples for the hooks feature.
```

---

## 7. Semantic Workspace Index

The semantic workspace index is a local cache of embedding-backed code and document records. It is useful when a prompt names a concept without naming every relevant file.

Build or refresh it from the shell:

```bash
nandocodego index build
nandocodego index refresh
nandocodego index status
nandocodego index clear
```

Manage it from the REPL:

```text
/semantic status
/semantic on
/semantic off
/semantic auto
/semantic explicit
/semantic deep
/index build
/index refresh
/index status
/index clear
```

Behavior:

- The index uses the local Ollama embedding model configured by `semantic_index.model`.
- `mode = "auto"` lets normal prompts use bounded semantic retrieval when the route policy decides it is useful.
- `mode = "explicit"` limits semantic retrieval to explicit controls such as `/semantic deep`.
- `/semantic deep` applies broader semantic retrieval to the next prompt only.
- `/index build` and `/index refresh` show scan/extract/embed/write progress in the TUI status area.
- Missing, stale, disabled, or incompatible indexes produce visible fallback messages; normal prompts can still proceed.

The response-time refactor keeps simple prompts on a fast chat-only path when semantic retrieval and tool schemas are unnecessary.

---

## 8. Built-In Tools

You do not call tools manually in normal use. You describe the task, and the model selects tools. This section explains what each tool enables so you can write better prompts and understand permission prompts.

### Bash

`Bash` runs shell commands in the current working directory.

What it enables:

- Running tests, linters, formatters, build commands, and project diagnostics.
- Inspecting git state with commands such as `git status --short`.
- Running read-only shell commands without manual copy/paste.

Example prompts:

```text
Run the Go tests for the config package and summarize any failures.
```

```text
Check git status and tell me which files have local changes.
```

```text
Run go test ./internal/permissions and fix any failing tests.
```

Permission behavior:

- Read-only commands can be allowed automatically.
- Mutating commands usually ask in `default` mode.
- Intrinsically destructive commands are denied by the Bash safety classifier even if broader modes are used.
- `plan` and `dontAsk` modes deny commands that would require a prompt.

### FileRead

`FileRead` reads UTF-8 text files inside allowed working directories.

What it enables:

- Inspecting implementation files before editing.
- Reading only part of a large file with `start_line` and `line_limit`.
- Recording file snapshots for stale-edit detection.
- Reusing unchanged already-in-context ranges via compact dedupe notices.

Example prompts:

```text
Read internal/agent/agent.go and summarize the terminal conditions.
```

```text
Inspect the config tests before making any changes.
```

### FileWrite

`FileWrite` writes complete text files atomically.

What it enables:

- Creating new files.
- Replacing full file content when that is safer than patching individual strings.
- Atomic writes so partially written files are avoided.

Example prompts:

```text
Create docs/config-examples.md with common config snippets for local development.
```

```text
Generate a small example skill file under .nandocodego/skills/test-writer.md.
```

Permission behavior:

- Asks in `default` mode.
- Denied in `plan` and `dontAsk` modes.
- Allowed in `bypass` unless another rule or hook denies it.

### FileEdit

`FileEdit` edits an existing file by replacing an exact `old_string` with `new_string`.

What it enables:

- Small targeted code or documentation changes.
- Safer edits because the old text must match the current file.
- Optional replace-all behavior when the old text appears multiple times.
- Staleness detection when a file changed after it was last read.

Example prompts:

```text
In USER_MANUAL.md, replace the installation section with clearer source-build instructions.
```

```text
Rename the config setting explanation from max_batch_size to concurrency.max_batch_size wherever it appears in the docs.
```

### Glob

`Glob` finds files by glob pattern.

What it enables:

- Discovering project structure.
- Finding files by extension or naming convention.
- Recursive patterns with `**`.

Example prompts:

```text
Find all Go test files related to permissions and summarize what each covers.
```

```text
List documentation files under docs and group them by phase number.
```

Typical internal patterns:

```text
**/*.go
internal/**/*_test.go
docs/*.md
```

### Grep

`Grep` searches file contents with regular expressions.

What it enables:

- Finding functions, command names, config keys, error messages, or TODOs.
- Restricting by include/exclude glob.
- Returning context lines around matches.

Example prompts:

```text
Find every place permission_mode is read or displayed.
```

```text
Search for TODO comments in Go files and propose a cleanup plan.
```

```text
Find all slash command handlers and document each command.
```

### WebFetch

`WebFetch` fetches `http://` or `https://` URLs and returns text from HTML, JSON, or plain text responses.

What it enables:

- Looking up public documentation when network access is allowed by permissions.
- Summarizing a webpage without opening a browser.
- Fetching plain JSON/text references.

Example prompts:

```text
Fetch https://go.dev/doc/effective_go and summarize the parts relevant to error handling.
```

Permission behavior:

- External HTTP requests ask in `default` mode.
- Denied in `plan` and `dontAsk` modes.
- Private and loopback IP addresses are blocked unless local fetch is explicitly allowed by tool context.

### TodoRead and TodoWrite

`TodoRead` and `TodoWrite` manage a session-scoped todo list for the agent.

What they enable:

- Breaking complex tasks into visible steps.
- Tracking `pending`, `in_progress`, and `completed` items.
- Prioritizing work with `high`, `medium`, and `low`.

Example prompts:

```text
Make a todo list before refactoring the config loader, then work through it step by step.
```

```text
Show your current todo list and mark completed items before continuing.
```

### Skill

`Skill` loads a named skill as behavioral context.

What it enables:

- Applying reusable procedures and standards during a session.
- Loading bundled, user, project, or MCP-provided instructions.

Example prompts:

```text
Use the code-review skill to review the current permissions package.
```

```text
Load the write-tests skill, then add missing tests for config warnings.
```

### Task Tools

The task tools are available to the agent in the interactive REPL:

| Tool | What it enables |
| :--- | :--- |
| `TaskCreate` | Start a background bash or agent task. |
| `TaskList` | List tracked background tasks. |
| `TaskGet` | Inspect one task and tail recent output. |
| `TaskOutput` | Read task JSONL output lines. |
| `TaskStop` | Stop a running task. |

Example prompts:

```text
Start a background bash task that runs go test ./..., then continue reviewing the docs while it runs.
```

```text
List background tasks and show the latest output for any failing task.
```

### Agent

`Agent` spawns a bounded sub-agent.

What it enables:

- Delegating a focused investigation or implementation task.
- Running work in the background when requested.
- Keeping recursive sub-agent creation blocked for safety.

Example prompts:

```text
Spawn a sub-agent to inspect the hooks package and summarize all supported hook events.
```

```text
Use a background agent to look for missing tests in internal/mcp while you update USER_MANUAL.md.
```

Permission behavior:

- Sub-agent execution asks for approval.
- Recursive sub-agent creation is blocked.

---

## 9. Permission System

Permissions decide whether a tool call is allowed, denied, or sent to an interactive prompt.

Resolution order:

1. Hooks can allow, ask, or deny early.
2. Session/user/project rules are matched.
3. The tool classifier evaluates the operation.
4. The active permission mode applies default behavior.
5. The TUI prompt is used when the result is `ask` and prompting is available.

### Permission Modes

Use `permission_mode` in config to choose the default mode:

```toml
permission_mode = "default"
```

Supported modes:

| Mode | Behavior | Use when |
| :--- | :--- | :--- |
| `default` | Allows read-only operations, asks for operations that may modify state, denies tool-classified unsafe actions. | Normal coding work. |
| `bypass` | Allows operations that would normally ask, but still respects intrinsic denials, rules, and hooks. | High-trust local work where you want fewer prompts. |
| `dontAsk` | Allows read-only operations and denies anything that would need a prompt. | Read-only audits or non-interactive safety. |
| `auto` | Currently behaves like `default` for ask/deny semantics because no separate auto-classifier is active. | Future-compatible config. |
| `acceptEdits` | Currently behaves like `auto` in this implementation. | Future-compatible config for edit-friendly workflows. |
| `plan` | Allows read-only operations and denies state-changing operations. | Asking for plans, reviews, or analysis only. |
| `bubble` | Asks for most decisions except tool denials, intended for parent/TUI escalation flows. | Sub-agent or escalation-heavy workflows. |

### Session Permission Rules

In the REPL, use `/permissions` to inspect or add session rules.

Show current mode and rules:

```text
/permissions show
```

Allow a specific command pattern for this session:

```text
/permissions allow Bash(go test ./...)
```

Allow all Go test commands:

```text
/permissions allow Bash(go test*)
```

Deny writes to generated files:

```text
/permissions deny FileWrite(**/generated/**)
```

Deny web access:

```text
/permissions deny WebFetch(*)
```

Rule syntax:

```text
ToolName(glob)
```

The glob matches the tool's permission target:

| Tool | Target used for matching |
| :--- | :--- |
| `Bash` | Command string. |
| `FileRead` | File path. |
| `FileWrite` | File path. |
| `FileEdit` | File path. |
| `WebFetch` | URL or serialized input fallback. |

Rule precedence is deny, then ask, then allow. Session rules added with slash commands last only for the current session.

### Practical Permission Recipes

Read-only review:

```toml
permission_mode = "plan"
```

Normal development:

```toml
permission_mode = "default"
```

Fast local refactor with fewer prompts:

```toml
permission_mode = "bypass"
```

Block network access during a session:

```text
/permissions deny WebFetch(*)
```

Allow repeated package tests:

```text
/permissions allow Bash(go test ./internal/config*)
```

---

## 10. Interactive Slash Commands

Type slash commands in the REPL input box.

### System Commands

| Command | What it does | Example |
| :--- | :--- | :--- |
| `/help` | Shows available commands. | `/help` |
| `/clear` | Clears transcript and message history. | `/clear` |
| `/exit` | Exits the REPL. | `/exit` |
| `/init` | Creates the default user config if missing. | `/init` |
| `/refresh-index` | Refreshes the TUI `@file` completion index. | `/refresh-index` |
| `/compact` | Requests TUI-managed context compaction. | `/compact` |
| `/bg` | Marks the active run as backgrounded or prints background run status. | `/bg` |
| `/btw <question>` | Runs an isolated side question in read-only permission mode when idle; queues one side question while a main run is active. | `/btw what files define the TUI?` |

Run-status notes:

- The TUI status/footer shows waiting, streaming, thinking, running-tool, permission, retry, compaction, queue, background, and queued `/btw` states when available.
- `/btw` uses read-only permission mode for the side run and does not append the side conversation to the main conversation history. The model-visible tool list is not yet reduced to read-only tools, so permission mode is the current enforcement boundary.
- If a main run is already active, `/btw` is queued and runs after that active run completes; it is not a concurrent side conversation yet.

### Model Commands

| Command | What it does | Example |
| :--- | :--- | :--- |
| `/model` | Shows the active model. | `/model` |
| `/model <name>` | Resolves local-first, then switches to Ollama Cloud when the model is cloud-only and credentials are available. | `/model qwen3.6:35b` |
| `/models` | Lists local Ollama models. | `/models` |
| `/models --cloud` | Lists direct Ollama Cloud API models. | `/models --cloud` |
| `/models --all` | Lists local and direct cloud catalogs. | `/models --all` |
| `/pull <name>` | Pulls a model through the local Ollama daemon. | `/pull llama3` |

Example workflow:

```text
/models
/model qwen3.6:35b
/cost
```

### Semantic and Index Commands

| Command | What it does | Example |
| :--- | :--- | :--- |
| `/semantic status` | Shows semantic retrieval mode and index status. | `/semantic status` |
| `/semantic on` | Enables automatic semantic retrieval. | `/semantic on` |
| `/semantic off` | Disables semantic retrieval. | `/semantic off` |
| `/semantic auto` | Enables route-policy-driven retrieval. | `/semantic auto` |
| `/semantic explicit` | Enables only explicit semantic retrieval controls. | `/semantic explicit` |
| `/semantic deep` | Uses broader semantic retrieval for the next prompt only. | `/semantic deep` |
| `/index build` | Builds the local semantic index. | `/index build` |
| `/index refresh` | Refreshes the semantic index, reusing compatible cache entries. | `/index refresh` |
| `/index status` | Shows semantic index compatibility and counts. | `/index status` |
| `/index clear` | Clears the local semantic index cache. | `/index clear` |

### Memory Commands

| Command | What it does | Example |
| :--- | :--- | :--- |
| `/memory list` | Lists active memory files. | `/memory list` |
| `/memory show <name>` | Shows one memory file. | `/memory show project-overview.md` |
| `/memory edit <name>` | Opens a memory file in `$EDITOR`, falling back to `vi`. | `/memory edit project-overview.md` |
| `/memory promote <name>` | Moves a pending memory file into the active memory set. | `/memory promote 2026-05-09-feedback.md` |

### Hook Commands

| Command | What it does | Example |
| :--- | :--- | :--- |
| `/hooks list` | Lists loaded hooks grouped by event. | `/hooks list` |
| `/hooks reload yes` | Reloads user and project `hooks.json`. | `/hooks reload yes` |

### Permission Commands

| Command | What it does | Example |
| :--- | :--- | :--- |
| `/permissions show` | Shows active mode and rules. | `/permissions show` |
| `/permissions allow <pattern>` | Adds a session allow rule. | `/permissions allow Bash(go test*)` |
| `/permissions deny <pattern>` | Adds a session deny rule. | `/permissions deny WebFetch(*)` |

### Skill Commands

| Command | What it does | Example |
| :--- | :--- | :--- |
| `/skills list` | Lists available skills. | `/skills list` |
| `/skills show <name>` | Displays one skill body. | `/skills show code-review` |

### Cost and Task Commands

| Command | What it does | Example |
| :--- | :--- | :--- |
| `/cost` | Shows prompt tokens, completion tokens, tool calls, LLM calls, runs, and session duration. | `/cost` |
| `/agents list` | Lists agent background tasks tracked in this session. | `/agents list` |

Note: background task creation is exposed to the agent as tools. In normal use, ask for it in natural language, for example: `Start a background task that runs go test ./...`.

---

## 11. Memory

Memory stores durable notes outside the chat transcript. It is useful for project conventions, recurring feedback, architectural facts, or user preferences that should survive across sessions.

### What Memory Enables

- Recalling relevant project notes automatically.
- Promoting pending notes into active memory.
- Keeping a lightweight project knowledge base alongside the repository.
- Separating memory by project scope.

### Memory File Format

Memory files are Markdown files with YAML frontmatter:

```markdown
---
name: Project test policy
description: Notes about how this project expects tests to be run
type: project
---

Use `go test ./...` before publishing broad changes.
For small package changes, prefer the narrow package first, then the full suite.
```

Supported memory types:

| Type | Use for |
| :--- | :--- |
| `user` | User preferences or recurring personal instructions. |
| `feedback` | Corrections from previous work. |
| `project` | Project-specific practices and architecture notes. |
| `reference` | Stable reference material that the agent may need later. |

### List Active Memory

```text
/memory list
```

### Show a Memory File

```text
/memory show project-test-policy.md
```

### Edit a Memory File

```text
/memory edit project-test-policy.md
```

This opens the file in `$EDITOR`. If `$EDITOR` is unset, it uses `vi`.

### Promote Pending Memory

```text
/memory promote 2026-05-09-project-test-policy.md
```

What this enables:

- Lets the agent or workflow place extracted memory drafts into `pending` first.
- Gives you control before a memory becomes active context.

### Prompt Examples

```text
Remember that this project prefers narrow package tests before go test ./... for large suites.
```

```text
List the active memory files and tell me which ones look stale.
```

```text
Use project memory while explaining how releases are built.
```

---

## 12. Skills

Skills are Markdown files with YAML frontmatter. They provide reusable behavior instructions that the agent can load during a session.

### What Skills Enable

- Reusable workflows such as code review, debugging, or test writing.
- Project-local standards without repeating them in every prompt.
- User-level standards across projects.
- Bundled default skills plus project/user overrides.

### Skill Locations

| Source | Location | Priority |
| :--- | :--- | :--- |
| MCP | Provided by MCP server | Highest |
| Project | `.nandocodego/skills/*.md` | High |
| User | `~/.nandocodego/skills/*.md` | Medium |
| Bundled | Embedded in the binary | Lowest |

If multiple skills have the same `name`, the highest-priority source wins.

### Skill File Example

Create `.nandocodego/skills/go-service-review.md`:

```markdown
---
name: go-service-review
description: Review Go service changes for correctness, tests, and operational risk
version: "1.0"
author: "Team"
tags: [go, review, backend]
---

When reviewing Go service code:

- Check error handling paths.
- Verify context cancellation is propagated.
- Look for missing table-driven tests.
- Prefer narrow actionable findings with file references.
- Do not suggest broad rewrites unless they reduce concrete risk.
```

### List Skills

```text
/skills list
```

### Inspect a Skill

```text
/skills show go-service-review
```

### Use a Skill

```text
Use the go-service-review skill to review the latest changes in internal/cli.
```

```text
Load the write-tests skill, then add tests for invalid MCP config.
```

Skills are hot-reloaded by the filesystem watcher. If a change is not picked up immediately, restart the REPL.

---

## 13. Hooks

Hooks run automation around agent lifecycle events. A hook can be a shell command, prompt, HTTP call, or agent hook depending on `kind`.

Hooks are loaded from:

| Layer | Path |
| :--- | :--- |
| User hooks | `~/.nandocodego/hooks.json` |
| Project hooks | `.nandocodego/hooks.json` |

Project hooks are useful for repository-specific policy. User hooks are useful for personal logging, notifications, or global safety checks.

### What Hooks Enable

- Block unsafe prompts before the model acts.
- Deny or ask before specific tool calls.
- Run formatting or audit scripts after tool use.
- Send notifications or telemetry to local systems.
- Stop an agent loop when a custom condition is met.

### Common Hook Events

| Event | When it runs | Can block? |
| :--- | :--- | :--- |
| `SessionStart` | At the beginning of an agent run. | No |
| `SessionEnd` | At terminal completion. | No |
| `UserPromptSubmit` | After the user submits a prompt. | Yes |
| `PreToolUse` | Before a tool call. | Yes |
| `PostToolUse` | After a tool succeeds or fails. | No |
| `PostToolUseFailure` | After a tool call fails. | No |
| `PermissionDenied` | After permission resolution denies a tool. | No |
| `Stop` | Before the agent stops. | Yes |

Other event names exist for future or specialized lifecycle integration, but the events above are the most useful for day-to-day project automation.

### Command Hook Example: Deny `rm` Commands

`.nandocodego/hooks.json`:

```json
{
  "hooks": [
    {
      "kind": "command",
      "event": "PreToolUse",
      "matcher": "Bash",
      "command": "case $(cat) in *'rm -rf'*) echo 'rm -rf is blocked by project policy' >&2; exit 2;; *) echo '{}';; esac",
      "timeout_sec": 3
    }
  ]
}
```

How it works:

- Hook input is sent to the command on stdin as JSON.
- Exit code `2` denies the operation.
- A successful hook can print JSON such as `{}` or `{"decision":"ask","reason":"review this command"}`.

### Command Hook Example: Ask Before Web Fetches

```json
{
  "hooks": [
    {
      "kind": "command",
      "event": "PreToolUse",
      "matcher": "WebFetch",
      "command": "printf '{\"decision\":\"ask\",\"reason\":\"Network access requires review\"}'",
      "timeout_sec": 2
    }
  ]
}
```

### Reload Hooks

```text
/hooks reload yes
```

### Inspect Loaded Hooks

```text
/hooks list
```

### Hook Result JSON

Hooks may return:

```json
{"decision":"allow","reason":"approved by hook"}
```

```json
{"decision":"ask","reason":"manual review required"}
```

```json
{"decision":"deny","reason":"blocked by policy"}
```

```json
{"warning":"hook saw unexpected input but did not block"}
```

Decision precedence across multiple hooks favors denial over ask over allow.

---

## 14. MCP Integration

MCP servers add external tools to the agent registry. Configure them in user or project `config.toml` under `[mcp.servers.<name>]`.

### What MCP Enables

- Repository, issue-tracker, database, browser, or custom business tools.
- Tooling that is independent from Nandocodego's built-in tools.
- Project-specific server definitions in `.nandocodego/config.toml`.
- User-level trusted server definitions in `~/.nandocodego/config.toml`.

### Stdio MCP Server Example

```toml
[mcp.servers.github]
enabled = true
trusted = true
transport = "stdio"
command = "npx"
args = ["-y", "@modelcontextprotocol/server-github"]
env = { GITHUB_TOKEN = "replace-with-token" }
```

What this enables:

- Starts the server as a local subprocess.
- Registers its tools if startup succeeds.
- Marks it trusted so Nandocodego will connect and expose tools.

### HTTP MCP Server Example

```toml
[mcp.servers.local_docs]
enabled = true
trusted = true
transport = "http"
url = "http://localhost:8080/mcp"
auth = "none"
```

### Project vs User Trust

User config defaults MCP servers to trusted. Project config defaults servers to untrusted unless explicitly set:

```toml
trusted = true
```

This is intentional. A repository should not silently cause arbitrary local commands or network connections to be trusted without review.

### Inspect MCP Status

```bash
nandocodego doctor
```

`doctor` lists configured servers, whether each is enabled/trusted, connection status, and tool count.

### Prompt Examples

```text
Use the GitHub MCP tools to summarize the latest open issues for this repository.
```

```text
Use the local_docs MCP server to find internal API documentation for the billing service.
```

If MCP tools do not appear, check:

- The server is configured under `[mcp.servers.<name>]`.
- `enabled = true`.
- `trusted = true` for project-defined servers.
- `doctor` reports the server as connected.
- Required environment variables are available.

---

## 15. Background Tasks and Sub-Agents

Background work is available through agent tools in the interactive REPL. You request it in natural language; the model calls the relevant task or agent tool.

### What Background Tasks Enable

- Run long test suites while continuing analysis.
- Tail task output later instead of blocking the main interaction.
- Stop tasks that are no longer useful.
- Store task output as JSONL under the session task directory.

### Start a Background Bash Task

```text
Start a background bash task named full test suite that runs go test ./..., then continue reviewing the config package.
```

The agent will request approval for task creation. After approval, it receives a task ID and output file path.

### List Background Tasks

```text
List background tasks and show their status.
```

Or use the slash command for agent tasks:

```text
/agents list
```

### Inspect Task Output

```text
Show the last 50 lines of output for the full test suite task.
```

### Stop a Task

```text
Stop the background test task if it is still running.
```

### Spawn a Sub-Agent

```text
Spawn a sub-agent to inspect internal/mcp and report the most important risks. Do not edit files.
```

What this enables:

- A bounded independent investigation.
- Optional background execution.
- Separate model and max-turn settings when requested.

Example with background intent:

```text
Start a background sub-agent to find missing tests in internal/hooks while you update the manual.
```

Safety notes:

- Sub-agent execution requires approval.
- Sub-agents cannot recursively spawn more sub-agents.
- Permission prompts from a child can be escalated back to the parent/TUI flow.

---

## 16. Observability and Cost

### View Session Usage

```text
/cost
```

This shows:

- Prompt tokens.
- Completion tokens.
- Total tokens.
- LLM calls.
- Tool calls.
- Agent runs.
- Session duration.

### Logging

Set log behavior in config:

```toml
log_level = "debug"
log_format = "json"
```

Or through CLI flags:

```bash
nandocodego --log-level debug --log-format json
```

### Telemetry Environment

`doctor` reports telemetry status from environment variables such as:

```bash
NANDOCODEGO_TELEMETRY=1
NANDOCODEGO_OTEL_ENDPOINT=http://localhost:4318
nandocodego doctor
```

Use telemetry when you need runtime measurements for local debugging or team-operated observability. Leave it unset for normal local use.

---

## 17. Practical Workflows

### Workflow: Read-Only Code Review

```bash
nandocodego --model qwen3.6:35b
```

Then:

```text
/permissions show
```

```text
Use the code-review skill to review @internal/permissions. Do not edit files. Focus on bugs, regressions, and missing tests.
```

Recommended config for stricter read-only review:

```toml
permission_mode = "plan"
```

### Workflow: Implement a Small Fix

```text
Find why /permissions show does not include ask rules added by hooks, fix the issue if present, and run the relevant tests.
```

What the agent may use:

- `Grep` to find permission rendering.
- `FileRead` to inspect state and command handlers.
- `FileEdit` to patch code.
- `Bash` to run tests.

### Workflow: Documentation Update

```text
Update @USER_MANUAL.md to add concrete examples for every slash command. Keep the command names aligned with implementation.
```

Useful follow-up:

```text
Review the updated manual against @internal/commands/registry.go and report any mismatches.
```

### Workflow: Let Tests Run in Background

```text
Start go test ./... in the background. While it runs, inspect @internal/mcp and document how MCP config works.
```

Later:

```text
Show the latest output from the background test task and summarize any failures.
```

### Workflow: Create and Use a Project Skill

```text
Create a project skill named docs-writer that tells the agent to write concise user-facing documentation with examples and no invented commands.
```

Then:

```text
Use the docs-writer skill to improve USER_MANUAL.md.
```

---

## 18. Troubleshooting

### Ollama Connection Refused

Symptom:

```text
failed to list models
connection refused
```

Check:

```bash
ollama serve
ollama list
nandocodego --ollama-url http://localhost:11434 --model qwen3.6:35b
```

### Model Not Found

Symptom:

```text
[Error: model not found locally: qwen3.6:35b. Try /pull qwen3.6:35b]
```

Fix:

```text
/pull qwen3.6:35b
/model qwen3.6:35b
```

Or from a shell:

```bash
ollama pull qwen3.6:35b
```

### Permission Denied

Cause examples:

- Current mode is `plan` or `dontAsk`.
- A session rule denies the tool call.
- A hook denied the operation.
- The tool classifier considers the operation unsafe.

Inspect:

```text
/permissions show
/hooks list
```

Fix examples:

```text
/permissions allow Bash(go test*)
```

```toml
permission_mode = "default"
```

### File Edit Failed Because Text Was Not Found

Cause:

- The target file changed.
- The requested old text was not exact.
- The text appears multiple times and the agent did not request replace-all.

Fix prompt:

```text
Re-read the file, then apply the edit with a more specific old_string context.
```

### File Edit Failed Because of Staleness

Cause:

- A file changed after it was last read.

Fix prompt:

```text
Re-read the file and retry the edit without overwriting unrelated changes.
```

### `@directory` Mentions Are Truncated

Cause:

- Directory or prompt caps were reached.

Relevant config:

```toml
max_prompt_files = 400
max_prompt_bytes = 2097152
max_dir_depth = 8
```

Better prompt:

```text
Review @internal/config only, not the whole repository.
```

### MCP Server Not Connected

Check:

```bash
nandocodego doctor
```

Then verify:

- The config section is `[mcp.servers.<name>]`.
- `enabled = true`.
- `trusted = true` for project config.
- `command` is set for `stdio` servers.
- `url` is set for `http` servers.
- Required environment variables are present.

### Hooks Do Not Run

Check:

```text
/hooks list
```

Reload:

```text
/hooks reload yes
```

Verify the JSON file is at one of:

```text
~/.nandocodego/hooks.json
.nandocodego/hooks.json
```

### `doctor` Reports Missing Security Baseline Files

`doctor` checks for repository files such as `SECURITY.md` and scripts under `tools/`. Run it from the repository root when diagnosing the source checkout.

---

## 19. Safety Guidelines

- Use `default` mode for normal coding.
- Use `plan` mode for analysis-only sessions.
- Add explicit deny rules for operations your project should never allow.
- Keep project MCP servers untrusted until reviewed.
- Prefer narrow prompts with `@file` or small `@directory` mentions over attaching the whole repository.
- Ask the agent to run narrow tests first, then broader suites.
- Inspect `/permissions show`, `/hooks list`, and `/cost` during long sessions.

---

## 20. License

Nandocodego is open source software. See `LICENSE` in the repository for the full license text.
