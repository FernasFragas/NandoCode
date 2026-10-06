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
- `nandocodego server` binds to `127.0.0.1:8080` by default. The `/v1` API
  always requires a bearer token: pass `--token`, or let the server generate
  one and print a launch URL (`http://127.0.0.1:8080/#token=...`) to the
  terminal. Binding beyond loopback requires an explicit `--token`. On
  loopback the server also rejects non-loopback `Host` headers (DNS
  rebinding), cross-origin browser requests, and non-JSON API request bodies.
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

Report vulnerabilities privately through GitHub private vulnerability reporting:
open the repository's **Security** tab and choose **Report a vulnerability**
(https://github.com/FernasFragas/NandoCode/security/advisories/new). Do not open
a public issue for a vulnerability, and do not post secrets, tokens, or exploit
payloads in public text. Reports are handled through a GitHub security advisory,
and fixes are disclosed in a coordinated way once a release is available.

## Release Security Checklist

Before a release:

- Run `go test ./...`.
- Run `go test -race ./...` when practical.
- Run `tools/check-allowed-deps.sh`.
- Run `tools/check-network-policy.sh`.
- Run `govulncheck ./...`.
- Review hook, MCP, server, task, sub-agent, file-write, and shell-command
  boundaries for fail-closed behavior.
