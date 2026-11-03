# The toolkit batch (D408)

The owner (2026-11-03) ordered the toolkit batch now, before the deferred UX rewrite: our own widget
layer, composed from small parts, with a pop-up apart from the dialog and a list that keeps context
([D408](../../decisions/working-notes.md#d408--the-app-builds-its-own-widget-layer-from-small-composed-parts-a-pop-up-is-not-a-dialog-and-carries-no-close-button-and-a-list-opens-on-its-current-choice-with-a-neighbour-either-side);
research: [ui-toolkit-2026-11-03.md](../research/ui-toolkit-2026-11-03.md)). Screens stay as they are.
End: one `make check-pr`, staging, the owner tries it.

## Steps

1. **The parts under the dialog.** Take out of `ModalDialog` (`web/apps/web/src/modal-dialog.tsx`) what
   every transient surface shares: the native `<dialog>` opened with `showModal()`, closing on Escape,
   a remote's Back, the browser's Back and a press outside, and focus back to the opener. `ModalDialog`
   keeps its ✕ and its width on top of them (D408 §2: dialogs keep the ✕).
2. **`Popover`**, built from the same parts: placed at its opener (today's `layAtOpener`,
   `drop-down-place.ts`), as big as its content (`fit-content`) within the space there is, no ✕, no
   minimum width. `DropDown` becomes a `Popover` holding a list; the ✕ band, its scroll padding and
   `keepFocusedClearOfClose` go.
3. **`ListBox`**: opens with focus on the current choice and at least one entry above and below in
   view; while the highlight moves, one entry of context stays either side (`scroll-padding-block` of
   one row, a maximum height a row short of the space, placement that keeps the current entry inside).
   Its options are its own component, so `drop-down.css` no longer styles whatever is inside it
   (`.drop-down__places a, button {width: 100%}`).
4. **The overlay's pop-ups on them**: 💬, 🎧 and ⁝ as `Popover` + `ListBox` (⁝'s steppers and
   switches as its own content), 🔈 as a `Popover` sized to its slider and mute button. The banner
   menu, ⁝ on pages and the choosers follow, since they share `DropDown`.
5. **Layout primitives** `Stack` and `Cluster` (the 2026-10-05 architecture research's R2), used by
   the new widgets and the pop-ups' contents; components stop taking a `className` that restyles them
   where this batch touches them (R5).
6. **The native `<select>`s** a TV cannot work (the 2026-10-27 widget audit: the audio and subtitle
   menus under the film first, `audio-tracks.tsx`, `subtitles.tsx`) become the chooser.
7. **Docs**: the web README's invariants, the `/ui-gallery` specimens, D408's status.

## The owner's comments on steps 1–4 (2026-11-03), folded into the batch

8. **A pop-up's width.** The banner's menu came out too narrow, wrapping its entries. The rule (the
   lead's wording of the owner's): a pop-up is as wide as its widest entry, but never wider than half
   the screen; an entry longer than that wraps. Half a phone's screen is too narrow for a word, so the
   cap is never under 16rem, and the margin (9) always wins. The width is its choices' alone: the
   menu's "Signed in as …" line only informs, and wraps rather than widening it (the owner,
   2026-11-03).
9. **A margin pop-ups keep clear of**: a small one (0.5rem, plus the device's safe-area insets) at
   every edge of the screen, which no pop-up covers.
10. **Cast and Watch Together open at once.** Their first press shows "Loading…" in place of the
    icon until the code arrives; it is loaded ahead, once the app is idle after its first page.
11. **🔈 grows upward out of its button.** The pop-up is placed so that its speaker sits where the
    button was, the slider above it: pressing 🔈 looks like the button growing into the control, and
    it shrinks back once something is chosen.

**Separately:** a one-day Preact spike measured on the Fire TV stick (D408 §5). Trying Preact adds a
dependency, which the hooks refuse in a feature session; it needs a session started with
`ORPHEUM_ALLOW_DEPS=1` (the owner starts one after steps 1–4). The Fire TV stick and the Pixel 3a are
dedicated to testing (the owner, 2026-11-03), so the spike uses them without asking.
