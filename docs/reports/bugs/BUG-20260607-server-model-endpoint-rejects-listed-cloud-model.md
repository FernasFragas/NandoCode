# BUG-20260607-server-model-endpoint-rejects-listed-cloud-model

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
