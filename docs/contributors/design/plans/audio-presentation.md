> **ABOUTME:** Why dialogue is too quiet and what we do about it — loudness measured once by
> the node and applied as gain, one downmix default plus a dialogue control, and dynamic range as a
> property of the room. The audio half of "it should just play the file, and sound right".

# Audio presentation

- **Status:** PLANNED — designed 2026-09-19 ([D83](../../decisions/working-notes.md), [audit-2026-09-19.md](../research/audit-2026-09-19.md) §5). **The dialogue boost is in Cut A; loudness measurement is not** ([D178](../../decisions/working-notes.md) §3–§4). **2026-09-23: the node, not the core, does the downmix, and Cut A's boost is on or off — two audio renditions in each prepared rendition** ([D193](../../decisions/working-notes.md) §1–§2). **2026-09-28: loudness levelling is in Cut A after all** ([D226](../../decisions/working-notes.md)): a film quiet enough that the owner could not hear it at full volume.
- **Depends on:** library-model.md, scanner.md
- **Blocks:** [library-model.md](library-model.md) — the per-track loudness columns are schema

## Why this is ours and not the client's

[D22](../../decisions/working-notes.md) puts ~~**audio encoding in the core**~~ **audio encoding
on the server**, because every GPL trigger in FFmpeg is a video encoder and the whole audio menu
is native LGPL. **The core does not downmix: the node runs every FFmpeg invocation**, and the
core relays what it produces ([D193](../../decisions/working-notes.md) §1). That was decided
on licensing grounds and it has a consequence nobody followed up: **when a 5.1 track is
downmixed to stereo for a television's speakers, we are the ones doing the downmix.** The
quiet dialogue that results is ours.

It is also one of the most common complaints about watching films at home, and it is not a
subjective one. Dialogue lives on the centre channel at a level that assumes its own speaker;
every downmix matrix attenuates it to avoid clipping the sum.

## Three separate mechanisms, routinely confused

| Problem | Mechanism | Where it belongs |
| --- | --- | --- |
| This film is quieter than the last one | **Loudness normalisation** — measure, store, apply gain | a node job per file + playback ([D226](../../decisions/working-notes.md) §2) |
| Dialogue is buried under the music | **Downmix matrix** and a dialogue control | the downmix, in the node ([D193](../../decisions/working-notes.md) §1) |
| Explosions are too loud at 11pm | **Dynamic range compression** (`dialnorm`/DRC metadata) | a per-device preference |

Treating them as one thing is what produces a "volume boost" setting that helps nobody.

## 1. Loudness: measure once, apply as gain

**Netflix fixes this at ingest**: their delivery spec requires **-27 LKFS ±2 LU, dialog-gated**,
true peak ≤ **-2 dBTP**, measured per ITU-R BS.1770. The whole catalogue is loudness-consistent
before a player sees it.

We cannot rewrite files ([principles §2](../../principles/README.md)), so we take the answer the
music world found under the same constraint — measure once, store the number, apply gain at
playback. The node already opens every file; FFmpeg's `ebur128` filter implements EBU R128.

Per audio track, stored as node-observed facts: **integrated loudness**, **loudness range**,
**true peak**. The core derives a playback gain towards a fixed target, -23 LUFS, capped by the
track's peak ([D226](../../decisions/working-notes.md) §3); a person chooses only whether it is
applied (D226 §5), not the target. *(Amended 2026-09-28 by D226: the gain was derived from the
user's target, and the measurement taken at scan; D226 §2 makes it a job per file.)*

**Stated limitation.** FFmpeg measures *programme* loudness, not Netflix's **dialogue-gated**
figure — dialogue gating needs Dolby's Dialogue Intelligence, which FFmpeg does not have. Our
number is comparable within our own library and **is not the same quantity**. Do not imply
parity anywhere user-facing.

### In Cut A ([D226](../../decisions/working-notes.md))

~~Not in Cut A ([D178](../../decisions/working-notes.md) §4)~~: a film the owner could not hear
at full volume brought it in.

- **Measured by a job, not by the scan:** `measure_loudness`, one per file with sound whose
  payload has no measurement, one at a time on the node. A scan stays minutes; the owner's 329
  films take two to three hours once, the time being mostly the read of each file (measured
  2026-09-28: 21 s for an 81-minute film).
- **Each track, and its stereo mix:** a track with more than two channels is also measured as
  the standard stereo mix of §2, whose renormalised gains play it 7 to 8 dB quieter than the
  original. Kept in persistent.db by payload hash and stream index.
- **The gain** goes towards -23 LUFS, both ways, and never lifts the true peak above -1 dBTP nor
  raises a track more than 20 dB. The boosted mix plays with the standard mix's gain.
- **The player applies it after its volume**, through Web Audio, so the volume control still runs
  to 100 and 100 means the film at the level. A prepared copy made after the measurement carries
  its gain, for Safari's own HLS on an iPhone, and the player applies only the difference.
- **A person may turn it off**; it is on until they do. The player shows the gain it applied.

~~**Cost is the open question.**~~ Answered by the measurement above: one read of each file,
once, in the background.

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

### In Cut A: on or off, as two prepared audio renditions

[D193](../../decisions/working-notes.md) §2. D83 wanted a continuous control, but a prepared
rendition is a shared cache ([D169](../../decisions/working-notes.md)), so each one made by the
node's prepare job — `remux` and `h264-8m` alike — carries **two stereo downmixes as two audio
renditions**: the standard matrix, and the same with the centre channel raised. The client
picks by the user's preference, and **switching between them mid-film needs no new job**.
Finer steps can come later as further renditions.

It still only works if we do the downmix ([D178](../../decisions/working-notes.md) §3): a device
that declares stereo speakers (§3 below) is sent a prepared rendition's stereo track rather than
the 5.1 original.

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
device play sound — built-in speakers, a soundbar or receiver, or headphones?* The answer sets whether
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

**Cut A:** on a television's own speakers, a 5.1 film plays through a prepared rendition whose
dialogue-boosted track is chosen by the user's preference, and turning the boost off or on
mid-film switches track without a new job. Two films from different decades play at about the
same level without touching the volume, and the player says what gain it applied (D226).

**Later:** dialogue is intelligible on a television's own speakers with no configuration; a home-cinema
user can turn dynamic-range compression off for that device only; nothing was written to a
media file; and the server can say, per track, what it measured and what gain it applied.
