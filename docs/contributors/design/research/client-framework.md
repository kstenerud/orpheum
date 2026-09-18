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

## 5. Kotlin Multiplatform / Compose Multiplatform — considered, rejected

The obvious third path, and one a commenter asked Shopify about directly ("is there a shared
core between iOS and Android, e.g. KMP or Rust?"): share logic in Kotlin, write native UI, or
share UI via Compose Multiplatform.

Compose Multiplatform reached **stable on iOS in 2026**, so this is a genuine option now and was
not a year ago. It is rejected here for two specific reasons:

- **There is no tvOS target, and no coherent cross-platform TV story.** Compose for TV exists on
  Android; Compose Multiplatform does not extend that to Apple TV. It would solve the platforms
  we are least worried about and skip the one we are.
- **It does not help the web client, which we need regardless** — it is also the admin UI. Our
  shared SDK is TypeScript either way, so KMP would add a *third* language for partial coverage.

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
