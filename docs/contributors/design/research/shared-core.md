> **ABOUTME:** Whether a native client could reuse the TypeScript shared SDK or must
> reimplement it — the question D28 left open. Answers it by first asking how much shared logic
> there actually is, which turns out to be the whole answer.

# Sharing Logic With a Native Client

Researched **2026-09-18**, closing the gap
[D28](../../decisions/working-notes.md) flagged: if the hybrid fallback is taken and the TV
client goes native, can it reuse the shared SDK through a bridge, or does it reimplement?

## 1. First decompose the question, because "the shared SDK" is not one thing

The instinct is to ask "how do we share TypeScript with Swift and Kotlin". The better first
question is **how much genuinely shareable logic exists**, and the answer deflates the problem
substantially.

| Part of the "shared SDK" | Shareable? | Why |
| --- | --- | --- |
| **API client** | **Doesn't need to be** — it is *generated* | It comes from the OpenAPI document ([D9](../../decisions/working-notes.md)). `openapi-generator` emits Kotlin, Kotlin-Multiplatform and Swift clients from the same spec. Regenerating is not porting. |
| **Capability probe** (codec support) | **Cannot be, by nature** | `MediaCodecList` on Android, `decodingInfo()` on web, VideoToolbox on Apple. Platform-specific by construction, so nothing is lost. |
| **Rendition negotiation** | **Mostly already server-side** | [D21](../../decisions/working-notes.md) made this a set intersection over ~11 named profiles, and **the server performs the intersection**. The client's share is "turn probe results into a prioritised list of names" — tens of lines. |
| **Language-preference resolution** | **Server-side** | [D24](../../decisions/working-notes.md)'s chain resolves on the server; the client displays the outcome. |
| **Playback queue / resume** | Yes, moderately | Real logic, moderate size. |
| **Offline download state machine** | **Yes — and this is the genuinely hard one** | Stateful, fiddly, interacts with [D8](../../decisions/working-notes-archive.md)'s change journal. The one part worth real effort to share. |

**Two conclusions follow, and the second is the clincher.**

First: the genuinely-shareable-and-hard portion is **the offline state machine and the queue**,
not "the SDK". Everything else generates, resolves on the server, or is platform-specific
anyway.

Second, and decisively for the hybrid case: **a TV does not go anywhere, so a TV client does
not need offline download at all.** The one piece of shared logic that would justify an exotic
sharing mechanism is a *phone* concern, and the hybrid fallback keeps phones on React Native.

So for the failure mode we actually expect — TV focus navigation disappointing — **a native TV
client reimplements a small amount of logic on top of a generated API client, and that is
fine.** This is not a lucky accident: [D9](../../decisions/working-notes.md)'s
generate-the-client rule and [D21](../../decisions/working-notes.md)'s closed-enumeration
negotiation are what made it small, and they were taken for other reasons.

## 2. The mechanisms, for the case where sharing *is* worth it

If React Native fails generally rather than only on TV, phones go native too, offline download
becomes a shared concern, and the question is real. Four mechanisms, one verified dead.

### Embedding a JavaScript engine — rejected

Run the existing TypeScript inside the native app via Hermes (which can be embedded standalone),
QuickJS, or JavaScriptCore.

Rejected: it **reintroduces exactly the bridge the migration was meant to escape**, adds a
runtime to ship and debug across, and makes stack traces worse. On iOS, JavaScriptCore outside
a `WKWebView` **cannot JIT** and runs interpreted. It is only justified when the shared logic is
large, and §1 establishes that it is not.

### A Go core via `gomobile` — verified dead for our case

Conceptually the most attractive option, because [D9](../../decisions/working-notes.md) already
makes the server Go and this would mean one language for the server and the client core.

**It does not support tvOS.** From `cmd/gomobile/env.go` in the current tree:

```go
var applePlatforms = []string{"ios", "iossimulator", "macos", "maccatalyst"}
```

No `tvos`, no `appletvos`. The option fails on **precisely the platform the fallback exists
for**, which settles it without needing to weigh the other trade-offs.

Worth recording that `gomobile` is otherwise in better health than its reputation suggests —
actively maintained through September 2026, only 37 open issues, with recent work on both the
Android NDK and Apple build paths. It is a reasonable choice for an Android+iOS core. It is not
a choice for us.

### A Rust core via UniFFI — the strongest technical option

Write the shared logic once in Rust; UniFFI generates bindings. Its coverage is remarkable and
is the reason it beats the alternatives on capability:

- **Swift** — which means iOS *and* tvOS, since tvOS is just another Apple target.
- **Kotlin**, including **Kotlin Multiplatform** bindings targeting both JVM and Native.
- **Wasm** via a generated `wasm-bindgen` crate — so **the web client is covered too**.
- **React Native Turbo Modules via Hermes bindings** — Mozilla and Filament shipped "UniFFI for
  React Native" for exactly this.

That last point is the interesting one: **a Rust core would serve the current React Native
architecture *and* every fallback**, which makes it the only option that is not a bet on which
way the client question resolves.

Maturity: used extensively by Mozilla in Firefox mobile and desktop; described by its own
maintainers as production-ready but "a long way from 1.0", with 292 open issues.

**The cost is the reason not to do it now:** Rust would be a **fourth language** (Go server,
TypeScript UI, Swift/Kotlin native modules, Rust core), against
[D9](../../decisions/working-notes.md)'s explicit one-language-on-the-server discipline and its
reasoning that every extra language is another toolchain, lint config, CI lane and set of
conventions. Paying that for an offline state machine we have not written yet, to hedge a
failure that may not happen, is speculative generality.

### A Kotlin Multiplatform core — [D28](../../decisions/working-notes.md)'s existing answer

Already designated as the general-purpose Plan B. Covers Android, Android TV, iOS and tvOS
natively; covers web only via Kotlin/JS or Kotlin/Wasm, which is the less-trodden path. Touchlab's
KaMPKit is the reference starter and is actively maintained.

Versus Rust/UniFFI: KMP has far larger adoption and a first-party vendor behind it, but UniFFI
has the cleaner web and React Native story. If the general failure happens, that trade needs
re-running with the actual code in hand.

## 3. Verdict

| Scenario | Answer |
| --- | --- |
| **TV focus prototype disappoints → native TV client** (the likely case) | **Reimplement.** Generate the API client from OpenAPI, port a few hundred lines of negotiation and queue logic, skip offline entirely because a TV does not need it. No bridge, no shared core, no fourth language. |
| **React Native fails generally → native everywhere** | The question becomes real, because offline download is then shared. Re-run **KMP vs Rust/UniFFI** at that point, with the offline state machine actually written so its size is known rather than guessed. |
| **Now** | **Do nothing.** No shared-core mechanism is justified by present requirements. |

## 4. What to do now: keep the decomposition true

The reason this answer is comfortable is the table in §1, and that table is a property of
current decisions rather than a law. It stays true only if:

1. **The OpenAPI document remains the source of the API client**, generated and CI-gated — never
   hand-maintained. The moment a hand-written client accumulates logic, porting cost appears.
2. **Negotiation stays a closed enumeration intersected server-side** (D21). If conditions creep
   into the menu, the client-side share grows and so does the reimplementation cost. This is the
   same no-grammar rule D21 already states, now with a second reason behind it.
3. **Offline logic is quarantined in its own module**, not spread through screens. It is the one
   genuinely portable asset, and it is only portable if it is separable.
4. **Anything that could live on the server does.** Every piece of logic moved server-side is a
   piece that needs porting zero times instead of three.

Rule 4 is the general form, and it is worth stating as the durable lesson: **the cheapest
cross-platform logic is logic that runs on the server.** That is available to us in a way it is
not available to most mobile apps, because we ship the server.
