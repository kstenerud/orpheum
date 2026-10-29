// SPDX-License-Identifier: MPL-2.0

import "./drop-down.css";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type JSX,
  type ReactNode,
  type RefObject,
} from "react";
import { reportFailure } from "@orpheum/sdk";
import { useTranslation } from "react-i18next";

import { listPlace } from "./drop-down-place.ts";
import { focusTargetsIn, isFocusTarget } from "./focus-engine.ts";
import { IconButton } from "./icon-button.tsx";
import { directionOfKey, keyCodeOf } from "./input-buttons.ts";
import { useCommands } from "./input-layer.tsx";
import { ModalDialog } from "./modal-dialog.tsx";
import { isPhoneWidth } from "./phone-width.ts";
import { CloseIcon } from "./player-icons.tsx";

/** What names the opening button for a screen reader and a pointer's hover, where its content does not. */
export interface OpenerNaming {
  readonly "aria-label"?: string;
  readonly "aria-describedby"?: string;
  readonly title?: string;
}

/**
 * Brings a focused place of the list wholly into its view, where the list scrolls within itself:
 * Firefox left a place partly below its end in view (the browser test, 2026-10-28), which a remote's
 * focus could then not show whole.
 */
function useFocusedPlaceShown(ref: RefObject<HTMLElement | null>): void {
  useEffect(() => {
    const list = ref.current;
    if (list === null) {
      return undefined;
    }
    const show = (event: FocusEvent): void => {
      if (event.target instanceof HTMLElement) {
        event.target.scrollIntoView({ block: "nearest", inline: "nearest" });
      }
    };
    list.addEventListener("focusin", show);
    return () => {
      list.removeEventListener("focusin", show);
    };
  }, [ref]);
}

/**
 * Focuses the list's current place as it opens, the choice a chooser shows (route-chooser.tsx),
 * marked aria-current, else, and always where isFromItsTop, its first place: never its Close, which
 * the browser's own opening focuses as the dialog's first control (the owner, 2026-10-28: "the
 * initial selected widget is the close button. It should have the first actual item selected
 * instead"). Focused without scrolling the page, which a drop-down's opening never moves; the list
 * shows it within itself (useFocusedPlaceShown).
 */
function useCurrentPlaceFocused(ref: RefObject<HTMLElement | null>, isFromItsTop: boolean): void {
  useEffect(() => {
    const list = ref.current;
    if (list === null) {
      return;
    }
    const current = isFromItsTop ? null : list.querySelector<HTMLElement>(currentSelector);
    const onCurrent =
      current === null || isFocusTarget(current) ? current : (focusTargetsIn(current)[0] ?? null);
    (onCurrent ?? focusTargetsIn(list)[0])?.focus({ preventScroll: true });
  }, [ref, isFromItsTop]);
}

/** A place marked as the current one: a page's link (`page`), a chosen button (`true`). */
const currentSelector = '[aria-current]:not([aria-current="false"])';

/** The middle of what has focus inside dialog, in CSS pixels from the screen's top; undefined for none. */
function focusedMiddleIn(dialog: HTMLDialogElement): number | undefined {
  const focused = document.activeElement;
  if (!(focused instanceof HTMLElement) || focused === dialog || !dialog.contains(focused)) {
    return undefined;
  }
  const box = focused.getBoundingClientRect();
  return box.top + box.height / 2;
}

/**
 * Lays dialog, a drop-down's open list, at opener (drop-down-place.ts); on a big screen, where the
 * screen's edge cuts it, cuts places, the part that scrolls, to the height shown; and where its
 * focused place is off the button's middle, scrolls places to bring it there as far as they can, as
 * a select's list opens. A list shown from its top (DropDown's isFromItsTop) has no choice to lay
 * over the button: its top is the button's, as a phone's is, and its places are not scrolled.
 */
function layAtOpener(
  dialog: HTMLDialogElement,
  places: HTMLElement,
  at: { readonly opener: HTMLElement; readonly isFromItsTop: boolean },
): void {
  const openerBox = at.opener.getBoundingClientRect();
  const isPhone = isPhoneWidth();
  dialog.setAttribute("data-placed", isPhone ? "over" : "at");
  dialog.style.setProperty("--drop-down-opener-width", `${String(openerBox.width)}px`);
  places.style.removeProperty("max-height");
  const box = dialog.getBoundingClientRect();
  const focusedMiddle = at.isFromItsTop ? undefined : focusedMiddleIn(dialog);
  const { left, top, height } = listPlace({
    opener: openerBox,
    list: box,
    focusedMiddle: focusedMiddle === undefined ? undefined : focusedMiddle - box.top,
    screen: { width: window.innerWidth, height: window.innerHeight },
    isPhone,
  });
  dialog.style.setProperty("left", `${String(left)}px`);
  dialog.style.setProperty("top", `${String(top)}px`);
  if (height < box.height) {
    const placesHeight = places.getBoundingClientRect().height;
    places.style.setProperty("max-height", `${String(placesHeight - (box.height - height))}px`);
  }
  const laidMiddle = at.isFromItsTop ? undefined : focusedMiddleIn(dialog);
  if (!isPhone && laidMiddle !== undefined) {
    places.scrollBy({ top: laidMiddle - (openerBox.top + openerBox.height / 2) });
  }
}

/**
 * Lays the open list at the button that opened it (layAtOpener), as it opens, its current place
 * focused (useCurrentPlaceFocused, before it), and again as the screen changes size. The dialog's
 * `data-placed` says how, which drop-down.css sizes it by: `at` a big screen's button, as wide as
 * the button at least, or `over` a phone's.
 */
function useListPlacedAtOpener(
  placesRef: RefObject<HTMLElement | null>,
  openerRef: RefObject<HTMLElement | null>,
  isFromItsTop: boolean,
): void {
  useEffect(() => {
    const places = placesRef.current;
    const dialog = places?.closest("dialog");
    if (places === null || dialog === null || dialog === undefined) {
      return undefined;
    }
    const lay = (): void => {
      if (openerRef.current !== null) {
        layAtOpener(dialog, places, { opener: openerRef.current, isFromItsTop });
      }
    };
    lay();
    window.addEventListener("resize", lay);
    return () => {
      window.removeEventListener("resize", lay);
    };
  }, [placesRef, openerRef, isFromItsTop]);
}

/** What has focus, in a few words for the core's log: its tag and classes, and whether in list. */
function focusedDescribed(list: Element | null): string {
  const focused = document.activeElement;
  if (focused === null || focused === document.body) {
    return "nothing";
  }
  const where = list?.contains(focused) === true ? "in the list" : "outside the list";
  const named = [focused.tagName.toLowerCase(), ...focused.classList].join(".");
  return `${named} ${where}`;
}

/**
 * Tells the core's log of a Left or Right that left the list open: on the owner's Fire TV (Silk 138,
 * spatial navigation) neither closed it with focus on a place, where Chromium and Firefox close it
 * by the same key (2026-10-28; idiosyncrasies.md). It says which key came, whether something had
 * taken it before the app heard it and after, and where focus was, so the next check on the device
 * shows which layer kept it. isClosingRef says the list was told to close, which it does a render
 * later.
 */
function useStuckArrowTold(
  ref: RefObject<HTMLElement | null>,
  isClosingRef: RefObject<boolean>,
): void {
  useEffect(() => {
    const checks = new Set<ReturnType<typeof setTimeout>>();
    const heard = (event: KeyboardEvent): void => {
      const direction = directionOfKey(event.key, keyCodeOf(event));
      if (direction !== "left" && direction !== "right") {
        return;
      }
      const isTakenBefore = event.defaultPrevented;
      const focused = focusedDescribed(ref.current);
      const check = setTimeout(() => {
        checks.delete(check);
        if (!isClosingRef.current) {
          reportFailure(
            new Error(
              `a drop-down stayed open at ${direction}: key ${event.key}, keyCode ${String(keyCodeOf(event))}, code ${event.code}, taken before the app ${String(isTakenBefore)}, taken after ${String(event.defaultPrevented)}, focus on ${focused}`,
            ),
          );
        }
      }, 0);
      checks.add(check);
    };
    // At the window, capturing: before the app's input layer and anything else of the page.
    globalThis.addEventListener("keydown", heard, true);
    return () => {
      globalThis.removeEventListener("keydown", heard, true);
      for (const check of checks) {
        clearTimeout(check);
      }
    };
  }, [ref, isClosingRef]);
}

/**
 * Closes the list when one of its places, a link or a button, is pressed: after what it does, as a
 * choice's route followed (link.tsx), heard at the document, past the page's root. Heard on the list
 * itself, the close ran first, between the browser's listeners, and the list was gone before React
 * heard the press at its root, so a link's own handling never ran and the browser followed it (the
 * browser test, 2026-10-28).
 */
function useChoiceCloses(ref: RefObject<HTMLElement | null>, close: () => void): void {
  useEffect(() => {
    const chosen = (event: MouseEvent): void => {
      const list = ref.current;
      const { target } = event;
      if (
        list !== null &&
        target instanceof Element &&
        list.contains(target) &&
        target.closest("a, button") !== null
      ) {
        close();
      }
    };
    document.addEventListener("click", chosen);
    return () => {
      document.removeEventListener("click", chosen);
    };
  }, [ref, close]);
}

/**
 * The one drop-down: a button that opens a list over the page, a chooser's (route-chooser.tsx) and
 * the banner's menu (app-shell.tsx) alike, the owner, 2026-10-28: "the menu should share code with
 * it, the only real difference being that there's no extra visual indicator that it's a dropdown",
 * and "the drop-down menu should be modal". The list is a modal dialog (modal-dialog.tsx): the page
 * behind it takes no press and focus stays in it while it is open. It closes when one of its places
 * is pressed; at a press outside it, which presses nothing else; at Escape and at Back, a key's or
 * the browser's own history step (Silk's on a Fire TV, D367 §10); at Left or Right, the list being one
 * column whose Up and Down move within it (the owner, 2026-10-28: "moving left/right while in a
 * chooser or menu closes it"), the press closing it alone; and by its own Close after its places,
 * which a remote's arrows reach. It opens at its button, a big screen's list its current place laid
 * over the button as a select's is, a phone's a pop-up over the button (useListPlacedAtOpener; the
 * owner, 2026-10-28: "the chooser opening a window in a different location feels weird"), on its
 * current place, else its first (useCurrentPlaceFocused); the menu, which has no choice, from its
 * top on its first place (isFromItsTop; the owner's Fire TV, 2026-10-29: "The menu opens up
 * partially scrolled down ... rather than showing from the top of the list"),
 * and neither its opening nor its closing moves the page under it (the owner, 2026-10-28, on a Fire
 * TV: the season picker "shifted the page up before displaying it"). Focus goes back to the button
 * (D361 §3).
 */
export function DropDown(props: {
  /** The list's name for a screen reader. */
  readonly label: string;
  /** What the button shows: a chooser's choice and its chevron, the menu's icon. */
  readonly opener: ReactNode;
  readonly openerClassName: string;
  readonly openerNaming: OpenerNaming;
  /** The class of the list's dialog, beside the drop-down's own. */
  readonly listClassName?: string;
  /**
   * Whether the list opens showing its top, on its first place, its top at the button's: a menu's,
   * which has no current choice to lay over its button, though the page it is on is marked.
   */
  readonly isFromItsTop?: boolean;
  /** Its places, each a list item. */
  readonly children: ReactNode;
}): JSX.Element {
  const [isOpen, setIsOpen] = useState(false);
  const openerRef = useRef<HTMLButtonElement>(null);
  const close = useCallback(() => {
    setIsOpen(false);
  }, []);
  return (
    <>
      <button
        ref={openerRef}
        type="button"
        className={props.openerClassName}
        aria-haspopup="dialog"
        aria-expanded={isOpen}
        {...props.openerNaming}
        onClick={() => {
          setIsOpen((wasOpen) => !wasOpen);
        }}
      >
        {props.opener}
      </button>
      {isOpen ? (
        <DropDownList
          label={props.label}
          className={props.listClassName}
          openerRef={openerRef}
          isFromItsTop={props.isFromItsTop === true}
          onClose={close}
        >
          {props.children}
        </DropDownList>
      ) : null}
    </>
  );
}

/** A drop-down's open list: its Close at the top-right, over its places. */
function DropDownList(props: {
  readonly label: string;
  readonly className: string | undefined;
  readonly openerRef: RefObject<HTMLElement | null>;
  readonly isFromItsTop: boolean;
  readonly onClose: () => void;
  readonly children: ReactNode;
}): JSX.Element {
  const { t } = useTranslation();
  const ref = useRef<HTMLDivElement>(null);
  const contentRef = useRef<HTMLDivElement>(null);
  const isClosingRef = useRef(false);
  const { onClose: tellClosed } = props;
  const onClose = useCallback(() => {
    isClosingRef.current = true;
    tellClosed();
  }, [tellClosed]);
  useChoiceCloses(ref, onClose);
  useFocusedPlaceShown(ref);
  useCurrentPlaceFocused(ref, props.isFromItsTop);
  useListPlacedAtOpener(ref, props.openerRef, props.isFromItsTop);
  useStuckArrowTold(contentRef, isClosingRef);
  // Heard on all the list holds, its Close too, which a remote's Up reaches (the owner, 2026-10-28:
  // "Left/right didn't close it", focus on the Close the opening had focused).
  useCommands(contentRef, (detail, event) => {
    if (detail.command === "left" || detail.command === "right") {
      event.preventDefault();
      onClose();
    }
  });
  const classes = ["modal-dialog--drop-down", "drop-down__list", props.className]
    .filter(Boolean)
    .join(" ");
  return (
    <ModalDialog
      className={classes}
      label={props.label}
      onClose={onClose}
      shouldCloseOnOutsidePress
    >
      <div ref={contentRef} className="drop-down__content">
        {/* Its Close, a cross at the top-right that stays put, its places scrolling under it (the
            owner, 2026-10-28: "a static close button in the top-right corner that the scrollable
            portion scrolls under"). */}
        <IconButton
          label={t("dropDown.close")}
          icon={<CloseIcon />}
          onClick={onClose}
          className="drop-down__close"
        />
        <div ref={ref} className="drop-down__places">
          <ul>{props.children}</ul>
        </div>
      </div>
    </ModalDialog>
  );
}
