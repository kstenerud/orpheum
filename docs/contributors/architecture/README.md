> **ABOUTME:** Where to find how the code is put together: the package map, each package's own
> invariants, and the shape the design intended.

# Architecture

How the code is actually put together lives beside it rather than here:

- **[`../design/architecture-map.md`](../design/architecture-map.md):** which package does which
  job, where each kind of data lives and whether it is backed up, and the seams between core,
  node, store and client. A check fails when a package has no row in it.
- **Each package's `doc.go` (Go) or `README.md` (web):** what it owns, its invariants and the
  test that keeps each one true, and the decisions to read before changing it.
- **[`../../../AGENTS.md`](../../../AGENTS.md) §Where things are:** the top-level layout.
- **[`../design/overview.md`](../design/overview.md) §Architecture:** the intended shape the code
  was built to.
