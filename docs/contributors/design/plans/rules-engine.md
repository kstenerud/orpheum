> **ABOUTME:** A stored, re-evaluatable query over library items — smart playlists, smart
> collections, retention policy — and the research needed before the schema is written.

# Rules engine

- **Status:** UNSPECIFIED — promoted into Phase 1 by [D20](../../decisions/working-notes.md); no design yet.
- **Depends on:** —

## Why it is urgent rather than merely wanted

It is **schema-shaping**, and `library-model.md` is Phase 1. A rule is a stored query over item
attributes, tags and provider fields, so what a rule can express constrains how all three are
modelled. Designing it after the schema means a migration.

The demand is ~7,000 stars across plugins and companion apps that exist *only* because no server
in this field lets a user express a rule about their library: Kometa (3,416), Maintainerr
(2,271), Janitorr (752), plus Jellyfin's smartlists (363), auto-collections (292), mergeversions
(439), media-cleaner (260) and the official tmdbboxsets (176).

## What the research must settle

- **Prior art, starting with the best small example.** Navidrome's `model/criteria` package is
  ~2,500 lines including tests and covers the whole surface: a JSON rule format, a field
  registry, an operator set (`is`, `isNot`, `contains`, `startsWith`, `endsWith`, `gt`, `lt`,
  `before`, `after`, `inTheRange`, `inTheLast`, `notInTheLast`, `any`, `all`), sorting and a
  walker, compiled to SQL in `persistence/criteria_sql.go`. Then iTunes/Plex smart playlists for
  the UX vocabulary, and Kometa/Maintainerr for what people actually write.
- **One engine or three?** Smart playlists, smart collections and retention policy look like the
  same thing evaluated at different times with different effects. If they are, say so and build
  one.
- **Evaluation model.** Materialised on write, on read, or on a schedule? Retention rules
  *delete things*, so they need a dry run, a preview and an audit trail.
- **Safety.** A rule engine that can delete files is the most dangerous feature in the product.
  What is the blast radius, what is reversible, and what requires confirmation
  (principles §2)?
- **Does it need to be user-visible SQL?** Almost certainly not, but the fallback for power
  users matters.
