# (unnamed media server)

A personal media server for **video**, on your hardware, without a company between you and your
files.

**Status: pre-implementation.** There is no code. This repository currently contains research
and design only — a survey of every serious competitor in this space (source read locally where
open), what their users complain about, an evaluation of the technology available, and the
architecture that follows.

Start at **[`docs/contributors/design/overview.md`](docs/contributors/design/overview.md)**, or
**[`docs/ROADMAP.md`](docs/ROADMAP.md)** for the implementation plan.

## The promise

These are product commitments, not aspirations:

- **No telemetry.** Off by default; opt-in only if ever added.
- **No account.** No third-party service is ever required to watch your own files.
- **No storefront.** No ads, no upsell, no recommendations for content you don't own.
- **Your files stay yours.** Media is read-only by default. The database is a rebuildable
  index, never the authority. Your metadata corrections are written back to disk so they
  survive us.

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

GPLv3 (intended). Provider API keys are always supplied by you, never shipped in the binary —
better for your privacy, your rate limits, and everyone's licensing.

## Docs

[`docs/`](docs/) — organised by role. Contributors start
[here](docs/contributors/README.md); the evidence behind every decision is in
[`docs/contributors/design/research/`](docs/contributors/design/research/).
