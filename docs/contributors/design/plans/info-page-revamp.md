> **ABOUTME:** The owner's revamp of a film's or episode's information page: one row of navigation,
> the picture with its watched mark beside the title, details and action buttons, the whole
> synopsis, and the rest under a fold. Read before changing `title.tsx` or `title-head.tsx`.

# The information page's revamp

- **Status:** PLANNED — the owner's layout, 2026-11-26, for a later batch (not the next one).
  Nothing is built. The open points at the end are the lead's, for the owner.
- **Depends on:** —

## What is wrong today (the owner, 2026-11-26)

- Two rows navigate to the parts of a series.
- An episode's name is shown twice.
- A "More" button hides the full synopsis.

## The layout the owner asked for

It flows on smaller devices.

1. **One navigation row, the top one**, which does not name the episode: "Series > the series'
   name > S1 > E2".
2. **Under it, the picture on the left.** It carries a checkmark when the film or episode has been
   watched. "Watched" is not a separate item.
3. **To the picture's right**, from the top:
   - the title;
   - the quick details: the year and the duration only;
   - the action buttons.
4. **Under the picture, above an item's parts, the full synopsis.** No "More" button.
5. **Under that, a fold** a person expands for the rest: the cast, the file's details and so on.

## The action buttons

In this order:

1. **Previous** (the "|<" glyph): goes to the previous video's information page; it plays nothing.
   Greyed out on a series' first episode.
2. **Play** (the right-facing triangle).
3. **Next** (the ">|" glyph): goes to the next video's information page; it plays nothing. Greyed
   out on a series' last episode, its extras not counted.
4. **More** (the "…" button), which holds Mark as watched.

The owner's answers, 2026-11-26:

- **A film has neither Previous nor Next:** absent, not greyed out.
- **Previous and Next cross a season's end,** the whole series through, as they do on the Play
  page. They will extend to a set of videos once sets exist.
- **Today's "Next episode" button goes.** It plays the episode after the one shown, at once, which
  the owner calls weird; Previous and Next replace it. The page's actions are then Play, Previous,
  Next and, under "…", Mark as watched.
