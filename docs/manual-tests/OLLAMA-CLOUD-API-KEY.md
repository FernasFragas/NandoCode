# Manual Test: Ollama Cloud API Key

Date: 2026-05-22 (header added 2026-10-06)

Status: TUI, `--print`, and server-API flow below. The browser flow is added with roadmap step B2 (cloud key entry, `docs/plans/WEB-UI-UX-PRODUCT-PLAN.md` BF-3).

## TUI, `--print`, And Server API

1. Clear environment/keychain credentials.
2. Select a local model and verify no credential prompt appears.
3. Run `/models` and verify local-only output.
4. Run `/models --cloud` and verify cloud list or unavailable message.
5. Run `/models --all` and verify grouped local + cloud rendering.
6. Run `/model gpt-oss:120b` and verify credential modal appears.
7. Press `Esc` and verify model/provider remain unchanged.
8. Run `/model gpt-oss:120b` again, choose `Use once`, and verify switch to Ollama Cloud.
9. Send a harmless prompt and verify streamed response.
10. Restart app and verify `Use once` did not persist.
11. Run `/model gpt-oss:120b`, choose `Save to keychain`, restart, and verify prompt is skipped.
12. Set `NANDOCODEGO_OLLAMA_CLOUD=0` and verify cloud lookup is disabled.
13. Run `--print` with cloud-only model and no credential and verify failure occurs before prompt packing.
14. Run server mode with cloud-only model and no credential and verify structured `requires_credential` response.

## Browser (TODO: write when B2 lands)

Cover at least: cloud model marked in the picker with its credential state; selecting it opens key entry before any project context is sent; `Use once` vs `Save to keychain` behavior across a server restart; the key is never logged, echoed, or stored in browser storage; `requires_credential` errors render clearly.
