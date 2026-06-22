# BLOCK-20260607-b008-unavailable-cloud-credential-path-not-safely-reproducible

## Scenario

`B-008` `--print` with unavailable credential fails non-interactively and clearly.

## Blocking Condition

The environment already has working access to the installed cloud model `kimi-k2.6:cloud`, and forcing an unavailable-credential state would require mutating user credential state.

## Category

- environment

## Evidence

- Command attempted:
  `env NANDOCODEGO_CONFIG_HOME=/private/tmp/nandocodego-config.VE8BGL NANDOCODEGO_DATA_HOME=/private/tmp/nandocodego-data.xcAJfP NANDOCODEGO_CACHE_HOME=/private/tmp/nandocodego-cache.2Ti8Oj NANDOCODEGO_STATE_HOME=/private/tmp/nandocodego-state.zExYvb go run ./cmd/nandocodego --model kimi-k2.6:cloud --print 'Respond with exactly: ok'`
- Output summary:
  `ok`

## What Was Still Verified

- The cloud-model `--print` path is non-interactive and succeeds with current credentials.
- The negative unavailable-credential branch was not exercised.

## Next Step Needed

Repeat in a clean environment without stored cloud credentials, or add a controllable auth-failure seam for the print path.

## Owner

Environment owner.
