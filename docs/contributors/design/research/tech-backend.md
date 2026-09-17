> **ABOUTME:** Language, runtime, framework and API-shape research for the server, weighed
> against the project's stated preferences — boring established tech, strong static typing, and
> strong linting so that agents can write quality code.

# Technology: Backend

## The brief

Two stated constraints drive this:

1. **Boring, established technology** unless there is a compelling reason otherwise.
2. **Strongly typed languages with good linting**, because they make it easier for agents to
   write quality software.

The second is worth unpacking, because it is a stronger constraint than it sounds. An agent
writing code benefits most from **fast, local, mechanical feedback**: a type error, a lint
failure, or a failing test that appears in seconds and says exactly what is wrong. That
favours: a fast compiler; a single canonical formatter; a single canonical linter with a
large rule set and a project-level config; a standard test runner; and, critically, **a small
language** — the fewer ways there are to express a thing, the less an agent (or a reviewer) has
to hold in its head, and the more of the codebase looks like the rest of the codebase.

## Candidates

### Go — recommended

**Boring:** yes, emphatically. Used by three of the competitors surveyed (Navidrome,
PhotoPrism, Stash) plus Kyoo's transcoder and auth services.

**Typing:** static, with generics since 1.18. Not as expressive as Rust's or TypeScript's; the
relevant question is whether it catches the errors that matter, and for a media server — where
the hard problems are process control, I/O and state, not type-level invariants — it does.
`nil` is the notable hole.

**Linting:** `golangci-lint` is the de-facto standard, runs dozens of linters in parallel under
one config, and is exactly the "fast mechanical feedback" the brief asks for. `gofmt` is not
configurable, which ends all formatting discussion permanently — a real advantage with agents.
The yoloAI template this project is based on ships a 38KB `.golangci.yml`, so a strict, proven
baseline is available to copy.

**Fit for this problem specifically:**

- **FFmpeg is a subprocess, and Go is very good at subprocesses.** `os/exec`, context
  cancellation, pipe handling and goroutine-per-stream are a natural fit for "spawn an encoder,
  stream its output, kill it when the client leaves". Cgo bindings to libav* exist but are a
  maintenance and build-complexity liability; every surveyed project that touches FFmpeg drives
  it as a **process**, and so should we.
- **Concurrency model matches the workload.** Many simultaneous long-lived streams, background
  scan jobs, watchers, and a shared transcode registry. Goroutines plus channels plus `context`
  is the right shape, and Kyoo's `RunLock`/`Keyframe` code is a readable demonstration.
- **Single static binary**, trivial cross-compilation to amd64/arm64 (Raspberry Pi and NAS
  boxes are a real deployment target), no runtime to install. This is worth a lot to
  self-hosters and is a direct advantage over .NET and Node.
- **Compile times in the 1–3s range for incremental builds** keep the agent feedback loop
  tight.

**Against:** the type system won't catch everything; error handling is verbose; the ecosystem
for image processing and ML is thinner than Python's (irrelevant if ML stays out-of-process).

### Rust — the serious alternative

**Typing and linting:** the best of any candidate. `clippy` and `rustfmt` ship with the
toolchain; the compiler eliminates whole categories of error that Go permits.

**Against, for this project:** incremental builds of 10–30s versus Go's 1–3s is a meaningful
tax on an agent-driven loop, repeated thousands of times. Async Rust adds real complexity to
exactly the code we write most (streaming I/O, subprocess supervision). The ecosystem for
media servers is thin — no comparable project to learn from. And it is the less boring choice.

**Verdict:** a defensible choice that would produce a more correct program more slowly. Not
recommended, but the gap is narrower than usual and this should be revisited if the transcode
core turns out to be the whole project.

### C# / .NET — what the incumbents use

**For:** genuinely strong typing, first-class analyzers (Jellyfin runs StyleCop, BannedApi and
IDisposable analyzers in-build), excellent tooling, and the largest body of directly relevant
prior art — Jellyfin and Kavita are both .NET and both readable.

**Against:** building the same thing in the same stack as the incumbent invites building the
same architecture. Runtime deployment is heavier than a static binary. And the strategic point
matters: we are trying to *not* be Jellyfin, and starting from its stack makes the
`DeviceProfile`-shaped hole in the ground very easy to fall into.

### TypeScript / Node — what Immich and Kyoo's API use

**For:** the best structural prior art in the survey is Immich's NestJS server, and it is
*good* — layered, tested, typed end-to-end with `kysely` and `zod`. One language across server
and web client is a genuine productivity win.

**Against:** `tsc --noEmit` is slower and weaker feedback than a compiler that produces the
artifact; runtime type safety needs `zod` bolted on at every boundary; the dependency graph is
enormous and churns (PhotoPrism's `package.json` comments are a warning); Node is a poor
host for supervising many long-lived FFmpeg processes compared to Go; deployment needs a
runtime. Audiobookshelf is the counter-example of what Node self-hosted software degrades into.

### Ruled out

Python (typing is optional and checked by a separate tool; wrong for the process-supervision
core — but the right language for an out-of-process ML service if we ever add one), PHP
(Nextcloud's constraint is a warning, not a model), Java/Kotlin on the server (heavier
deployment for no gain here; Kotlin remains the right answer for an Android client).

## Recommendation

**Go, with `golangci-lint` in strict mode, as the single server language.**

A deliberate secondary rule: **one language on the server.** Kyoo's five-language, two-package-
manager monorepo is the thing not to copy. Every additional language is another toolchain,
another lint config, another CI lane, another set of conventions for an agent to get right.
The only sanctioned exception is an ML service, which would be Python, out-of-process, behind
an HTTP interface, and optional — the shape Immich uses — and which is not in the first year's
scope anyway.

## Framework and library stance

Boring and minimal. The pattern across the successful Go competitors is "standard library plus
a router":

| Concern | Choice | Notes |
| --- | --- | --- |
| HTTP router | `net/http` + `go-chi/chi` | Navidrome's choice. `stdlib` routing since 1.22 covers much of it; chi adds middleware ergonomics. Kyoo uses Echo; either is fine. Avoid anything that owns your application structure. |
| Config | Environment + a file, parsed once into a typed struct | Navidrome's `conf/configuration.go` is 1,208 lines — a warning about letting config sprawl. |
| Logging | `log/slog` (stdlib), structured, with a **dedicated scan channel** | Directly addressing the "scanner logs are unreadable" complaint in [sentiment.md](sentiment.md). |
| Jobs | An in-process typed job queue persisted to the database | No Redis, no external broker. Immich's BullMQ model is right; its infrastructure is more than we need. Jobs must be listable, cancellable, retryable and *inspectable* — Immich's `Better failed job management` request is the lesson. |
| DI | Constructor injection, wired by hand or with `wire` | Navidrome uses `google/wire` (compile-time). Avoid runtime reflection containers. |
| API schema | **OpenAPI, generated from the code, as a build artifact** | Clients are generated from it. This is the contract between server and every client, and it must never be hand-maintained. |
| Auth | OIDC from day one, plus local accounts | Kyoo does this; Navidrome's top-five requests are LDAP (+148) and SSO (+64). Retrofitting auth is painful. |
| Telemetry | Off by default, opt-in, local-only | See [sentiment.md](sentiment.md) §"Don't put a business between me and my files". |

## API shape

**REST + OpenAPI**, not GraphQL. Stash demonstrates GraphQL works here, but REST wins on: TV
platform HTTP clients, cacheability of media and image endpoints, debuggability with `curl`,
and the fact that our highest-traffic endpoints are byte ranges and segments, which are not
GraphQL-shaped. Generate the OpenAPI document from the handlers, generate client SDKs from the
document, and gate the build on the document being current.

**Additionally: consider serving a Jellyfin-compatible API surface.** Navidrome implements both
Subsonic *and* a Jellyfin-compatible API (`server/jellyfin/` — items, browsing, playlists,
sessions, images, lyrics, streaming, websocket) purely to inherit other people's clients. For
a new video server facing a client-ecosystem cold start, a compatibility shim over our own
model may be the cheapest possible path to "works with Findroid/Swiftfin/Infuse on day one".
This deserves its own plan and its own decision; it is a strategy, not a footnote.
