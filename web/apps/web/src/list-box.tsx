// SPDX-License-Identifier: MPL-2.0

import "./list-box.css";

import {
  createContext,
  use,
  useEffect,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type JSX,
  type ReactNode,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import { ButtonByRole } from "./button-by-role.tsx";
import { caretOf, firstFocusTargetIn, isFocusTarget } from "./focus-engine.ts";
import { useCommands } from "./input-layer.tsx";
import { useLatest } from "./latest.ts";
import { Link } from "./link.tsx";
import { keepsOpenAttribute, tellPopoverContentChanged } from "./popover.tsx";
import type { Route } from "./router.ts";
import { ServicesContext } from "./services.tsx";

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
    if (list !== null) {
      focusCurrentEntryIn(list, isFromItsTop);
    }
  }, [ref, isFromItsTop]);
}

/** Focuses list's current entry, else, and always where isFromItsTop, its first (useCurrentEntryFocused). */
function focusCurrentEntryIn(list: HTMLElement, isFromItsTop: boolean): void {
  const current = isFromItsTop ? null : list.querySelector<HTMLElement>(currentSelector);
  const onCurrent =
    current === null || isFocusTarget(current) ? current : firstFocusTargetIn(current);
  (onCurrent ?? firstFocusTargetIn(list))?.focus({ preventScroll: true });
}

/**
 * Which submenu (ListSubmenu) holds its list's place, by its id, undefined for none, and the way to
 * open one or return from it: each list's own, and each open submenu's for its entries.
 */
interface SubmenuShown {
  readonly openId: string | undefined;
  readonly setOpenId: (id: string | undefined) => void;
}

const SubmenuShownContext = createContext<SubmenuShown>({
  openId: undefined,
  setOpenId: () => undefined,
});

/**
 * Whether an entry is hidden, a submenu of its list holding the list's place (ListSubmenu, D490):
 * hidden, not taken away, so a stepper's or a switch's state stays as it was when the list returns.
 */
function useIsUnderASubmenu(): boolean {
  return use(SubmenuShownContext).openId !== undefined;
}

/**
 * Whether entries are a picker screen's (PickerScreen), a submenu's open list: a menu has at most
 * one level of them (D538), so a submenu inside one is refused.
 */
const IsOnAPickerScreenContext = createContext(false);

/** The entries (children) of a list or of an open submenu, one of whose submenus may hold their place. */
function SubmenuScope(props: { readonly children: ReactNode }): JSX.Element {
  const [openId, setOpenId] = useState<string | undefined>(undefined);
  const shown = useMemo(() => ({ openId, setOpenId }), [openId]);
  return <SubmenuShownContext value={shown}>{props.children}</SubmenuShownContext>;
}

/**
 * A list of entries in one column, a pop-up's (popover.tsx) as a chooser's and a menu's: it opens on
 * its current entry, else its first, with at least an entry above and below it in view where it has
 * them, and keeps an entry either side of the one focused as the arrows move (D408 §3: the owner,
 * 2026-11-03, "showing the current value under the highlight, and at least 1 entry below and 1 entry
 * above"), scrolling within itself where it is taller than the room it is given. Its entries are
 * ListOption, a choice drawn as one; ListEntry, which holds a control of its own; ListNote, a line
 * that only informs; and ListSubmenu, an entry whose list takes the list's place (D490). It styles
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
      <SubmenuScope>{props.children}</SubmenuScope>
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
 * history's entry replaced where isReplacing, D367 §9), a link to a page elsewhere (`outside`),
 * opened in a tab of its own and told nothing of this one, or a button that does something
 * (`onChoose`), a button by role (ButtonByRole), as a pop-up of a long list's page holds them.
 * Choosing it closes the pop-up it is in (popover.tsx).
 */
export function ListOption(props: ListOptionTold & ListOptionDoes): JSX.Element {
  const isHidden = useIsUnderASubmenu();
  return (
    <li className="list-box__entry" hidden={isHidden}>
      <ListOptionControl {...props} />
    </li>
  );
}

/** What a choice of a list does: goes to a route, to a page elsewhere, or does something. */
type ListOptionDoes =
  | { readonly to: Route; readonly isReplacing?: boolean }
  | { readonly outside: string }
  | { readonly onChoose: () => void };

/** A choice's control: its link or its button (ListOption). */
function ListOptionControl(props: ListOptionTold & ListOptionDoes): JSX.Element {
  if ("outside" in props) {
    return (
      <a
        href={props.outside}
        className="list-box__option"
        target="_blank"
        rel="noreferrer noopener"
        aria-label={props.spoken}
      >
        {props.children}
      </a>
    );
  }
  if ("to" in props) {
    return (
      <Link
        to={props.to}
        className="list-box__option"
        isCurrent={props.isCurrent === true}
        isReplacing={props.isReplacing === true}
        {...(props.spoken === undefined ? {} : { label: props.spoken })}
      >
        {props.children}
      </Link>
    );
  }
  return (
    <ButtonByRole
      className="list-box__option"
      aria-current={props.isCurrent === true ? "true" : undefined}
      aria-label={props.spoken}
      onClick={props.onChoose}
    >
      {props.children}
    </ButtonByRole>
  );
}

/**
 * An entry of a list (ListBox) that holds a control of its own, drawn as that control draws itself:
 * a stepper, a switch, a line of text. Where isKeptOpen, pressing its buttons keeps the pop-up open,
 * for the next press, as a stepper's do (popover.tsx's keepsOpenAttribute). Where isInsetAsChoices,
 * what it holds is set in from the list's sides as a choice's text is, so a stepper's text lines up
 * with the choices' (⁝'s, player-menus.tsx). It takes no class from its user: a look a list needs
 * is a variant of its own (the 2026-10-05 architecture research's R5).
 */
export function ListEntry(props: {
  readonly isKeptOpen?: boolean;
  readonly isInsetAsChoices?: boolean;
  readonly children: ReactNode;
}): JSX.Element {
  const isHidden = useIsUnderASubmenu();
  const classes =
    props.isInsetAsChoices === true ? "list-box__entry list-box__entry--inset" : "list-box__entry";
  return (
    <li
      className={classes}
      hidden={isHidden}
      {...(props.isKeptOpen === true ? { [keepsOpenAttribute]: "" } : {})}
    >
      {props.children}
    </li>
  );
}

/**
 * A line of a list (ListBox) that only informs, as who is signed in where heads the banner's menu.
 * It wraps to the width the list's choices and controls set and never widens the list for itself:
 * what informs may wrap, what acts wraps only where nothing else will do (the owner, 2026-11-03).
 * Its look beyond that is its user's, on an element of its own inside it (app-shell.tsx's who).
 */
export function ListNote(props: { readonly children: ReactNode }): JSX.Element {
  const isHidden = useIsUnderASubmenu();
  return (
    <li className="list-box__note" hidden={isHidden}>
      {props.children}
    </li>
  );
}

/** What a press that keeps the pop-up open carries: a submenu's entry's and its back row's (popover.tsx). */
const keepsOpen = { [keepsOpenAttribute]: "" } as const;

/**
 * An entry of a list (ListBox) whose own list takes the list's place (D490; the owner, 2026-11-15:
 * "Putting everything at the top level is a time bomb"): it shows what it sets and its value now,
 * "Data use: 0.6 GB per hour ›", and opens at a press, OK or a click, or at Right. Its list's first
 * row is a back row, "‹ Data use", which returns, as Left and Back do, the remote's and the
 * browser's (OpenSubmenu), focus back on the entry. Its list opens on its current choice, else on
 * its back row, as a chooser's does (D408 §3). Opening and returning keep the pop-up open, laid at
 * its button again (popover.tsx's tellPopoverContentChanged); a choice in it closes the pop-up, as
 * any choice does. Its entries are the list's own kinds, but a submenu: a menu has at most one level
 * of them (D538), and one inside a submenu throws.
 */
export function ListSubmenu(props: {
  /** What it sets, in the person's words: "Data use". */
  readonly name: string;
  /** Its value now, as its list's current choice says it: "0.6 GB per hour". */
  readonly value: string;
  readonly children: ReactNode;
}): JSX.Element {
  const { t } = useTranslation();
  const id = useId();
  if (use(IsOnAPickerScreenContext)) {
    throw new Error(`a submenu inside a submenu (${props.name}): a menu has one level (D538)`);
  }
  const { openId, setOpenId } = use(SubmenuShownContext);
  const isOpen = openId === id;
  const entryRef = useRef<HTMLDivElement>(null);
  const wasOpenRef = useRef(false);
  useLayoutEffect(() => {
    if (wasOpenRef.current === isOpen) {
      return;
    }
    wasOpenRef.current = isOpen;
    if (!isOpen) {
      entryRef.current?.focus({ preventScroll: true });
    }
    tellPopoverContentChanged(entryRef.current);
  }, [isOpen]);
  useCommands(entryRef, (detail, event) => {
    if (detail.command === "right") {
      event.preventDefault();
      setOpenId(id);
    }
  });
  return (
    <li className="list-box__entry" hidden={openId !== undefined && !isOpen}>
      <ButtonByRole
        ref={entryRef}
        className="list-box__option list-box__submenu-entry"
        hidden={isOpen}
        {...keepsOpen}
        onClick={() => {
          setOpenId(id);
        }}
      >
        <span>{t("listBox.submenu.entry", { name: props.name, value: props.value })}</span>
        <span className="list-box__submenu-mark" aria-hidden="true">
          ›
        </span>
      </ButtonByRole>
      {isOpen ? (
        <PickerScreen
          name={props.name}
          onBack={() => {
            setOpenId(undefined);
          }}
        >
          {props.children}
        </PickerScreen>
      ) : null}
    </li>
  );
}

/**
 * A picker screen (D538), a submenu's list in its menu's place (ListSubmenu): its Back, "‹ Data use",
 * kept at its top while its entries scroll under it, then its entries, focused on its current choice
 * as it opens, else on Back. Left, but in a text field, where it is the
 * caret's, and Back return from it, a remote's Back and the browser's alike: an entry held in the
 * history while it is open, as a dialog's is (modal-layer.tsx), takes the browser's Back, which Silk
 * on a Fire TV sends for its remote's.
 */
export function PickerScreen(props: {
  /** What it picks, in the person's words, which its Back names: "Data use". */
  readonly name: string;
  readonly onBack: () => void;
  readonly children: ReactNode;
}): JSX.Element {
  const { t } = useTranslation();
  const ref = useRef<HTMLUListElement>(null);
  const onBackRef = useLatest(props.onBack);
  useLayoutEffect(() => {
    if (ref.current !== null) {
      focusCurrentEntryIn(ref.current, false);
    }
  }, []);
  useCommands(ref, (detail, event) => {
    const focused = document.activeElement;
    const isLeft =
      detail.command === "left" && (focused === null || caretOf(focused) === undefined);
    if (detail.command === "back" || isLeft) {
      event.preventDefault();
      onBackRef.current();
    }
  });
  const navigation = use(ServicesContext)?.navigation;
  useEffect(
    () =>
      navigation?.holdBack(() => {
        onBackRef.current();
      }),
    [navigation, onBackRef],
  );
  return (
    <ul ref={ref} className="list-box__submenu" aria-label={props.name}>
      <IsOnAPickerScreenContext value={true}>
        <SubmenuScope>
          <li className="list-box__entry list-box__submenu-head">
            <ButtonByRole
              className="list-box__option list-box__submenu-back"
              aria-label={t("listBox.submenu.back", { name: props.name })}
              {...keepsOpen}
              onClick={props.onBack}
            >
              <span className="list-box__submenu-mark" aria-hidden="true">
                ‹
              </span>
              <span>{props.name}</span>
            </ButtonByRole>
          </li>
          {props.children}
        </SubmenuScope>
      </IsOnAPickerScreenContext>
    </ul>
  );
}
