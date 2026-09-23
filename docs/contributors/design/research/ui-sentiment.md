> **ABOUTME:** What users actually say about media-server interfaces — measured from Plex's own
> forum and the issue trackers — plus the mods, skins and client replacements people build to
> escape them, and what they reveal about which UI properties matter.

# UI sentiment

Researched **2026-09-21**. Sources: `forums.plex.tv` via its Discourse JSON API, the GitHub
search API sorted by reactions across four Jellyfin repositories, GitHub repository search for
theming projects, and Hacker News via Algolia. Reddit remains unreachable
([sentiment.md](sentiment.md) §Method).

## 0. The headline: UI is the majority of the conversation

Of the **9,013 likes** across the fifty most-liked Plex forum topics of the past year,
**4,557 — 51% — are on threads about the interface**: navigation, the new experience, rollback
requests, lost controls, and one unwanted social feature.

| Likes | Posts | Thread |
| --- | --- | --- |
| **1,124** | 304 | *Vote to roll back to Plex Classic!* |
| 763 | 212 | *Comments on: Plex Roku Preview: A Faster, Simpler Navigation Experience* |
| 441 | 128 | *"Discussions" is a nightmare* |
| 326 | 106 | *How do I get the old experience back?* |
| 302 | 126 | *Plex has updated the Plex Roku Preview…* |
| 252 | 199 | *A New Experimental Plex Front End* |
| 205 | 108 | *I remain unhappy with "plex vista"* |
| 176 | 73 | *Fire TV app — Hopelessly Crippled* |
| 153 | 89 | *iOS App 2026.14.0 has no on-screen controls* |
| 136 | 36 | *What is going on with Plex? This interface is terrible!* |
| 134 | 34 | *The new Interface is HORRIBLE!* |
| 110 | 41 | *New UI for Fire Stick 4K June 2026* |
| 78 | 43 | *Workaround: Roll back the Fire TV Plex app to version 10.30.9.4262* |
| 70 | 20 | *Hate the new UI …so so much* |

For comparison, the price thread — *Elephant in the room. Lifetime Plex Pass* — is 938.
**The interface out-argues the money.**

> **Corrected 2026-09-23** ([ui-competitors.md](ui-competitors.md) §0): this sample's one-year
> window missed the four largest "new experience" threads, created just before it — about
> **20,400 likes** between them, the largest 6,482. The conclusion holds and is understated.

## 1. What they hate, specifically — and it is not aesthetics

Nobody in this corpus says "it's ugly". Coded across ten threads, the complaint density runs:
**remote/focus behaviour (45 mentions) > rollback requests (29) > lost features (25) ≈ missing
customisation (25) > click depth (23) > sluggishness (12)**. Verbatim:

- **Clicks to target.** *"…without having to hit up 1,000 times to get back to the menu or to be
  able to switch between library and collections with one click instead of 30."*
- **Information architecture moved for no gain.** *"30+ years of UI layout has categories on the
  left. The categories are still a drop down, now a nested drop down."*
- **User-arranged order taken away.** *"I prefer my own library folders/names on the sidebar as
  before. I could arrange the order. Your categories are not arrangeable as the sidebar
  previously."*
- **Focus latency on a television.** *"4k Firestick. It's so slow and unresponsive. Scrolling
  between titles and the 'selection' lags by an entire second."*
- **Controls that shrank or vanished.** *"Can't find subtitles anymore since the menu pops up is
  only like 100 pixels and off to the side. Can't easily toggle between streams."* And an entire
  153-like thread because an iOS release shipped **with no on-screen playback controls at all**.
- **Their own library demoted below content they do not own.** *"you seemed to have prioritized
  it for streaming and your live and streaming options. PLEX is/was an escape from
  commercials."*

**The lesson is narrow and useful: these are complaints about *information architecture* and
*input latency*, not about visual design.** A beautiful client with a nested dropdown and a
one-second focus delay would receive this same thread.

## 2. The unwanted-feature case, which has its own rule in it

*"Discussions"* — a social layer — drew 441 likes of objection. *"who asked for this? why would
i want this? why would anybody want this?"*

The sharpest comment is not about the feature but about the **scope of its off switch**:

> *"This should not be a client side setting, buried in some submenu forcing you to find it to
> disable. It should be available via … your account settings."*

A preference that is conceptually about **a person** was implemented as a setting on **a
device**, so opting out had to be repeated per client and was hidden. That is a scoping bug
with a UI symptom, and it is the failure our
[D24](../../decisions/working-notes.md) preference axes and
[D103](../../decisions/working-notes.md)'s device/person distinction exist to prevent.

## 3. The mods — and the biggest one is not a skin

This is where the evidence is strongest, because building a mod costs more than posting.

### PM4K / PlexMod for Kodi — the field's most significant workaround

| | |
| --- | --- |
| Thread | *PM4K / PlexMod for Kodi (18, 19, 20, 21, 22+)* |
| **Likes** | **5,312** |
| Posts | **6,494** |
| Views | **203,921** |
| Running since | **2019-10-25** |

**It has 4.7× the likes of the "roll back to Plex Classic" thread** and is the largest thing on
that forum. It is not a theme — it is **a complete replacement client**, Plex's backend driven
through Kodi's interface.

Its own feature list is headed **"Features (vs. official)"**, and the first two entries are the
indictment:

> **Maintained. Very stable.** AV1 support … **Directplay anything** (as long as your setup and
> Kodi supports it) … **DirectStream support for every codec, not just AVC/MP3** … Passthrough
> audio enhancements (e.g. prefer or force AC3/DTS depending on channel config) … **Correctly
> identify and display video/audio streams** (Atmos, DV, etc.)

A community mod's headline advantages over the first-party client are *that it is maintained
and that it is stable*, followed by **direct play, codec breadth and correct stream
identification** — which is, item for item, the thesis of
[D2](../../decisions/working-notes.md), [D4](../../decisions/working-notes.md) and
[D17](../../decisions/working-notes.md).

There is even **a mod of the mod**: `jpalenz77/pm4kvertical`, *"Plex Mod Vertical UI — Kodi
addon for vertical home layout"*. People are re-laying-out the replacement client's home
screen.

### Jellyfin's theme ecosystem: ~5,800 stars of CSS

| Stars | Project | What it is |
| --- | --- | --- |
| 2,163 | `lscambo13/ElegantFin` | *"A Jellyfin theme inspired from Jellyseerr"* |
| 1,026 | `prayag17/JellySkin` | minimal CSS theme with custom icons |
| 994 | `CTalvio/Ultrachromic` | *"the true evolution of the chromic theme saga"* |
| **660** | **`danieladov/jellyfin-plugin-skin-manager`** | **a plugin whose only job is installing other people's CSS** |
| 460 | `tedhinklater/finimalism` | theme |
| 207 | `MakD/Jellyfin-Media-Bar` | *"a Featured Content Bar to show off your media collection"* |
| 155 / 63 / 61 / 26 / 25 | Monochromic, better-jellyfin-ui, Kaleidochromic, Medusa, jellygray | themes |

Roughly **5,840 stars on projects that exist only to change how Jellyfin looks** — the same
order as the ~7,000 stars of rules-engine demand [D75](../../decisions/working-notes.md) was
argued from.

Three things follow. **The skin-manager plugin is the tell**: when a community builds a package
manager for workarounds, the seam underneath is missing rather than merely rough.
**`Jellyfin-Media-Bar` is a feature request in CSS form** — people are adding a hero row the
product does not have. And **ElegantFin is "inspired from Jellyseerr"**: the *request* tool is
the design people want their *media server* to look like.

### The Plex front-end attempt, and the honest size of it

*A New Experimental Plex Front End* drew **252 likes** — but the project it announces,
`chrisjdavis/Genus-Public`, has **3 stars**. **The demand is real and the replacement is not.**
Worth recording precisely, because the thread alone would support a much stronger claim than
the evidence does.

### And rollback, which is the universal workaround

A dedicated thread — *Workaround: Roll back the Fire TV Plex app to version 10.30.9.4262*, 78
likes — plus requests throughout the others: *"Is there any way to uninstall the current
version and reinstall the previous version on an Amazon Fire TV Cube?"* This is exactly what
[D108](../../decisions/working-notes.md) already promises the opposite of.

## 4. What they like, and the most instructive case is in-house

**Plexamp**, Plex's music client, is the same company's beloved app: *"the UI is probably the
best music player I have ever used and is buttery smooth and looks great"*, *"nicely polished,
with loads of configuration options"*. Two of the top-50 threads are Plexamp release notes,
warmly received, in a list otherwise full of anger.

The difference the reviewers name is **focus**: Plexamp is *"a dedicated Plex music player"*
while the main app is a general entertainment platform. One team, one codebase family, two
outcomes — separated by whether the app tries to be one thing.

That is [D10](../../decisions/working-notes.md)'s *"one excellent TV client beats five mediocre
ones"* with a same-company control group, and it supports
[D1](../../decisions/working-notes.md)'s scope discipline from an angle the original argument
did not use.

## 5. The concrete feature asks, by reaction count

From the Jellyfin trackers — these are the UI requests people actually vote for:

| Reactions | Repo | Request |
| --- | --- | --- |
| **214** | androidtv | **The Playback Rewrite** |
| 56 | androidtv | PIN protection for users |
| 49 | androidtv | Force "direct play" in client |
| 37 | androidtv | Video Backdrop and Theme song playback |
| 33 | androidtv | **Dual Subtitles** |
| 32 | androidtv | **Audio/subtitle sync adjustment menu in the player** |
| **30** | androidtv | **List view** |
| **28** | androidtv | **Show titles in poster view** |
| 28 | androidtv | Play via external player from the item screen |
| 18 | androidtv | **Selectable audio and subtitle tracks *before* playback starts** |
| 18 | androidtv | Configurable default language for audio/subtitle |
| 145 | Swiftfin | Local Downloads |
| 10 | Swiftfin | Skip button for media segments |
| 9 | Swiftfin | Secondary subtitles |
| 5 | web | **Search results are limited to 24** |

**"List view" and "show titles in poster view" are the quiet ones worth noticing.** A poster
wall is the default in every product in this field, and a measurable number of people want a
**dense, titled list** instead — which is the same information-density complaint as Plex's
nested dropdowns, arriving as a feature request rather than as anger.

And `jellyfin-web`'s open issues are **dominated by subtitles**: desync on resume (35, 62
comments), *Subtitles disappear when fullscreen*, *Subtitle offset text field is clamped to
slider min/max*, *Inconsistent size of subtitles*, *Subtitles dark background box*. The player
surface is where the web client's reputation is made and lost.

## 6. The commercial services, measured with the same instrument

**2,205 App Store reviews**, 245 most-recent per app, coded the same way. The absolute star
averages are weak evidence — a subscription service's rating conflates catalogue and price with
interface — but **the theme mix is reliable**, and it separates the two worlds cleanly.
(HBO Max returned no reviews for the id used and is absent from the sample.)

| App | n | mean★ | 1★ | #1 theme | #2 theme |
| --- | --- | --- | --- | --- | --- |
| Roku | 245 | **3.88** | 19% | navigation 3% | playback bugs 2% |
| Netflix | 245 | **3.75** | 22% | price 14% | ads 7% |
| Apple TV | 245 | 3.05 | 35% | price 6% | navigation 5% |
| Paramount+ | 245 | 2.63 | 47% | playback bugs 19% | ads 13% |
| Prime Video | 245 | 2.56 | 42% | **ads 26%** | redesign 4% |
| Peacock | 245 | 2.44 | 51% | **ads 26%** | price 13% |
| Disney+ | 245 | 2.17 | 54% | **ads 34%** | playback bugs 11% |
| **Plex** | 245 | **2.04** | 58% | **redesign 20%** | playback bugs 13% |
| Hulu | 245 | 1.49 | 70% | **ads 36%** | playback bugs 16% |

**Two facts stand out.**

**The commercial services' dominant complaint is advertising** — 36%, 34%, 26%, 26% of reviews
— and that is a **business-model failure we structurally cannot have**
([D13](../../decisions/working-notes.md)). Netflix is the exception because most of its base is
not on the ad tier, and its top complaint is price instead.

**Plex is the only app in the sample whose leading complaint is the redesign**, at 20% — and
it sits **below every commercial service except Hulu**. An independent instrument, a different
population, and the same conclusion as §0.

### What they get wrong that we could also get wrong

The commercial complaints that are *not* about money are worth reading closely, because they
are all reachable from where we stand:

- **Resume is broken at companies with enormous budgets.** Disney+: *"No automatic episode
  tracker. Every other streaming service that is out there will keep track of what episode
  you're on… Except Disney+. Every day, I have to scroll through MULTIPLE episodes to find
  where I left off."* Paramount+: *"if I fall asleep watching and want to go back to an
  episode, when you scroll down, it defaults to the series very last episode."* This is
  [D81](../../decisions/working-notes.md)'s subject, failing at scale, daily, visibly.
- **The watchlist is buried.** Apple TV: *"Finding your wish list or watchlist is a chore…
  you have to scroll through at least three pages of the homepage just to find it."*
- **Each redesign buries things one level deeper.** Prime Video: *"You've moved the in-store
  code button several times recently. Each time, you bury it deeper in the app… not three
  swipes and three taps deep!"*
- **The store outranks the library.** Apple TV: *"Essentially an entertainment store… It acts
  more like an App Store than what your typical streaming apps offer."* Prime Video's homepage
  mixes Prime-included, rentals and third-party channels *"often without clear labeling"*.
  **This is the same complaint Plex users make about Discover** — and it is the one failure a
  product with nothing to sell cannot commit.
- **The end-of-episode overlay steals the ending.** Hulu: *"Before the show ends the interface
  minimizes the show and you try to see the last 20 seconds and you can't."*
- **Pause needs two presses.** Paramount+: *"You have to double click to pause or play."*

### And Netflix's redesign got the identical backlash

Netflix's 2025 TV redesign — its first major layout change in **12 years** — drew the same
complaints in the same words: *"borderline unusable"*, larger artwork that means **more
scrolling to see the same amount of content**, and **genre browsing buried deeper**. That is
Plex's complaint set, at the company with the largest UX research budget in the industry.

**But Netflix reports that in testing more users preferred the new design than the old.** That
is the uncomfortable fact in this whole round and it has to be carried: **vocal backlash and
measured preference can point in opposite directions.**

The reconciliation is not "ignore the loud users". It is that the two groups are answering
different questions — *"which layout do you prefer?"* against *"what happened to my thing?"*.
Every service in this sample takes the change away irreversibly, so the second question has
no good answer anywhere. **The anger is about lost agency more than about pixels**, which is
why the remedy is reversibility rather than conservatism.

### Separating "angry at change" from "angry at worse"

Backlash follows *every* redesign, so its existence proves nothing. Four ways to tell them
apart, best first ([D139](../../decisions/working-notes.md) §3a):

1. **Keep the old layout and let usage decide.** Change-anger decays as people relearn;
   degradation does not. *"What share is still on the old layout after 30 days"* is a number
   that needs no survey — and **nobody in this sample can run that test**, because they all
   removed the old answer.
2. **Instrument the task, not the opinion.** Clicks-to-play and focus latency are falsifiable
   where *"I hate it"* is not. A change that holds both is a change; one that worsens either
   is a regression regardless of its reception.
3. **Read the complaint's shape.** Degradation reports **name a task and reproduce** —
   *"the subtitle menu is only like 100 pixels and off to the side"*. Change-anger is
   affective and unlocated. **This corpus is overwhelmingly the first kind**, which is why §1
   concludes what it does rather than dismissing it as novelty.
4. **Fresh preference minus incumbent satisfaction is the transition cost.** Netflix's
   divergence is not a paradox — their test asked people with no incumbency, the field
   contains people who lost something. The gap *is* the change-anger, isolated.

**And "users resist all change" is true and is not a defence.** It predicts that backlash will
exist, never what it contains. Used against a specific, reproducible, task-naming complaint it
has stopped being an observation.

## 7. What this says for us

1. **Interface regressions outrank price.** 51% of the likes, against 938 for the price thread.
   [D108](../../decisions/working-notes.md) was right and is under-weighted rather than over.
2. **Optimise click depth and focus latency, not looks.** Every top complaint is one of those
   two. *"One click instead of 30"* and *"the selection lags by an entire second"* are the
   sentences to design against.
3. **Let the user arrange their own home.** The removal of a re-orderable sidebar produced
   named anger. Ordering one's own library rows is a feature, not a preference.
4. **Ship a theming seam deliberately.** ~5,840 stars say it will exist regardless; the only
   choice is whether it is supported. And an unsupported one breaks every release, which is
   [D85](../../decisions/working-notes.md)'s compatibility problem pointed at the DOM.
5. **A settings scope is a UI decision.** The *Discussions* objection was that a
   person-shaped preference was implemented per device and buried.
6. **The mod to fear is a client replacement, not a skin** — and PM4K's own feature list says
   what earns one: being maintained, being stable, direct-playing anything, and identifying
   streams correctly.
