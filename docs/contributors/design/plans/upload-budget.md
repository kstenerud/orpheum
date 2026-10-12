> **ABOUTME:** D286 §4 and §5 as they will be built: the server's upload the owner gives as a
> setting, a ceiling the server sets on a playback's quality and tells its player with each chunk,
> the qualities it lowers and merges when its players would need more than it can send or make,
> and, last, the newest member of a Watch Together group removed. The shape, the steps, and what
> waits on the owner.

# The server's upload and its players' qualities

- **Status:** IN-PROGRESS — U1 to U4 built 2026-10-12, U5 and U6 left; written 2026-10-12 on the owner's word (*"go ahead"*); its two questions
  answered by the owner the same day (*"To your two questions: Agreed"*): solo players count and are
  lowered, and the server acts early, from the setting.
- **Depends on:** D286 §4 and §5 (decided 2026-10-12, §5's details in its text), D265 (the player's
  own quality controller), D250 §6 (a node's runs kept faster than playback), D247 §10 (chunks and
  their qualities), D271 §4 (a guest's rights).

## 1. What the owner gives: the upload

- **A setting, `server_upload_mbps`**, shown on the settings page with a question while unset
  (D200): *"How fast can this server send, in Mbit/s? Your connection's upload speed, or as much of
  it as you want Orpheum to use."* Unset, nothing in this plan acts: no ceilings, no one removed.
- **A button to measure it** by a speed test site is left for later (the owner, D286 §5).
- **The share the films may use: 85% of it**, a constant (`filmsShareOfUpload`): the rest is for
  voice, playlists, pictures and the bursts a player loading ahead sends above its quality's
  average. A player whose quality fills its whole share never loads ahead.

## 2. What the server counts: each playback's need

- **A playback's need is the average rate of what it loads now**: its picture's quality, the one
  its last chunk request named (the server sees the stream in every request:
  `/bytes/media/chunks/{token}/{streamName}/…`), plus its sound's; the film's own picture at the
  file's average rate. A playback that asked nothing for 30 s (`inUseWithin`, as D250 §6) needs
  nothing. A playback of the file as it is, or of a prepared copy, needs its file's rate and cannot be
  lowered (§3); it is counted all the same.
- **The total need is each playback's, under its ceiling where it has one.** It is counted from
  what players ask, not measured from the bytes sent: the server acts before the link is full,
  rather than after players have stalled. The bytes actually sent, over 20 s, are logged beside the
  need, so the owner can see whether the setting is right.

## 3. The one mechanism: a ceiling the player obeys

- **The server may set a playback's ceiling**: the highest quality, in kbit/s, its player may load.
- **The player learns it with every chunk it loads**, from a response header
  `Orpheum-Ceiling-Kbps` (absent: no ceiling). hls.js's requests already carry our headers
  (`xhrSetup`, D262 §2); the same hook reads the answer's. Solo and group players alike, with no
  new connection and no group protocol; a player that never loads a chunk is not loading anything
  to lower.
- **The controller obeys it as a cap** (D265): it never chooses a quality above the ceiling, drops
  to it at its next chunk, and steps up again only under it. The film's own picture counts as its
  file's rate.

## 4. Who is lowered, and to what (D286 §4, §5)

Every 5 s, while the total need is above the films' share:

1. **The playback with the highest need is lowered**, ties going to the newest; never below its
   lowest quality.
2. **To a quality the server already makes for another playback of the same film, if one is lower
   and not far below** (one quality down at most, else the next lower quality): one encode feeds
   both, and the node's work does not grow as the link's falls (§4's wish).
3. **Repeat** until the need fits or every lowerable playback is at its lowest quality.

**A node too slow to make a run as fast as it plays** (D250 §6) is the same move, for its CPU rather
than the link: before its run is evicted, the run's playbacks get a ceiling at a quality already
made for another of the film's playbacks; eviction stays for a run with no such quality.

**Ceilings rise again one quality at a time**: when the need with one playback's ceiling raised a
quality would stay under 75% of the share for 30 s, the oldest-lowered playback's is raised; a
playback stopped or idle loses its ceiling.

## 5. The last resort: the newest member removed (D286 §5)

- **When every playback that can be lowered is at its lowest and the need is still above the share
  for 20 s**, the member of a Watch Together group who joined most recently, never the host, is
  removed: every device of theirs disconnects with `uploadFull`, and their page says *"The host's
  connection can't carry everyone right now."*
- **They may rejoin by the same link at once** (the owner: *"yes, booting the newest member is ok"*).
  Rejoining over the share makes them the newest again, and they are removed again after the same
  20 s.
- **A solo player is never removed**: it is lowered as far as it goes and plays on. A playback
  that cannot be lowered (§2) is never lowered nor, if solo, removed.

## Steps

| Step | What | Test |
| --- | --- | --- |
| U1 | The setting and its question; each playback's need from its chunk requests, the total, and the bytes sent, logged every 20 s while anyone plays | BUILT 2026-10-12: `server_upload_mbps`, `internal/core/playback/upload.go`, the API's `countedMedia`; the log line "the server's upload" |
| U2 | The ceiling: kept per playback, sent as `Orpheum-Ceiling-Kbps` on every chunk, obeyed by the player's controller | BUILT 2026-10-12: `CeilingHeader`, `Choosing.obeyServer`, `chunks.Service.Ladder` |
| U3 | Lowering and raising (§4), as a pure function of the playbacks, their needs and the share, and the 5 s watch that applies it | BUILT 2026-10-12: `internal/core/playback/lowering.go`, `KeepUnderShare` every 5 s |
| U4 | D286 §4: a node too slow lowers onto a quality already made before evicting | BUILT 2026-10-12: `chunks.Service.MovePlayersBy`, `playback.Service.MoveOffStream`, a ceiling of 2 minutes |
| U5 | Removing the newest member (§5): the hub's removal, the `uploadFull` close, the page's words | the hub's and the page's tests |
| U6 | A VQ item: the owner's devices on a setting set low on purpose, watching qualities fall, merge and rise, and the newest member removed | device |

## The owner's answers

1. **Solo players count and are lowered too** (§3, §5): the server's upload is shared by everyone
   it serves, a household member watching alone in another room included; only group members are
   ever removed. The owner, 2026-10-12: *"Agreed."*
2. **The server acts early, from the setting** (§1, §2): on what its players ask, before the link
   is full, not on stalls it measures. The owner, 2026-10-12: *"Agreed."*
