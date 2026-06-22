---
name: write-tests
description: Write robust Go tests with table-driven style.
version: 1.0.0
author: nandocodego
tags: [tests, go]
---

Test-writing guide:
1. Prefer table-driven tests and subtests.
2. Keep unit tests deterministic and isolated.
3. Avoid live network and external process I/O in unit tests.
4. Validate error paths, boundary conditions, and cancellation.
5. Use clear expected values and failure messages.

