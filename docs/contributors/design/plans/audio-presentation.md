> **ABOUTME:** Why dialogue is too quiet and what we do about it — loudness measured at scan
> and applied as gain, one downmix default plus a dialogue control, and dynamic range as a
> property of the room. The audio half of "it should just play the file, and sound right".

# Audio presentation

- **Status:** PLANNED — designed 2026-09-19 ([D83](../../decisions/working-notes.md), [audit-2026-09-19.md](../research/audit-2026-09-19.md) §5).
- **Depends on:** library-model.md, scanner.md
- **Blocks:** [library-model.md](library-model.md) — the per-track loudness columns are schema

## Why this is ours and not the client's

[D22](../../decisions/working-notes.md) puts **audio encoding in the core**, because every GPL
trigger in FFmpeg is a video encoder and the whole audio menu is native LGPL. That was decided
on licensing grounds and it has a consequence nobody followed up: **when a 5.1 track is
downmixed to stereo for a television's speakers, we are the ones doing the downmix.** The
quiet dialogue that results is ours.

It is also one of the most common complaints about watching films at home, and it is not a
subjective one. Dialogue lives on the centre channel at a level that assumes its own speaker;
every downmix matrix attenuates it to avoid clipping the sum.

## Three separate mechanisms, routinely confused

| Problem | Mechanism | Where it belongs |
| --- | --- | --- |
| This film is quieter than the last one | **Loudness normalisation** — measure, store, apply gain | scan time + playback |
| Dialogue is buried under the music | **Downmix matrix** and a dialogue control | the downmix, in the core |
| Explosions are too loud at 11pm | **Dynamic range compression** (`dialnorm`/DRC metadata) | a per-device preference |

Treating them as one thing is what produces a "volume boost" setting that helps nobody.

## 1. Loudness: measure at scan, apply as gain

**Netflix fixes this at ingest**: their delivery spec requires **-27 LKFS ±2 LU, dialog-gated**,
true peak ≤ **-2 dBTP**, measured per ITU-R BS.1770. The whole catalogue is loudness-consistent
before a player sees it.

We cannot rewrite files ([principles §2](../../principles/README.md)), so we take the answer the
music world found under the same constraint — measure once, store the number, apply gain at
playback. The node already opens every file; FFmpeg's `ebur128` filter implements EBU R128.

Per audio track, stored as node-observed facts: **integrated loudness**, **loudness range**,
**true peak**. The core derives a playback gain from the user's target.

**Stated limitation.** FFmpeg measures *programme* loudness, not Netflix's **dialogue-gated**
figure — dialogue gating needs Dolby's Dialogue Intelligence, which FFmpeg does not have. Our
number is comparable within our own library and **is not the same quantity**. Do not imply
parity anywhere user-facing.

**Cost is the open question.** It is an extra decode pass at scan time, on a machine that is
already decoding — but on a Celeron NAS with fifty thousand files that is not free. Whether it
is deferrable per library belongs in
[performance-targets.md](performance-targets.md).

## 2. Downmix: one default and one control

Jellyfin ships five algorithms and its own documentation records a dialogue complaint against
nearly every one:

| Algorithm | Jellyfin's own caveat |
| --- | --- |
| `None` (ffmpeg's matrix) | *"may produce weaker volume levels"*, *"too low perceived dialog"* |
| `Dave750` | *"dialogues might sound too quiet compared to music"* |
| `NightmodeDialogue` | *"Too quite sound effects is not ideal for movies"* |
| `RFC7845` | *"too-quiet center channel, which can cause sound effects and music to potentially overpower dialogues"* |
| `AC-4` | — |

Five wrong answers in a dropdown is what a problem looks like after it has been handed to the
user. We ship **one standard matrix as the default** and **one dialogue-boost control**, which
is a user preference of the same kind as subtitle size and rides
[D24](../../decisions/working-notes.md)'s preference table at the same scopes.

Which matrix is the default is a listening decision against real content, not a documentation
decision; the constraint is that it is one, it is named, and changing it is one setting rather
than a taxonomy.

## 3. Dynamic range is a property of the room

AC-3 and E-AC-3 carry `dialnorm` and DRC metadata authored with the content. FFmpeg's decoder
**applies DRC by default** (`drc_scale = 1`), which is right for a phone on a train and wrong
for a home cinema. Kodi carried this as a bug for years; Jellyfin has it open as *"Can't set
Dynamic Range Compression (DRC) decoding preference when ffmpeg transcodes Dolby Digital
audio"* (#10148).

The correct value depends on a fact only the device knows. That is
[D2](../../decisions/working-notes.md) applied to audio: **the client declares it, per device,
and the core honours it** — three states (full range, light compression, night mode), not a
float.

**The device is asked, not defaulted** ([D184](../../decisions/working-notes.md)): on its first playback, *how does this
device play sound — TV speakers, a soundbar or receiver, or headphones?* The answer sets whether
we send a downmixed stereo mix (and so whether dialogue boost can act, [D178](../../decisions/working-notes.md)) and the DRC
state, and it is changeable from the player. Quiet dialogue from a mix nobody chose is the failure
this exists to prevent.

**And it is stored, keyed by device** ([D103](../../decisions/working-notes.md)), which needed
a decision of its own because the naive version rebuilds the device-profile database
[D21](../../decisions/working-notes.md) exists to prevent. The line: a device *capability* is
declared per request and never stored; a device *preference* is stored. Night mode on the
living-room television is a fact about a person in a room, not about hardware.

## What this does not do

- **It does not rewrite files.** Ever.
- **It does not add a server-wide "night mode".** The correct value differs per listener and
  per room, which is why it is a device preference and not a setting.
- **It does not touch a track that is passed through untouched.** A direct-played 5.1 bitstream
  to a receiver gets the gain as metadata for the client to apply, or nothing at all — never a
  re-encode for the sake of a volume change.

## Done when

Two films from different decades play at the same perceived level without touching the remote;
dialogue is intelligible on a television's own speakers with no configuration; a home-cinema
user can turn dynamic-range compression off for that device only; nothing was written to a
media file; and the server can say, per track, what it measured and what gain it applied.
