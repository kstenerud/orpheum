> **ABOUTME:** Why tvOS is the platform that breaks every cross-platform option, what actually
> works there, who skips it — and the measurement that reframes the problem: tvOS is expensive
> to reach from outside the Apple stack and nearly free from inside it.

# tvOS: Why It Breaks Everything, and What Doesn't Break

Researched **2026-09-18**, prompted by noticing that tvOS is the consistent failure point across
every client-technology decision in this tree.

## 1. The observation is correct

Every cross-platform option evaluated so far failed on the same platform:

| Option | Verdict | Reason |
| --- | --- | --- |
| Compose Multiplatform | Rejected | **No tvOS target** |
| Flutter | Rejected | **No official tvOS support**; request open since 2019 |
| Go core via `gomobile` | Verified dead | `applePlatforms` has no `tvos` |
| Lightning.js | TV-only, unusable here | Web-based — and tvOS has no web runtime (§2) |
| React Native | **Works** — via a *fork*, not first-party | See §4 |
| KMP | Works, at a lower support tier | See §4 |

One platform, the same answer, six times. That is a structural fact, not a coincidence.

## 2. Why it is hard: three structural reasons, not one

### (a) There is no web runtime at all

**tvOS has no WebKit.** No `WKWebView`, no browser, no embedded web view of any kind. The
classes to *fetch* a page exist; nothing to parse, build a DOM, render, style or run JavaScript
does.

Apple's reasons, as generally reported, are deliberate: control of the user experience,
discouraging apps that are thin wrappers around web content, and reducing attack surface on a
living-room appliance.

**This single fact kills an entire family of approaches** — Capacitor, Ionic, Lightning.js,
Tizen/webOS-style HTML apps, and the Jellyfin-style "wrap the existing web UI in a shell".

And it creates an asymmetry worth internalising: **on every other TV platform, "ship the web UI
in a webview" is a viable panic button. On tvOS it does not exist.** Tizen and webOS apps *are*
web apps. Android TV has WebView. Only Apple removed the escape hatch.

### (b) The Focus Engine needs real UIKit views, and canvas renderers have none

tvOS navigation is the UIKit **Focus Engine**: the system moves focus between focusable views
in response to the remote. It is not an input-event model an app interprets; it is a
platform-owned traversal over the real view hierarchy.

**A canvas-rendering framework paints pixels and therefore presents no focusable views.**
Flutter and Compose Multiplatform both draw through Skia into a single surface. From the Focus
Engine's perspective there is one view and nothing to traverse, so the platform's entire
navigation model has nothing to attach to. Reimplementing focus inside the canvas means
reimplementing a platform behaviour users have deep expectations about.

**This is why React Native works on tvOS and the canvas frameworks do not.** RN maps its
components to real UIKit views, so the Focus Engine works on them natively. Verified in the
`react-native-tvos` tree: UIFocus handling lives in the Fabric component views
(`RCTViewComponentView`, `RCTRootComponentView`, `RCTEnhancedScrollView`), alongside
`TVFocusEventHandler`, `TVEventHandler` and `TVViewConfig`.

This is the cleanest predictor available: **if a framework renders to its own canvas, expect
tvOS to be absent or bad. If it maps to platform views, expect it to work.**

### (c) It is a whole new engine port, for 8% of the market

Flutter's own maintainers, on the tvOS issue, list what it would take: "an entirely new embedder
in the engine, as well as test harnesses", plus "code changes in the buildroot, and some changes
to Dart and Skia". They also note a prerequisite that is pure tvOS friction — **wireless
debugging must land first, because tvOS has no USB tether.** The issue is labelled **P3**,
explicitly not a roadmap priority.

The economics behind that P3: Apple's tvOS is around **8% of US primary streaming viewing**,
tied with Samsung Tizen for fourth, against Roku ~37%, Fire TV ~17% and Google TV ~14%.

### (d) Apple closed its own escape hatch

Worth knowing, because it forecloses an obvious idea. **TVMLKit** — Apple's own framework for
writing TV apps in JavaScript and XML templates — was **deprecated at WWDC 2024**. It remains
present but is explicitly not growing with the platform, and Apple's guidance is to migrate to
SwiftUI.

Apple tried the "write your TV app in a scripting language" path itself, and retired it. Nobody
should expect a blessed non-native route to appear.

## 3. Yes, companies simply skip it

**Spotify is the clearest case.** In 2016 a Spotify product owner said tvOS had been
"down prioritized" and "you should not expect a release supporting it any time soon". Spotify
shipped a proper Apple TV app in **October 2025** — roughly nine years later. A company of that
size treated tvOS as optional for the better part of a decade and suffered no visible
consequence.

In the self-hosted field the pattern is stronger still: **Immich has no TV client at all**, and
**Kyoo has none**. Among the video servers, only the incumbents with long histories (Plex, Emby,
Jellyfin) ship one — and Jellyfin's is a separate, community-driven native project (Swiftfin).

Skipping tvOS is a normal, defensible decision, and it is the majority position outside the
large streamers.

## 4. What actually works — and the measurement that reframes everything

### SwiftUI is the answer, and real clients confirm it

Apple's stated direction post-TVMLKit is SwiftUI, and the best client in this domain agrees.
**Swiftfin** — the Jellyfin Apple client, 4,171 stars — imports SwiftUI in **456 files** against
UIKit in **39**, with 54 focus-engine touchpoints where the platform demands it.

### The finding: tvOS is ~3% of an Apple client, not a second app

Measured from the Swiftfin tree:

| Target | Lines | Files | Views |
| --- | ---: | ---: | ---: |
| **`Shared/`** | **59,018** | 604 | 89 |
| `Swiftfin` (iOS) | 12,361 | 116 | 93 |
| **`Swiftfin tvOS`** | **2,382** | **22** | **5** |

**About 80% of the codebase is shared between iOS and tvOS, and the tvOS-specific portion is
roughly 3.2%.** Five tvOS-only views.

This reframes the entire problem:

> **tvOS is expensive to reach from outside the Apple stack, and nearly free from inside it.**

The difficulty everyone runs into is not a property of tvOS as a *platform*; it is a property of
trying to reach it from a non-Apple toolchain. Same language, same SwiftUI, same frameworks,
mostly the same views — a different focus model and a handful of TV-specific screens.

### The other things that work

- **`react-native-tvos`** — because it maps to UIKit views (§2b). First-class tvOS support with
  real focus bridging, but a fork rather than upstream React Native.
- **KMP** — Kotlin/Native does provide `tvosArm64` / `tvosX64` targets, so shared logic compiles
  for tvOS. Qualify it though: tvOS sits **below iOS's tier 1**, and sources conflict between
  tier 2 and tier 3, the latter described as untested on CI with compatibility not tracked. It
  works; it is not first-class.

## 5. What this means for us

**It does not overturn [D10](../../decisions/working-notes.md) or
[D28](../../decisions/working-notes.md) — it re-prices the fallback, favourably.**

1. **The hybrid fallback is cheaper than assumed.** If React Native's TV story disappoints on
   Apple, going native there buys **iOS *and* tvOS for about the cost of iOS plus 3%**. The
   escape is unusually affordable, which makes D10 a safer bet than it looked.
2. **The real question is not "which framework reaches tvOS" but "do we ever write Swift?"** If
   the answer is yes for iOS, tvOS is nearly free. If it is no, tvOS is a wall. Framing it that
   way makes the decision tractable.
3. **Never plan a tvOS story that depends on web technology.** No WebKit means the
   ship-the-web-UI panic button that exists on *every other platform* — including Tizen, webOS
   and Android TV — is absent here. Any plan whose tvOS contingency is "fall back to the web
   client" is already broken.
4. **Use the canvas-versus-views test on any future framework.** It predicted all six outcomes
   in §1 and costs nothing to apply.
5. **Skipping tvOS remains legitimate**, and Spotify's nine years plus Immich's and Kyoo's
   absence are the precedent. But note the qualitative counter-signal for *our* audience: the
   most-praised client in the sentiment research is **Infuse, which is Apple-only**, and HN
   threads repeatedly pair Apple TV with it. The 8% market figure probably understates this
   segment.

## Position taken, 2026-09-19

[D67](../../decisions/working-notes.md): **tvOS never gates a feature.** Where it cannot do
something, it does without. The temptation at each obstacle catalogued above is to design around
the weakest platform, and the cost of that is paid by every other client and every future
feature.

Watch Together supplied a fresh example: `react-native-webrtc` has no data channels on tvOS,
needs a locally built framework, and carries an open tvOS 16 issue — and an Apple TV has no
app-accessible microphone at all.

This is safe rather than reckless because the escape hatch already exists.
[D18](../../decisions/working-notes.md)'s Jellyfin-compatible adapter means Swiftfin and Infuse
work against our server today, so an Apple TV user gets a good third-party client with a reduced
feature set. If the list of exceptions keeps growing, **dropping first-party tvOS support is on
the table and is the adapter doing its job**. The trigger to watch for: a tvOS workaround
changing a design that would otherwise be simpler.
