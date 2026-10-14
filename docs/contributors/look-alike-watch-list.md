> **ABOUTME:** Pieces of code that look alike where it is not yet clear whether they are the same
> thing (D229 §3). Each waits for the question that would settle it; audits revisit them.

# The look-alike watch list

[D229](decisions/working-notes.md#d229--code-that-looks-alike-is-merged-only-when-it-is-the-same-thing-settled-look-alikes-are-listed-here-and-doubtful-ones-are-watched)
says code is merged only when it is the same concept. What an audit found alike and could not
judge goes here, with the question that would settle it. A settled entry leaves this list: merged
if it is one thing, or added to D229 §2's table, with why, if it is two. Audit checklist item 10
reads this list and D229 §2 before reporting duplicates.

| Look-alikes | Found | The question that settles it |
| --- | --- | --- |
| CSS `.poster-grid__card` (browse) and `.continue-watching__play` (home) | audit-2026-09-29 | Is a Continue Watching item a poster card with progress on it, or its own kind of tile? If the first, they are one component. |
| The wait-then-attach hook in `play.tsx` and `subtitles.tsx` | audit-2026-09-29 | The two are the same pattern; a generic hook would defeat the `exhaustive-deps` lint. Is there a shape that keeps the lint's check, or does a third copy appear? |

## Twins

Code copied between this module's public and internal trees, or between Orpheum and
`orpheum-connect`, is a **twin** (the owner, 2026-10-14): at the same path in each, and the same
file but for its licence line and its module path in imports. A change to a twin is made in every
copy, and checked to work there. A test holds each pair together:

| Twin | Copy | Held by |
| --- | --- | --- |
| `internal/core/secrets/value.go` | `orpheum-connect`'s, same path | `orpheum-connect`'s twins test, in its `internal/twins` |
| `internal/store/open.go` (the opening; `ours.go` is each project's own) | `orpheum-connect`'s, same path | the same |
| `deploy/staging/tunnel.sh` | `orpheum-connect`'s, same path | the same |

Still copies, not yet twins, each waiting on the owner:

- `internal/platform/atomics`' `WriteFile` and `discovery/single`'s `writeWhole`: the copy moves to
  `discovery/internal/atomics/atomics.go`, byte for byte, once the lint exemption for its renames
  names that file instead of `discovery/single/record.go`.
- `internal/platform/clock` and `clocktest`, and `orpheum-connect`'s smaller `internal/clock`: the
  move to the same path needs `orpheum-connect`'s lint exemption for the clock moved with it.
- `internal/core/providers/cloudflare` and `orpheum-connect`'s `internal/zone` client: two jobs now
  (one ACME challenge through a household's token; a zone's records through the service's), sharing
  only Cloudflare's answer envelope. Twins, one client for both, or two things (D229 §2)?
- `internal/store`'s migration runner, with the migration window (D272 §5), and
  `orpheum-connect`'s numbered steps without one.

