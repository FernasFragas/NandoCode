# BUG-20260607-server-model-endpoint-rejects-listed-cloud-model

> **Owner: roadmap step B1 (P0).** See `docs/roadmap/NEXT-PHASES-IMPLEMENTATION-PLAN.md` and `docs/plans/WEB-UI-UX-PRODUCT-PLAN.md` BF-4.

## Summary

The server advertises `kimi-k2.6:cloud` in `GET /v1/models`, but `POST /v1/sessions/{id}/model` rejects that same model with `400 model not found`. This makes server-side cloud model selection inconsistent with the published model catalog.

## Severity

- Severity: `sev2_high`
- Disposition: `confirmed`
- Area: `server`

## Environment

- Commit: `cd6743c7968ea6c809859a9a1f8a8f30ea930e88`
- OS: `Darwin 25.5.0 arm64`
- Go version: `go version go1.26.2 darwin/arm64`
- Ollama version: client `0.30.5`
- Model: `kimi-k2.6:cloud`
- Relevant env vars:
  - `NANDOCODEGO_CONFIG_HOME=/private/tmp/nandocodego-config.VE8BGL`
  - `NANDOCODEGO_DATA_HOME=/private/tmp/nandocodego-data.xcAJfP`
  - `NANDOCODEGO_CACHE_HOME=/private/tmp/nandocodego-cache.2Ti8Oj`
  - `NANDOCODEGO_STATE_HOME=/private/tmp/nandocodego-state.zExYvb`

## Preconditions

- Local server started successfully on `127.0.0.1:18082`
- Installed model inventory includes `kimi-k2.6:cloud`
- Session created successfully via `POST /v1/sessions`

## Reproduction Steps

1. Start the server:
   `env NANDOCODEGO_CONFIG_HOME=/private/tmp/nandocodego-config.VE8BGL NANDOCODEGO_DATA_HOME=/private/tmp/nandocodego-data.xcAJfP NANDOCODEGO_CACHE_HOME=/private/tmp/nandocodego-cache.2Ti8Oj NANDOCODEGO_STATE_HOME=/private/tmp/nandocodego-state.zExYvb go run ./cmd/nandocodego server --no-ui --model qwen3.6:35b --port 18082`
2. Confirm the model is listed:
   `curl -fsS http://127.0.0.1:18082/v1/models`
3. Create a session:
   `curl -fsS -X POST http://127.0.0.1:18082/v1/sessions`
4. Attempt to switch the session model:
   `curl -sS -D - -X POST http://127.0.0.1:18082/v1/sessions/sess_1780829314267233000/model -H 'Content-Type: application/json' -d '{"model":"kimi-k2.6:cloud"}'`

## Expected Result

The model switch should succeed for a model the server itself just advertised, or it should expose a credential-specific failure if cloud access is the issue.

## Actual Result

The request returns:

- `HTTP/1.1 400 Bad Request`
- body: `model not found`

## Evidence

- Command output summary:
  - `GET /v1/models` returned `200 OK` and included `kimi-k2.6:cloud`
  - `POST /v1/sessions/{id}/model` returned `400 Bad Request`
- Artifact paths: none
- Sanitization notes: no secrets present

## Frequency

- always
- attempt count: `1`

## Evidence Level

- `E1`

## Impacted Scenarios

- `B-009`
- `B-010`
- `G-009`

## Regression Risk

Any server client that trusts `/v1/models` to drive model-selection UI can present options that the switch endpoint refuses, especially for cloud-backed models.

## Suspected Root Cause

The server model-switch path appears to validate model names differently from the model-listing path, or it treats cloud-backed advertised models as unavailable during mutation.

## Recommended Fix Direction

Unify the listing and model-switch validation paths so a model listed by `/v1/models` can either be selected successfully or fail with a precise credential/access reason instead of `model not found`.

## Related Files

- `internal/server/server.go`
- `internal/server/session.go`
- `internal/llm/modelresolver`

## Retest Plan

1. Start the server on loopback.
2. Confirm `kimi-k2.6:cloud` appears in `GET /v1/models`.
3. Create a session and switch to that model.
4. Confirm the switch succeeds or returns a credential-specific error instead of `model not found`.

## Closure Criteria

- Server-listed cloud models are selectable through the session model endpoint, or the API contract is updated so `/v1/models` does not advertise models that cannot be selected.

## Retest - 2026-10-05

- Result: **still reproducible (partially fixed)**. Priority raised to **P0** by
  [ADR-002](../../adr/ADR-002-BROWSER-UI-PRIMARY-SURFACE.md) because the browser
  model picker is driven by `/v1/models`.
- Build: `update-documentation` branch at `088a677`, `make build`, server on
  `127.0.0.1:18082` with temporary `NANDOCODEGO_*_HOME` dirs and `OLLAMA_API_KEY` set.
- `GET /v1/models` listed `glm-5.1:cloud`, `glm-5.3:cloud`, `kimi-k2.6:cloud`.

| Requested model | Result |
| --- | --- |
| `kimi-k2.6:cloud` | `200` `{"base_url":"https://ollama.com","model":"kimi-k2.6","provider":"ollama_cloud_api"}` (original repro now passes) |
| `glm-5.1:cloud` | `400 model not found` (listed, but rejected) |
| `not-a-real-model:latest` | `400 model not found` (expected) |

Root cause (confirmed):

- `/v1/models` lists the **local** Ollama tags, which include `:cloud` stub
  entries pulled earlier (`glm-5.1:cloud`, remote host `https://ollama.com:443`).
- `modelresolver.Resolve` treats any name ending in `:cloud` as cloud-only
  (`trimColonCloudSuffix` → `resolveCloudOnly`), strips the suffix, and checks
  the **live** `https://ollama.com/api/tags` catalog. It never consults the local list.
- Ollama Cloud has retired `glm-5.1` (the catalog lists `glm-5.2`, `glm-5.3`,
  `glm-5.3-flash`), so the stale local stub is advertised but cannot be selected.

Fix direction (pick one, both acceptable under the closure criteria):

1. `/v1/models` cross-checks `:cloud` stubs against the cloud catalog and
   omits or flags entries the cloud no longer serves; or
2. the switch path returns a precise error such as
   `model glm-5.1 is no longer available in Ollama Cloud` (not `model not found`),
   and the browser picker shows it as unavailable.

Add a `modelresolver` unit test with a local `:cloud` stub that is missing from
the cloud catalog, and a server handler test for the error contract.
