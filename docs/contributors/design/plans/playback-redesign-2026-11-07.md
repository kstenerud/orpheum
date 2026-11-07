# The playback redesign (2026-11-07)

The owner opened this design round on 2026-11-07: in full screen, Cast and Watch Together are hard to
reach, and leaving full screen to reach them is cumbersome, so they become situational controls of
the full-screen view. This file is the design agreed in the dialogue that followed, written for the
owner's approval before any building, and approved whole on 2026-11-07 as D440. **Agreed** marks the owner's words in the dialogue;
**Proposed** marks the lead's filling-in, for the owner to confirm or change.

Groundwork:

- What the player shows today, and what works in each cast mode:
  [`playback-controls-inventory-2026-11-07.md`](../research/playback-controls-inventory-2026-11-07.md).
- Whether voice chat can travel with a cast:
  [`voice-on-cast-2026-11-07-research.md`](../research/voice-on-cast-2026-11-07-research.md).

## 1. Nothing stays over the film

**Agreed.** "People want to watch the movie, not see things permanently overlaid on their screens."
Every control, the microphone's state included, shows only while the overlay is up, and nothing
pops up while a film plays: no "you're muted" note, as a false alarm is worse than none.

## 2. One volume control: a row of sliders

**Agreed.** The overlay's volume control opens a row of sliders: the film's, marked by a film icon,
then one per person in the Watch Together session.

- **Telling them apart (agreed):** each person's slider carries their colour (the one their marker
  has elsewhere in Watch Together) and their first letter; two letters where first letters
  collide; the focused slider shows the person's full name above it.
- **Where the sound plays does not matter to the person (agreed):** each slider sets its sound's
  level wherever that sound plays: on this device, or on the TV while casting.
- **Under the TV's own volume (agreed):** while film and voices play on a TV, the sliders are each
  channel's level on the receiver, independent of the TV's master volume, so neither drowns the
  other. They last for the session and are not saved.
- **Alone (proposed):** with no session, the control is the film's slider alone, as today's.
- **Only where there is mixing (the owner, 2026-11-07):** "Only audio levels when mixing. With
  desktop, everything is by definition mixing." The row shows always on a computer; on a TV, a phone
  or a tablet, whose own buttons set the volume, only while a session's voices play with the film.
  With a remote or game controller, Left and Right move between sliders and Up and Down move the
  focused one; on touch, each slider is dragged.

Today's per-person volumes (D281 §4: silent to four times as loud) and the film's volume carry over;
what changes is where they are reached.

## 3. The microphone in the overlay

**Agreed.** The microphone button sits in the overlay while in a Watch Together session: as today,
blue and pulsing with one's voice while on, grey while off. Pressing it is D281 §7's one toggle.
That is the whole signal of its state; nothing else shows it.

**Proposed:** it shows only on devices with a microphone, so not on a TV playing alone.

## 4. One Cast control

**Agreed.** One Cast control casts by every means: Orpheum's own casting, Chromecast and AirPlay.
The work now is Orpheum's own casting, made as good as it can be; Chromecast and AirPlay come later
(the owner has both devices at home for tests), each offering the subset of the controls it can
carry, and the React Native apps later reuse the web app's design and as much of its code as they
can.

- **Proposed:** the control opens one list of the targets this device can reach: Orpheum's own
  receivers on every device; Chromecast only where the browser can cast to it; AirPlay only in
  Safari on Apple devices. Today only Orpheum's own casting is built (Chromecast is a lab spike,
  AirPlay has no code), so until then the list holds Orpheum's receivers alone.
- **A guest (the owner, 2026-11-07):** "A watch together guest can be signed in, in which case they
  get a cast icon. Otherwise there's nothing for them to cast to, so they only get airplay or
  chromecast targets (if available)". A signed-in guest's page is a screen in the shell (D372 §9),
  which gives the overlay its Cast control, so they have it already; a guest by link alone gets one
  only with Chromecast or AirPlay (step 4 of §9).

## 5. Casting: the phone becomes the control centre

**Agreed.** A cast started from full screen leaves full screen and goes to the cast controls page.
While casting, the phone (or laptop) is the control centre, since it shows no film. The page shows
only the controls that work in that cast mode, the slider row and the microphone, the Watch Together
controls when in a session, and a button to stop casting and go back to playing here in full screen.

What the cast controls page lacks today, in every mode (inventory §2), and is to gain:

- the TV's channel volumes and mute (§2; no message exists for them yet);
- subtitles;
- choosing a quality (shown today, not chosen);
- ⏮ (restart or previous).

D434 stands: the remote's choices go to the receiver in the cast's session, whatever else it plays
in. D385 stands: the receiver taking control back ends the cast, and the sender plays on.

## 6. Watch Together from one button

**Agreed.** Watching together looks the same as watching alone, locally or cast: the slider row and
the microphone are part of the overlay (§2, §3), and the rest is behind one Watch Together button,
tuned to the role. What each role may do is unchanged (D246, D271): the host grants pause and seek,
a guest cannot add people, and the host leaves only by ending the session. A guest's button offers
Leave; the host's, adding and removing people, the grants, and End.

**Proposed:** the button shows in the overlay with no session too, so that a session can be started
from full screen (today the sign shows only while a session runs). This amends D423 §1 and D433,
whose dialog becomes the button's.

## 7. Voices on the TV: optional

**Agreed.**

- A TV takes no microphone, and that is fine: the microphone stays on the phone.
- Voices can be cast with the film, optionally. Then all sound comes from one device, and a person
  can wear headphones on the TV and speak into the phone: the ideal, as headphones are the only
  mechanical cure for echo.
- Voices need not be synced with the film: they are separate sounds played at the same time.
- Picture alone on the TV with the sound on the phone is parked: 50 ms between devices is out of
  reach in browsers.

**Echo, bounded.** Echo comes from any sound a speaker plays that the phone's echo canceller did not
play itself, the film's sound as much as the voices. How each setup sounds to the others:

| Setup | What the others hear from you |
| --- | --- |
| Not casting: all sound on the phone | Clean (the phone cancels its own sound) |
| Film on the TV, voices on the phone's speaker | Your film leaks, less as the TV goes down |
| Film on the TV, headphones on the phone | The same film leak, less with a headset's microphone |
| Film and voices on the TV's speakers | Your film and their own voices |
| Film and voices on the TV, headphones on the TV, talking into the phone | Clean |

Bluetooth headphones on a TV add a delay of their own that many TVs do not make up for; the existing
sound timing control corrects the film's. A native app could later cancel the film's leak using the
soundtrack as the canceller's reference; a browser cannot be given that reference.

**Delay.** The phone network's standard (ITU-T G.114) calls under 150 ms one-way unnoticeable and
150–400 ms workable; studies of free conversation put the point where people notice delay at about
300–560 ms; and casual talk over a film is at the tolerant end. Voices played on a TV arrive about
400 ms after they are spoken (the TV's own 300 ms and more, plus the network): noticeable, fine for
chat. On the phone, about 100–150 ms.

**Proposed:** the switch is "Play voices on the TV", on the cast controls page, off by default,
offered only where the receiver has passed a film-and-voice test (the Fire TV's browser is proven
for voice alone; the TCL TV is unmeasured; Chromecast needs a device test; never AirPlay).

## 8. What this asks of the decisions

- **Amended:** D423 §1 and D433 (the Watch Together sign becomes the overlay's button, shown with
  no session too), D422 §1 (the Cast sign becomes the one Cast control), and D281 §5 (the host's
  pop-up's volumes move to the slider row).
- **Standing:** D246 and D271 (roles), D281 §1–4 and §6–7 (voice), D385, D389 and D434 (casting).
- Recorded as a new decision once the owner approves this file.

## 9. Building it, once approved

Proposed order, each a batch of its own:

1. The overlay: the slider row, the microphone, the Watch Together button (with no session too),
   on every device.
2. The cast controls page: channel volumes and mute (a new message), subtitles, quality, ⏮, and the
   Watch Together controls.
3. Voices on the TV: Orpheum's own receiver first, behind the film-and-voice test, measured on the
   Fire TV and the TCL TV.
4. Chromecast, then AirPlay, each with the subset it carries.
