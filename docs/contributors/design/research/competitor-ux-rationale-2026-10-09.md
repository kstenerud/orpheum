> **ABOUTME:** Why the big streaming services, and a few smaller products that broke from them,
> made the interface decisions they did, in their own words: which reasons serve the viewer and
> transfer to Orpheum, which serve engagement or ads and do not, what they reversed and why.

# Competitor UX Rationale: Why They Chose What They Chose, in Their Own Words

- **Date:** 2026-10-09.
- **Gaps filled:** 2026-10-09, in a second pass with web search, from what §8 of the first pass
  listed as not reached: Netflix's Post-Play, homepage, "Are you still watching?", Moments and Play
  Something; Apple's Up Next split; the HBO Max release; Disney's brand tiles and GroupWatch notice;
  Amazon's own Watch Party notice; Hulu, Discord, Kast and Rave on co-watching; MUBI's library;
  Peacock, Criterion Channel, Kanopy, Letterboxd, BBC iPlayer, Roku 2026, Kodi, Emby and Tristan
  Harris. §8 lists what is still unreached.
- **Requested by** the owner, for M5: *"Find everything you can that our competitors have said
  about why they took the UI and UX decisions that they did. I'm mostly interested in the big
  commercial outfits like Netflix, Amazon, etc, but if there are other compelling narratives from
  smaller places that mention they are breaking from the others' conventions, that could be useful
  as well."*
- **Builds on, and does not repeat:** [ui-competitors.md](ui-competitors.md) (what works and fails
  by surface, the reversals table, Skip Intro's origin, Roku's certification),
  [ui-design-practice-2026-10-05.md](ui-design-practice-2026-10-05.md) (what competitors look like
  and what users say now), [ux-principles-2026-10-09.md](ux-principles-2026-10-09.md) (the
  practitioners' principles U1–U25), [watch-together.md](watch-together.md) (which co-watching
  features were withdrawn) and [feature-inventory-2026-10-09.md](feature-inventory-2026-10-09.md).
  Where this document adds the *reason* to a fact recorded there, it says so.
- **Why it matters here.** Almost every big-service decision is justified by an A/B test against
  engagement or retention. Orpheum has no ads, no telemetry and no catalogue to sell, and two
  people use it first. So the useful question about each decision is not *"does it work for
  Netflix?"* but *"does the reason survive without the incentive?"* Each entry below answers that.
- **Sources:** company newsrooms and blogs (about.netflix.com, the Netflix TechBlog, aboutamazon.com,
  Apple Newsroom and WWDC session transcripts, blog.youtube, spotify.design, blog.google,
  disneyplus.com, press.wbd.com, plex.tv/blog), the Plex, Firecore and Channels forums through their
  Discourse JSON APIs, one ACM journal paper, and press interviews that quote design and product
  leads (The Verge, Deadline, TechCrunch, Vulture, Fast Company, LA Times, IGN). The second pass
  added Netflix's Help Center and Tudum, archived Netflix blog posts, nbcuniversal.com, kodi.tv and
  forum.kodi.tv, Discord's support pages through archive.org, the BBC Internet Blog through a
  syndicated copy, and interviews in IndieWire, Collider, Screen Daily, dot.LA and others.
- **Cache:** every fetched page is under `/yoloai/cache/http/competitor-rationale/` (`<name>.raw` as
  fetched, `<name>.txt` as extracted); working notes with fuller quotes are in this session's
  scratchpad and are summarised here.
- **Legend:**
  - **[1P]** the company's own words: its blog, newsroom, paper, talk, forum staff post, or an
    employee quoted directly in the press.
  - **[J]** a journalist's or analyst's interpretation.
  - **[I]** this researcher's inference.
  - **Commercial** marks a reason that serves engagement, retention, ads or sales; **User** marks
    one that serves the viewer. Many are both.
  - **For Orpheum** says whether the decision still makes sense without the commercial incentive.
  - Everything was read at its source on 2026-10-09 unless marked **[S]** (second-hand).

## 1. Summary

- **The big services say two different kinds of thing.** Their *product* reasons are nearly all
  engagement: rows, previews, personalised artwork, Top 10, vertical clip feeds and autoplaying
  heroes are justified by "browse less, watch more" A/B results, and Netflix states the business
  objective outright (§2.1). Their *design teams'* reasons are often the opposite: Apple's tvOS
  designers warn against being *"paralyzed by choice"* and against forcing sound on the viewer;
  YouTube's 2026 principles say *"the interface shouldn't compete with the video"* (§2.4, §2.5).
  The second kind transfers almost whole; the first mostly does not.
- **What they reversed is the most useful evidence.** Off switches for autoplay, ways to remove
  things from automatic lists, navigation returned to where it was, colour toned down for
  legibility, feature retirements after a client rewrite. In each case users were right and the
  first design was wrong for them (§4).
- **The best contrarian is MUBI** (§5.1): a small set chosen by someone you trust, against an
  infinite catalogue, with the whole library behind it since 2020. In a household the trusted
  chooser is a person, not a model.
- **The smaller and public services say the opposite of the big ones in plain words** (§5.4–§5.7).
  BBC iPlayer found that *"58% of our users come to iPlayer with something particular in mind"*, so
  finding comes first and featured content *"dims down to get out of your way"*; Netflix retired
  Play Something for the same reason, by its spokeswoman's account (§4). Criterion Channel wants
  collections *"curated thoughtfully by real people, not by algorithms"*, small enough to finish.
  Letterboxd builds *"algorithms to help people find what they love, rather than to suck them in"*.
  Netflix's own 2015 homepage post wants *"some stability so that people are familiar with their
  homepage"* (§2.1).
- **For the M5 questions** (§6.2): the evidence leans against a separate Home banner and towards
  Continue Watching first with larger art; towards one well-chosen portrait poster size first;
  towards an artwork tint only as a dim, scrimmed field with text on a neutral surface; and, for
  guests' controls (now decided by [D271](../../decisions/working-notes.md) §4), towards Apple's
  rule that a scrub's preview stays on the scrubbing device and only the release is shared.
  Discord's own rule backs D271's default: *"We recommend Hosting when in big groups and Sharing
  when in small groups!"*; and Hulu's personal pause with a "catch up" button is the shape Apple
  later chose too (§2.11).

## 2. What each company said, and why

### 2.1 Netflix

| Decision | Stated reason | Date, source | Evidence claimed | Commercial or user | For Orpheum |
| --- | --- | --- | --- | --- | --- |
| **The "90 seconds" to capture a member** | *"a typical Netflix member loses interest after perhaps 60 to 90 seconds of choosing, having reviewed 10 to 20 titles (perhaps 3 in detail)"*; then *"the risk of the user abandoning our service increases substantially"* [1P] | Dec 2015, Gomez-Uribe and Hunt, ACM TMIS ([doi](https://doi.org/10.1145/2843948)) | unpublished "consumer research", hedged "perhaps" | Commercial: the risk named is churn | The figure is not a measured user need, and the 2016 posts dropped the "perhaps" (below). Do not cite it as fact. |
| Same claim, unhedged | *"if you don't capture a member's attention within 90 seconds, that member will likely lose interest"*; artwork was *"over 82% of their focus while browsing"*, *"1.8 seconds considering each title"* [1P] | 2016-05-03, Nick Nelson, [The Power of a Picture](https://about.netflix.com/en/news/the-power-of-a-picture); also the TechBlog's [artwork A/B post](https://netflixtechblog.com/selecting-the-best-artwork-for-videos-through-a-b-testing-f6155c4595f6), section "Gone in 90 seconds" | early-2014 consumer research; A/B tests that raised total hours, not just shifted them | Both | The 82% and 1.8 s findings transfer: the poster *is* the browsing surface. Ensemble art with more than three people *"dramatically dropped"* because it is *"too complex at small sizes"*. |
| **"A few compelling choices simply presented"** | *"what customers ask for (as much choice as possible, comprehensive search and navigation tools, and more) and what actually works (a few compelling choices simply presented) are very different"* [1P] | Dec 2015, same paper | A/B testing | Both | Transfers as a design stance. The method (test, don't ask) needs scale Orpheum lacks; with two users, watch them. |
| Rows, each with a reason | *"Each row has a title that conveys the intended meaningful connection"*; rows let a member *"skip videos that would be good choices for a different time, occasion, or member of the household"* [1P] | 2012-04-06, [Beyond the 5 stars](https://netflixtechblog.com/netflix-recommendations-beyond-the-5-stars-part-1-55838468f429); 2015 paper | retention rises with tailored rows placed higher | Commercial: *"maximize member satisfaction and month-to-month subscription retention"* | The row's honest title transfers ("Continue watching", "Added this week"); the ranking engine does not. |
| **A homepage that stays familiar** | Rows let members *"quickly decide whether a whole set of videos in a row is likely to contain something"*, which *"would not be the case if ... the page contained a large, unorganized collection"*. Against freshness: *"we also want some stability so that people are familiar with their homepage and can easily find videos they've been recommended in the recent past"*; and the next episode or a rewatch *"normally falls outside the realm of recommendation"*. *"videos presented in the upper left are much more likely to be seen than those in the lower right"* [1P] | 2015-04-09, Chris Alvino and Justin Basilico, [Learning a Personalized Homepage](https://netflixtechblog.com/learning-a-personalized-homepage-aa8ec670359a) (read through archive.org's copy of the original) | the old template, *"first row would be Continue Watching (if any)"*, *"evolved ... using A/B testing"* | Both | Transfers: stability is a user reason even at Netflix, and resuming sits outside recommendation. Rows in a fixed, person-chosen order (D138) and Continue Watching in the upper left are the household form. |
| **Post-Play: the next episode in 15 seconds** | *"Many Netflix members love watching multiple episodes of TV shows in a row. It is almost a phenomenon"*; *"If you do nothing, the next episode will start to play in 15 seconds. You can also stop it"*; after a film, *"three recommendations to try next"* [1P] | 2012-08-15, Chris Jaffe and Michael Spiegelman, the Official Netflix Blog (*"New 'Post-play' Experience Makes It Easy To Continue Watching"*, read through [archive.org](https://web.archive.org/web/20120816204840/http://blog.netflix.com/2012/08/new-post-play-experience-makes-it-easy.html)) | none given | Commercial (*"making it easier to enjoy more TV shows and movies"*), with a user half: the next episode is what a viewer mid-series wants | The next episode offered at the end transfers (Next Up). Starting it unasked is the case Tristan Harris names (§5.7); if Orpheum ever does it, the person sets the bound. |
| **"Are you still watching?"** | *"so you don't lose your place or use internet data when you're not actually watching"* [1P] | Netflix [Help Center 114059](https://help.netflix.com/en/node/114059), current; archive.org copies from 2020 give the same reason | — | User | The rule changed and the reason did not: two episodes in 2020 and 2021; three episodes or 90 minutes from 2022; on TVs today *"after 3 episodes and 90 minutes of uninterrupted watching"*. There is **no switch to turn the question off**; the only route the page gives is turning off autoplay of the next episode. D188 adopted the TV rule; "don't lose your place" is the reason that carries over, data less so on a home network. |
| Time-to-play is not the goal | Better evidence on a card *"might cause members to skip choices ... resulting in a better eventual choice and more satisfaction, but associated with a longer time to first play"* [1P] | Dec 2015, same paper | — | User | Transfers: U4's "shortest path to the picture" is about the path once chosen, not hurrying the choice. |
| **Personalised artwork** | Artwork gives *"visual 'evidence' for why the title might be good for you"*. Its own risk: *"Does changing artwork reduce recognizability of the title and make it difficult to visually locate the title again"*; they *"control the exploration such that artwork selections do not change too often"* and penalise *"clickbait"* [1P] | 2017-12-07, [Artwork Personalization](https://netflixtechblog.com/artwork-personalization-c589f074ad76) | offline replay and A/B lift in core metrics | Commercial (take rate) | The personalisation does not transfer. The **recognisability** risk does: a title's poster should not change under a person who has learned it ([D170](../../decisions/working-notes.md) already makes refetching an act of the user). |
| **Autoplay previews on TV** | Previews *"help members make faster and more confident decisions"*; *"people watched more of a story ... browse less and watch more"* [1P] | 2016-12-06, [new TV experience](https://about.netflix.com/en/news/new-netflix-tv-experience-includes-video-previews-that-speed-your-next-selection); TechBlog 2017: *"after A/B testing, eye-tracking research, and customer feedback"* | A/B, eye tracking | Commercial | Declined by [D188](../../decisions/working-notes.md). See §4 for the off switch. |
| **Stars to thumbs, then "% Match"** | Members were *"confused about what they do"*: stars read as public averages; thumbs *"teach the system"* [1P]. Todd Yellin: *"What you do versus what you say you like are different things"* [1P via J] | 2017-04-05, [Goodbye Stars](https://about.netflix.com/en/news/goodbye-stars-hello-thumbs); [The Verge](https://www.theverge.com/2017/3/16/14952434/netflix-five-star-ratings-going-away-thumbs-up-down) | a year of testing; *"200% increase in ratings activity"* | Commercial (data) and user (clarity) | A rating whose purpose is unclear is not used. Orpheum has no recommender; if it ever asks for an opinion, it must say what the answer does. |
| **Profiles** | The origin was *"'Honey, you ruined my Netflix!'"*; profiles give *"more choice and control (with features like autoplay, subtitles, and more)"* [1P] | 2023-08-01, [10 years of profiles](https://about.netflix.com/en/news/celebrating-10-years-of-netflix-profiles); 2013 launch via [TechCrunch](https://techcrunch.com/2013/08/01/netflix-user-profiles/) | *"a majority of Netflix accounts"* have a second profile | Both | Transfers fully: per-person place, choices and history are already the design. |
| Kids' favourites row | *"like a kid walking into their own room, where they know where every Lego piece is"* [1P via J] | 2021-04-13, [The Verge](https://www.theverge.com/2021/4/13/22381917/netflix-kids-profile-design-favorites-row-on-screen-characters) | — | User | Stable placement aids recognition (U3). |
| **Top 10 row** | *"help create more of these shared moments, while also helping all of us find something to watch more quickly"* [1P]; the A/B's decision metric was engagement [1P] | 2020-02-24, [Top 10](https://about.netflix.com/en/news/see-whats-popular-on-netflix); 2021 [What is an A/B Test?](https://netflixtechblog.com/what-is-an-a-b-test-b08cc1b57962) | 6+ months' tests in Mexico and the UK | Commercial | Does not transfer: popularity across two people is noise. |
| **Playback speed** | Long on DVD players, *"frequently requested"*, for rewatching and for subtitles; endorsed by US deaf and blind associations. Concessions to filmmakers: not on TVs, 0.5–1.5×, pitch-corrected, and members must *"vary the speed each time they watch something new"* [1P] | 2019-10-28, updated 2020, [player control tests](https://about.netflix.com/en/news/player-control-tests) | surveys: no effect on perceived quality | User; the limits are a stakeholder's | The feature transfers. The limits and the reset were concessions to creators, whom Orpheum does not answer to: a person's speed may persist and the range may be wider [I]. |
| **Remove from Continue Watching, with undo** | *"We heard you"*; one click, undo by the back arrow [1P] | 2022-01-27, [tidy up Continue Watching](https://about.netflix.com/en/news/now-you-can-easily-tidy-up-your-continue-watching-row-on-all-devices) | — | User | Adopted by D188. |
| Downloads, Smart Downloads, partial downloads | *"You watch, we do the work"*, and *"members can choose when they want to use Smart Downloads"*; partial downloading because *"There's nothing worse than ... discover that it didn't finish downloading"* [1P] | 2019-02-08, [Smart Downloads](https://about.netflix.com/en/news/downloading-on-netflix-just-got-smarter-1); 2021-11-30 | — | User | Transfers when offline download is built ([feature-inventory](feature-inventory-2026-10-09.md) §2): automation that can be switched off, and a half-fetched film still plays. |
| Subtitles split into dialogue-only and CC | *"nearly half of all viewing hours on Netflix in the US happen with subtitles or captions on"* [1P] | 2025-04-24, [a new way to experience subtitles](https://about.netflix.com/en/news/introducing-a-new-way-to-experience-subtitles) | viewing data | User | Transfers: D188's track names already distinguish SDH; the label should say "with sound descriptions" in words. |
| **Hawkins design system** (internal studio tools, not the consumer app) | *"If users know how to fill out forms, access data in a table or receive notifications in one application, they will intuitively know how to in the next"*; *"There will only be one implementation of buttons, tables, forms"* [1P] | 2021-02-10, [Hawkins](https://netflixtechblog.com/hawkins-diving-into-the-reasoning-behind-our-design-system-964a7357547) | — | User and engineering | Transfers to Orpheum's admin screens and to rule 11 (one way to do each job). |
| **The 2024–2025 TV redesign** | Pat Flemming: *"We often see members doing gymnastics with their eyes as they're scanning the home experience"*; details now expand in the focused tile instead of a billboard at the top [1P via J]. Eunice Kim: shortcuts *"were somewhat hidden on the left-hand side"*; and *"Our current TV experience was built for streaming shows and movies ... This one is designed to give us a more flexible canvas"* for games, live and ads [1P] | 2024-06-06, [The Verge](https://www.theverge.com/2024/6/6/24172351/netflix-redesign-homepage-experience-test); 2025-05-07, [Netflix](https://about.netflix.com/en/news/unveiling-our-innovative-new-tv-experience); [Vulture](https://www.vulture.com/article/why-netflix-had-to-redesign-its-homepage.html) | internal tests since late 2022 [J] | Eye strain and hidden navigation: user. Flexible canvas: commercial | The first two transfer: information where the eye already is, navigation visible (U2). Vulture also reports that the artwork's colour now reflects into the page background [J]; see §6.2. |
| **Moments**: save a scene, replay it, share it | *"you can easily save, relive, and share your favorite Netflix scenes"*; on a rewatch *"it will start playing right from the scene you bookmarked"* [1P]. Its roll-out *"coincides with a new Netflix global brand campaign"*. In 2025, *"it's clear fans love having a way to hold on to the scenes that stick"*, so clips gained start and end points [1P] | 2024-10-28, Tudum, [Moments](https://www.netflix.com/tudum/articles/netflix-moments) (the launch text through archive.org; the live page holds the 2025-09-03 update) | none given | Sharing and the campaign: commercial; Deadline read it as *"a bid to boost engagement"* [J]. Keeping a scene: user | A per-person bookmark that plays from the scene transfers [I]. Sharing clips outside the household does not. |
| Clips and Fast Laughs (vertical feeds) | *"a personalized highlight reel that helps you decide what to watch or play next, without endless scrolling"* [1P] | 2021-03-03; 2026-04-30, [mobile](https://about.netflix.com/en/news/introducing-exciting-new-ways-to-find-and-enjoy-your-next-favorite-on-mobile) | — | Commercial | Does not transfer. |

### 2.2 Amazon Prime Video

| Decision | Stated reason | Date, source | Evidence | Commercial or user | For Orpheum |
| --- | --- | --- | --- | --- | --- |
| **2022 redesign**: left rail, less clutter, a blue check for "included", a gold bag for "costs extra" | designed to be *"less busy and overwhelming for our customers"* [1P via J]. Helena Cerna: *"In usability testing, we repeatedly heard the phrase, 'wow, I didn't even know Prime Video had live TV'"* [1P via J] | 2022-07-18, [The Verge](https://www.theverge.com/2022/7/18/23268285/amazon-prime-video-2022-redesign-preview-announcement) | 18 months, usability testing [J] | Labelling what costs money: user. Surfacing "underutilized perks": commercial | Nothing in Orpheum costs extra, so no labels. **The finding transfers**: a feature that is not in the navigation does not exist for its users. That argues for Watch Together's always-visible menu entry ([m5.md](../plans/m5.md), from the owner's tests). |
| **2024 redesign**: top bar, a tab per add-on, autoplaying hero and Live TV, LLM-shortened synopses | Kam Keshmiri, VP of Design: *"many are in search of a more intuitive streaming experience"*, and to *"sign-up or switch add-on subscriptions with just a few clicks"*; synopses shortened *"so you can quickly glance at a title description, rather than having to scroll"* [1P] | 2024-07-23, [aboutamazon.com](https://www.aboutamazon.com/news/entertainment/prime-video-updated-steaming-experience); [Deadline](https://deadline.com/2024/07/prime-video-updates-user-experience-streaming-1236018211/) | *"always listening to customers"*, no numbers | Mostly commercial | A short synopsis on the card transfers (the first sentence, not a model's rewrite). The rest does not. |
| **X-Ray** | answers *"'Who is that actor?' or 'What's that song?'"*; *"X-Ray features do not disrupt the viewing experience, are easily accessible for customers who want to engage with them"* [1P] | 2018-06-27, [behind the scenes with X-Ray](https://www.aboutamazon.com/news/entertainment/behind-the-scenes-with-x-ray) | none | User (and IMDb's) | The need transfers; scene-accurate data does not exist for a home library. "Who is in this" from the film's credits, behind a pause, is the cheap half [I]. |
| X-Ray Recaps (generative) | addresses *"forgetting where they left off"*; rewinding *"takes you away from the streaming experience"* [1P] | 2024-11-04, [X-Ray Recaps](https://www.aboutamazon.com/news/entertainment/amazon-prime-video-x-ray-recaps) | none | User need, commercial means | The need transfers; generated text that can be wrong does not (its pull for errors is in ui-competitors §2, [S]). |
| Ads in Prime Video | *"To continue investing in compelling content"* [1P] | 2023-09-22, [aboutamazon.com](https://www.aboutamazon.com/news/entertainment/prime-video-update-announces-limited-ads) | — | Commercial | None. |

### 2.3 Disney+

- **2025 homepage redesign** [1P] ([Disney+, 2025-11-07, revised 2026-01-09](https://www.disneyplus.com/explore/articles/disney-plus-app-redesign-new-features)):
  *"Our goal is to make it easy for you to find and stream the stories that you want to watch."*
  - Profiles made more prominent *"so it's easier to see which profile you're watching from"*.
    User; transfers: whose account is this is visible on Home.
  - Rows moved to *"more cinematic poster-style artwork"*. Badges such as "New Series" and "Season
    Finales" make *"just-added episodes and titles"* easy to find. User; both transfer, and
    "new episode" badges bear on Next Up.
  - A tab per brand and a video hero carousel that *"puts our characters and stories front and
    center"*. Commercial; neither transfers.
- **Remove from Continue Watching** [1P]
  ([Disney+, 2025](https://www.disneyplus.com/explore/articles/remove-from-continue-watching)):
  *"You asked, we listened!"* It lists the reasons: something *"queued by accident, already
  finished"*, or *"someone in your Disney+ Household binged a series without switching out of your
  profile"*. Pure user benefit. Every one of those reasons occurs in a two-person household, which
  confirms D188's choice.
- **Brand tiles, 2019** [1P, flagged]: at Disney's Investor Day (2019-04-11), in the live demo,
  the brands are called *"the single biggest differentiator for our service. This is captured by
  the brand tiles at the center"*, and each brand area is *"a unique opportunity for us to surprise
  and delight our consumers"*. The transcript PDF has no text layer; it was decoded here by letter
  substitution and the speaker (Kevin Mayer) was inferred from its labels, so check the wording
  against the [PDF](https://thewaltdisneycompany.com/app/uploads/2019/03/Disney_Investor_Day_2019_transcript.pdf)
  before quoting it elsewhere. Commercial: brand differentiation, told to investors. Does not
  transfer: a home library has no brands.
- **Rows that look alike tire the eye, 2021** [1P]
  ([Disney Streaming design blog, 2021-11-12](https://medium.com/disney-streaming/introducing-our-latest-ux-enhancements-for-disney-f7c93a4e38cb),
  Patrick Edelbacher, Tony Sokolowski, Brian Ziffer): the carousel and the brand tiles *"command
  most of the real estate on the Home Screen"*; below them *"the predominant driver ... is choice"*.
  A poster row was added because *"our findings showed that viewers can experience a certain
  fatigue when browsing multiple rows of similar-looking tiles"*. User; transfers: vary the shape
  of rows rather than repeat one [I].
- **GroupWatch** — see §2.11.

### 2.4 Apple (TV app, tvOS, SharePlay)

- **WWDC19, *Mastering the Living Room With tvOS* (session 211)** [1P]
  ([video](https://developer.apple.com/videos/play/wwdc2019/211/)): Apple's own designers give the
  clearest *user* rationale any big company has published. Nearly all of it transfers.
  - On rows: *"You're presented with rows and rows and rows of content ... we've become paralyzed
    by choice and robbed by indecision."*
  - *"remove barriers to entry, get rid of any hurdles between the content and the viewer."*
  - On autoplay: Top Shelf waits five seconds on a poster before a trailer, because *"we don't force
    sound on the viewer ... Too much sound and video when you first launch into something, it can
    be destructive."*
  - *"Every swipe or click on the remote, it adds to somebody's cognitive load."* *"Speed and
    pacing are essential ingredients for minimizing cognitive load. Try and remove latency".*
  - On overstimulation: too many animations, icons and type styles are like *"11 people ... talking
    to you loudly all at once"*; the goal is that people are *"focused and relaxed"*.
  - Metadata is secondary and shown when asked: *"By giving control to the viewer over the how and
    when to access this extra information, the path to pressing play ... becomes simplified."*
  - The commercial half is stated too: a different module between rows gives *"their eye a moment
    of pause"* and is a slot for promotion, and the talk closes on people being *"more likely to
    stay and browse"*.
- **2016 TV app and Up Next** [1P]
  ([Newsroom](https://www.apple.com/newsroom/2016/10/apple-unveils-new-tv-app-for-apple-tv-iphone-and-ipad/)):
  Up Next lists what you are watching *"in the order they are most likely to watch first"*, and the
  next episode *"will automatically appear at the start of the Up Next queue"*. Aggregation across
  apps is commercial; the ordering is user benefit and transfers (Next Up in
  [m5.md](../plans/m5.md)).
- **Up Next split into Continue Watching and Watchlist (2024)**: shipped in tvOS, iOS and macOS
  18.1 on 2024-10-28 with no stated reason. The tvOS 18.1 release note says only *"This update
  includes performance and stability improvements"* [1P]; no Newsroom post or WWDC session was
  found. 9to5Mac's reading [J]: *"The old Apple TV app concept of Up Next — using a single list that
  combined watch queue and currently watching content items — was quite confusing for users coming
  to the TV app for the first time."* The user lesson, on that [J] reading: "what I'm watching" and
  "what I mean to watch" are two lists.
- **WWDC21, tvOS playback** [1P]
  ([10191](https://developer.apple.com/videos/play/wwdc2021/10191/)): controls moved into the
  transport bar *"improved the discoverability of common playback tasks"*; menus *"support up to one
  level of nesting"*. Already in ui-competitors §2; the reason is discoverability.
- **tvOS 18 (2024)** [1P]
  ([Newsroom](https://www.apple.com/newsroom/2024/09/tvos-18-is-now-available/)): *"Subtitles now
  automatically appear at just the right moments, including when users mute, when they skip back
  while watching, or when the language ... does not match the device language."* User, no
  commercial angle, and new to this tree: **subtitles for a few seconds after a skip back** is the
  most direct "what did they say?" answer there is [I].
- **tvOS 26 (2025)** [1P]
  ([Newsroom](https://www.apple.com/newsroom/2025/06/apple-tv-brings-a-beautiful-redesign-and-enhanced-home-entertainment-experience/)):
  *"designed to keep the focus on what's playing"*; new poster art *"allows more shows and movies to
  be displayed"*; profiles on wake offered as a *"choice"*. The press said the profile picker was on
  by default ([S], ui-competitors §2).
- **SharePlay** — see §2.11.

### 2.5 YouTube

- **2022 new look and ambient mode** [1P]
  ([blog.youtube, 2022-10-24](https://blog.youtube/news-and-events/an-updated-look-and-feel-for-youtube/);
  [design follow-up, 2022-11-28](https://blog.youtube/inside-youtube/youtube-ambient-color-mode-visual-language-redesign/)):
  - Users wanted *"a cleaner, more lively design"*. Ambient mode: *"the app background color adapts
    to match the video. We were inspired by the light that screens cast out in a darkened room"*.
    Dark theme only.
  - How it is built: from thumbnails, *"pulled, stretched, blurred and scrimmed"*, with a neutral
    warm or cool glow on low-end devices, and kept so that *"the video and thumbnail remained the
    biggest visual influences"*.
  - Evidence: *"hundreds of concepts"* tested with *"tens of thousands of users via research,
    experimentation and diary studies"*, and *"For the first time, we were able to ascribe a value
    to aesthetics"*.
  - On the watch page: *"the main reason you come to YouTube is, well, to watch your favorite
    content. So we're making improvements to bring the focus back to the video player."*
  - User benefit. **The most detailed first-party account of an art-derived tint**; see §6.2.
- **2024 TV watch page** [1P]
  ([blog.youtube, 2024-03-13](https://blog.youtube/news-and-events/designing-a-richer-youtube-experience-for-your-tvs/)):
  research found viewers *"flow between leaning back to watch, and leaning in"*; three prototypes
  tested with a remote; *"obscuring the video would be detrimental"*, so the video shrinks beside
  the comments instead of being covered; *"prioritize simplicity over the introduction of additional
  lightweight controls"*. Partly commercial (shopping panels). **Transfers to Watch Together on a
  television**: a participant or voice panel shrinks the picture, on request, rather than covering
  it [I].
- **2026 design principles** [1P]
  ([blog.youtube, 2026-08-13](https://blog.youtube/inside-youtube/design-principles-use-put-creators-center-stage/)):
  *Make content the star; Prioritize relevance ruthlessly; Choose the simpler option; Be a good
  neighbor; Build for the ecosystem.* *"The interface shouldn't compete with the video"*; changes
  made *"without turning your hard-earned muscle memory upside down"*; *"We prioritize the
  consistency of the entire app over any single feature"*. Actions were cut by *"looking at what
  actions were used most"*. Mostly user; transfers directly (U19, D108).
- **2021 dislike count hidden** [1P]
  ([blog.youtube](https://blog.youtube/news-and-events/update-to-youtube/)): harassment of small
  creators; *"our experiment data showed a reduction in dislike attacking behavior"*. A public
  platform's problem; not applicable.

### 2.6 Hulu

- **2017 redesign** [J quoting 1P]
  ([The Verge, 2017-05-03](https://www.theverge.com/2017/5/3/15512650/new-hulu-tv-service-announced-live-shows-price-channels)):
  *"a low-density but highly visual design"* with *"a gradient splash that changes color as you
  browse"*, inspired by James Turrell, and thin type. Research from in-house labs, eye tracking and
  home visits; Jill Strawbridge: *"where they get quiet is quite telling because it generally means
  they're having to devote more attention to something."* User intent, art-led.
- **2018–2019, walked back** [J quoting 1P]: a *"good, old-fashioned channel guide"*, *"one of the
  most requested features"* ([2018-05-09](https://www.theverge.com/2018/5/9/17336158/hulu-live-tv-guide-channel-surfing-app-update));
  removing items from history and Keep Watching, so people can *"tell us when our recommendations
  ... don't hit the mark"* ([2018-05-16](https://www.theverge.com/2018/5/16/17360344/hulu-recommendations-watch-history-new-guide));
  a separate web layout for the *"wide range of motion you have with your mouse"*
  ([2018-09-20](https://www.theverge.com/2018/9/20/17882988/hulu-new-website-specifically-designed-computer-browsing-mouse));
  and in October 2019 Hulu *"improved the legibility of its home screen by toning down the bright
  backgrounds and text opacity"* [J]
  ([2019-10-16](https://www.theverge.com/2019/10/16/20917156/hulu-like-dislike-new-features-better-home-screen-search)).
  The Verge's verdict three years later: Hulu *"pushed too far in a new direction"* and spent *"many
  months reining in some of the changes and returning to what was familiar"* [J]. See §4.

### 2.7 HBO Max and Max

- **Hover previews, 2024** [1P]
  ([WBD](https://press.wbd.com/us/media-release/hbo-max/max-updates-homepage-user-interface-enhanced-video-elements)):
  previews to *"confidently decide what to watch"*, and *"To provide subscribers with control over
  their experience, users will be able to toggle the video and audio on or off"* from the first day.
  Commercial. The transferable part is only that anything that moves on its own ships with its
  switch.
- **Max (2023) back to HBO Max (2025)** [1P]
  ([WBD, 2025-05-14](https://press.wbd.com/us/media-release/warner-bros-discovery-announces-max-become-hbo-max-summer);
  [The Verge](https://www.theverge.com/news/666707/hbo-max-returning-rebrand-warner-bros-discovery)):
  Casey Bloys, in WBD's own release: *"we believe HBO Max far better represents our current consumer
  proposition."* The release adds *"no consumer today is saying they want more content, but most
  consumers are saying they want better content"*, and JB Perrette wants *"not everything for
  everyone in a household, but something distinct and great for adults and families."* Brand
  strategy, not UI: the release gives no interface reason, and the app's store note said *"Same app. New-ish name"* [S]. The
  only lesson is that a rename costs the recognition a name had built (Orpheum's name is
  provisional, D195).

### 2.8 Google TV and Roku

- **Google TV, 2020** [1P]
  ([blog.google](https://blog.google/products/google-tv/entertainment-you-love-google-tv/)):
  *"it can take a long time just to find something to watch. That's why we made Google TV"*; one
  watchlist added to from a phone *"will be waiting on your TV when you get home"*. A sibling post
  claims *"over 11 minutes to find something to watch"* with no source
  ([2020-08-05](https://blog.google/products/google-tv/find-new-faves-faster/)), and admits the
  teasers are *"sponsored by media partners"*.
- **Google TV profiles, 2021** [1P]
  ([blog.google](https://blog.google/products/google-tv/more-you-personalized-profiles/)):
  *"Everyone brings their likes, dislikes and 'no WAYs' to the sofa"*; each account's watchlist stays
  separate. User; Orpheum is per person already.
- **Roku** [1P via interview]
  ([Fast Company, 2018-05-14](https://www.fastcompany.com/90171888/jana-kovacevic-roku-designer)),
  Jana Kovacevic, interaction design manager: TV is *"a lean-back experience ... They don't want to
  sit in front of the TV and solve problems"*; *"even if a user makes a mistake, we always offer them
  a path forward. A customer should never feel like they've hit a dead end"*. User; transfers (U18).
- **Roku's new home screen, 2026** [1P]
  ([press release, 2026-05-27](https://www.businesswire.com/news/home/20260527563154/en/Roku-Unveils-New-TV-Home-Screen-for-the-Next-Era-of-Streaming)):
  *"the first significant update of the Roku Home Screen in over a decade"*, which *"maintains Roku's
  signature simplicity"*; 82% of those asked *"would love if they turned on their TV and the show they
  wanted to watch was right on their Home Screen"*.
  - Preston Smalley, VP product, to Fast Company [1P via J]
    ([Fast Company](https://www.fastcompany.com/91548233/why-rokus-iconic-home-screen-finally-got-a-reboot)):
    *"One of the things we found is that not very many people actually customized those app screens
    ... They'd end up scrolling all the way to the bottom of this long list."* For those who did,
    *"there are ways for them to do that even in this design"*; a setting restores the old grid [J].
    Recommendations are *"heavily"* weighted to what a person already has.
  - Margaret Schmidt, VP UX and Design, to TechRadar [1P via J]: *"Simplicity has always been our
    north star"*.
  - The commercial half, from Anthony Wood on the February 2026 earnings call [1P via J]: *"We believe
    it will increase monetization over time"*.
  - Transfers: few people customise, so the defaults must be good; the old layout survives as a
    setting (D108); a home screen weighted to what the person already has is Orpheum's case
    entirely.

### 2.9 Spotify (stated rationale only)

- **Home with preview feeds, 2023** [1P]
  ([newsroom](https://newsroom.spotify.com/2023-03-08/new-home-page-scroll-clips-previews/)): the
  feeds are for discovery (commercial), but *"your Favorites aren't going anywhere. Your shortcuts,
  or most recently played, will still appear at the top of your Home feed"*. Even a discovery-led
  redesign kept resume on top.
- **Black-on-green play buttons and sentence case** [1P]
  ([spotify.design](https://spotify.design/article/better-in-black-rethinking-our-most-important-buttons)):
  black on green reaches 10.9:1; upper-case labels ran out of room in 60+ languages, sentence case
  *"takes up 20-30% less space"* and *"feels less shouty, more human"*. Evidence: measured contrast
  and research with disabled users. Pure user benefit; transfers to every Orpheum label.
- **The now-playing bar** [1P]
  ([spotify.design](https://spotify.design/article/small-but-mighty-weve-rolled-out-changes-to-the-now-playing-bar)):
  *"It also wasn't clear to all listeners that when you tapped the NPB, it would lead to a more
  immersive 'now playing view'"*. Designers argued *"for prioritizing design- and experience-driven
  work over metrics-based feature improvements"*. Transfers: a mini-player must look like it opens.
- **Machine learning in design** [1P]
  ([spotify.design](https://spotify.design/article/three-principles-for-designing-ml-powered-products)):
  automating a choice away *"shouldn't be your default ... first consider whether augmentation—or
  assisting—might result in a better user experience."*
- **Blend, social listening** [1P]
  ([spotify.design](https://spotify.design/article/spotify-blend-designing-for-a-social-listening-experience)):
  *"there's still something very intimate and private about music"*; *"In designing social
  experiences, we advocate for user control"*, opt in and opt out. Transfers to what a Watch
  Together guest sees of the host's library and history [I].
- **Design principles, 2013 to 2019** [1P]
  ([spotify.design](https://spotify.design/article/introducing-spotifys-new-design-principles)):
  six principles became three because there were *"just too many principles to keep track of"*.
  A note for M5's principles document: fewer survive use.

### 2.10 Plex, Jellyfin, Infuse, Channels, Emby, Kodi

- **Plex's "new experience", in its own words over two years** [1P]:
  - 2024-11-22, launch ([blog](https://www.plex.tv/blog/the-new-plex-experience/)): *"Through
    extensive testing, we found that this new structure not only helps existing users navigate more
    intuitively but also gives new users a much clearer sense of what Plex is all about"*; goodbye
    to *"hidden hamburger menus"*; a single codebase because separate ones *"slowed development"*;
    and users asked to spend *"a week or two, or even three"* to *"fully settle in"*. No figures.
    New-user comprehension and engineering speed are company reasons; Watchlist and Discover feed
    Plex's own streaming business.
  - 2025-09-18 ([forum, 4,759 likes](https://forums.plex.tv/t/roku-new-experience-release-update/931239)):
    *"at this time we aren't planning on rolling back this update"*. Features users thought removed
    were behind the remote's `*` key; *"we are re-evaluating how we can make this option more
    visible"*. Favourites moved to the account *"to provide a more consistent experience across all
    apps"*, which *"prevents different setups between different devices"*.
  - 2025-11-05 ([forum](https://forums.plex.tv/t/plex-roku-preview-a-faster-simpler-navigation-experience/933212)):
    *"While we won't be returning to the old sidebar layout, we have taken that feedback to heart"*;
    again, use it *"ideally a full week"*.
  - 2026-08-17 ([forum](https://forums.plex.tv/t/navigation-updates-coming-to-preview-users/941751)):
    *"we have received consistent feedback from the start regarding the navigation. Our fall update
    was the first step ... but they weren't enough."* The primary navigation returns to a
    collapsible left side panel with favourite libraries.
- **Jellyfin web, 2025** [1P]
  ([PR #6582](https://github.com/jellyfin/jellyfin-web/pull/6582)): library links moved from a
  pinned sidebar into the header *"to reduce the amount of space wasted by the pinned sidebar ...
  that causes a lot of layout issues on smaller screens."* The opposite move to Plex's, for a
  different reason (space, not findability); both keep the person's library order.
- **Infuse 8** [1P]
  ([Firecore, 2024-10-15](https://firecore.com/blog/infuse-8-now-available)): browsing rebuilt
  because libraries grew to *"tens of thousands of massive 4K videos"*; on Apple TV the new layout is
  opt-in through a "Details View" option. A paid product with no ads keeping the old layout as a
  choice: [D108](../../decisions/working-notes.md) in practice.
- **Channels (DVR)** [1P]
  ([forum, 2021-03-07](https://community.getchannels.com/t/26479)), co-founder Eric Maddox:
  *"When redesigns happen, the old ways of how they worked aren't going to be preserved as we move
  forward. Change isn't the same as breaking things."* Also a deliberate convention: tap left or
  right to seek *"since this is what other video players are starting to standardize on"*. The
  counter-position to Infuse, from a product its users also like.
- **Emby Theater, 2015** [1P]
  ([Emby blog through its Tumblr mirror, 2015-10-12](https://embyapp.tumblr.com/post/131027714510/coming-soon-the-new-emby-theater/amp)):
  *"All functions are being designed around 6 buttons – up, down, left, right, ok, and back. Your
  family members and guests will have no trouble controlling the new Emby Theater."* *"Free of
  Clutter"*; and the detail screen was rebuilt because *"A common complaint ... is that it's pretty
  but slow to navigate"*. User; transfers: six buttons is the television client's whole budget, and
  "guests will have no trouble" is a household test. Emby's 2025 home-screen guide only explains
  customising (*"for some the default layout isn't the perfect setup"*); no reason for the default
  rows was found.
- **Kodi's Estuary, 2016** [1P]: Team Kodi
  ([Martijn Kaijser, 2016-03-07](https://kodi.tv/article/brand-new-look-future-kodi-versions/)):
  *"we want to keep the default skins as simple and easy to use as possible so we will not start
  adding every customization or feature that comes up"*; *"we tried to do our best to fit the most
  common use cases"*, and for the rest *"there is an abundance of other skins available"*. The old
  skin was kept for upgraders: *"not everyone would like to be forced to switch"*. Its designer,
  phil65, listed his requirements on the forum
  ([2016-11-16](https://forum.kodi.tv/showthread.php?tid=296531&page=4)): *"Easy to use on first
  use"*; *"Always move visual focus into the direction the user presses"*; type sizes *"in line with
  the major UX guidelines out there (Amazon, Google, Apple), but dont try to look like a clone"*;
  *"dont clutter the screen with needless information"*; *"consistent animations"*; no Python
  add-ons, *"just too fragile to rely on"*. Asked for an info-mode toggle: *"Nope, there cant be. It
  would make skin code much much more complex"*. And: *"If someone calls empty spaces 'wasted space',
  then you know that you arent talkin to experts."* After complaints: *"As we are not deaf to these
  comments ... phil65 went back to the drawing board"* (Kodi blog, 2016-11-18). Non-commercial and
  user; transfers strongly: focus follows the press, a simple default with an escape hatch, and an
  option refused when it would double the code (rule 11).

### 2.11 Co-watching: why they built it, how they gave control, why they stopped

| Product | Why built, in their words | Who controls | Why retired | For Orpheum |
| --- | --- | --- | --- | --- |
| **Teleparty** (Netflix Party) | CEO Shaurya Jain: students who watched together on campus ended up in long-distance relationships; *"I realized that there was an opportunity to build a better experience for them"* [1P via J] ([LA Times, 2021-08-05](https://www.latimes.com/business/technology/story/2021-08-05/teleparty-blew-up-during-the-pandemic-now-what)) | anyone | still running; demand peaks when students separate for the summer | **Its founding use case is Orpheum's**: two people apart. |
| **Plex Watch Together** (2020) | *"less about getting eyeballs and more about letting people reconnect"* [1P] ([blog](https://www.plex.tv/blog/coming-in-hot-watch-together-chill/)) | everyone: *"Before you ask, yes, 'that guy' can ruin it for everyone"* [1P] | 2025, with the client rewrite; the blog's note adds *"tremendous response"* and a possible return *"using new tooling"* [1P] | Not retired for lack of use. The cause, a per-client feature that did not survive a rewrite, is watch-together.md §4's. |
| **Disney+ GroupWatch** (2020) | Jerrell Jimerson: *"a way to safely connect virtually by co-viewing"* during the pandemic [1P via J] ([The Verge](https://www.theverge.com/2020/9/29/21492148/disney-plus-groupwatch-friends-family-us-subscribers-scener-netflix)). Michael Cerdà, VP product: *"In our research, one of the big takeaways was that people wanted to feel like they were a part of something, but we didn't want to get in the way of the content"*; so reactions only (six emoji) and no text chat, *"a purposeful decision"*, and up to seven people because more was *"too noisy"* [1P via J] ([dot.LA, 2021-01-27](https://dot.la/netflix-watch-party-2650144968.html)) | anyone [J] | 2023-09-18. The help-centre notice, quoted near-identically by four outlets: *"We're always working to deliver the best product experience on Disney+, and as part of recent updates, the GroupWatch feature was removed"* [1P via J] ([ComicBook](https://comicbook.com/irl/news/disney-plus-groupwatch-feature-no-longer-available/), [How-To Geek](https://www.howtogeek.com/disney-quietly-removed-its-groupwatch-feature/)); support pointed users to SharePlay | No stated reason; a pandemic feature quietly dropped. The guesses are journalists': low use after the pandemic, the price rises, and IGN's password-sharing crackdown [J]. **Cerdà's reason transfers**: to be part of something without getting in the way of the film. |
| **Prime Video Watch Party** (2020) | no rationale beyond the feature; desktop only, up to 100 people, text chat [J] ([TechCrunch](https://techcrunch.com/2020/06/29/amazon-prime-video-introduces-watch-party-a-social-coviewing-experience-included-with-prime/)) | **the host alone**: *"synchronized playback controlled by the host (keeping anyone from skipping ahead)"* [J] (Deadline, 2020-06-29) | 2024. Amazon's own page, as quoted and translated by [Gigazine](https://gigazine.net/gsc_news/en/20240306-amazon-prime-video-watch-party/) [1P via J]: *"Watch Party is scheduled to be discontinued as of March 31, 2024"* and *"there are no plans to provide similar functionality at this time"*; creation stopped on 2024-04-02 ([PC-WELT](https://www.pcwelt.de/article/2299367/prime-video-amazon-streicht-watch-party.html)). **Amazon gave no reason.** The notice that *"usage of Watch Parties on Twitch has declined"* ([IGN](https://www.ign.com/articles/twitch-will-end-its-prime-video-watch-party-feature-next-month)) is Twitch's, about Twitch's own Watch Parties, not Prime Video's | No stated reason, so nothing transfers. Host-only control suited rooms of 100. |
| **Hulu Watch Party** (2020) | Jason Wong, director of product management: *"streaming has inherently become even more social"*, and *"once we identified this need from our viewers, our team was very motivated to quickly deliver something"* [1P via J] ([dot.LA](https://dot.la/netflix-watch-party-2650144968.html)). It began as a hackathon project; in tests over 75% of hosts said they would do it again [J] | **each person their own**: viewers *"control their own playback without impacting the group"*, and anyone who paused or lagged *"can rejoin the party by hitting a button in the chat window marked 'click to catch up'"* [J] (Deadline, 2020-05-28); Hulu: participants *"react in real time through the chat function in addition to controlling their own playback"* [1P via J] (MacRumors) | Unknown. Hulu's help article was live through February 2024 and later redirected to a page that renders by script. Claims of a 2021 removal for low adoption come from SEO sites that contradict each other [S], and are not used | **Transfers strongly**: personal pause with a catch-up button is the model Apple's SharePlay later chose for interruptions. No Hulu statement says *why* control was personal. |
| **Discord Watch Together** (tested 2021, launched with Activities 2022) | Discord's blog [1P]: *"if you want to watch a video with everybody, you don't have to all hit 'play' at the same time like you're doing the world's lowest-stakes heist"* ([Activities](https://discord.com/blog/server-activities-games-voice-watch-together)). The press tied its revival to YouTube shutting down two music bots in 2021 [J] | the host by default, who may hand it to everyone. 2022 FAQ: *"For a more democratic viewing experience, the Controller also has the option to share the Remote"*; *"Volume changes are always local"*; if the controller leaves, control passes to *"the user who has been in the Activity the longest"*. 2024 FAQ: *"If the host wants others to be able to control the video playback, they can toggle to Sharing mode"*; **"We recommend Hosting when in big groups and Sharing when in small groups!"** [1P] (support pages, through archive.org) | live | **The most directly transferable line found**: two people is the smallest group, so shared control is right by Discord's own rule, as D271 §4 decided. Local volume transfers; so does handing control to the longest-present person, if Orpheum's host ever leaves. |
| **Kast** (2020) | Mark Ollila, CEO: *"we desire a communal experience, shared moments together"*; *"just like you were in your living room but now with the ability to chat and see each other while sharing the content"* [1P] (press release, 2020-08-25, via SHOOTonline) | whoever has *"kasting permission"*: a screen-share model, no reason given [1P] | — | The reason transfers; the screen-share architecture does not. |
| **Rave** | Michael Pazaratz, CEO: *"enable anyone, on any platform to watch any content together with friends in perfect sync"*; a 65-inch virtual screen in VR *"feels too isolating"* [1P] (The Ghost Howls, 2017-06-25) | a leader holds the privacy settings; *"If the leader leaves the rave, the next most senior participant will become the leader"* [1P]. Who controls playback: not found | — | "Any platform" is Biddle's device-breadth problem (below) seen as a goal. Succession by seniority matches Discord's. |
| **Netflix** (never built one) | Gibson Biddle, VP Product 2005–2010: Netflix "Party" on Xbox reached *"5% of Xbox members"* and was killed in 2010; a feature needs *"likely 20% of members"* to lift retention; *"five-percenters gunk up the experience and make it harder to use"* (edge cases such as *"Why can't I use 'Party' on devices other than my laptop?"*) [former 1P, personal opinion] ([LinkedIn, 2020-12-09](https://www.linkedin.com/pulse/why-wont-netflix-party-gibson-biddle)) | — | — | The 20% threshold is commercial and void here: for Orpheum co-watching is the point. **The complexity argument is not void**: every device it half-works on is a new complaint. |
| **Apple SharePlay** (WWDC21) | the goal is *"for your users to feel as if they were physically together"*; *"this does not stream media from device to device but synchronizes playback"* [1P] ([10225](https://developer.apple.com/videos/play/wwdc2021/10225/), [10184](https://developer.apple.com/videos/play/wwdc2021/10184/)) | everyone: *"if the user pauses, everyone else should pause, too"*; *"Try to keep your users together as much as possible"* | live | Three details transfer and are new to this tree (below). |

Apple's three details, from session 10225 [1P]:
- **A scrub's preview is local**: frames shown while scrubbing stay on the scrubbing device, and the
  new time is shared only on release, to *"keep random video frames from flashing in front of
  everybody"*.
- **An interruption is personal**: an alarm, a stall or an interstitial pauses only that device,
  which rejoins on its own (watch-together.md §1 has the mechanism).
- **Catching up is personal**: a "what happened?" button seeks one viewer back, plays faster, and
  rejoins. People *"will often prefer to stick with the group, even if it has some kind of
  drawback"*.
- Also, from session 10184: during SharePlay a person is *"essentially multitasking ... So
  automate as much as you can and minimize unneeded upfront interaction"*.

Across the table, two control models recur and both have their reasons: the host alone for large
rooms (Amazon's 100, Discord's *"big groups"*), and shared or personal control for small ones
(Disney+, Plex, Teleparty, Apple, Hulu, Discord's *"small groups"*). Scener's coverage gave a
business reason only.

### 2.12 Peacock

- **Vertical video, 2026** [1P]
  ([NBCUniversal, 2026-06-22](https://www.nbcuniversal.com/article/peacock-deepens-mobile-engagement-vertical-video-and-original-microdramas)):
  Matt Strauss, chairman of NBCUniversal Media Group, quoted from The Hollywood Reporter in NBCU's
  own release: *"We're trying to give people reasons to open up our app every single day, and we
  know the value of one incremental hour per user per month"*; the aim is to win *"more of that share
  of time"* and *"video calories"*. The feed is *"designed to deepen engagement among existing
  users"*. The plainest statement of the engagement motive found from any service; the
  counter-example for Orpheum.
- **Continuity across screen sizes, 2025** [1P]
  ([Android Developers Blog, 2025-05-20](https://android-developers.googleblog.com/2025/05/peacock-optimizes-streaming-jetpack-compose.html)):
  Diego Valente, head of mobile: *"users can start watching on a smaller screen and continue
  instantly on a larger one when they unfold the device—no reloads, no friction."* User and
  engineering; transfers: resizing the web player's window must not reload or restart the film [I].
- **TV navigation moved to the side, 2022** [J]
  ([9to5Google, 2022-05-02](https://9to5google.com/2022/05/02/peacock-redesign-announced-2022/)):
  the top navigation made it *"very difficult to switch categories as you scroll down through the
  rows"*. No Peacock staff were quoted.

## 3. What the big players converge on, and why

### 3.1 Reasons that transfer (user benefit)

| Pattern | Who said it | Their reason | Orpheum |
| --- | --- | --- | --- |
| **Resume first** | Netflix (2015 template, 2026 homepage, ui-competitors §1), Apple Up Next (2016), Spotify (2023), Roku certification and its 2026 home screen (82% want *"the show they wanted to watch ... right on their Home Screen"*), BBC iPlayer 2014 (*"lazy favouriting"*) | the person's own unfinished things are what they most often want | already D188; §6.2 on the banner |
| **Finding first; featured content gets out of the way** | BBC iPlayer 2014 (*"58% ... come ... with something particular in mind"*), Netflix 2023 (Play Something retired because users *"come to the service with a specific show, movie or genre in mind"*, a spokeswoman's account [J]) | most visits have a target | search and the library one step from Home; §6.1 |
| **A home screen that stays where it was** | Netflix 2015 (*"some stability so that people are familiar with their homepage"*), Netflix kids' row (2021), YouTube 2026 | people find things by place | rows in a fixed, person-chosen order (D138) |
| **Never a dead end** | Roku 2018, BBC iPlayer 2014 (a principle by name) | the lean-back viewer does not solve problems | U18 |
| **The person can edit the automatic lists** | Netflix 2022, Hulu 2018, Disney+ 2025, Wholphin (ui-competitors §1) | lists filled by the system are wrong about real households: accidents, finished titles, someone else on your profile | D188's remove with undo; the same applies to Next Up when it comes |
| **Navigation visible, not hidden** | Netflix 2025 (*"somewhat hidden"*), Plex 2024 (*"hidden hamburger menus"*) and its 2026 return to a side panel, Prime 2022 (*"didn't even know"*), Apple WWDC21 (discoverability) | what is not visible is not used | U2; Watch Together's menu entry |
| **Portrait posters for density** | Apple tvOS 26, Disney+ 2025, Prime 2022 | more titles visible at once | §6.2, poster size |
| **Art is the interface; keep it recognisable** | Netflix 2016 (82% of attention), Netflix 2017 (recognisability), YouTube 2022 (art stays the biggest influence) | people find a title by its picture | posters do not change under the person; one image per title |
| **Details on request, not all at once** | Apple WWDC19, Netflix 2024 (info in the focused tile), Prime 2024 (short synopses) | each extra element adds load | title page and cards carry little; the rest is one step away |
| **Calm: no forced sound, no overstimulation** | Apple WWDC19, YouTube 2026 (*"shouldn't compete with the video"*) | relaxed, focused viewing | D188 declines autoplay; U11 |
| **Per-person everything** | Netflix profiles, Google TV profiles, Disney+ (which profile am I in) | households share a screen, not a taste | already the model; show whose account is in use |
| **A switch for anything that moves on its own, from day one** | Max 2024, Netflix 2020, Roku 2024 | some people hate it | D188 already requires one if autoplay is ever proposed |
| **One-purpose controls that appear when relevant** | Netflix Skip Intro (ui-competitors §1), Apple tvOS 18 subtitles on skip-back | a single need, a single press | §6.1 |
| **One component system, so habits carry** | Netflix Hawkins, Spotify Encore, YouTube 2026 (*"consistency of the entire app"*) | learn once, use everywhere | rule 11; admin screens in the same skin |
| **Don't break muscle memory** | YouTube 2026, Spotify desktop (*"weren't too disruptive to existing habits"*), Infuse 8, BBC iPlayer 2014 (*"routes they're familiar with"*), Roku 2026 and Kodi 2016 (the old layout kept as an option) | familiarity is a feature | D108 |
| **Few people customise, so the default is the product** | Roku 2026 (*"not very many people actually customized"*), Kodi 2016 (*"fit the most common use cases"*) | settings serve the few | good defaults first; a setting when someone asks (§6.2, Q8) |

### 3.2 Reasons that do not transfer (engagement and commerce)

| Pattern | Who | Stated reason | Orpheum |
| --- | --- | --- | --- |
| Autoplaying previews and heroes | Netflix 2016, Max 2024, Prime 2024 | "browse less, watch more" by A/B | declined, D188 |
| Personalised artwork | Netflix 2017 | take rate | no; it fights recognisability |
| Popularity rows (Top 10) | Netflix 2020, Prime 2022 | shared moments, engagement | meaningless for two people |
| Vertical clip feeds | Netflix 2021, 2026; Spotify 2023; Peacock 2026 (*"one incremental hour per user per month"*) | discovery, time in app | no |
| The next episode starting by itself | Netflix Post-Play 2012 | *"easier to enjoy more"* | the next episode is offered; where it plays on, D188's "still watching?" bounds it (§5.7) |
| Brand, add-on and store tabs | Disney+ 2025, Prime 2024 | sell subscriptions | no |
| A rating scheme tuned for data volume | Netflix 2017 | 200% more ratings | no recommender to feed |
| Algorithmic row ordering | Netflix 2012–2015 | retention | rows in a fixed, person-chosen order (D138) |
| "Decision time" urgency (90 seconds, 11 minutes) | Netflix 2015–16, Google 2020 | churn | not a design target; time-to-picture *after* choosing is (U4) |
| Restrictions on playback speed | Netflix 2019 | filmmakers' objections | no such stakeholder; speed may persist per person |

## 4. Reversed or retired, and why

Extends ui-competitors.md §2 with the reasons, where they were found.

| What | First decision and its reason | Reversal and its reason | Lesson |
| --- | --- | --- | --- |
| **Netflix autoplay previews** | 2016: A/B tests and eye tracking showed less browsing | 2020-02-06: *"We've heard the feedback loud and clear — members can now control whether or not they see autoplay previews"* [1P, tweet via [Engadget](https://www.engadget.com/2020-02-06-netflix-autoplay-preview-settings.html)]; complaints came from members and from filmmakers [J, [The Verge](https://www.theverge.com/2020/2/6/21126867/netflix-autoplay-feature-disable-homepage-episodes-series)]. Still on by default. | A metric win can still be hated; the switch was the compromise between the metric and the people. |
| **Netflix thumbs** | 2017: binary thumbs for simplicity and data | 2022: "Two Thumbs Up" added; Christine Doig-Cardet: *"providing those mechanisms to give more of the control back to the user"* [1P via J] ([The Verge](https://www.theverge.com/2022/4/11/23017274/netflix-double-thumbs-up-button-recommendation-curation)) | Simplifying a person's expression too far takes control from them. |
| **Netflix interactive titles** | 2017: kids *"are eager to 'play'"*, from qualitative research [1P] | 2024: *"The technology served its purpose, but is now limiting as we focus on technological efforts in other areas"* [1P via J] ([The Verge](https://www.theverge.com/2024/11/4/24287857/netflix-removing-interactive-titles-games)) | A feature that needs special code in every client dies when the platform moves: the same cause as Plex's Watch Together. |
| **Netflix "Are you still watching?"** | 2012: Post-Play starts the next episode in 15 seconds [1P] | The check's reason stayed (*"so you don't lose your place"*); its threshold moved from two episodes (2020–2021) to three episodes or 90 minutes (2022 on) [1P, Help Center through archive.org]. No off switch was ever added; turning off autoplay of the next episode is the only route | A play-on default needs a bound; the bound was tuned, not removed. |
| Netflix Play Something / Surprise Me | 2021: *"There are times when we just don't want to make decisions"* [1P] ([Netflix](https://about.netflix.com/en/news/play-something-netflix-does-the-work-for-you)); Cameron Johnson: *"When finding your next story, let the story find you"* [1P via J] (Variety, 2021-04-28); Greg Peters: members can *"skip browsing entirely, click one button"* [1P via J] | January 2023. A spokeswoman told the WSJ it was dropped *"because of relatively low use"*, and that *"users tend to come to the service with a specific show, movie or genre in mind"* (the reporter's paraphrase [J]). Netflix's statement: *"We will continue to explore other ways to give members more options and ways to explore and discover content they want to watch"* [1P via J]. Both read second-hand, through Slashdot's and Tom's Guide's copies of the paywalled story [S] | Most visits have a target: the same finding as BBC iPlayer's 58% (§5.4). A "pick for me" button serves the rare visit. |
| **Hulu's 2017 redesign** | art-led, colour-shifting gradients, thin type, a mixed live and on-demand Lineup | 2018–2019: the channel guide returned, history and Keep Watching became editable, a mouse-specific web layout, and colour and text opacity were toned down for legibility [J quoting 1P] | A bold look loses to legibility, and a ten-foot design does not suit a mouse. |
| **Prime Video navigation** | 2022: a left rail | 2024: a top bar with a tab per add-on [1P] | Placement churned with the business (add-ons), not a user finding. |
| **Plex navigation** | 2024: top navigation, *"extensive testing"*, *"settle in"* | 2026: *"consistent feedback from the start ... they weren't enough"*; the side panel returns [1P] | "Give it a week" did not dissolve the complaint. When the same complaint persists for months, it is about worse, not about change (ui-sentiment §6). |
| **Plex favourites** | 2025: account-wide, for consistency | admitted the same day: *"prevents different setups between different devices"* [1P] | Consistency across devices can take away a choice people used. |
| **Max name** | 2023: "Max", for families [J] | 2025: back to HBO Max; Casey Bloys in WBD's release: *"HBO Max far better represents our current consumer proposition"* [1P] | A rename costs recognition. |
| **Apple's Up Next** | 2016: one list, *"in the order they are most likely to watch first"* [1P] | 2024 (18.1): split into Continue Watching and Watchlist, with no stated reason [1P release note]; *"quite confusing for users coming to the TV app for the first time"* [J, 9to5Mac] | What I'm watching and what I mean to watch are two lists. |
| **Roku's home screen** | for over a decade: what Smalley calls *"a static app grid"* [1P via J] | 2026: dynamic and weighted to what the person has, because *"not very many people actually customized"* [1P via J]; the old grid stays as a setting [J] | Change the default; keep the old one for those who built it. |
| **Co-watching** | 2020: connection during the pandemic | Disney 2023 (*"as part of recent updates"*, no reason), Amazon 2024 (no reason; *"no plans to provide similar functionality"*), Plex 2025 (client rewrite, despite *"tremendous response"*) | Two retirements are silent; the one with a stated cause names a rewrite. Twitch's "usage declined" was about Twitch's watch parties, not Amazon's. |
| **Removing from Continue Watching** | lists filled only by the system | Netflix 2022, Hulu 2018, Disney+ 2025: *"You asked, we listened!"* | Every big service learned this late. |

## 5. The contrarians

### 5.1 MUBI: a small set, chosen by someone you trust

- **What it did:** for years, 30 films, one added and one leaving each day.
- **Why, in its founder's words** [1P via J]: Efe Çakarel compares MUBI to *"the 'staff picks'
  section at your favorite video store ... you don't care how many thousands of titles they have,
  you only go for the staff picks ... we are a trusted advisor"* and invites the reader to *"Think
  about your own Netflix experience and how frustrating it is — how long it takes you to find a
  film that you want to watch"*
  ([The Verge, 2015-10-06](https://www.theverge.com/2015/10/6/9463225/mubi-streaming-service-independent-films-efe-cakarel-interview)).
  In 2016: *"out of the 2,000 films they have, I can't find ten that I want to watch"*, and *"I
  really need someone to choose for me"*
  ([Freunde von Freunden](http://www.freundevonfreunden.com/workplaces/efe-cakarel-on-curating-art-house-films-in-the-binge-watching-era/)).
- **Commercial too:** The Verge notes that 30-day windows also cut licensing costs [J], and by his own
  account the model was born when MUBI nearly failed.
- **The library behind the daily film, 2020** [1P via J]: in May 2020 MUBI opened a library of
  films it had shown before, while Now Showing *"will continue to introduce a new film every day"*
  ([Variety, 2020-05-21](https://variety.com/2020/digital/news/mubi-library-section-1234614213/), [J]). Çakarel's reason is rights, not research
  ([Screen Daily, 2020-12-28](https://www.screendaily.com/features/perspectives-on-2020-efe-cakarel-on-mubis-boom-and-shattered-windows/5155855.article)):
  *"If you have the rights for 12 years to a film like Limbo, it doesn't make sense to just show it
  for 30 days"*; and *"We are never losing our one-new-film-everyday, but now we just open up the
  world behind our kitchen a little bit."* Commercial (licence economics). The daily pick stays the
  front door; the library is behind it.
- **What Orpheum can take** [I]: a household's library is already chosen, so the "too much choice"
  problem is smaller. What carries over is the *trusted advisor*: in a two-person home the person
  who picks is the other person. A shelf one person fills for the other ("Karl suggests") is the
  household form of staff picks, with no algorithm. MUBI's 2020 shape is the household's too: a
  small front shelf, with the whole library behind it. It belongs with the person-arranged Home of
  [D138](../../decisions/working-notes.md), not in M5 unless the owner wants it.

### 5.2 Apple's living-room designers against their own industry

WWDC19 session 211 (§2.4) is a contrarian statement from inside a big company: rows *"and rows and
rows"* paralyse, sound must not be forced, overstimulation is like *"11 people ... talking to you
loudly"*. It is the closest thing to a published "calm technology" stance by a major video service.
Apple still ships autoplaying Home previews on tvOS (2019 [1P]); the design team's principles and the
product's choices diverge, which is itself the lesson: the principles held where nothing was for sale.

### 5.3 Infuse and Channels: two answers to "keep the old layout?"

Infuse 8 kept the old detail view as an option; Channels' co-founder said old ways *"aren't going
to be preserved"* and that *"Change isn't the same as breaking things"* (§2.10). Both are liked by
their users. The difference [I]: Channels fixed things its users had named (search hidden on its own
page), so the change was visibly *better*; Infuse's new layout was a matter of taste (*"a little busy
and cramped"*, ui-design-practice §3.1), so the choice was kept. That is the line D108 needs: a fix
replaces; a change of taste is offered.

Kodi and Roku took Infuse's side (§2.10, §2.8): Kodi kept its old skin for upgraders in 2016
because *"not everyone would like to be forced to switch"*, and Roku's 2026 home screen keeps the old
grid as a setting for the few who had arranged it [J]. Kodi also drew the other line: a toggle that
*"would make skin code much much more complex"* was refused. An option is kept when it is the old
default, not when it doubles the code.

### 5.4 BBC iPlayer: find first, never a dead end

A public service with no ads, so its reasons have no engagement motive to discount. Kutlu
Canlıoglu, creative director for TV and iPlayer, on the 2014 redesign [1P]
([BBC Internet Blog, 2014-03-17](https://www.goodreads.com/author_blog_posts/5929307-new-bbc-iplayer-redesign-insights),
read through Goodreads' syndicated copy):

- Four principles: *"1. Simplify finding 2. Encourage discovery 3. Pave the way to a more personal
  iPlayer 4. Never a dead-end"*.
- *"58% of our users come to iPlayer with something particular in mind to watch."* So discovery
  must not get *"in the way of those users who have come with something specific in mind"*: *"if you
  use the find tools, the featured content dims down to get out of your way."*
- Recently Watched *"provides a 'warm trail' into programmes. We think of this as a form of 'lazy
  favouriting'"*.
- *"most journeys in the new iPlayer are biased towards getting users to the latest episode as
  quickly as possible. So the first user need on the playback page is to establish that this is the
  right episode"*; on TV the controls start expanded *"to help you make sure you're on the correct
  episode"*.
- At the end, *"we display the next episode if it's already available, and give you
  recommendations"*: offered, not started.
- *"once someone is using iPlayer on one device, they'll be familiar with all instances of iPlayer
  across all their devices"*, and existing users *"can still find what they're looking for using
  routes they're familiar with"*.

User throughout, and it transfers almost whole: finding first, the right-episode check, recently
watched as a trail, no dead ends, one shape across devices. The 58% agrees with Netflix's reason for
retiring Play Something (§4).

### 5.5 Criterion Channel and Kanopy: curated by people, sized to finish

- **Criterion Channel** [1P via J], at its 2019 launch. Peter Becker, president
  ([IndieWire, 2019-04-08](https://www.indiewire.com/features/general/criterion-channel-after-filmstruck-1202056861/)):
  *"It's not just a matter of making it available. It's a matter of making it navigable, making it
  discoverable"*; *"I'm not saying that algorithms are terrible, but they're not good engines for
  discovery. They're not going to lead you on an adventure"*; big platforms are *"doing a lot to
  engineer your experience and behavior"*. Penelope Bartlett, programmer
  ([Collider, 2019-04-08](https://collider.com/criterion-channel-interview-programmer-penelope-bartlett/)):
  *"we're trying not to present a series with 25 or 30 titles ... Because we think often people are
  sort of overwhelmed by the amount of choices"*, so *"curate things down to a point where you can
  actually watch all of the films"*; and *"we really want the channel to feel like it was curated
  thoughtfully by real people, not by algorithms."* User, with brand differentiation beside it.
  Transfers: collections small enough to finish, and "navigable" before "available"; an editorial
  staff does not.
- **Kanopy** [1P via J]. Olivia Humphrey, founder
  ([IDA, 2017-09-05](https://www.documentary.org/online-feature/conversation-kanopy-founder-and-ceo-olivia-humphrey)):
  *"being able to press Play very quickly after arriving on our platform is one of our key metrics"*;
  *"We want it to be time well spent, when you finish a film and you think that was really
  worthwhile."* The model is pay per play, *"the libraries don't pay anything unless the films are
  watched"*, so the monthly play caps are a cost control set by libraries; no first-party statement
  calls them a design choice against bingeing [S]. Transfers: time to play as a goal (U4), not as
  telemetry; the caps do not.

### 5.6 Letterboxd: algorithms that help, not ones that pull

Karl von Randow, co-founder [1P via J]
([CULTURED, 2026-04-23](https://www.culturedmag.com/article/2026/04/23/film-letterboxd-founders-matthew-buchanan-karl-von-randow/)):
*"our product decisions are about how we support things that people already want to do. We're
creating algorithms to help people find what they love, rather than to suck them in."* Matthew
Buchanan on the origin [1P]
([Letterboxd Journal](https://letterboxd.com/journal/a-tiny-announcement-matthew-buchanan/)):
*"a social film diary, perhaps with lists"*, and the product *"still very closely resembles the
product that we launched with"* ([Idealog, 2023](https://idealog.co.nz/venture/2023/02/letterboxd-the-kiwis-behind-cinemas-most-influential-platform)).
User; transfers as a principle: support what people already do (log, list, return). A design quote
widely attributed to Buchanan (*"a film magazine you could carry in your pocket"*) has no traceable
source and is not used.

### 5.7 A critic: Tristan Harris on autoplay

Not a competitor, but the clearest statement of the case against autoplay [1P]
([tristanharris.com, 2016-05](http://www.tristanharris.com/2016/05/how-technology-hijacks-peoples-minds%e2%80%8a-%e2%80%8afrom-a-magician-and-googles-design-ethicist/)).
Under *"Bottomless bowls, Infinite Feeds, and Autoplay"*: *"Take an experience that was bounded and
finite, and turn it into a bottomless flow that keeps going"*; sites *"autoplay the next video after
a countdown instead of waiting for you to make a conscious choice (in case you won't)"*; *"Tech
companies often claim that 'we're just making it easier for users to see the video they want to
watch' when they are actually serving their business interests."* His alternative: *"imagine if
technology companies empowered you to consciously bound your experience"*. The section leans on a
Wansink bowl study; other Wansink papers were later retracted, so that evidence is not carried here.

For Orpheum [I]: Harris's test fits Netflix's own words. Post-Play's *"easier to enjoy more"* (2012)
is the claim he describes, and "Are you still watching?" (§2.1) is a bound added afterwards.
Playing on is acceptable where the person set the bound; D188's question after three episodes is
the floor, and D188 already declines previews and heroes that start themselves.

### 5.8 Still not found

No product was found that explicitly rejected algorithmic *artwork* in its own words; Criterion and
Letterboxd reject algorithmic *curation* (§5.5, §5.6). YouTube's "Take a break" reminder is on by
default only for users aged 13–17 ([support](https://support.google.com/youtube/answer/9012523)), a
wellbeing default limited to minors. A 2019 US bill to ban autoplay and endless scroll as dark
patterns
([The Verge](https://www.theverge.com/2019/7/30/20746878/josh-hawley-dark-patterns-platform-design-autoplay-youtube-videos-scrolling-snapstreaks-illegal))
is third-party criticism, not a competitor's choice. The Center for Humane Technology's current
design page says nothing specific about autoplay.

## 6. Implications for Orpheum's M5

### 6.1 Implications, each tied to its sources

1. **Judge each borrowed pattern by its reason, not its owner.** Of the convergent patterns, the
   ones with user reasons (§3.1) are worth copying even from a service whose business is the
   opposite of Orpheum's; the ones with commercial reasons (§3.2) are not, however polished. This is
   the filter for every "Netflix does X" proposal during M5's iteration.
2. **Subtitles for a few seconds after a skip back, and on mute** (Apple tvOS 18, §2.4). A
   one-purpose control with no setting, answering "what did they say?". It is new to this tree and
   cheap in a web player. A candidate for M5's player or the feature inventory; the owner decides.
3. **Playback speed as a person's setting, not reset per title** (§2.1). Netflix's limits were
   concessions to filmmakers. If Orpheum offers speed, nothing argues for the reset [I].
4. **Information where the eye already is** (Netflix 2024's *"gymnastics with their eyes"*, Apple's
   details on request). On Home, a focused or hovered card may show its title, year and progress in
   place; a separate panel at the top of the page makes the eye travel.
5. **Watch Together's details from SharePlay** (§2.11): a guest's scrub previews locally and sends
   the seek on release, which D268 §4's controls can do from the start; and the seam for a personal
   "catch up" later, which Hulu shipped in 2020 as a personal pause and a "click to catch up"
   button. Volume stays each person's own (Discord). On a television, a participant panel shrinks
   the picture rather than covering it (YouTube 2024). Disney's reason for reactions without chat
   holds for the voice and event line too: *"we didn't want to get in the way of the content"*.
6. **Labels in sentence case, contrast measured** (Spotify, §2.9), and **a mini-player that looks
   like it opens** if one is ever built.
7. **Navigation that does not hide** (Netflix 2025, Plex 2024–2026, Prime 2022): the menu split of
   m5.md is right, and nothing reachable only by a long press, a hidden key or hover (Plex's `*`
   key).
8. **Fewer principles survive use** (Spotify went from six to three): M5's principles document
   should rank U1–U25 into a short list that is checked, with the rest as reference [I].
9. **Iteration with two people** (§4, Plex): when the owner or his partner repeats a complaint
   across sessions, treat it as a defect, not as adjustment to change; Plex's "settle in for a week"
   was wrong for nine months.
10. **Finding first** (BBC iPlayer's 58%, Netflix's reason for retiring Play Something, §4, §5.4).
    Search and the library are one step from Home, and anything featured gives way once the person
    starts looking. A "pick for me" button is not worth M5's time.
11. **A home screen that stays put** (Netflix 2015, §2.1; Roku 2026). Rows keep their order and
    place between visits (D138); good defaults matter more than customisation, because few people
    customise.
12. **The playback page first confirms the episode** (BBC iPlayer, §5.4): the title, season and
    episode are visible at the start, so a wrong pick is caught before it plays on. The end of an
    episode offers the next one; where it plays on, D188's "still watching?" is the bound (§5.7).
13. **Collections small enough to finish, and rows that differ in shape** (Criterion, §5.5; Disney
    2021, §2.3) [I]: a hand-made collection is a short list, and Home should not repeat one row
    shape all the way down.

### 6.2 The open M5 questions

- **Q7, a static resume banner on Home.**
  - **Against a separate banner:**
    - Netflix's 2024 redesign moved the details out of the top billboard into the focused tile
      because of eye travel (§2.1).
    - Apple's designers call extra modules competition for attention (§2.4).
    - Netflix's own homepage finding is that rows with larger artwork reduce scrolling, and a
      Continue Watching row near the top has high stopping power (ui-competitors §1). Its 2015
      template already put Continue Watching first, and *"the upper left"* is what is seen (§2.1).
    - Roku's 2026 research asked for *"the show they wanted to watch ... right on their Home
      Screen"*, which a first row answers without a separate surface (§2.8).
  - **For one:** nobody's reason is promotion-free; every big hero is commercial.
  - **Lean:** Continue Watching as Home's first row with larger art than the rows below, and no
    separate banner. If a banner is still wanted, it is that row's first item, static, with
    *Resume at 41:12*, which is what ui-design-practice §5.1 item 7 already bounds [I].
- **Q8, a poster-size choice in Cut A.**
  - No big service offers a size choice; each picks portrait art for density (Apple, Disney+, Prime,
    §3.1).
  - Netflix's evidence that small art punishes complex images argues for a size large enough to read
    a poster, not the smallest that fits.
  - D271 §2 already makes poster size a person's setting eventually.
  - Roku found that few people customise (§2.8), so the default size is the one most people will
    ever see.
  - **Lean:** one well-chosen portrait size per window class first; the setting when a user asks for
    it [I].
- **Q9, the artwork's colour tinting the title page.**
  - **For, done YouTube's way:** YouTube's ambient mode is a first-party precedent (blurred and
    scrimmed, dark theme only, a neutral fallback for weak devices, and the art kept the strongest
    element). Vulture reports that Netflix's 2025 TV page reflects the art's colour too [J].
  - **The caution:** Hulu's 2017 colour-shifting gradients were toned down two years later for
    legibility [J].
  - **Lean:** a tint is defensible only as a dim, blurred field behind the page's top, with every
    text pixel on the neutral surface or a measured scrim, and a neutral fallback when the art is
    missing or its colour fails contrast. The page is dark throughout (D271 §3), which is the case
    YouTube built for [I].
- **Guest controls in Watch Together.** Decided by D271 §4: a guest may play, pause and seek
  unless the host withholds it. The sources agree with the decision and add four things to build:
  - **Named events.** Everyone's control is the norm for small groups (Apple, Disney+, Plex,
    Teleparty); Amazon's host-only design, built for 100, is the one that was retired. Discord, which
    offers both, states the rule outright: *"We recommend Hosting when in big groups and Sharing when
    in small groups!"* (§2.11). Plex's *"'that guy' can ruin it for everyone"* is the cost, and the
    host's per-invite rights are Orpheum's answer to it. The event line naming who paused (U22) is
    what makes shared control tolerable.
  - **A personal pause, then catch up.** Hulu let each viewer pause without stopping the group and
    rejoin with a "catch up" button (2020); Apple's suspensions are the same model. It is the seam
    for a guest whose right to pause the group was withheld [I].
  - **Scrubs stay local until release** (Apple, §2.11).
  - **An interruption is the interrupted person's own** (Apple's suspensions). How a stall is
    handled is already D246's territory and is not reopened here.

## 7. Where the evidence is weak

- **Most rationale is post hoc and promotional.** Newsroom posts announce a decision and give the
  flattering reason; the losing alternatives and the metrics are rarely published. Netflix's
  TechBlog and Apple's WWDC sessions are the exceptions, and even they omit numbers.
- **"Extensive testing" is never shown.** Plex, Amazon and Netflix all cite tests without figures;
  Plex's promised post on its testing was not found.
- **The 90-second figure** rests on unpublished consumer research, hedged in the paper and unhedged
  later. Google's "11 minutes" has no source at all. Neither should be cited as a measured fact.
- **Retirement reasons are mostly missing.** Disney said only *"as part of recent updates"*; Amazon
  gave none (the "usage declined" notice is Twitch's, about Twitch's feature); Apple gave none for
  splitting Up Next. Netflix's reason for Play Something is a spokeswoman's, paraphrased by the WSJ
  and read here only through Slashdot's and Tom's Guide's copies. IGN's link between GroupWatch and
  the password-sharing crackdown is a journalist's inference.
- **Some sources were weaker and are flagged or left out.** Disney's 2019 Investor Day transcript
  was decoded from a PDF without a text layer by letter substitution, with the speaker inferred
  (§2.3). Amazon's Watch Party notice is read in Gigazine's translation from Japanese. A quote that
  search results give to Netflix's Cameron Johnson about *"linear TV"* has no located original and
  is not used; nor is a design quote attributed to Letterboxd's Buchanan by a design guide (§5.6),
  nor Harris's Wansink evidence (§5.7). Claims that Hulu retired Watch Party in 2021 for low
  adoption come from SEO sites that contradict each other.
- **Biddle's account of Netflix "Party"** is a former executive's personal essay, ten years after
  the fact.
- **Several items are journalists quoting staff** (The Verge on Hulu, Prime and Netflix's 2024
  test); the quotes are first-party, the framing around them is not, and the framing is marked [J].
- **Search was unavailable in the first pass.** Its discovery went through archive.org's URL index,
  Hacker News's Algolia API, Discourse APIs and known URLs. The second pass had web search and
  filled most of the gaps; what is still not found may yet exist (§8).

## 8. Not tried

What is still unreached after the second pass, and why:

- **Netflix:** the WSJ story on retiring Play Something and its MarketWatch copy (paywall and script
  wall, no archive copy), so the reason is read second-hand; any designer's statement on that
  retirement; the about.netflix.com Moments post (404, no archive copy; Tudum's was used).
- **Apple:** any first-party reason for splitting Up Next; the release notes are silent and no
  Newsroom post or WWDC session was found.
- **Disney:** the GroupWatch help page itself (it redirects to the Help Center's home, and
  archive.org holds no copy); a designer's case study on the brand tiles (password-protected).
- **Amazon:** the US and Japanese Watch Party pages render by script, so Amazon's notice is read
  through Gigazine and PC-WELT; the English help text only through a search summary (Neowin
  returned 403).
- **Hulu Watch Party:** whether and when it was retired (the help page renders by script), and why
  control was made personal.
- **Rave:** who controls playback (its FAQ is silent).
- **Peacock:** its own reasons for the 2022 TV navigation (Streamable's articles are behind a
  paywall).
- **Kanopy:** any statement that its play caps are a design choice.
- **Letterboxd:** the Letterboxd Show transcript with both founders (403, no archive copy).
- **BBC:** iPlayer's 2021 statement on autoplay, which was a post on x.com.
- **Emby:** any reason for its default home rows.
- **Not attempted:** conference talks without transcripts (Netflix's RecSys and design-conference
  talks on video), patents, Reddit, and any logged-in session of the services to check what they
  ship now.
