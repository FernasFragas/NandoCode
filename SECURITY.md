# Security Policy

## Security Posture

`nandocodego` is a local-first development tool. By default it talks to a local
Ollama daemon at `http://localhost:11434` and runs against files in the current
workspace. It can still read files, write files, run shell commands, fetch URLs,
start background tasks, connect to configured MCP servers, and expose an HTTP
server when explicitly started, so it should be treated as a powerful local
automation tool rather than a sandbox.

## Trust Boundaries

- The user controls which workspace is opened and which permission mode is used.
- Tool execution is mediated through the central permission resolver.
- Project-controlled hooks are parsed and reported, but execution is restricted
  until the project trust model is complete.
- MCP servers should be configured only from trusted sources. Untrusted servers
  can expose tool descriptions or data that influence model behavior.
- `nandocodego server` binds to `127.0.0.1:8080` by default. Use `--token` when
  exposing it beyond a local-only development environment.
- Direct Ollama Cloud access is opt-in through model selection and credentials.

## Credential Handling

Ollama Cloud credentials are read from `OLLAMA_API_KEY` or the OS keychain
(`service: nandocodego`, `account: ollama.com`). API keys must not be stored in
project config files or committed to the repository. Logs and provider errors
should redact credential values.

## Network Policy

The project is local-first. Expected network behavior is limited to:

- Local Ollama or compatible model endpoints configured by the user.
- Ollama Cloud API calls when cloud model use is selected and credentials exist.
- Explicit user/model-requested web fetch tool calls.
- Explicitly configured MCP HTTP/SSE transports.
- HTTP server mode when the user starts `nandocodego server`.

Run `tools/check-network-policy.sh` before release changes that add new network
endpoints.

## Reporting Security Issues

For now, report security issues through the repository issue tracker with a
clear `security` label and avoid posting secrets, tokens, or exploit payloads in
public text. Before public release, replace this section with a private security
contact and coordinated disclosure process.

## Release Security Checklist

Before a release:

- Run `go test ./...`.
- Run `go test -race ./...` when practical.
- Run `tools/check-allowed-deps.sh`.
- Run `tools/check-network-policy.sh`.
- Run `govulncheck ./...`.
- Review hook, MCP, server, task, sub-agent, file-write, and shell-command
  boundaries for fail-closed behavior.
