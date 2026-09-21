> **ABOUTME:** Language, script and locale handling across the UI, the metadata and the
> library — what was decided, what it costs, and in what order it has to happen. Cheap to
> decide now, expensive to retrofit, and larger than it looks given who actually extends
> media servers.

# Internationalisation

- **Status:** PLANNED — researched 2026-09-19; backed by [D45](../../decisions/working-notes.md)–[D58](../../decisions/working-notes.md) (the research doc itself backs D45–D54; the font chain, D55–D58, is the round's tail and lives in [stream-service.md](stream-service.md)).
- **Depends on:** library-model.md (schema), scanner.md (regex corpus, path normalisation)

## Why it is on the list at all

The plugin survey turned up something not anticipated: **the most popular third-party metadata
providers in this field are regional.** MetaShark (2,141), Bangumi (1,134), Douban (664) and
Danmu (659) are all Chinese-market and together outweigh every official Jellyfin provider
combined ([D20](../../decisions/working-notes.md)). The people most motivated to extend a media
server are not English-speaking by default.

There is a second reason, visible only once you go looking. i18n complaints are **individually
small and collectively large**: Jellyfin's metadata-language, sort-order, RTL and fallback
issues each carry a handful of reactions, spread across communities that do not overlap. They
never win a prioritisation contest, so they are never fixed — *Language Selection Fallback
Heirarchy* has been open since **2020-03-05**. A competitor's permanent backlog is an opening.

The full evidence is in
[research/internationalisation.md](../research/internationalisation.md).

## What was decided

| | Decision |
| --- | --- |
| [D45](../../decisions/working-notes.md) | Metadata is multi-language in the schema: invariant row + translation rows, chosen per request from `Accept-Language` |
| [D46](../../decisions/working-notes.md) | No translated string is ever stored in the database |
| [D47](../../decisions/working-notes.md) | Sort order is a materialised CLDR sort key, per item per locale |
| [D48](../../decisions/working-notes.md) | Two FTS5 tokenisers routed by script and length, and exactly one normaliser |
| [D49](../../decisions/working-notes.md) | Unicode discipline at the boundaries, enforced by tests |
| [D50](../../decisions/working-notes.md) | Build libass correctly; turn off VSFilter bug-compatibility for non-authored subtitles |
| [D51](../../decisions/working-notes.md) | react-i18next, catalogues served by us, Weblate deferred |
| [D52](../../decisions/working-notes.md) | Pseudo-localisation is the default development build |
| [D53](../../decisions/working-notes.md) | Font coverage is not assumable; the server serves fonts on demand |
| [D54](../../decisions/working-notes.md) | Plan-B font: Unicode Last Resort HE bundled (588 KB), Noto served, Unifont optional |
| [D55](../../decisions/working-notes.md) | The served set is the CTA-708 style axis in variable fonts, subset per subtitle track |
| [D56](../../decisions/working-notes.md) | Fonts embedded in media are off by default; the client must ask |
| [D57](../../decisions/working-notes.md) | That opt-in is per-user, sticky per series; the client may still refuse |
| [D58](../../decisions/working-notes.md) | Omit the resource from the manifest rather than rejecting the request (→ principles §10) |

## Schema consequences

Three of those reach into [library-model.md](library-model.md) and must land before it, or the
first migration is an i18n migration:

1. **Every metadata-bearing entity splits in two.** A language-invariant row (kind, dates,
   runtime, ratings, external ids, **genres as an enumeration**) and translation rows keyed
   `(item, language)` carrying name, description, tagline, aliases, free-text tags, **and
   artwork** — posters have title text baked into the pixels, so poster language is its own
   axis. The original-language record is distinguished rather than being one translation among
   many: it always exists, so it is both the guaranteed fallback and the meaning of "sort by
   original title".
2. **A sort-key column per (item, locale-in-use)** — an indexed `BLOB` of
   `collate.KeyFromString` output, ~140 bytes each. Not a transliteration, and not a custom
   SQLite collation.
3. **Two FTS5 indexes**, `unicode61 remove_diacritics 2` and `trigram`, plus a
   normalised-alternates column. The tokeniser is baked into the virtual table at creation, so
   this cannot be deferred.

## Order of work

Ordered by *when the decision must be made*, not by size.

| Work | Deadline | Why then |
| --- | --- | --- |
| NFC path normalisation in the node report contract | **now** | It is a field in a wire format ([D39](../../decisions/working-notes.md)) |
| Unicode-safe regex corpus + its CI fixture | **now** | The scanner is Phase 1 |
| Translation split, `original` record, genre enum | **before the schema** | Otherwise a migration |
| Sort-key column | **before the schema** | Same |
| Two FTS5 indexes + routing rule | **before the search schema** | The tokeniser is immutable |
| "No translated strings in the database" | **now** | A rule; free if adopted early |
| `Accept-Language` on the read API | with the API | Standard, and it *is* the fallback chain |
| react-i18next + server-served catalogues | with the first client | Extraction is miserable to retrofit |
| libass build flags + feature toggles | with the subtitle subsystem | Changes what we compile |
| Pseudo-localisation in debug builds | with the first client | Free before there are strings; a retrofit after |
| One RTL locale in the device lab | with [device-lab.md](device-lab.md) | One locale, one pass |
| Font-coverage enumeration across the rig | with the device lab | Cheap, and D53 rests on a guess until it runs |
| Font-serving endpoint | with the subtitle subsystem | Same mechanism as embedded subtitle fonts |
| Bundle Last Resort HE in every client | with the first client | 588 KB, and it is the floor everything else sits on |
| Assemble the CTA-708 face set + `hb-subset` in the node | with the subtitle subsystem | ~2.4 MB served once, ~441 KB per CJK film |
| Weblate | **not yet** | It needs translators to exist first |

## The two things to build first, in detail

Both are small, both are free now, and both are silent failures later.

**Path normalisation.** Nodes on different operating systems report **different bytes for the
same file** — `Amélie` is 8 bytes from macOS and 7 from Linux. Under `UNIQUE(path)` that is two
library items; under a reconciling scanner it is delete-and-add churn every time the reporting
node changes. The report contract carries **both**: an NFC form for identity, dedup, display and
search, and the raw bytes for opening the file, because APFS normalises on lookup and ext4 does
not.

**The regex corpus.** Go's `\w \d \s \b [[:alpha:]]` are ASCII-only, so Jellyfin's episode
expressions ported verbatim fail on `進撃の巨人` — and on **`Amélie`**. `\s` also misses U+3000,
the usual separator in Japanese and Chinese filenames. The failure is silent: the expression
does not match, the name falls through to a worse expression, and the result is a misparse. So
the corpus bans those classes, requires `\p{L} \p{N} \p{M} \p{Z}`, and CI runs every expression
against a non-Latin fixture.

## Cost of the translation platform, stated honestly

Weblate is the convention — 9 of 11 surveyed projects — and it is AGPL and self-hostable. It is
also two GitHub Actions workflows, a GitHub App with scoped tokens, and a required PR check that
**fails any PR touching a non-English catalogue without the bot's approval**, because humans may
edit only English and every other file is machine-owned. Immich's release flow locks the
component, forces a commit and push, finds the single `weblate` PR, approves it, merges it, polls
for the merge, and unlocks, with cleanup on timeout.

None of that buys anything before there are translators. Design the catalogue format for it now;
connect it when somebody volunteers.

## What the commercial players do, and what transfers

Researched alongside the rest; the detail is in
[research/internationalisation.md](../research/internationalisation.md) §8. Three things carried
over and one did not.

**Carried over.** Netflix serves UI strings from a **runtime service** rather than shipping them
in builds, explicitly because "a build might need additional approval from a third party like
Apple" — independent confirmation of D51, arrived at by a company that built a three-region
microservice to get a property we get for free by being on the same LAN. Their
**pseudo-localisation** is now D52. Their **Hydra** screenshot pipeline — UI automation posting
locale × device screenshots to a catalogue, one property-file line per new locale — is the shape
[device-lab.md](device-lab.md) should take for its locale axis.

**Did not transfer, and it is the important one.** Netflix's primary browse surface is **ranked
rows**, not an alphabetical grid: personalised ordering largely replaces collation. A personal
media server's primary surface *is* the A–Z grid, so the hardest problem in this round — that
every locale wants a different order and Go's implementation gets Korean wrong — is more central
for us than for them, on a far smaller budget. That asymmetry is why
[D47](../../decisions/working-notes.md) spends a schema column on it.

**On fonts, we can do better than they do.** Netflix Sans covers "Cyrillic, Greek, Latin, Latin
(Vietnamese Extension)" and nothing else; Amazon Ember is the same shape. Both fall through to
the platform cascade for CJK, Arabic, Hebrew, Thai and Indic — which the Android CDD does not
guarantee exists. **On a device without a Japanese font, Netflix renders tofu too**; their
remedy is commercial relationships with device manufacturers, not engineering. Ours is
topological: we are a server on the same LAN, so we can send the font
([D53](../../decisions/working-notes.md), [D54](../../decisions/working-notes.md)).

**And one honest ceiling.** Netflix mandates IMSC 1.1 for Japanese timed text: vertical writing
mode, *tate-chū-yoko*, ruby with reserved space, *bouten* emphasis marks. libass has **zero** ruby
support and only the VSFilter `@font` convention for vertical text, with open bugs. We render
everything a personal library actually contains; we do not render broadcast-grade Japanese
typesetting. Stated, not discovered.

## Open

- **Korean collation is wrong** and will stay wrong: `x/text/collate` does not implement script
  reordering and [golang/go#19087](https://github.com/golang/go/issues/19087) has been open since
  2017 with no comments. Three ways out — accept it, ship a small tailoring, or link ICU for the
  key generator only, which runs on the *node* where the cross-compilation objection is weaker.
  Not resolved.
- **Whether per-language artwork earns its storage** at household scale. Unmeasured.
- **How CJK identification actually fails.** That the top four Jellyfin metadata plugins are
  Chinese-market says the built-in identification does not serve those libraries; separating
  "ASCII-only regexes" from "provider coverage" from "different release-naming conventions"
  needs a corpus we do not have.
- **How many locales one household uses.** The per-locale sort key is cheap at one to three and
  annoying at ten.
