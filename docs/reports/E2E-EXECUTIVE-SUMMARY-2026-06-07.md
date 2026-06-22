# E2E Executive Summary - 2026-06-07

- Status: `blocked`
- Commit: `cd6743c7968ea6c809859a9a1f8a8f30ea930e88`
- Automated baseline: `pass`
- Release blockers confirmed: `0`
- Highest confirmed severity: `sev2_high`
- Current risks:
  - `/v1/models` advertises `kimi-k2.6:cloud`, but `/v1/sessions/{id}/model` rejects it with `400 model not found`
  - cheap `--print` prompts still persist full tool-schema sets in prompt dumps
  - interactive TUI, memory/hooks, multi-agent task flows, and most performance scenarios remain incomplete
  - some regression and live-model checks require unsandboxed loopback access in this environment
- Next actions:
  - fix and retest server cloud-model selection against the advertised model list
  - fix and retest print-mode cheap-prompt routing
  - complete Lane `EF`, Lane `I`, and remaining interactive Lane `D/G` coverage in a PTY/browser-capable run
