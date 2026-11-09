> **ABOUTME:** How the server's settings and its config file behave, stated once: the registry,
> seeding and the start-up sweep, the modified mark, healing, the config file's starting values,
> unused rows, and where each is shown. The decisions behind it are D200–D202.

# Settings and the config file

- **Status:** IMPLEMENTED — see `internal/core/settings` and `cmd/orpheum` (M1). Decided by
  [D200](../../decisions/working-notes.md), refined by [D201](../../decisions/working-notes.md)
  and [D202](../../decisions/working-notes.md), all 2026-09-24; this page is their sum, so a
  reader need not combine three decisions and their strike-throughs.
- **Depends on:** [schema.md](schema.md) (`setting`), [migrations.md](migrations.md) (files),
  [principles §15](../../principles/15-safe-visible-defaults.md).

## The rule

**No behaviour has a default in code.** Every opinionated value the server acts on is either a
setting — a row in `persistent.db`'s `setting` table — or a key of the config file. Both have a
**starting value** that is written down, shown, and changeable; neither is a value that exists
only in the program.

## Settings

- **The registry** (`internal/core/settings/registry.go`) declares every key: its type, bounds,
  starting value, whether the settings page shows it (`Exposed`), and what it means. A key whose
  answer must be asked, not assumed, starts as JSON `null` — a question (`shouldCheckForUpdates`, D184).
- **Every start sweeps the settings before anything reads one**, in one transaction:
  - a key with no row gets its starting value;
  - a row **nobody set** follows its starting value, so a release that changes one — to fix a bad
    choice — moves it, and the start-up log says which;
  - a row **a person set** is never touched.
- **Modified is a person's mark**, stored in `setting.is_modified` (migration 0002). Any manual
  set marks it, even to the starting value; **set to default** (`POST /config/{key}/reset`)
  writes the starting value and clears it.
- **A row that disappears while the server runs heals**: reading it writes its starting value and
  logs a warning. A value is always a row, never a value held in memory.
- **Unused rows** — keys this release does not read — are never removed automatically: an older
  release on a downgrade still reads them (D181), and deleting a typo deletes the evidence of what
  was meant. The registry keeps **retired keys** with the release that retired each and why, so an
  unused row reads "retired in vX: reason" or "probably a typo". A person deletes one with
  `DELETE /config/unused/{key}`, which refuses a key in use.

## The config file

- **It holds only what must be known before `persistent.db` opens**, or must not be changeable by
  someone holding only an admin account: the listen address, the two directories and the socket
  directory, the HTTP timeouts, the store's reader minimum and migration space factor, the node's
  name, roots, FFmpeg path, confinement, video and subtitle extensions, error limit, probe timeout and status
  interval (the node has no database), and the log level and format. Everything else is a
  setting.
- **Every key has a starting value, or is marked as having none** because it is specific to the
  machine (the listen address, the directories, the media roots, the FFmpeg path).
- **A key the file lacks runs on its starting value**, in memory, with a warning at start; the
  file is never rewritten unasked (migrations.md, Files). **A key with no starting value refuses
  start**, naming the key and what it is.
- **Keys nothing reads** are reported and ignored, never fatal.

## Where it is shown

| What | Where |
| --- | --- |
| Settings the settings page shows | `GET/PUT /settings` |
| Every setting beside its starting value, and whether a person set it | `GET/PUT /config`, `?onlyModified=true` for only those a person set |
| The config file's keys as the server runs with them, and which came from starting values | `GET /config`, read-only `configFileKeys` |
| Unused setting rows | `GET /config/unused`, `DELETE /config/unused/{key}` |
| Everything that should be fixed: config keys on starting values, config keys nothing reads, unused rows | `orpheum doctor`, and the start-up log |

The settings page that shows all of this is the first screen of M2's web work.
