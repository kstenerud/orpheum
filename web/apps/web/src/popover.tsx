// SPDX-License-Identifier: MPL-2.0

import "./popover.css";

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

import { listPlace, type ScreenBox } from "./drop-down-place.ts";
import { directionOfKey, keyCodeOf } from "./input-buttons.ts";
import { useCommands } from "./input-layer.tsx";
import { useModalLayer, type ModalLayerTold } from "./modal-layer.tsx";
import { isPhoneWidth } from "./phone-width.ts";

/** What names the opening button for a screen reader and a pointer's hover, where its content does not. */
export interface OpenerNaming {
  readonly "aria-label"?: string;
  readonly "aria-describedby"?: string;
  readonly title?: string;
}

/**
 * The attribute that marks a part of a pop-up whose buttons change something in it, a stepper's or
 * the volume's, as the player's ⁝ and 🔈 hold (player-menus.tsx): pressing one keeps the pop-up open,
 * for the next press.
 */
export const keepsOpenAttribute = "data-keeps-open";

/** What has focus inside dialog, other than dialog itself; undefined for nothing. */
function focusedIn(dialog: HTMLDialogElement): HTMLElement | undefined {
  const focused = document.activeElement;
  return focused instanceof HTMLElement && focused !== dialog && dialog.contains(focused)
    ? focused
    : undefined;
}

/** The nearest box around element, inside dialog, that scrolls its content; undefined for none. */
function scrollerOf(element: HTMLElement, dialog: HTMLDialogElement): HTMLElement | undefined {
  for (let at = element.parentElement; at !== null && at !== dialog; at = at.parentElement) {
    const { overflowY } = getComputedStyle(at);
    if (overflowY === "auto" || overflowY === "scroll") {
      return at;
    }
  }
  return undefined;
}

/** How a pop-up is laid at its button, besides the room at the screen's edges. */
interface Placing {
  /**
   * Whether it opens from its top, its top at the button's, rather than its current choice laid over
   * the button: a menu's, which has no current choice, and a pop-up that holds no list.
   */
  readonly isFromItsTop: boolean;
}

/**
 * The part of the screen dialog, an open pop-up, may cover: the screen less the room at its edges
 * (.popover__room, tokens.css), as the browser lays that room out.
 */
function roomOf(dialog: HTMLDialogElement): ScreenBox {
  return (
    dialog.querySelector(".popover__room")?.getBoundingClientRect() ?? {
      left: 0,
      top: 0,
      width: window.innerWidth,
      height: window.innerHeight,
    }
  );
}

/**
 * Scrolls the content of dialog, an open pop-up laid at its button, so that focused, its current
 * choice, comes to the button's middle as far as it can, as a select's list opens, on a big screen;
 * and then brings it into view with the context its scroller keeps around it (its scroll-padding,
 * list-box.css): a neighbour either side, though that moves the choice off the button's middle
 * (D408 §3).
 */
function scrollFocusedToOpener(
  dialog: HTMLDialogElement,
  focused: HTMLElement,
  at: { readonly opener: DOMRect; readonly isPhone: boolean },
): void {
  const laid = focused.getBoundingClientRect();
  if (!at.isPhone) {
    scrollerOf(focused, dialog)?.scrollBy({
      top: laid.top + laid.height / 2 - (at.opener.top + at.opener.height / 2),
    });
  }
  focused.scrollIntoView({ block: "nearest", inline: "nearest" });
}

/**
 * Lays dialog, an open pop-up, at opener (drop-down-place.ts), inside the room at the screen's edges
 * (roomOf), as a list is; on a big screen, where the room's edge cuts it, cuts it to the height shown, its content
 * scrolling within it (a list, list-box.css), and brings what has focus in it, its current choice,
 * to the button (scrollFocusedToOpener). A pop-up shown from its top (isFromItsTop) has no choice to
 * lay over the button: its top is the button's, as a phone's is, and its content is not scrolled to
 * the button.
 */
function layAtOpener(
  dialog: HTMLDialogElement,
  at: { readonly opener: HTMLElement } & Placing,
): void {
  const openerBox = at.opener.getBoundingClientRect();
  const isPhone = isPhoneWidth();
  dialog.style.setProperty("--popover-opener-width", `${String(openerBox.width)}px`);
  dialog.style.removeProperty("max-height");
  const box = dialog.getBoundingClientRect();
  const room = roomOf(dialog);
  dialog.setAttribute("data-placed", isPhone ? "over" : "at");
  const focused = at.isFromItsTop ? undefined : focusedIn(dialog);
  const focusedBox = focused?.getBoundingClientRect();
  const { left, top, height } = listPlace({
    opener: openerBox,
    list: box,
    focusedMiddle:
      focusedBox === undefined ? undefined : focusedBox.top + focusedBox.height / 2 - box.top,
    room,
    isPhone,
  });
  dialog.style.setProperty("left", `${String(left)}px`);
  dialog.style.setProperty("top", `${String(top)}px`);
  if (height < box.height) {
    dialog.style.setProperty("max-height", `${String(height)}px`);
  }
  if (focused !== undefined) {
    scrollFocusedToOpener(dialog, focused, { opener: openerBox, isPhone });
  }
}

/**
 * Lays the open pop-up at the button that opened it (layAtOpener), as it opens, after what it holds
 * has focused its current choice, and again as the screen changes size. The dialog's `data-placed`
 * says how: `at` a big screen's button, or `over` a phone's.
 */
function usePlacedAtOpener(
  ref: RefObject<HTMLDialogElement | null>,
  openerRef: RefObject<HTMLElement | null>,
  placing: Placing,
): void {
  const { isFromItsTop } = placing;
  useEffect(() => {
    const dialog = ref.current;
    if (dialog === null) {
      return undefined;
    }
    const lay = (): void => {
      if (openerRef.current !== null) {
        layAtOpener(dialog, { opener: openerRef.current, isFromItsTop });
      }
    };
    lay();
    window.addEventListener("resize", lay);
    return () => {
      window.removeEventListener("resize", lay);
    };
  }, [ref, openerRef, isFromItsTop]);
}

/** What has focus, in a few words for the core's log: its tag and classes, and whether in it. */
function focusedDescribed(popover: Element | null): string {
  const focused = document.activeElement;
  if (focused === null || focused === document.body) {
    return "nothing";
  }
  const where = popover?.contains(focused) === true ? "in the list" : "outside the list";
  const named = [focused.tagName.toLowerCase(), ...focused.classList].join(".");
  return `${named} ${where}`;
}

/**
 * Tells the core's log of a Left or Right that left the pop-up open: on the owner's Fire TV (Silk
 * 138, spatial navigation) neither closed it with focus on a place, where Chromium and Firefox close
 * it by the same key (2026-10-28; idiosyncrasies.md). It says which key came, whether something had
 * taken it before the app heard it and after, and where focus was, so the next check on the device
 * shows which layer kept it. isClosingRef says it was told to close, which it does a render later.
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
 * Closes the pop-up as the page enters or leaves full screen, a layout's change under it: on a Fire
 * TV Silk's Back leaves full screen and is no key the page hears, so it is the one press that closes
 * it there (D407 §8; idiosyncrasies.md, a modal dialog removed while open).
 */
function useFullScreenChangeCloses(close: () => void): void {
  useEffect(() => {
    document.addEventListener("fullscreenchange", close);
    return () => {
      document.removeEventListener("fullscreenchange", close);
    };
  }, [close]);
}

/**
 * Closes the pop-up when a link or a button in it is pressed (not one marked keepsOpenAttribute):
 * after what it does, as a choice's route followed (link.tsx), heard at the document, past the page's
 * root. Heard on the pop-up itself, the close ran first, between the browser's listeners, and it was
 * gone before React heard the press at its root, so a link's own handling never ran and the browser
 * followed it (the browser test, 2026-10-28).
 */
function useChoiceCloses(ref: RefObject<HTMLElement | null>, close: () => void): void {
  useEffect(() => {
    const chosen = (event: MouseEvent): void => {
      const popover = ref.current;
      const { target } = event;
      if (
        popover !== null &&
        target instanceof Element &&
        popover.contains(target) &&
        target.closest("a, button") !== null &&
        target.closest(`[${keepsOpenAttribute}]`) === null
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

/** What a pop-up is told. */
interface PopoverProps extends Placing {
  /** Its name for a screen reader. */
  readonly label: string;
  /** The button that opened it, which it is laid at. */
  readonly openerRef: RefObject<HTMLElement | null>;
  readonly onClose: () => void;
  readonly children: ReactNode;
}

/**
 * A pop-up, a quick control at the button that opened it (D408 §2): the modal layer
 * (modal-layer.tsx), so it closes at Escape, Back, the browser's Back and a press outside, which
 * presses nothing else, focus going back to its button; laid at its button (layAtOpener); as big as
 * what it holds within the screen, no minimum width and no Close (the owner, 2026-11-03: "For quick
 * control popups this doesn't make sense, especially since selecting within them closes the
 * widget"). It closes too when a link or a button in it is pressed (useChoiceCloses), at Left or
 * Right, what it holds being one column whose Up and Down move within it (the owner, 2026-10-28:
 * "moving left/right while in a chooser or menu closes it"), the press closing it alone, and as full
 * screen changes. Neither its opening nor its closing moves the page under it (the owner, 2026-10-28,
 * on a Fire TV: the season picker "shifted the page up before displaying it").
 */
export function Popover(props: PopoverProps): JSX.Element {
  const ref = useRef<HTMLDialogElement>(null);
  const contentRef = useRef<HTMLDivElement>(null);
  const isClosingRef = useRef(false);
  const { onClose: tellClosed } = props;
  const onClose = useCallback(() => {
    isClosingRef.current = true;
    tellClosed();
  }, [tellClosed]);
  const toldRef = useRef<ModalLayerTold>({ onClose });
  useEffect(() => {
    toldRef.current = { onClose };
  });
  useModalLayer(ref, toldRef);
  useChoiceCloses(ref, onClose);
  useFullScreenChangeCloses(onClose);
  usePlacedAtOpener(ref, props.openerRef, {
    isFromItsTop: props.isFromItsTop,
  });
  useStuckArrowTold(contentRef, isClosingRef);
  useCommands(contentRef, (detail, event) => {
    if (detail.command === "left" || detail.command === "right") {
      event.preventDefault();
      onClose();
    }
  });
  return (
    <dialog ref={ref} className="popover" aria-label={props.label}>
      <div ref={contentRef} className="popover__content">
        {props.children}
      </div>
      <div className="popover__room" aria-hidden="true" />
    </dialog>
  );
}

/**
 * A button that opens a pop-up (Popover) of what it holds, and closes it when pressed again: the
 * player's 🔈, and every drop-down (drop-down.tsx). It says whether the pop-up is open
 * (aria-expanded), and tells onOpenChange each time it opens or closes, as the player's overlay holds
 * its time (D407 §6).
 */
export function PopoverButton(props: {
  /** The pop-up's name for a screen reader. */
  readonly label: string;
  /** What the button shows. */
  readonly opener: ReactNode;
  readonly openerClassName: string;
  readonly openerNaming: OpenerNaming;
  /** Whether the pop-up opens from its top (Popover's isFromItsTop). */
  readonly isFromItsTop: boolean;
  readonly onOpenChange?: ((isOpen: boolean) => void) | undefined;
  readonly children: ReactNode;
}): JSX.Element {
  const [isOpen, setIsOpen] = useState(false);
  const openerRef = useRef<HTMLButtonElement>(null);
  const { onOpenChange } = props;
  const onOpenChangeRef = useRef(onOpenChange);
  useEffect(() => {
    onOpenChangeRef.current = onOpenChange;
  });
  const setOpen = useCallback((isNowOpen: boolean) => {
    setIsOpen(isNowOpen);
    onOpenChangeRef.current?.(isNowOpen);
  }, []);
  const close = useCallback(() => {
    setOpen(false);
  }, [setOpen]);
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
          setOpen(!isOpen);
        }}
      >
        {props.opener}
      </button>
      {isOpen ? (
        <Popover
          label={props.label}
          openerRef={openerRef}
          isFromItsTop={props.isFromItsTop}
          onClose={close}
        >
          {props.children}
        </Popover>
      ) : null}
    </>
  );
}
