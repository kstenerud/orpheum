# Orpheum

*(Provisional name — [D195](docs/contributors/decisions/working-notes.md).)*

A personal media server for **video**, on your hardware, without a company between you and your
files.

**Status: ready to build.** The repository holds the research, the design, the build contract
for the first release ([`cut-a.md`](docs/contributors/design/plans/cut-a.md)), the standards, and
the code skeleton with every gate in place. Feature code starts at milestone M1. To build, read
[`AGENTS.md`](AGENTS.md) and [`CONTRIBUTING.md`](CONTRIBUTING.md), then run `make check`.

Start at **[`docs/contributors/design/overview.md`](docs/contributors/design/overview.md)**,
**[`docs/ROADMAP.md`](docs/ROADMAP.md)** for the implementation plan, or
**[`docs/FEATURES.md`](docs/FEATURES.md)** for every feature we know of, ranked — including
what we have declined and what would make each one possible again.

## What it is for

A player for media you have the right to use. It does not provide, locate or download content,
and it does not remove copy protection. You are responsible for complying with copyright law where
you live, including when you share access with others. *(Draft wording, pending legal review — see
[D187](docs/contributors/decisions/working-notes.md).)*

## The promise

Product commitments, not aspirations:

- **No telemetry.** Off by default; opt-in only if ever added.
- **No storefront.** No ads, no upsell, no recommendations for content you don't own.
- **We won't move your furniture.** A layout change is opt-in and the previous one stays
  reachable for at least a release — and **every client build is published as a directly
  installable file, with old versions kept up**, so a bad update is something you can back out
  of rather than something you petition your television's manufacturer about. The loudest
  topic on Plex's own forum in the past year is *"Vote to roll back to Plex Classic!"* with
  1,124 likes; the second-loudest thing their Fire TV users needed was instructions for
  installing last month's app. See [D108](docs/contributors/decisions/working-notes.md).
- **Your files stay yours.** Media is read-only by default — and **running that way
  permanently is a supported choice**, not a cautious setting: you get the whole product minus
  deleting and minus writing sidecars. If you do grant write access, **deleting moves to a
  trash you can undo from**, and nothing empties it but you. The database is a rebuildable
  index, never the authority. Your data survives us two ways, and they do different jobs: an
  **automatic backup** of everything that cannot be re-derived — watch state, preferences,
  rules, shares — which is on by default and restores anywhere; and, if you grant a writable
  path, **sidecar files beside your media** carrying your metadata corrections in **NFO**, the format any
  other media server can read. The first protects you from a dead disk. The second protects you
  from *us*. Per-user state stays out of the sidecars deliberately — the media tree is shared,
  and nobody else in the house needs to see what you watched. **We read the `.nfo` files you
  already have, and we write them back** — so arriving from Kodi keeps the corrections you
  already made, and leaving for Kodi keeps the ones you made here. See
  [D98](docs/contributors/decisions/working-notes.md) and
  [D141](docs/contributors/decisions/working-notes.md).
- **We would only ever charge for things that genuinely require another machine.** Everything
  that can run on your own hardware does, free, forever — and no feature that works locally
  will ever be moved behind a payment.

That last one replaces a flat "no accounts ever", which turned out not to survive contact with
reality: two machines behind different NATs cannot find each other without a third party, and a
home server cannot get a warning-free HTTPS certificate without one either. Where that is true
we say so, we let you run the third party yourself, and we never paywall anything that was
working without it. See [D25](docs/contributors/decisions/working-notes.md).

Concretely, for the one thing that does need another machine: **the relay is a mode of this same
binary**, and a €5 VPS running it carries roughly 2,200 film-hours a month at 10 Mbps, uncapped
— about five times the bitrate of Plex's paid relay. If we ever host one, it runs that identical
binary and **holds no data**, so leaving takes an afternoon and migrates nothing. There is no
free hosted tier, because a free tier whose cost scales with usage is the exact trap that made
Plex claw remote access back from its own users — and because here the free path is not
crippled, it is better. See [D71](docs/contributors/decisions/working-notes.md) and
[D72](docs/contributors/decisions/working-notes.md).

## What makes it different

- **The client decides what to play.** No server-side device-capability database. The server
  advertises every rendition it can produce — including the untouched original — and the client
  picks, because the client is the only party that can ask its own decoder what it will
  actually accept.
- **Direct play is the default** — for most files the server hands over the bytes and gets out
  of the way. Transcoding is an optional add-on component for the files that need it, not
  something baked into the core.
- **Subtitles actually work.** Extracted at scan time with their embedded fonts, rendered with
  libass everywhere.

## Licence

**Server, media node and relay: GPLv3. Clients and the shared SDK: MPL-2.0.** Both intended,
and the split is deliberate: a GPLv3 application cannot be distributed through the Apple App
Store — VLC was pulled in January 2011 and came back in July 2013 only after relicensing — so a
single project-wide GPL would have meant no iPhone, iPad or Apple TV client at all. MPL's
copyleft is per-file, so the client code stays open and the app ships. Swiftfin and Streamyfin
made the same choice against a GPL Jellyfin server. See
[D80](docs/contributors/decisions/working-notes.md).

Provider API keys are always supplied by you, never shipped in the binary — better for your
privacy, your rate limits, and everyone's licensing.

## Docs

[`docs/`](docs/) — organised by role. Contributors start
[here](docs/contributors/README.md); the evidence behind every decision is in
[`docs/contributors/design/research/`](docs/contributors/design/research/).
