// SPDX-License-Identifier: MPL-2.0

import "./list-box.css";

import { useEffect, useRef, type JSX, type ReactNode, type RefObject } from "react";

import { focusTargetsIn, isFocusTarget } from "./focus-engine.ts";
import { Link } from "./link.tsx";
import { keepsOpenAttribute } from "./popover.tsx";
import type { Route } from "./router.ts";

/**
 * Brings a focused entry of the list into its view with the context its scroll padding keeps, an
 * entry either side (list-box.css; D408 §3), where the list scrolls within itself: Firefox left an
 * entry partly below its end in view (the browser test, 2026-10-28), which a remote's focus could
 * then not show whole, and with no padding the entry moved to sat against the list's edge.
 */
function useFocusedEntryShown(ref: RefObject<HTMLElement | null>): void {
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

/** An entry marked as the current one: a page's link (`page`), a chosen button (`true`). */
const currentSelector = '[aria-current]:not([aria-current="false"])';

/**
 * Focuses the list's current entry as it opens, the choice a chooser shows (route-chooser.tsx),
 * marked aria-current, else, and always where isFromItsTop, its first (the owner's Fire TV,
 * 2026-10-29: "The menu opens up partially scrolled down ... rather than showing from the top of the
 * list"). Focused without scrolling the page, which a pop-up's opening never moves; the list shows it
 * within itself (useFocusedEntryShown), and the pop-up lays it at its button (popover.tsx).
 */
function useCurrentEntryFocused(ref: RefObject<HTMLElement | null>, isFromItsTop: boolean): void {
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

/**
 * A list of entries in one column, a pop-up's (popover.tsx) as a chooser's and a menu's: it opens on
 * its current entry, else its first, with at least an entry above and below it in view where it has
 * them, and keeps an entry either side of the one focused as the arrows move (D408 §3: the owner,
 * 2026-11-03, "showing the current value under the highlight, and at least 1 entry below and 1 entry
 * above"), scrolling within itself where it is taller than the room it is given. Its entries are
 * ListOption, a choice drawn as one, and ListEntry, which holds a control of its own; it styles
 * nothing else inside them.
 */
export function ListBox(props: {
  /** Whether it opens on its first entry, a menu's, which has no current choice. */
  readonly isFromItsTop: boolean;
  readonly children: ReactNode;
}): JSX.Element {
  const ref = useRef<HTMLUListElement>(null);
  useFocusedEntryShown(ref);
  useCurrentEntryFocused(ref, props.isFromItsTop);
  return (
    <ul ref={ref} className="list-box">
      {props.children}
    </ul>
  );
}

/** What a choice of a list is told beside what it does. */
interface ListOptionTold {
  /** Whether it is the current choice, marked for the eye and a screen reader alike. */
  readonly isCurrent?: boolean;
  /** Its name for a screen reader, where what it shows says only part of it: "Name, A–Z". */
  readonly spoken?: string | undefined;
  readonly children: ReactNode;
}

/**
 * A choice of a list (ListBox), drawn as one, the width of the list: a link to a route (`to`, its
 * history's entry replaced where isReplacing, D367 §9), or a button that does something (`onChoose`).
 * Choosing it closes the pop-up it is in (popover.tsx).
 */
export function ListOption(
  props: ListOptionTold &
    ({ readonly to: Route; readonly isReplacing?: boolean } | { readonly onChoose: () => void }),
): JSX.Element {
  return (
    <li className="list-box__entry">
      {"to" in props ? (
        <Link
          to={props.to}
          className="list-box__option"
          isCurrent={props.isCurrent === true}
          isReplacing={props.isReplacing === true}
          {...(props.spoken === undefined ? {} : { label: props.spoken })}
        >
          {props.children}
        </Link>
      ) : (
        <button
          type="button"
          className="list-box__option"
          aria-current={props.isCurrent === true ? "true" : undefined}
          aria-label={props.spoken}
          onClick={props.onChoose}
        >
          {props.children}
        </button>
      )}
    </li>
  );
}

/**
 * An entry of a list (ListBox) that holds a control of its own, drawn as that control draws itself:
 * a stepper, a switch, a line of text. Where isKeptOpen, pressing its buttons keeps the pop-up open,
 * for the next press, as a stepper's do (popover.tsx's keepsOpenAttribute).
 */
export function ListEntry(props: {
  readonly isKeptOpen?: boolean;
  readonly className?: string;
  readonly children: ReactNode;
}): JSX.Element {
  const classes = ["list-box__entry", props.className].filter(Boolean).join(" ");
  return (
    <li className={classes} {...(props.isKeptOpen === true ? { [keepsOpenAttribute]: "" } : {})}>
      {props.children}
    </li>
  );
}
