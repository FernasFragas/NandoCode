# BLOCK-20260607-b007-cloud-credential-gate-not-observable-with-preconfigured-cloud-access

## Scenario

`B-007` cloud-only model selection requests credentials before sending context.

## Blocking Condition

The installed cloud model `kimi-k2.6:cloud` is already runnable in this environment, so the missing-credential gate is not observable without altering user credential state.

## Category

- environment

## Evidence

- Command attempted:
  `env NANDOCODEGO_CONFIG_HOME=/private/tmp/nandocodego-config.VE8BGL NANDOCODEGO_DATA_HOME=/private/tmp/nandocodego-data.xcAJfP NANDOCODEGO_CACHE_HOME=/private/tmp/nandocodego-cache.2Ti8Oj NANDOCODEGO_STATE_HOME=/private/tmp/nandocodego-state.zExYvb go run ./cmd/nandocodego --model kimi-k2.6:cloud --print 'Respond with exactly: ok'`
- Output summary:
  `ok`

## What Was Still Verified

- A real installed cloud model can be selected non-interactively in `--print`.
- This run does not prove the credential-before-context gate because the environment already has working cloud access.

## Next Step Needed

Repeat the scenario in a credential-clean environment or with a dedicated test seam that simulates missing cloud credentials without mutating user state.

## Owner

Environment owner.
