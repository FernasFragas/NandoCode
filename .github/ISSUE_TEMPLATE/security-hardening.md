---
name: Security Hardening
about: Report a security concern, hardening opportunity, or defense-in-depth improvement
title: '[SECURITY] '
labels: security, enhancement
assignees: ''
---

## Security Concern

**Component affected:**
<!-- e.g., Permission system, MCP integration, Shell tool, File operations, Memory system -->

**Category:**
<!-- e.g., Prompt injection, Secret exfiltration, Path traversal, Permission bypass, DoS -->

## Expected Behavior

<!-- What security property should hold? What should be protected? -->

## Observed Behavior

<!-- What happens instead? What is the gap in protection? -->

## Reproduction Steps

<!-- Detailed steps to reproduce the security concern -->

1. 
2. 
3. 

**Environment:**
- OS: <!-- macOS, Linux, Windows -->
- `nandocodego` version: <!-- output of `nandocodego --version` -->
- Ollama version: <!-- output of `ollama --version` -->
- Model used: <!-- e.g., qwen3, llama3.2 -->

## Impact Assessment

**Assets at risk:**
<!-- What could be compromised? Source code, credentials, filesystem, etc. -->

**Attack vector:**
<!-- How could an attacker exploit this? -->

**Exploitability:**
<!-- Low / Medium / High - How difficult is this to exploit? -->

**Scope:**
<!-- Local only / Requires MCP / Requires network / Requires malicious repo -->

## Proposed Mitigation

<!-- If you have suggestions for how to fix or mitigate this, please share them -->

## Secrets Exposure Checklist

**⚠️ IMPORTANT: If this issue involves potential secret exposure, please answer:**

- [ ] Could credentials have been written to logs?
- [ ] Could credentials have been written to memory files?
- [ ] Could credentials have been written to task output files?
- [ ] Could credentials have been sent to an external endpoint?
- [ ] Could credentials have been displayed in the TUI?
- [ ] If yes to any above, have affected users been notified?

## Additional Context

<!-- Any additional information, logs, screenshots, or context -->

---

**Note:** Do not include secrets, tokens, or working exploit payloads in a public issue. Follow the current private-reporting guidance in `SECURITY.md`.
