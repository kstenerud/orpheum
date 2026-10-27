# The owner's answers on navigation and widgets (2026-10-27)

The owner's answers to the questions of two reports,
[navigation-back-up-2026-10-27.md](../research/navigation-back-up-2026-10-27.md) and
[ui-widgets-audit-2026-10-27.md](../research/ui-widgets-audit-2026-10-27.md), kept here until the
decisions they feed are written (the Up decision's revision; the widget set's).

## Widgets

1. **Icon buttons' sizes:** the owner read it as a question of the icons' format: "The icons should be
   SVGs so that they can be sized without quality loss." They are (the `Icon` component). The question
   was of the button around an icon (its touch target, 2.75rem; the status tray's narrower 2.25rem); put
   plainly, the owner answered: "Let's try sizing them the same as the others and see how that feels."
   One size, 2.75rem, the tray's too, to be looked at on devices.
2. **Every top-bar pop-up is modal**: "This makes it easier to get out of a menu by clicking anywhere,
   without accidentally activating something else." And: "maybe watch together and cast should be their
   own screens rather than a popup. Research the implications. The warning sign should also take you to
   its own page."
3. (Answered in 2.) The warning sign leads to a page of its own.
4. **"Approve removals"** ("Remove what the last scan did not find", D208) was explained to the owner;
   the lead recommends it ask first, since one press removes many files (nothing deleted under D363).
5. **Play is a button like every other**, "a special color since it's the most important button on the
   page".

## Navigation

- Silk's fake history after a reload exists already (so Back does not ask to quit); it works on the
  owner's Fire TV.
1. **One series page with a season picker: yes**, "ideally when coming back it preselects the season of
   the episode you're coming back from."
2. **Choosing a season replaces the history entry.**
3. **Leaving Watch Together's page by Back keeps the session, if signed in**, the tray leading back. "If
   you're just a transient participant, there's nowhere for you to go. I wonder if it's a good idea to
   allow a transient guest who's followed a link or QR code to come back in via that same code? Only the
   browser that redeemed it would be able to." (To research.) Then: "Remember the threat model here. The
   stakes aren't very high": the design keeps rejoin simple, its protections proportionate.
4. **Up is named by the parent's name.** And: "don't some competitors show where you are in the
   drill-down, for example Series / Copper Kettle / s1e1 blah blah." The lead proposed both, and the owner
   agreed: on computers and phones a trail (`Series › Copper Kettle › S1E1 Pilot`, each part a link, the one
   before last the Up); on a TV only "‹ Copper Kettle", one stop for focus.

The owner agreed every other answer and recommendation above (2026-10-27).
