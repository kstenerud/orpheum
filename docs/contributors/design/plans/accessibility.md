> **ABOUTME:** The accessibility surface — caption presentation controls, audio description,
> and screen readers — which has a statutory floor in both our largest markets and which we
> already half-meet by accident.

# Accessibility

- **Status:** PLANNED — designed 2026-09-19 ([D90](../../decisions/working-notes.md), [audit-2026-09-19.md](../research/audit-2026-09-19.md) §12).
- **Depends on:** stream-service.md, web-client.md, tv-client.md

## Why there is a floor, and where it is

Two regimes bind every commercial video player, and neither is aspirational.

**United States — 47 CFR §79.103** (CVAA). Caption display controls that must be present:

| Control | Requirement |
| --- | --- |
| Font | the **eight fonts required by CEA-708**, user-assignable per style |
| Size | **50% – 200%** of default |
| Text colour | override the authored colour, from a palette of **at least 8** (white, black, red, green, blue, yellow, magenta, cyan) |
| Background colour | same 8-colour minimum, out of CEA-708's 64 |
| Background opacity | opaque / semi-transparent / transparent |
| Caption **window** colour and opacity | again, and separately from the background |
| Character edge | **five**: none, raised, depressed, uniform, drop-shadow |

Since **2026-08-17** those settings must additionally be *"readily accessible"* — reachable,
not merely implemented.

**European Union — Directive (EU) 2019/882**, the European Accessibility Act, applicable since
**2025-06-28**: the player must expose caption size and font controls and make audio
description reachable. Audiovisual media services are explicitly not exempt.

We are not a covered commercial entity and nobody is going to file against a self-hosted media
server. That is not the point. **This is a specification that thirty years of accessibility
work produced, written down, free, and already what our users are asking for** one issue at a
time:

> `jellyfin-web`, open, by reactions: *Subtitles disappear when fullscreen*, *Subtitle offset
> text field is clamped to slider min/max*, *Inconsistent size of subtitles*, *Subtitles dark
> background box*.

## And on the cast path the platform already implements it

[D137](../../decisions/working-notes.md). Cast's `TextTrackStyle` carries `fontFamily`,
`fontGenericFamily`, `fontScale`, `fontStyle`, `foregroundColor`, `backgroundColor`,
`edgeType`, `edgeColor`, `windowType` and `windowColor`, and Google's documentation states
that *"the window concept is defined in CEA-608 and CEA-708"*. **Every row of the table above
is covered** — the eight fonts, the sizing, both colours, both opacities, the separate window,
and the edge attributes.

So the cast receiver serves WebVTT and sets `TextTrackStyle`, and **ships no subtitle renderer
of its own**. Reimplementing this on a platform that already contains the reference
implementation of the model the regulation was written around would be worse and would cost
us money.

## We are already most of the way there, by accident

[D55](../../decisions/working-notes.md) chose the served font set as "the CTA-708 style axis —
six upright variable faces covering the seven real caption styles for Latin, Greek and
Cyrillic", for typographic reasons. **That is the statutory list.** Make the eight-style mapping
explicit and a font-selection choice becomes a conformance artefact.

Likewise [D50](../../decisions/working-notes.md)'s libass build — HarfBuzz, current FriBidi,
libunibreak, `WHOLE_TEXT_LAYOUT` — is what makes the rendering correct enough for the controls
to mean anything.

## The overlay rendition forecloses all of it

[D78](../../decisions/working-notes.md) noticed one row of that table: *"a bitmap cannot honour
a user's subtitle-size setting"*. It is worse than that — a pre-rendered bitmap has a fixed
size, font, colour, background, window **and** edge. Every control above is gone.

So, and this belongs in the manifest rather than in a document:

- **the overlay is never the only offer for a caption track.** Text is the default and stays
  available whenever a text form exists;
- **where the overlay is chosen, the client says which controls it lost**, in the same "why is
  this different?" surface that explains a transcode ([principles §3](../../principles/README.md));
- burn-in — already the labelled exception — is the strongest version of the same loss.

## Audio description is a track kind

It currently appears only in [identification.md](identification.md)'s track-kind derivation, as
a disposition flag we happen to parse. It needs to be a kind in the client contract and a value
in the preference axes ([D24](../../decisions/working-notes.md)), so that "prefer audio
description where available" is a setting rather than a per-file act of remembering which track
number it was.

## A design requirement, not a Cut A gate

[D188](../../decisions/working-notes.md). Every one of these projects triages by votes, and a population that
cannot use the product does not vote, so accessibility never wins a ranking
([ui-competitors.md](../research/ui-competitors.md) §3). It is therefore built into the
structure rather than scheduled:
- accessible names derived from what an element is
- keyboard and remote reach everything the pointer does, and focus is always visible
- no state is shown only by colour
- text scales without breaking the layout, and reduced motion is honoured

**Cut A is not held for it.** Shortfalls found while two people use it are recorded, not
blocking. Whether this gates a release anyone else installs is decided before that release.

## Screen readers

Unaddressed anywhere and not researched here. The requirement is stated so it is not forgotten:
the web client is a normal web application and gets WCAG-shaped treatment; the TV client's
focus model must work with TalkBack, and that is the same focus-navigation work
[tv-client.md](tv-client.md) already identifies as the thing to prototype before writing real
screens. The two problems are the same problem, which is a reason to do them together.

## Done when

Every control in the table above exists in the web client and the TV client, reachable in at
most two steps from playback; a caption track always offers a text form where one exists; a
client using the overlay says so and says what it costs; "prefer audio description" is a
preference that sticks; and the TV client is navigable with TalkBack on.
