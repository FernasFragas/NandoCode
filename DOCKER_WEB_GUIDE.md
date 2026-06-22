# Running nandocodego Server with Docker and Browser Access

This guide reflects the current implementation where `nandocodego` includes an HTTP server mode.

## Server Command

Use:

```bash
nandocodego server --bind 0.0.0.0 --port 8080
```

Key flags:

- `--token` for bearer auth (required by the app when binding to non-loopback)
- `--no-ui` to disable the embedded browser UI
- `--model`, `--ollama-url`, `--num-ctx`
- `--max-sessions`, `--idle-timeout`, `--read-timeout`, `--write-timeout`
- `--llm-stream-idle-timeout`, `--cloud-llm-stream-idle-timeout`

Cloud credentials:

- Set `OLLAMA_API_KEY` in the container environment when the selected model resolves to direct Ollama Cloud.
- Server mode is non-interactive and returns a structured `requires_credential` error if cloud credentials are missing.
- For long cloud responses over SSE, prefer `--write-timeout 0` (default) so HTTP write timeout does not cut the stream before the watchdog timeout.
- Long-idle model streams emit SSE event `llm_idle_warning` with `provider`, `timeout_ms`, and `timeout_str`; clients can display it as status while the run continues.
- If the stream stays idle until the configured watchdog timeout, the run still ends with `llm stream watchdog timeout`.

## API Endpoints

- `POST /v1/sessions`
- `GET /v1/sessions/{id}`
- `DELETE /v1/sessions/{id}`
- `GET /v1/sessions/{id}/events` (SSE, supports `Last-Event-ID`)
- `POST /v1/sessions/{id}/messages`
- `POST /v1/sessions/{id}/permissions/{req_id}`
- `GET /v1/health`
- `GET /v1/models`
- `GET /` (embedded UI unless `--no-ui`)

## Docker Run

Build:

```bash
docker build -t nandocodego:latest .
```

Run with UI exposed:

```bash
docker run --rm -p 8080:8080 nandocodego:latest server --bind 0.0.0.0 --port 8080 --token YOUR_TOKEN
```

Open:

```text
http://localhost:8080
```

## Docker Compose

Example command override:

```yaml
services:
  nandocodego:
    build:
      context: .
      dockerfile: Dockerfile
    image: nandocodego:latest
    ports:
      - "8080:8080"
    command: ["server", "--bind", "0.0.0.0", "--port", "8080", "--token", "${NANDOCODEGO_TOKEN}"]
```

## Host Ollama from Docker

If Ollama runs on host:

- macOS/Windows: use `http://host.docker.internal:11434`
- Linux: add `extra_hosts: ["host.docker.internal:host-gateway"]`

Example:

```yaml
command: ["server", "--bind", "0.0.0.0", "--port", "8080", "--token", "${NANDOCODEGO_TOKEN}", "--ollama-url", "http://host.docker.internal:11434"]
```

## Smoke Script

Use the included smoke script against a running server:

```bash
TOKEN=YOUR_TOKEN BASE_URL=http://127.0.0.1:8080 tools/smoke-server.sh
```
