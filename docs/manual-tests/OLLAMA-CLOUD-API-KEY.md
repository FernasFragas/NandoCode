# Manual Test: Ollama Cloud API Key

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
