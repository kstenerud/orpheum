> **ABOUTME:** A re-examination of the React Native decision against 2026 evidence — including
> Shopify's reversal back to native — with a correction to the evidence base originally cited
> for it, and the measured share ratio from a production client in this exact domain.

# Client Framework: Re-examining React Native

Researched **2026-09-18**, as a deliberate challenge to
[D10](../../decisions/working-notes.md) rather than a confirmation of it.

## 0. First, a correction to what D10 claimed

D10 said the React-Native-shell-plus-native-player pattern was validated because "both Kyoo and
Streamyfin independently arrived at this shape". Two errors:

- **Kyoo has not shipped a TV client.** Its README lists Android TV as "planned, hopefully
  soon". It validates the pattern for *phone and web*, not for television — which is the case
  we were using it to support. The independent evidence for RN-on-TV in this domain is
  **one project, not two.**
- **The star count cited for Streamyfin (302) was the wrong repository** — that is its
  companion Jellyfin *plugin*. The application is **5,197 stars**.

So the evidence base is narrower in count than stated and stronger in weight. Both corrections
matter; the second one matters more.

## 1. The counter-signal: Shopify reversed, and its reasoning is uncomfortably close to ours

**"Shopify is moving from React Native back to Swift and Kotlin"** — Hacker News, 2026-09-10,
1,272 points, 958 comments. Shopify was React Native's flagship enterprise case study after
going all-in in 2020, so this is the strongest available negative signal.

Their stated reason, from a Shopify engineer in the thread:

> "We don't hold on to a decision just because it was successful at the time. When a core
> assumption changes, we're willing to go back and ask whether it's still the right call. **LLMs
> changed one of the core assumptions behind our 2020 decision**, so we reevaluated our mobile
> stack from first principles. What we found led us back to native."

And the clearest formulation of the argument, from the thread:

> "React Native always incurred a bit of a performance & UX penalty, but many people decided
> that this tradeoff was worth it for the increase in product development velocity. **LLMs
> change the tradeoff** and organizations would be remiss to not reconsider previous decisions."

**This is uncomfortable for us specifically**, because "we can't afford to write it N times" is
exactly the constraint that agent-written code relaxes — and this project's entire premise is
that agents write the code.

## 2. Why the Shopify argument applies *less* to us than it does to Shopify

Three reasons, two of them from the top of that same thread.

**It is a resourcing argument, and we are on the other side of it.** From the thread's own
analysis: *"I think it'll become a trend at larger, well-capitalized companies and startups, but
not universally. The article mentions hundreds of engineers have worked on Shopify's RN apps, so
they're well positioned to maintain two native codebases with agents and practically infinite
token spend. **Migration will be a harder sell for resource-constrained businesses.**"* And:
*"For companies the size of Shopify and Airbnb that makes sense. But that doesn't mean a small
ten person startup should do the same thing."*

**The divergence-cost argument cuts against us going native, not for it.** The thread's
highest-signal objection:

> "The hard part is keeping them the same. Feature parity isn't an implementation cost. **It's a
> divergence cost.** It accrues over years across experiments, analytics, accessibility, edge
> cases, bug fixes, platform behavior, and a thousand little decisions which current agents
> aren't great at tracking. Agents can write code…"

Agents generating two implementations that look alike today is not the same as them staying
alike. **Fewer maintainers means fewer people to notice drift**, so a small project carries
*more* divergence risk from parallel codebases than Shopify does, not less.

**It is eight days old and unproven.** It is a migration plan with one simpler app done, not a
shipped result. *"Wait until the new apps have rolled out and users are happy… big rewrites like
this have a big enough chance of going off the rails that I wouldn't shout success from the
rooftops just yet."*

## 3. Domain evidence beats general evidence, and in this domain native has not won

General mobile-development sentiment is not the same as evidence about *media clients*. Within
the Jellyfin client ecosystem — the closest possible comparison set — the picture is the
opposite of the industry narrative:

| Client | Stack | Created | Stars | TV status |
| --- | --- | --- | --- | --- |
| **Streamyfin** | **React Native / Expo** | 2024-07 | **5,197** | Ships TV builds (`EXPO_TV=1`), with TV-specific native modules |
| Findroid | Native Kotlin | 2021-05 | 4,255 | **"Not optimized for TV use yet"**; under rewrite |
| Swiftfin | Native Swift | 2021-05 | 4,171 | tvOS works |
| Jellyfin Android TV | Native Kotlin/Java | — | — | Top issue is **"The Playback Rewrite", open since 2021-07** (+214) |

The React Native client is **two and a half years younger than both native clients and has more
stars than either**. Meanwhile both *native* Android clients have unresolved TV problems. Native
is not a shortcut to a good TV app in this domain; it is a different set of problems.

## 4. "You end up dropping into native anyway" — measured, not argued

The sharpest technical objection in the Shopify thread: *"using React Native you end up having
to drop into native code to do anything interesting or optimized, so it feels kind of pointless
to not just use the platforms directly."*

That objection lands harder on a media client than on a commerce app, because our player, codec
probing, PiP, media session, HDR passthrough and background download are all native by design
([D10](../../decisions/working-notes.md)). So: how much is actually left to share?

**Measured from the Streamyfin tree:**

| | Lines |
| --- | --- |
| Native (Swift + Kotlin, in `modules/`) | **33,097** |
| TypeScript (app + components) | **73,950** |
| Screens | 79 |

**Roughly 31% native, 69% shared.** Two conclusions, and they pull in opposite directions:

- The objection is **wrong for this domain as a blanket claim** — over two-thirds of a shipping
  media client is still shared across phone, TV, web and both operating systems.
- But a third being native is **far above what a CRUD app would carry**, and it confirms that a
  media client is a native-heavy case. Anyone choosing this stack should budget for writing and
  maintaining real Swift and Kotlin, not for writing JavaScript.

Also worth noting: 79 screens. A media client is not "a grid and a player" — the surface that
benefits from sharing is large.

## 5. The alternatives, and which is the designated Plan B

Dropping React Native does **not** imply going fully native. Three families exist, and an
earlier version of this document conflated the first two, which understated the best of them.

### KMP ≠ CMP, and the distinction is the whole point

- **KMP (Kotlin Multiplatform)** — share *logic* (networking, models, state, offline), write
  **native UI on every platform**. Conservative.
- **CMP (Compose Multiplatform)** — also share the *UI*, rendered via Skia onto a canvas.
  Newer, more ambitious.

They have very different adoption and very different risk.

### KMP's adoption is real, and this is the Plan B

- Usage among professional developers **roughly doubled in a year, 7% → 18%**.
- Production users include **Cash App, Forbes** (reported 80%+ of business logic shared),
  **McDonald's, Quizlet**, and **Block's Bitkey** (reported ~95% shared, with CMP for UI).
  **Google and JetBrains are both invested** — Google being the vendor of one of the two mobile
  platforms is the part that matters.
- Kotlin 2.x retired the historical objections: the memory model, build setup and compiler
  flakiness.

**One widely-repeated claim needs qualifying**, because it is the one that looks most relevant
to us and is not: **Netflix's KMP usage is in its *Studio* applications** — internal production
tooling — not in the consumer streaming client. It is not evidence that KMP is used for
consumer media playback at scale. Treat "hundreds of millions of daily users" figures as vendor
framing.

### CMP is not a fit for us, for one hard reason

**No tvOS target** — and [tvos.md](tvos.md) explains why that is structural rather than a
backlog item: a Skia-canvas renderer presents no focusable UIKit views, so tvOS's Focus Engine
has nothing to traverse. Expect the same of any future canvas-based framework. Compose for TV exists on Android, and Compose Multiplatform does not extend
it to Apple TV. CMP would solve the platforms we worry about least and skip the one we worry
about most. Web is also Wasm-alpha-ish. That is disqualifying for this project specifically —
not a judgement on CMP generally, which is clearly maturing fast.

### Flutter is worse than React Native for our target set

Verified from Flutter's own tracker and LG's developer site:

| Target | Flutter status |
| --- | --- |
| **tvOS** | **No official support** — community efforts only; the request issue has been open since 2019 |
| Android TV | Unofficial. Flutter's own umbrella issue lists gaps in "tooling, I/O, navigation, focus, core design language, documentation, samples, and tests" |
| **webOS** | **Officially supported** — LG ships a `flutter-webOS` SDK, which is genuinely new and interesting |
| Phones | Excellent |

Losing tvOS entirely and having Android TV documented by its own maintainers as gap-ridden makes
Flutter a worse choice than React Native here, despite being the more popular framework overall.

### Lightning.js is a complement, not an alternative

Comcast's WebGL/JavaScript TV framework, used across RDK set-tops and smart TVs, explicitly
targeting memory-constrained legacy devices. It is **TV-only and canvas-based** — no DOM, no
mobile story — so it cannot replace a multiplatform stack. It is, however, the strongest option
if the v3 Tizen/webOS targets are ever built, precisely because it is designed for the weak
hardware those platforms run.

### So the fallback depends on *how* React Native fails

| Failure mode | Fallback | Cost |
| --- | --- | --- |
| **TV only** (the likely one — focus navigation or performance on weak sticks) | **Hybrid**: keep RN for web and phone, native for TV (Compose for TV + SwiftUI) | Two TV UIs — but a TV UI is rows, a grid, a detail page and a player. Risk stays where it lives. |
| **Generally** (fork stalls, ecosystem decays) | **KMP for shared logic + native UI everywhere** | Real work, but a proven, well-adopted path. Web still needs its own client — unless Kotlin/JS or Kotlin/Wasm is used for its logic layer, which is possible and less-trodden. |
| We want maximum sharing and can drop Apple TV | CMP | Not acceptable — tvOS is a target. |

The **hybrid** is the underrated answer. React Native's weak spot is exactly and only TV; its
strong spots are phone and web. Isolating the fallback to the platform that failed is cheaper
than a wholesale migration, and nothing in the architecture forbids it.

## 5a. The structural point: the expensive asset is already framework-independent

The most reassuring fact about this whole question, and the reason it is less alarming than it
sounds.

**The native player modules survive any of these migrations.** Streamyfin's 33,097 lines of
Swift and Kotlin are not React Native code — they are `MPVLayerRenderer`, `PiPController`,
`MPVNowPlayingManager`, ExoPlayer and libmpv integration, wrapped in a *thin* Expo module
bridge. Moving to native, or to KMP, means dropping the wrapper and keeping the implementation.
**The most expensive part of a media client is the part that does not care which UI framework
sits above it.**

Add to that what the architecture already makes durable: the API contract
([D9](../../decisions/working-notes.md)'s generated OpenAPI), the closed rendition menu
([D21](../../decisions/working-notes.md)) and the probe-and-pick logic
([D2](../../decisions/working-notes.md)) — a small, well-specified interface rather than a
sprawl.

So a framework switch is **a UI rewrite, not a system rewrite**. Screens and navigation are
rewritten; the player, the protocol, the negotiation and the server are untouched.

### Keeping the exit cheap costs nothing now

Three rules, adopted as [D28](../../decisions/working-notes.md), that preserve this property
rather than assuming it:

1. **The native player modules stay free of React Native types at their core.** Player engine,
   renderer, PiP and media-session code are plain Swift/Kotlin; only a thin adapter layer knows
   about RN. This is roughly how Streamyfin is already structured — the point is to make it a
   rule rather than an accident.
2. **The shared SDK stays framework-agnostic TypeScript** — no React, no React Native imports in
   the API client, negotiation or offline logic. It should be usable from a plain web page, and
   the device-lab web harness is the forcing function that proves it.
3. **Measure the native fraction.** Streamyfin sits at 31%; passing roughly half is one of D10's
   bail-out triggers, and it is only observable if it is tracked.

## 6. `react-native-tvos` is healthier than "community fork" suggested

D10 recorded the fork as a risk. Checked, 2026-09-18:

- Releases track upstream closely — `v0.87.1-1` published **today**, `v0.86.3-0` yesterday,
  following upstream React Native 0.86/0.87 within weeks.
- **15 open issues.**
- The recent commit log is dominated by *upstream React Native core contributors*, because it is
  a thin tracking fork rather than a divergent rewrite.

The residual risk is real but is better characterised as "a thin fork with a small maintainer
team, tracking upstream promptly" than as "an abandoned community project". Caveat in the other
direction: a low issue count is also consistent with few users, so it is not proof of health on
its own.

## 7. Conclusion

**D10 stands, with the evidence restated honestly.** The Shopify reversal is the strongest
argument against it in years, and it is a *resourcing* argument that points the other way for a
project of this size — with the divergence-cost objection cutting specifically against a small
team maintaining parallel native codebases. In this domain, the React Native client is
outperforming the native ones by adoption, and the native Android clients have the worse TV
stories.

**What has not changed is where the actual risk sits: TV focus navigation.** Not performance,
not the fork, not code sharing. [`tv-client.md`](../plans/tv-client.md) already says to build a
throwaway focus-navigation prototype before any real screens, and nothing here alters that.
What is added is a **bail-out criterion**, so the prototype is a decision point rather than a
formality.

### What would change our mind

Stated now, so the reversal is falsifiable rather than a matter of taste:

1. **The focus prototype feels wrong on the oldest Fire Stick** — dropped frames on directional
   navigation, focus traps, or latency that a native app would not have. This is the most likely
   failure and it is cheap to discover.
2. **`react-native-tvos` falls materially behind upstream** — say, two minor React Native
   versions for more than a quarter.
3. **The native fraction in our own tree passes roughly half.** Streamyfin's 31% is tolerable;
   at 50%+ we are maintaining three codebases while paying for a fourth abstraction.
4. **Shopify's migration is reported as a success with numbers, in a year**, and someone
   demonstrates the divergence cost is genuinely managed by agents rather than deferred.
