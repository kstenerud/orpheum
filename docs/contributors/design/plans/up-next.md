> **ABOUTME:** The player's "Up next" offer near an episode's end and on its end screen, and the
> person's autoplay, which counts it down and plays the next episode; on a cast's receiver and
> its remote too. Read before changing what the player does as a film ends.

# Up next and autoplay

- **Status:** IN-PROGRESS — 2026-10-25, at the owner's request: *"Add 'up next' button and
  user-configurable autoplay."* Built: §1–§8 below, and D339's
  *Still watching?* and group following (§10); on devices, VQ67–VQ70; left, §9. It is closed-beta B1's *"The next-episode countdown, which a
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
- **The fixed lead stays until credits are detected; a run of three autoplays asks "Still
  watching?"; a host's autoplay moves their Watch Together group on**
  ([D339](../../decisions/working-notes.md#d339--up-next-keeps-its-30-s-lead-until-credits-are-detected-a-run-of-three-autoplays-with-no-press-asks-still-watching-and-a-hosts-autoplay-moves-their-group-on)).
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
8. **On a cast**, the receiver shows the offer, which cannot be pressed there (its presses are Take
   back control's), and counts it down by the person's setting; at its end the cast moves on to the
   next episode in the same session of one (`POST /me/casts/{id}/next`), the receiver handed it
   again and the remote following. With autoplay off the receiver waits at the end with the offer.
   The remote offers *Up next* while its film has a next one. With none, the receiver goes home and
   the remote is told, as before.
9. **Not built:** the credits cue; and *Still watching?* on a cast's receiver, which keeps
   counting, since its presses are Take back control's and its remote has no card yet.
10. **Built by D339:** after `stillWatchingAfterAutoplays` (3) episodes autoplayed in a row with no
   press on the page (a remote's command, a key, a tap or a click), the next film's end shows *Still
   watching?* in the offer's place, Continue focused (plays the next) and Stop (leaves the film), and
   counts nothing; the run is the tab's (`still-watching.tsx`). A host's player moving on to another
   film's player, by Up next or its autoplay, does not send the stop that leaving the player sends
   (D246 §5); the next player puts its film on for everyone, so the group follows the host into the
   next episode without its screens going black (`together-host.tsx`).
