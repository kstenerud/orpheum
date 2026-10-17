> **ABOUTME:** Principle §6: Untrusted code is sandboxed, or it does not run.

# §6 — Untrusted code is sandboxed, or it does not run

Plugins, scrapers and metadata providers run in a WASM sandbox with an explicit capability list
(D12). There is no tier of "trusted" third-party code.

**Bites when:** a plugin API would be so much simpler as an in-process interface.
