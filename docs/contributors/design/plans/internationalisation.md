> **ABOUTME:** Language, script and locale handling across the UI, the metadata and the
> library — cheap to decide now, expensive to retrofit, and larger than it looks given who
> actually extends media servers.

# Internationalisation

- **Status:** UNSPECIFIED.
- **Depends on:** —

## Why it is on the list at all

The plugin survey turned up something not anticipated: **the most popular third-party metadata
providers in this field are regional.** MetaShark (2,141), Bangumi (1,134), Douban (664) and
Danmu (659) are all Chinese-market and together outweigh every official Jellyfin provider
combined ([D20](../../decisions/working-notes.md)). The people most motivated to extend a media
server are not English-speaking by default.

[D24](../../decisions/working-notes.md) already forced part of this into the open — BCP 47
normalisation, ISO 639-2's competing bibliographic and terminological codes, script subtags
mattering for Chinese. That was about *track* language. This is about everything else.

## What the research must settle

- **UI translation infrastructure**, chosen before there are strings. Kyoo uses Weblate;
  Jellyfin and Immich both have large translation communities. Retrofitting extraction into an
  existing UI is miserable.
- **Sorting and collation.** "The Gold Rush" under T or M; Japanese sorted by reading, not by
  codepoint; Chinese by pinyin or stroke. This is a database collation decision and therefore a
  schema decision.
- **Search across scripts** — romaji for Japanese titles, simplified/traditional Chinese
  equivalence, diacritic folding. Postgres FTS configuration, `unaccent`, `pg_trgm` behaviour on
  CJK (where trigrams behave differently and may need a different strategy).
- **Metadata in multiple languages at once.** A title has a native name, an English name and a
  transliteration, and different users want different ones. That is a data-model question, not a
  display question.
- **RTL layout** for the UI.
