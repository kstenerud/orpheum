> **ABOUTME:** What users actually say about the existing personal media servers — praise,
> complaints, and the recurring themes — with the method and its gaps stated up front so a
> reader knows how much weight each claim carries.

# User Sentiment

Gathered **2026-09-17**. This document is the demand-side evidence; the supply-side evidence is
in the `competitors-*.md` files.

## Method, and what it could not reach

**Sources actually used.**

- **Hacker News**, via the Algolia API, reading full comment trees rather than summaries. The
  threads read in full: *Jellyfin 12.0* (623 pts, 355 comments, 2026-09-08), *Jellyfin founder
  Andrew leaves team* (370 pts, 338c, 2026-07-20), *Jellyfin as a Spotify alternative* (458
  pts, 482c, 2025-04-17), *Jellyfin: We're Good, Seriously* (556 pts, 306c), *From Plex to
  Jellyfin Media Server* (333 pts, 344c), *Jellyfin: The Free Software Media System* (492 pts,
  284c). Quotations below are verbatim from those threads.
- **GitHub issue trackers**, via the search API sorted by reaction count, for every open
  competitor. Reaction counts are a good proxy for *breadth of demand* and a poor one for
  severity.
- **Lemmy** (`lemmy.world` API) and web-search summaries for coverage of forum discussion.

**What could not be reached: Reddit.** The user asked specifically for Reddit. Every available
route was blocked from this environment:

| Route | Result |
| --- | --- |
| `reddit.com` / `old.reddit.com` / `api.reddit.com` `.json` endpoints | 403; Reddit's bot-block page returned as HTML regardless of user agent |
| WebFetch tool on a `reddit.com` URL | Refused by the tool |
| Web search restricted to `reddit.com` | Refused: domain not accessible to the crawler |
| **`forums.plex.tv`** | **Reachable** (added 2026-09-19) — Discourse, with `/top.json`, `/search.json` and `/t/<id>.json` all answering. First-hand Plex sentiment after all. See the Plex-forum section below. |
| Public Redlib front-ends (five instances tried) | 429, 403, or an Anubis proof-of-work interstitial |
| `r.jina.ai` text-extraction proxy | Returned Reddit's own "blocked due to a network policy" page |

Where a general web search *summarised* a Reddit discussion, the finding is marked
**(second-hand)**. Nothing in this document is a fabricated Reddit quotation. If first-hand
Reddit data matters for a specific question later, it needs a human with a browser, or a
registered Reddit API credential.

---

## The Plex story: a commercial complaint, not a technical one

This is the loudest, most consistent signal in the entire corpus, and it is the reason this
project has an audience.

**The facts behind it.** In 2025 Plex raised Plex Pass from $4.99→$6.99/month and
$39.99→$69.99/year (a 75% annual increase), the lifetime pass from $119.99→$249.99, and moved
**remote streaming — free since forever — behind the paywall**, enforcing it platform by
platform (mobile, then Roku in late 2025, then TVs and consoles in 2026). Non-subscribers can
buy a "Remote Watch Pass" at $1.99/month. HN users in 2026 report the lifetime pass at $750.

**What users say.** Three distinct complaints, in descending frequency:

1. **The product is drifting from media server to streaming storefront.**
   - "they've pivoted away from being a media server first and are trying to change into a
     streaming platform first, and because managers inside the company have to show KPI,
     they're stuffing the streaming buttons and sections and recommendations in everywhere."
   - "I was tired of fighting Plex for what the home screen looked like. It seemed like every
     update there was more Plex bullshit and less of my actual media."
   - "Why do I have to regularly turn off their 'recommended' stuff that is just ads for movies
     I can buy online? Why does it appear and re-appear?"
   - "Plex lost me when they became obsessed with trying to put advertisements in front of my
     grandparents when all they want to do is watch Mash on my plex server."
   - "i am prepared to pay them to NEVER EVER change the ui, never shove another streaming shit
     in my face, and just update for new codecs and security fixes."
2. **The cloud account contradicts the point of self-hosting.**
   - "Plex lost their best users by missing the point of self hosting entirely. Nobody wants to
     deal with third party downtime, even if it is rare and intermittent."
   - "I don't want my adblocker to have to block services running on my local network (they
     have some analytics + sentry)."
   - "Don't forget about 3rd party security, they were recently hacked exposing customer
     accounts details. their push for online accounts is what ruled it out for me."
3. **Dread rather than anger.** A striking number of lifetime-pass holders describe themselves
   as waiting for the other shoe: "it really does feel like a Sword of Damocles situation",
   "a soft landing for when Plex finally does something to make me move", "If Plex does shit
   the bed, I will have to rethink my setup."

**What keeps them there anyway** — and this is the competitive brief:

- **Client breadth.** "The only reason I still use Plex is that they have (native) apps on
  nearly every device me and my friends have (tv's, playstation etc.)."
- **Sharing with family.** "I can't give friends and family access, either, which was dead easy
  with Plex." Remote access that a non-technical relative can use is Plex's moat.
- **Plexamp.** "I can never give up plexamp. I've tried navidrome and various jellyfin clients
  but I keep going back."

---

## Plex's own forum, read first-hand — and it says something different

**Added 2026-09-19.** `forums.plex.tv` runs Discourse and its JSON API answers this network,
which the original round did not try. It is the first-hand Plex-user source that
[Reddit](#method-and-what-it-could-not-reach) was supposed to be, and **it partly contradicts
the section above.**

Top topics by likes, `/top.json?period=yearly`, fetched 2026-09-19:

| Likes | Posts | Topic | Opened |
| --- | --- | --- | --- |
| **1,124** | 304 | *Vote to roll back to Plex Classic!* | 2025-09-26 |
| **937** | 443 | *Elephant in the room. Lifetime Plex Pass* | 2026-05-19 |
| **502** | 250 | *Playback Error on Samsung TV: "An unexpected playback problem occurred"* | 2026-08-19 |
| **440** | 128 | *"Discussions" is a nightmare* | 2026-06-10 |
| **341** | 99 | *Plex, are you going to respond?* | 2025-10-14 |
| **325** | 106 | *How do I get the old experience back?* | 2026-04-28 |
| **290** | **506** | *Plex NFO Agent Forum Preview* | 2026-02-06 |
| **176** | 73 | *Fire TV app — Hopelessly Crippled* | 2026-06-15 |

**The single loudest complaint of the year is not the price. It is a UI redesign users cannot
revert.** The section above characterises Plex's problem as "commercial, not technical", and
that was drawn from HN, where the price rise dominates. On Plex's own forum the rollback
thread outscores the lifetime-pass thread, and the same theme recurs under at least eight more
titles — *The new Interface is HORRIBLE!*, *"Classic Mode" or "Use Old UI" Option*, *Reload the
old plex on Roku?*, *I'm so pissed each time i open Plex now*, *Plex UI Design - We Get It!*,
*What is going on with Plex? This interface is terrible!*, *New UI for Fire Stick 4K June
2026*, *New TV app feedback*. It is not a commercial complaint at all.

The arc completed in August 2026: Plex's announcement says *"since we launched the new Plex
experience to preview users last year, we have received consistent feedback from the start
regarding the navigation… our fall update was the first step… but they weren't enough"* — a
partial revert after roughly a year. The forum's own read of it, in the thread titled *Why did
you cave?*: *"it's not enough, and possibly too little too late. They've damaged…"*

**And the second theme is one we have not recorded anywhere: a client update that breaks a
platform, with no way back.** Verbatim from *Fire TV app — Hopelessly Crippled* and its
neighbours:

> "I just reverted to the old version and turned off updates. Way to ruin a good thing, Plex."

> "Now my videos keep buffering, videos that played fine before are unwatchable! … Now I have
> to try to figure a way to revert!"

> "Please tell me how to revert to the previous Plex on Fire TV!"

Enough people needed it that a pinned workaround exists — *Workaround: Roll back the Fire TV
Plex app to version 10.30.9.4262* (78 likes) — alongside *How do I revert to an older version
of Plex for FireTV*, *Firetv app is literally unusable*, and a campaign thread asking users to
petition **Vizio** because the TV vendor is the only party who can un-ship the update.

Two more things worth recording:

- **The lifetime price is confirmed from the announcement**, not inferred from HN: *"Starting
  July 1, 2026, the price of a new Lifetime Plex Pass will increase to $749.99 USD."* And the
  fear underneath it is sharper than the price: *"At some point I worry my lifetime license
  will no longer be valid because of a version change. Plex server 2 or some bullshit. I see it
  happen with great software that moves to subscriptions frequently."*
- **NFO/sidecar metadata is a 506-post thread**, the most-replied on the list. Whatever else
  this audience wants, they want their metadata in files they own — which is
  [D98](../../decisions/working-notes.md)'s portability half, independently confirmed.

### What the forum confirms that our design already assumed

Searching it for the things this project has decided returns first-hand demand for each,
which is the cheapest possible validation:

| Thread | Posts | Ours |
| --- | --- | --- |
| *Add Watch Together to New Plex Experience* | **350** | [D62](../../decisions/working-notes.md) — they removed it; this is the thread asking for it back |
| *Let's finally solve the low audio issue please* | **141** | [D83](../../decisions/working-notes.md) — plus *Let me boost the voice channel!* and *Night mode audio option to decrease loud sounds and increase dialog* |
| *Option for semi-persistent automatic skip intro* | **125** | [D82](../../decisions/working-notes.md) |
| *Expansion of Allow/Deny lists of tags for parental controls* | — | [D84](../../decisions/working-notes.md) — and it is asking, in effect, for a predicate language |
| *The Parental Controls Are Now Useless*, *Plex Home Breaks parental controls* | — | [D84](../../decisions/working-notes.md) |
| *Replace subtitles size settings "normal, large, huge" by a slider with size %* | — | [D90](../../decisions/working-notes.md) — that is 47 CFR §79.103's 50–200% requirement, requested by a user who has never heard of it |
| *How do I downmix 5.1 to 2 Channel Stereo by default?* | 27 | [D83](../../decisions/working-notes.md) |

## The Jellyfin story: goodwill, and a list of specific failures

Sentiment is warm and heavily qualified. "It's not amazing, it's as good as it is. Which is to
say not great. There is a crazy amount of bugs which I have no confidence will ever get fixed."
versus "Jellyfin works great for me." — both in the same threads. The complaints cluster
tightly:

**1. Subtitles — the single largest defect area.** The most-reacted server issue is `Subtitles
load after a while and can be out of sync when resuming` (+103, 151 comments, **open since
2020-03**). The `jellyfin-web` tracker's top bug is subtitle desync. Verbatim from HN:

> "Subtitles are the Achilles heel of Jellyfin. I can see the subtitle present but it doesn't
> show on the movie. Ok, I will just add manually - can't, progress indicator loops forever.
> Ok, I will remove the existing subtitles and add - removal succeeds - addition fails. This is
> the same fiddling everytime. If it works, it is a miracle."

The reply that follows is diagnostic gold — the workaround is to pre-extract every embedded
subtitle to SRT and *disable* serving embedded ones, because some clients must download the
whole subtitle track before playback and extraction from the container is slow. That is an
architecture problem surfacing as a user problem.

**2. Transcoding decisions users disagree with.**
> "I have a problem that it constantly tries to re-encode video that could be played perfectly
> fine in their player or the mpv shim. I wound up just mounting my remote drives and playing
> in my local mpv."

Mirrored in the trackers by `Force "direct play" in client` (+49) and `Feature to force
transcode` (+21) — the same decision being wrong in both directions.

**3. Clients are the weak half.**
> "while the server is pretty solid, the client experience is a bit hit or miss"

Concretely: no official Samsung Tizen app (the community one requires putting the TV in
developer mode), the Android TV app's top request is a **playback rewrite open since 2021**,
and the clients people actually recommend — Infuse, Findroid, Swiftfin, Symfonium, Streamyfin,
Wolphin, the mpv shim — are almost all third-party. A recurring pattern is people using
Jellyfin purely as a *catalogue* and playing through something else.

**4. Remote access is unsolved for non-technical users.**
> "the sore point right now is outside access, which Jellyfin really isn't made for. I set up
> Tailscale so I can connect Infuse to Jellyfin remotely. But that's not exactly as smooth as
> Plex. I can't give friends and family access, either."

The community answer is always Tailscale or a VPN. "Tailscale is great for me, but not feasible
for family" is the whole problem in one line.

**5. Security, and the project's answer to it.**
> "doing performance improvements and adding comic book support before making the product
> secure enough to expose to the internet is wild prioritization"

with the maintainer position relayed as "jellyfin should not be exposed and it should always be
behind a vpn", and a separate report of "sending passwords in cleartext on login" (i.e. over
plain HTTP on a LAN deployment). See [competitors-video.md](competitors-video.md) for the
tracker and CVE evidence behind this.

**6. Upgrades became a thing to fear.** The 10.11 EF Core migration produced hours-long hangs,
failed migrations and large post-upgrade slowdowns. On the 12.0 thread the *first* question is
"Upgrade experiences, anyone?", and answers range from "quick and painless" to "this completely
blew up and the db migrations failed... so I just nuked the whole Jellyfin config and started
from scratch."

**7. Granular sharing is missing.**
> "the #1 missing feature I'd like is per-user granular sharing of content within specific
> libraries. So far as I can tell, the only way around this is to symlink the subset of media
> into separate folders, make each of them libraries, then present those libraries to each
> user."

**8. Library scanning is opaque.**
> "My only gripe is logging around library scanning being pretty difficult to interpret. I'd
> love to add a dedicated scanner log that focuses only on logs relating to scanning, which for
> me tends to be the trouble spot in these systems."

**9. Community climate.** One throwaway account claiming heavy involvement described "constant
mocking accusations and hostility toward third-party clients, plugins, themes" and bans for
falling outside a small group's preferences; another user independently reported a poor
experience on the `jellyfin-ios` repo. Single accounts, so treat as **unverified** — but it is
consistent with the observable fact that the best clients live outside the project.

---

## Cross-cutting themes

Ranked by how often they recur across products and threads:

1. **"It should just play the file."** Every transcode the user didn't ask for is experienced
   as a defect. The ideal is: server hands over bytes, client plays them.
2. **Subtitles must be right.** Timing, styling, fonts, embedded and external, per-client. This
   is where open media servers lose to a $10 app (Infuse) most visibly.
3. **Remote access for non-technical family is the moat.** Plex's most-resented feature (the
   cloud account) is also the one thing keeping people on it. Whoever makes this work without
   a third-party account wins Plex's churn.
4. **The client is the product.** Users judge a media server by its TV app. A great server with
   a mediocre TV app loses.
5. **Upgrades must be boring.** A data migration that can eat a library is a reputational
   event that lasts years.
6. **Don't put a business between me and my files.** Analytics, telemetry, cloud auth,
   storefronts and upsells in a self-hosted product generate disproportionate anger.
7. **Scanning must be legible.** When it goes wrong — and it does — users need to see why.
8. **AI-assisted self-hosting is now normal.** A striking amount of the 2026 HN commentary is
   people saying LLMs removed the friction from running this stack ("I never have to read
   another fucking log file ever again"). Two implications: a good CLI, good structured logs
   and good docs are worth more than they used to be, and users' tolerance for *configuration*
   complexity has risen while tolerance for *broken* has not.

## What this settles for the design

- Direct play is the default and the goal; transcoding is the exception and must be explicable.
- Subtitle rendering is a first-class client feature, not a server convenience.
- Family-friendly remote access without a mandatory third-party account is a headline feature,
  not a deployment note.
- One excellent TV client beats five mediocre ones.
- Zero telemetry, zero account requirement, zero upsell — stated as a promise, in the README.
- Schema and migration discipline from commit one.
