> **ABOUTME:** The interface standard: the rules every screen follows on a desk, a phone, a touch
> screen and a TV; which control to choose and how a form saves; pop-ups and dialogs; what goes over
> a film; the catalogue of shared parts; and the checklist each interface change passes.

# The interface standard

Start here before changing anything a viewer or an admin sees. This page says how the web client
looks and behaves on screen. It does not cover what any API call does.

The owner (2026-11-22): "I want a solid UI/UX design spec with clear rules and policies, and a set
of UX and UI principles we live by from now on." This page is that spec, written from the design
system the owner approved, as D538 amends it. The plan's "The UI revamp"
([navigation-and-widgets-answers-2026-10-27.md](../design/plans/navigation-and-widgets-answers-2026-10-27.md#the-ui-revamp-the-owner-2026-11-22))
records the review behind it. The *why* is in the interface principles
([§31](../principles/31-the-art-is-the-colour.md) to
[§41](../principles/41-one-way-for-each-interaction.md)). As every standard here does, each rule
names what enforces it. *Review* means only a reader can tell.

## The sources, in order of authority

1. **The owner's answers**: D538 and the plan's "The UI revamp".
2. **The rules on this page**, which apply everywhere, including on screens no spec mentions.
3. **The specs' words** (the handoff's menu, dialog, page, form and player specs, kept outside the
   repo), which say what changes on each screen.
4. **The mockups**, which are reference only.

Where two disagree, the higher one wins. Report the disagreement so that the lower source gets
corrected (principles [§27](../principles/27-decisions-are-revisable.md)).

## The rules

### 1. Use the shared part

A screen never hand-builds what a shared part already does (D538 §1). Pages use Page, Section and
ActionRow for their frame. Forms use Field, Checkbox, Switch, Stepper and the pop-up chooser.
Empty, problem and loading states use StatusNote, a page-wide note uses Banner, modal work uses
Dialog, and a note over video uses Pill. In a page, a raw `<input type="checkbox">`, a `<select>`,
`type="number"` or a `<p role="alert">` is a defect. When no part fits, the need goes to the
catalogue below first, and the screen uses the part once it exists.

*Enforced by:* review, and the catalogue below. A source test in the style of
`src/colour-tokens.test.ts` is planned once batch 2 has moved the screens onto the parts.

### 2. Every target is at least 44 × 44 px

Every interactive element takes a press over at least `--size-control` (44px) square: list
options, rail letters, banner icons, checkboxes and "more" buttons included. What is drawn can be
smaller (a 20px checkbox box, a 24px icon), but the area that takes the press cannot.

*Enforced by:* each part's browser test (`src/layout-parts.browser.test.tsx`,
`src/player-widgets.browser.test.tsx`); review elsewhere.

### 3. Hover is an extra, never the only way

- Hover styles sit inside `@media (hover: hover)`, so a tapped control on a phone does not stay
  painted.
- No information or action lives only in a tooltip or a `title`. Help is visible text, or a
  HelpButton that opens it on a press (WCAG 1.4.13). A TV and a phone have no hover.
- Nothing is reachable only by a pointer, and nothing only by a drag. A slider also takes the arrow
  keys, and a tap places it (WCAG 2.5.7).

*Enforced by:* review.

### 4. Focus is always visible, and never covered

Every focusable element shows the global two-colour ring (tokens.css's `:focus-visible`). A part
never removes or restyles it. A sticky bar or the player's overlay never covers what has focus, and
the scrub bar takes the same ring and does not fade (D538 §6, amending D410 §3).

*Enforced by:* `src/focus-ring-contrast.test.ts`; each part's browser test, which checks the ring
on whatever has focus.

### 5. The d-pad

- Every row is one focus stop. Up and Down move between rows in the order they are drawn. Group
  lines and sub-headings are never stops.
- A control of several parts in one row (a Stepper, a menu's row of buttons) is a focus row
  (`data-focus-row`, focus-engine.ts). Left and Right move inside it, and only its ends lead out.
- In a pop-up, Left or Right past a row's end, and Back, close the pop-up. Select presses, toggles
  or opens.
- Back always closes the innermost layer: a picker's pop-up, then a dialog, then the page.
- When a pop-up or a dialog closes, focus returns to what opened it.

*Enforced by:* `src/focus-navigation.browser.test.tsx`, `src/page-walk.browser.test.tsx` and the
page walks beside it, and each part's browser test.

### 6. Pop-up or dialog

These are two different things (D408), and each behaves its own way.

| | Pop-up (Menu, a chooser) | Dialog |
| --- | --- | --- |
| Is | a quick control at its opener | modal work, or a question that must be answered |
| Size | its content's, 16rem to 24rem wide, within the screen (D408 §7) | a card up to 32rem wide; a full-screen sheet in a narrow container |
| Closes on | a choice, a press outside, Escape, Back | a footer button whose action succeeds, a press outside, Escape, Back |
| ✕ | none | none (D538 §2, amending D408 §2) |
| Footer | none | always; every button closes it; "Cancel" pairs with the main action, and a dialog with nothing to do has one "Done" |
| Text fields | none | at most one (D538 §3) |

- **A press outside closes every dialog and every pop-up** (D538 §3). Keeping a dialog to one text
  field is what makes that safe: a stray press loses little. An exception needs a reason recorded
  where the dialog is built that would satisfy a reviewer, such as a sign-in's name and password.
  Otherwise, a task that needs more fields is a page.
- **A dialog's header and footer stay on screen while its body scrolls** (D538 §2), so a way out is
  always in view. A sheet uses `dvh` and the safe-area insets, so its header and footer never sit
  under a phone's browser bar or notch.
- **A dialog starts on its main action, except a confirm, which starts on Cancel** (D538 §4,
  amending D421): every dangerous action asks first, and the question defaults to not doing it.
- **A dialog opened from a dialog replaces it,** and going back brings the first one back with
  focus on what opened the second (D538 §5). Dialogs never stack.
- A main action that fails keeps the dialog open and says why, next to the cause.
- A dialog whose task becomes a result is replaced by the next dialog, not changed in place.
- A choice of one from a list is a pop-up at its opener. A task of several steps is a page.

*Enforced by:* `src/modal-dialog.browser.test.tsx`, `src/popover.browser.test.tsx` and
`src/drop-down.browser.test.tsx`; review for the one-field rule.

### 7. Switches for instant settings, checkboxes for choices you save

A setting that takes effect at once is a Switch, with the whole row as its target. A choice that
waits for a Save button is a Checkbox. The control table in rule 14 says the rest.

*Enforced by:* review.

### 8. One way to confirm

An action that cannot be undone asks first, in a confirm Dialog whose action button is
irreversible, with focus starting on Cancel (D538 §4). A confirm before something disruptive but
harmless (restarting the server, renewing a link) uses a plain main button, and its focus starts
on Cancel too: every confirm defaults to not doing it. The button that backs
out is always "Cancel". Where an action can be undone, do it and offer Undo instead. There is no
other way to confirm.

`ConfirmButton` opens the confirm, `ConfirmDialog` (`src/confirm-button.tsx`, the web README's first
invariant); a control that is not a button of its own, as a bin or a "…" entry, opens
`ConfirmDialog` itself.

*Enforced by:* `src/dangerous-actions.test.ts` (every dangerous action listed and drawn through the
confirm, the irreversible red nowhere else), `src/confirmed-actions.test.tsx`,
`src/confirm-button.browser.test.tsx`; review.

### 9. Layout follows its container

A part takes its narrow form when its own container is under 36rem: an action row stacks, a
heading's actions go under it, and a dialog becomes a sheet. It asks its container (`@container`),
never the window. The window's two breakpoints, 600px and 1200px, set only the page margins and the
poster sizes. A page's content stops at `--size-page` (72rem), its gutter included.

*Enforced by:* `src/layout.browser.test.tsx`, `src/layout-parts.browser.test.tsx`.

### 10. Contrast

- Text that informs is at least 4.5:1 on its ground, a search box's placeholder included, the only
  place a placeholder is used.
- A control's edge, a state that tells on from off, an icon and the focus ring are at least 3:1.
  `--color-border` is not, so where an edge is a control's only boundary it uses the control-edge
  token instead (WCAG 1.4.3, 1.4.11).
- Colour never carries a meaning alone. A current choice has a ✓ as well as the accent, and an
  error has words and an icon as well as its colour.

*Enforced by:* `src/focus-ring-contrast.test.ts`, `src/colour-tokens.test.ts`; review for the
rest.

### 11. Motion, layers and sizes are tokens

- Durations and easings are tokens (`--duration-quick` for hover and focus, and the longer ones
  for a pop-up and a sheet, which batch 1 adds), and reduced motion zeroes all of them.
- Stacking uses the `layer-*` tokens, and over a film the `layer-video-*` ones, which batch 1 adds.
  A raw `z-index` number is a defect.
- A colour, a length that recurs and a size are tokens in tokens.css. A part never writes one out.

*Enforced by:* `src/colour-tokens.test.ts` for colours; review for the rest.

### 12. One source for the input mode

Whether the person uses a mouse, a finger or a remote comes from one module. The pointer and hover
media queries tell touch from a mouse, and a single TV flag stands for the d-pad. Parts read that
module. No part reads the user agent itself.

*Enforced by:* review.

### 13. Problems come first, and the way to them is marked

- A page that can have problems shows a **Needs you** section while it has any (Page's `needsYou`):
  each problem with its fix or a link to it. It comes first, or straight after the page's one-line
  status where the page has one. With nothing wrong, the section is absent.
- Each kind of problem has one home page. Anywhere else shows it as a line linking there.
- A main-menu entry whose page has a problem shows the warning icon at the end of its row, and its
  accessible name gains ", needs attention". The icon is a mark, not a button.
- The header's warning sign leads to the Needs attention page (D538 §12, D280 §4), which lists every
  problem grouped by its home page, each line linking to where it is fixed.
- The menu, the sign and each page read the same counts, so they never disagree.

*Enforced by:* `src/layout-parts.browser.test.tsx` (Needs you first, or absent); review.

### 14. Forms

Every form is filled in the same way, so a person who has used one has used them all.

- **Every input is a Field**: its label, hint, error, then the control. A search box is the only
  place with a placeholder, and examples go in the hint (WCAG 3.3.2).
- **Required is the default and carries no mark.** An optional field's label ends in "(optional)".
- **Two ways to save, never mixed in one section.**
  - *A form that saves itself* (settings, the details of something that exists): a Switch saves
    when flipped, a chooser on a choice, a Field or a Stepper on Enter or on leaving it, and each
    shows "Saved".
  - *A submit form* (signing in, creating, inviting, searching, sending): nothing is kept until the
    button is pressed, and its yes-or-no choices are Checkboxes.
- **The submit button is always enabled.** On a press, each problem shows at its own Field and focus
  moves to the first (WCAG 3.3.1). A problem with no one field is a StatusNote above the button.
  Forms set `novalidate`, and errors show after the person leaves a field or presses the button.
- **Busy** is a spinner in the button, its label kept, with `aria-busy="true"`. The button ignores
  presses but stays focusable.
- **Success always says or shows the outcome.**
- **Choose the control by what is chosen:**

  | What is chosen | Control |
  | --- | --- |
  | On or off, taking effect at once | Switch |
  | On or off in a submit form, or any number of a list | Checkbox rows |
  | One of 2 to 5, each worth seeing | Radio rows |
  | One of more, or of 2 to 5 where room is short | Pop-up chooser |
  | A small whole number in a range | Stepper (D538 §9) |
  | A number that is an identifier | Field, numeric |
  | A position or a level | Slider (the player only) |

- **No native `<select>` and no `type="number"`.** Checkbox and radio rows are 44px tall, the whole
  row the target.
- **A password** is a PasswordField with its Show password button: one field, no confirm field.
  Sign-in and account fields set `autocomplete` (WCAG 1.3.5).
- **On a TV**, OK on a text Field opens the keyboard, the keyboard's Enter does what Enter does on a
  computer, and Back closes the keyboard first. A remote's OK on a Checkbox toggles it and never
  submits.

*Enforced by:* the form parts' tests; review for the choice of control and of save.

### 15. Over the film

Everything drawn over a film goes in one of its places, and each place has one part.

| Place | What goes there | Part |
| --- | --- | --- |
| Centre | Feedback for what the person just did, and the spinner | Pill |
| Top-right corner | An offer to take or leave: Skip, Up next, Still watching? | OfferCard |
| Bottom-left, above the controls | News about other people ("Sam joined") | Pill |

- **One Pill at the centre and one OfferCard at a time;** a new one replaces the old. The notes at
  the bottom-left stack, three at most. A double-tap seek's "+10" stays in the third that was tapped.
- **One sound-on prompt.** Wherever a film starts muted (a Watch Together guest or host, a cast
  receiver), one action Pill at the top centre turns the sound on.
- **The overlay has three rows that never wrap**: the top row (the parent link, which truncates,
  and the end time), the scrub row, and the buttons row, which ends with the menus, Watch Together
  and Cast (D538 §7, amending D422 §1 and D423 §1).
- **Everything about the sound is in the Sound menu** (D538 §8). The gear keeps no sound setting.
- **On a touch phone**, the transport buttons are round, in the centre of the picture.
- **One button for each job**: Full screen is the same button everywhere.
- **Banners over a film**: warnings before information, two at most. While a film fills the window,
  only "Lost the connection" shows.
- **Layers** come from the `layer-video-*` tokens, never raw numbers.

*Enforced by:* `src/layout-parts.browser.test.tsx` for the parts; review for their places until
batch 4 places them.

## Words

The words on screen follow principles [§38](../principles/38-plain-specific-words.md): plain,
specific and in full sentences. Every word is in a locale file under `src/locales/` and reaches the
screen through `t()`. *Enforced by:* the `no-restricted-syntax` rule against text written in JSX
([typescript.md](typescript.md)).

## The catalogue of parts

Use the part listed for each job, and add a part here before a screen grows its own (rule 1). The
files are under `web/apps/web/src/`. A part the revamp's batch 1 is still building has no file
named yet.

| Part | File | When to use it |
| --- | --- | --- |
| Page | `page.tsx` | Every routed screen: its trail, heading, actions, one-line job, status and Needs you, then its sections |
| Section | `section.tsx` | A named block of a page, with its own actions (a "See all", a "Rescan") |
| ActionRow | `action-row.tsx` | The buttons that end a form, a dialog or a card: the way back, then the main action |
| Pill, ActionPill | `pill.tsx` | A note over a film (feedback, news about other people); ActionPill when it offers one action |
| OfferCard | `offer-card.tsx` | An offer over a film to take or leave: Skip, Up next, Still watching? |
| Dialog | `modal-dialog.tsx` | Modal work, or a question that must be answered (rule 6) |
| Pop-up | `popover.tsx` | A quick control at its opener (rule 6) |
| Chooser and menu list | `drop-down.tsx`, `list-box.tsx` | One of many, at its opener; the menu's list |
| Stepper | `number-input.tsx`, becoming the Stepper | A small whole number in a range (D538 §9) |
| Confirm | `confirm-button.tsx`, until batch 2 | Today's ask-first for what cannot be undone (rule 8) |
| IconButton | `icon-button.tsx` | A button that is an icon alone, with its accessible name; the cast remote's pad takes it round at 56, 64, 72 and 96 px (D548 §11) |
| CastRemote | `cast-remote.tsx` | The page that controls a film on another screen: the picture in a card with the offers over it, the pad, the player's buttons row (D548) |
| Icon | `icons.tsx`, `player-icons.tsx` | Every drawing, one library each for the client and the player |
| Stack, Cluster | `layout.tsx` | A column, or a row that wraps, its gap a step of the space scale |
| GridList | `grid-list.tsx` | A list of rows whose columns line up, each row one target |
| The trail | `up-trail.tsx` | Where a page sits and the way up; Page draws it |
| Copy | `copy-button.tsx` | Copying a text, and saying it was copied |
| Switch, Checkbox, Field, PasswordField, HelpButton | batch 1 | The form controls (rule 14) |
| Menu, PickerScreen, Banner, StatusNote | batch 1 | The player's menus, a picker on a phone, a page-wide note, a state's note |

Each part is shown at `/ui-gallery` (`src/ui-gallery.tsx`), drawn by the part itself.

## The checklist

Every change to what a person sees passes this before it is committed (rule 17 of `AGENTS.md`, the
audit at every commit). A part's browser test walks the first four for it.

- [ ] **A mouse** presses every control, and every hover sits inside `@media (hover: hover)`.
- [ ] **A finger** (`pointerType` touch) presses every control, each at least 44px square, and
      nothing is left painted after a tap.
- [ ] **A keyboard** reaches every control by Tab in reading order; Enter and Space press; Escape
      closes the innermost layer and focus returns to its opener.
- [ ] **A TV remote**, arrows and OK alone, reaches every control; Back closes the innermost
      layer; focus never lands on nothing.
- [ ] The **focus ring** shows on whatever has focus and nothing covers it.
- [ ] It works at **360px, 1280px and 1920px**: nothing overflows, the narrow forms take over under
      36rem of their container, and a page stops at 72rem.
- [ ] Every value is a **token**, and every word is in a **locale file**.
- [ ] It uses the **shared part** for each job, from the catalogue above.
- [ ] Colour is never the only cue, and the **contrast** meets rule 10.
- [ ] Problems come **first**, and each has one home.
