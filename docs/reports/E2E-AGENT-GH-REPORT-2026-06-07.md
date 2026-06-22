# E2E Agent GH Report - 2026-06-07

## Scope

- Lane: `G` server/browser/API and `H` semantic index/retrieval
- Owner agent: `Lane GH worker`
- Functional areas: HTTP server startup, auth/bind policy, health/models/session APIs, embedded UI fetch, semantic CLI index lifecycle
- Source commit: `cd6743c7968ea6c809859a9a1f8a8f30ea930e88`
- Start/end time: `2026-06-07T11:23:00+01:00` / `in progress`

## Environment

- OS: `macOS 26.5.1 (build 25F80)`, `Darwin ... arm64`
- Go version: `go version go1.26.2 darwin/arm64`
- Ollama status: CLI installed at `/usr/local/bin/ollama`; escalated server/API checks reached a running Ollama instance; non-escalated `curl http://localhost:11434/api/tags` failed from the sandbox
- Model/provider: local Ollama server via `http://127.0.0.1:18080`; models observed so far: `qwen3-embedding:8b`, `qwen3.6:35b`, `kimi-k2.6:cloud`
- Isolated config/data/cache/state paths:
  - `NANDOCODEGO_CONFIG_HOME=/private/tmp/nandocodego-config-2TSl5O`
  - `NANDOCODEGO_DATA_HOME=/private/tmp/nandocodego-data-X3lJ6d`
  - `NANDOCODEGO_CACHE_HOME=/private/tmp/nandocodego-cache-abn7hJ`
  - `NANDOCODEGO_STATE_HOME=/private/tmp/nandocodego-state-WUhmp7`
  - `GOCACHE=/private/tmp/go-nandocodego-gocache-FR1nmo`
- Browser/terminal details where relevant: browser automation not yet exercised; current checkpoint uses HTTP fetch of embedded UI HTML only

## Checkpoint

Updated after coordinator-run direct server validation on the same date. This report now includes final evidence for the main loopback API path, SSE replay, tree fetch, and the cloud-model switch inconsistency.

## Scenario Results

| Scenario | Priority | Automation | Evidence | Status | Bug/Block | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| `G-001` `nandocodego server` startup on loopback | `p0` | `automated` | `E0` | `pass` |  | Escalated loopback launch succeeded; non-escalated launch was sandbox-limited. |
| `G-002` auth-required behavior on non-loopback bind | `p0` | `automated` | `E0` | `pass` |  | `server --bind 0.0.0.0 --port 18081` returned `Error: refusing non-loopback bind without --token`. |
| `G-003` health endpoint | `p0` | `automated` | `E0` | `pass` |  | `GET /v1/health` returned `200 OK` with `{\"ollama\":\"reachable\",\"status\":\"ok\"}`. |
| `G-004` models endpoint | `p1` | `automated` | `E0` | `pass` |  | `GET /v1/models` returned `200 OK` and listed 3 models. |
| `G-005` create session | `p0` | `automated` | `E0` | `pass` |  | `POST /v1/sessions` returned `201 Created` with session `sess_1780828230870088000`. |
| `G-006` connect SSE and receive live events | `p0` | `automated` | `E1` | `pass` |  | SSE stream returned `session_ready`, route, run, timing, thinking, `assistant_text_delta`, and `terminal` events incrementally. |
| `G-007` post message and stream response | `p0` | `automated` | `E1` | `pass` |  | `POST /messages` returned `{"queued":true}` and SSE later emitted `assistant_text_delta` content `ok`. |
| `G-009` update session model | `p1` | `automated` | `E1` | `fail` | [BUG-20260607-server-model-endpoint-rejects-listed-cloud-model](./bugs/BUG-20260607-server-model-endpoint-rejects-listed-cloud-model.md) | `/v1/models` advertised `kimi-k2.6:cloud`, but `/model` returned `400 model not found`. |
| `G-010` tree endpoint and path safety | `p1` | `automated` | `E1` | `pass` |  | `GET /tree?path=.` returned `200 OK` with structured entries rooted at the workspace. |
| `G-012` replay via `Last-Event-ID` | `p1` | `automated` | `E1` | `pass` |  | Reconnect with `Last-Event-ID: 171` replayed events `172`, `173`, and `174`. |

## Commands And Results

1. Build CLI:

```sh
go build -trimpath -o /private/tmp/nandocodego ./cmd/nandocodego
```

Result:
- Exit `0`
- stderr warning: `go: writing stat cache ... operation not permitted`
- Binary created at `/private/tmp/nandocodego`

2. Verify binary:

```sh
/private/tmp/nandocodego version
```

Result:
- Exit `0`
- Output: `nandocodego 0.0.0-dev (unknown)`

3. Confirm sandbox-visible Ollama reachability:

```sh
curl -sS -m 3 http://localhost:11434/api/tags
```

Result:
- Exit `7`
- Output: `curl: (7) Failed to connect to localhost port 11434 after 0 ms: Couldn't connect to server`

4. Non-escalated loopback server attempt:

```sh
NANDOCODEGO_CONFIG_HOME=/private/tmp/nandocodego-config-2TSl5O NANDOCODEGO_DATA_HOME=/private/tmp/nandocodego-data-X3lJ6d NANDOCODEGO_CACHE_HOME=/private/tmp/nandocodego-cache-abn7hJ NANDOCODEGO_STATE_HOME=/private/tmp/nandocodego-state-WUhmp7 GOCACHE=/private/tmp/go-nandocodego-gocache-FR1nmo /private/tmp/nandocodego server --bind 127.0.0.1 --port 18080
```

Result:
- Sandbox-limited
- On interruption, stderr surfaced: `Error: listen tcp 127.0.0.1:18080: bind: operation not permitted`

5. Escalated loopback server launch:

```sh
NANDOCODEGO_CONFIG_HOME=/private/tmp/nandocodego-config-2TSl5O NANDOCODEGO_DATA_HOME=/private/tmp/nandocodego-data-X3lJ6d NANDOCODEGO_CACHE_HOME=/private/tmp/nandocodego-cache-abn7hJ NANDOCODEGO_STATE_HOME=/private/tmp/nandocodego-state-WUhmp7 GOCACHE=/private/tmp/go-nandocodego-gocache-FR1nmo /private/tmp/nandocodego server --bind 127.0.0.1 --port 18080
```

Result:
- Running
- Log line: `time=2026-06-07T11:29:51.989+01:00 level=INFO msg="server starting" addr=127.0.0.1:18080`

6. Health:

```sh
curl -i -sS http://127.0.0.1:18080/v1/health
```

Result:
- Exit `0`
- Status: `HTTP/1.1 200 OK`
- Body: `{"ollama":"reachable","status":"ok"}`

7. Models:

```sh
curl -i -sS http://127.0.0.1:18080/v1/models
```

Result:
- Exit `0`
- Status: `HTTP/1.1 200 OK`
- Body listed `qwen3-embedding:8b`, `qwen3.6:35b`, `kimi-k2.6:cloud`

8. Embedded UI fetch:

```sh
curl -i -sS http://127.0.0.1:18080/
```

Result:
- Exit `0`
- Status: `HTTP/1.1 200 OK`
- Body: embedded HTML page titled `nandocodego server`

9. Create session:

```sh
curl -i -sS -X POST http://127.0.0.1:18080/v1/sessions
```

Result:
- Exit `0`
- Status: `HTTP/1.1 201 Created`
- Body included `{"session_id":"sess_1780828230870088000", ... "state":"ready","running":false}`

10. Non-loopback without token:

```sh
/private/tmp/nandocodego server --bind 0.0.0.0 --port 18081
```

Result:
- Exit `1`
- Output: `Error: refusing non-loopback bind without --token`

11. SSE connect:

```sh
curl -fsS -N http://127.0.0.1:18080/v1/sessions/sess_1780828944221401000/events
```

Result:
- Initial event: `session_ready`
- Subsequent incremental events included `retrieval_route_decided`, `semantic_skipped`, `run_started`, `llm_request_started`, `llm_stream_opened`, `first_token_received`, `assistant_text_delta`, and `terminal`

12. Post message:

```sh
curl -fsS -X POST http://127.0.0.1:18080/v1/sessions/sess_1780828944221401000/messages -H 'Content-Type: application/json' -d '{"prompt":"Respond with exactly: ok"}'
```

Result:
- Exit `0`
- Body: `{"queued":true}`

13. Last-Event-ID replay:

```sh
curl -fsS -N --max-time 2 -H 'Last-Event-ID: 171' http://127.0.0.1:18080/v1/sessions/sess_1780828944221401000/events
```

Result:
- Replayed `id: 172`, `id: 173`, and `id: 174`

14. Tree endpoint:

```sh
curl -fsS 'http://127.0.0.1:18080/v1/sessions/sess_1780828944221401000/tree?path=.'
```

Result:
- Exit `0`
- Structured JSON tree rooted at `/Users/fernando/Desktop/to_sync/ai_projects_etc/go-nandocode-llm`

15. Cloud model switch:

```sh
curl -sS -D - -X POST http://127.0.0.1:18082/v1/sessions/sess_1780829314267233000/model -H 'Content-Type: application/json' -d '{"model":"kimi-k2.6:cloud"}'
```

Result:
- `HTTP/1.1 400 Bad Request`
- body: `model not found`

16. Smoke harness script:

```sh
tools/smoke-server.sh
```

Result:
- Exit before first request with `tools/smoke-server.sh: line 12: _auth[@]: unbound variable`

## Coverage Notes

- Functional paths covered: server launch, bind policy, health, model listing, session creation, embedded UI static delivery, SSE stream, message post, replay, tree fetch
- Positive paths covered: loopback startup, session creation, health/models responses, incremental SSE, replay, tree response
- Negative/error paths covered: non-loopback bind without token; sandbox listener restriction before escalation; listed cloud model rejected by session model endpoint; broken smoke harness under default macOS Bash
- Performance or reliability evidence captured: none yet
- Known coverage gaps: SSE, message streaming, permission broker, model switch, tree safety, session delete/replay, browser interactive flow, semantic CLI build/refresh/status/clear, semantic retrieval routing

## Findings

| Finding | Severity | Disposition | Impacted Scenarios | Evidence |
| --- | --- | --- | --- | --- |
| Listed cloud model rejected by session model endpoint | `sev2_high` | `confirmed` | `G-009` | `GET /v1/models` included `kimi-k2.6:cloud`; `POST /model` returned `400 model not found` |
| Smoke harness script fails before first request when `TOKEN` is unset on default macOS Bash | `sev4_low` | `confirmed` | `G-006`, `G-007` | `tools/smoke-server.sh` failed with `_auth[@]: unbound variable` |
| Non-escalated listener binding fails inside this tool sandbox, but succeeds unsandboxed | `sev4_low` | `environmental` | `G-001` | Non-escalated bind failed with `operation not permitted`; escalated bind succeeded immediately. |

## Bugs

- [BUG-20260607-server-model-endpoint-rejects-listed-cloud-model](./bugs/BUG-20260607-server-model-endpoint-rejects-listed-cloud-model.md)
- [BUG-20260607-smoke-server-script-fails-on-empty-auth-array](./bugs/BUG-20260607-smoke-server-script-fails-on-empty-auth-array.md)

## Blocks

- Browser-interactive flows and permission-broker round trips remain unproven in this report.

## Risk Assessment

- Top user-facing risks: server clients can see a cloud model in `/v1/models` that they cannot activate through `/model`
- Top release risks: server cloud-model selection is inconsistent across listing and mutation
- Top test-confidence risks: browser-only flows are not yet proven

## Rerun Recommendation

- Scenarios to rerun immediately: `G-009` after fixing cloud model selection; smoke script after shell-compatibility fix
- Scenarios to rerun after fixes: any browser flow that depends on server-side model switching
- Scenarios that need a different environment: browser automation may need a GUI-capable or Playwright-capable environment

## Lane Recommendation

- `blocked`

Rationale: core HTTP/SSE server functionality is proven, but one confirmed high-severity model-switch defect and one harness bug remain, and browser-only coverage is still incomplete.
