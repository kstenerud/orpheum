> **ABOUTME:** The player's "Up next" offer near an episode's end and on its end screen, and the
> person's autoplay, which counts it down and plays the next episode; on a cast's receiver and
> its remote too. Read before changing what the player does as a film ends.

# Up next and autoplay

- **Status:** IN-PROGRESS — 2026-10-25, at the owner's request: *"Add 'up next' button and
  user-configurable autoplay."* Built: §1–§8 below; on devices, VQ67 and VQ68; left, §9. It is closed-beta B1's *"The next-episode countdown, which a
  person can turn off"* ([D313](../../decisions/working-notes.md) §2), built before the
  intro and credits detection it would sit on ([D82](../../decisions/working-notes.md)).
- **Depends on:** [play-state.md](play-state.md) (Next Up over an ordering, D160),
  [episode-orderings.md](episode-orderings.md) (the ordering a person watches in, D159 §1),
  [cast-to-device.md](cast-to-device.md) (§9 Q4, §11 assumption 7),
  [media-segments.md](media-segments.md) (the credits cue the offer should move to).

## What is decided

- **Next Up is a query over a collection's ordering** ([D160](../../decisions/working-notes.md)),
  in the ordering the person watches the series in (D159 §1, `ViewingOrdering`).
- **The next-episode countdown has a setting to turn it off** (D313 §2; closed-beta.md B1:
  *"Autoplay is the countdown's ending, with its setting"*).
- **The end-of-episode overlay must not steal the ending** ([D139](../../decisions/working-notes.md)):
  the prompt belongs on the typed `outro`/`credits` boundary (D82), not a fixed timer, once there
  is one.
- **At a film's end the cast's receiver plays the next episode as the sender would, else goes to
  its home page and the remote is told** (D337's §9 answers, Q4).
- **A person's own choices that hold in every file live in the preference table at the person's
  scope**, the dialogue boost and levelling among them (D83, D226 §5; `internal/core/preferences`).

## What was built, and the assumptions it made

Where nothing above decides, the build chose the conventional behaviour:

1. **The next episode is the member after the playing one in the person's ordering**, not Next Up
   (which, while an episode plays, is that episode). An episode the ordering leaves out has none.
   It plays as Next Up's Play does: the file the person is partway through, else its first.
   `GET /files/{id}/up-next`.
2. **No credits cue exists yet** (no segment table nor detector, D82), so the offer shows a fixed
   **30 s before the end** (`upNextLeadMs`, a justified constant), and on the end screen.
3. **The countdown is 10 s and starts at the film's end** (`upNextCountdownMs`), not when the offer
   shows: a fixed lead counting down would cut the programme's last seconds on a show with short
   credits, which is D139's complaint. Once credits are detected it should start at the credits.
4. **Autoplay is a person's own switch, on until they turn it off** (the convention of every
   service), kept as the axis `autoplay` at the person's scope beside the boost and levelling;
   *Play the next episode automatically* on the Account page, the person's own settings.
5. **Any press cancels the countdown and leaves the offer; Back or Escape dismisses it.** A click
   or tap elsewhere cancels too. The offer's button takes focus as the film ends, and as it shows
   when nothing else has focus, so OK presses it.
6. **Films have no next one**: no collection of films exists yet. The query is over any collection
   (D160), so one will.
7. **A next episode that cannot be read now is not offered.**
8. **On a cast**, the receiver shows the offer, which it cannot be pressed (presses there are Take
   back control's), and counts it down by the person's setting; at its end the cast moves on to the
   next episode in the same session of one (`POST /me/casts/{id}/next`), the receiver handed it
   again and the remote following. With autoplay off the receiver waits at the end with the offer.
   The remote offers *Up next* while its film has a next one. With none, the receiver goes home and
   the remote is told, as before.
9. **Not built:** the unattended-viewing pause (play-state.md, *"still watching?"*), a Watch
   Together group carried to the next episode for its viewers (B1's own item), and the credits cue.
