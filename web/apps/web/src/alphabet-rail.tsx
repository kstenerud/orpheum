// SPDX-License-Identifier: MPL-2.0

import {
  createContext,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type JSX,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import { ButtonByRole } from "./button-by-role.tsx";
import { clearOfTheBannerPx, focusAndShow } from "./focus-navigation.ts";
import { useLatest } from "./latest.ts";
import { standTabStopAt, useRovingTabStop } from "./roving-tab-stop.ts";
import type { Pages } from "./use-pages.ts";

/** A letter of the alphabet rail pressed, counted, so the same letter pressed again jumps again. */
export interface LetterJump {
  readonly letter: string;
  readonly count: number;
}

/** The letters of the alphabet rail, a list's names' first characters: digits and marks, then A to Z. */
export const railLetters = [
  "#",
  ...Array.from({ length: 26 }, (_, index) => String.fromCodePoint(0x41 + index)),
];

/**
 * A list's alphabet beside its grid, by name only, to jump to a letter (the owner, 2026-10-25): a
 * letter moves the list to its first title, # to the first of all, those beginning with a digit or
 * a mark, a focus container of its own. Given letters (D349 §9.1), only those shown: the ones some
 * title in the list is under, as its entries say (lettersOf); every letter, undefined, for a list
 * not read yet. A list Z to A lists its letters Z to A, # last (D366 §4).
 * It is one Tab stop, as a poster grid is (useRovingTabStop; the owner, 2026-11-11): Tab enters it at
 * the letter last focused and leaves it with the next, the arrows moving from letter to letter.
 * Its letters are buttons by role (ButtonByRole), not <button>s: Silk on a Fire TV lays the whole
 * page out each time a key focuses a <button>, and a move onto the rail took 300 ms, the first 5 s.
 */
export function AlphabetRail(props: {
  readonly onLetter: (letter: string) => void;
  /** The rail, for the list beside it to stand it at its focused poster's letter (useRailStand). */
  readonly railRef?: RefObject<HTMLOListElement | null> | undefined;
  readonly letters?: readonly string[] | undefined;
  readonly isZToA?: boolean | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const given =
    props.letters === undefined
      ? railLetters
      : railLetters.filter((letter) => props.letters?.includes(letter) === true);
  const shown = props.isZToA === true ? [...given].reverse() : given;
  const ownRef = useRef<HTMLOListElement>(null);
  const railRef = props.railRef ?? ownRef;
  useRovingTabStop(railRef);
  return (
    <ol
      ref={railRef}
      className="alphabet-rail"
      aria-label={t("browse.alphabet")}
      data-focus-container=""
    >
      {shown.map((letter) => (
        <li key={letter}>
          <ButtonByRole
            className="alphabet-rail__letter"
            onClick={() => {
              props.onLetter(letter);
            }}
          >
            {letter}
          </ButtonByRole>
        </li>
      ))}
    </ol>
  );
}

/** Stands a list's alphabet rail at a letter, # or A to Z, by the entry's letter the list says. */
export type StandRailAt = (letter: string | undefined) => void;

/**
 * Where a poster grid says its focused poster's letter: to the rail beside it (useRailStand), so Tab
 * or an arrow from the poster to the rail comes to its letter (the owner, 2026-11-11); nowhere by
 * default, a grid with no rail.
 */
export const RailStandContext = createContext<StandRailAt>(() => undefined);

/**
 * A list's alphabet rail, and how to stand it at a letter: its letter becomes the rail's Tab stop and
 * where an arrow entering the rail goes (standTabStopAt), unless focus is in the rail already. A
 * letter the rail does not show (its letters still loading) leaves it as it is.
 */
export function useRailStand(): {
  readonly railRef: RefObject<HTMLOListElement | null>;
  readonly standAt: StandRailAt;
} {
  const railRef = useRef<HTMLOListElement>(null);
  const standAt = useCallback((letter: string | undefined) => {
    const rail = railRef.current;
    const button = [...(rail?.querySelectorAll<HTMLElement>(".alphabet-rail__letter") ?? [])].find(
      (each) => each.textContent === letter,
    );
    if (rail !== null && button !== undefined) {
      standTabStopAt(rail, button);
    }
  }, []);
  return useMemo(() => ({ railRef, standAt }), [standAt]);
}

/**
 * The rail letters a list's entries are under, in the rail's order: only those some entry is under
 * (D349 §9), from the list itself, which comes whole with each entry's letter (D475), so its poll
 * keeps them; undefined while it is read. Films, Series and the Libraries page all take them this
 * one way.
 */
export function lettersOf(
  entries: readonly { readonly letter?: string | undefined }[] | undefined,
): readonly string[] | undefined {
  if (entries === undefined) {
    return undefined;
  }
  const under = new Set(entries.map((entry) => entry.letter));
  return railLetters.filter((letter) => under.has(letter));
}

/**
 * Where a jump to a letter lands among a list's entries by name, by the letter each is under: A to
 * Z, the first under the letter or one after it, # the first of all; Z to A, the first under the
 * letter or one before it, Z the first of all (D366 §4). -1 where none is.
 */
export function placeOfLetter(
  entries: readonly { readonly letter?: string | undefined }[],
  letter: string,
  isZToA: boolean,
): number {
  const pressed = railLetters.indexOf(letter);
  return entries.findIndex((entry) => {
    const under = railLetters.indexOf(entry.letter ?? "");
    return under >= 0 && (isZToA ? under <= pressed : under >= pressed);
  });
}

/**
 * Says a list's rail letters (lettersOf) to the rail beside it, as the list is read and as its poll
 * reads it again: the rail is the list's sibling, which holds them for it.
 */
export function useHeldLetters<T extends { readonly letter?: string | undefined }>(
  pages: Pages<T>,
  onLetters: (letters: readonly string[] | undefined) => void,
): void {
  const { held } = pages;
  useEffect(() => {
    onLetters(lettersOf(held));
  }, [held, onLetters]);
}

/**
 * The letters a list's rail shows, as its list says them (useHeldLetters), and a way for the list
 * to say them: kept by the page that holds both, the list and its rail being siblings. The same
 * letters said again keep the ones shown.
 */
export function useRailLetters(): [
  readonly string[] | undefined,
  (letters: readonly string[] | undefined) => void,
] {
  const [letters, setLetters] = useState<readonly string[] | undefined>(undefined);
  const onLetters = useCallback((said: readonly string[] | undefined) => {
    setLetters((shown) => (shown?.join() === said?.join() ? shown : said));
  }, []);
  return [letters, onLetters];
}

/**
 * Focuses a poster jumped to, its row scrolled to the view's top, clear of the banner, and kept there
 * while the list around it settles (keepAtTop); whether the page's end stopped it short, below the
 * top, as it does where too little of the list is drawn after the poster to scroll that far.
 */
export function showAtTop(poster: HTMLElement): boolean {
  // A test's DOM may have no scrolling at all.
  if (typeof poster.scrollIntoView === "function") {
    poster.scrollIntoView({ block: "start" });
  }
  // Focused as a key would, a remote's press being one; scrolled on, gently, clear of the banner.
  focusAndShow(poster, true);
  keepAtTop(poster);
  const isScrolledToTheEnd =
    window.scrollY >= document.documentElement.scrollHeight - window.innerHeight - 1;
  return isScrolledToTheEnd && poster.getBoundingClientRect().top > clearOfTheBannerPx() + 1;
}

/**
 * The poster a jump shows, by its place in the list's items, the last's for -1, and once shown, the
 * poster itself.
 */
interface JumpedTo {
  readonly place: number;
  readonly shown?: HTMLElement;
}

/**
 * What a person does to go on from a poster: a page scrolled by its layout alone (the browser
 * keeping what is on screen in place as the list grows) is none of these.
 */
const goingOnEvents = ["wheel", "touchstart", "keydown", "pointerdown"] as const;

/**
 * How long a poster jumped to is kept at the top while the list around it settles, in milliseconds:
 * the few frames its newly drawn cards take to be laid out at their own heights, with room for a
 * slow device's frames. A justified constant, a bound on a wait, not a measure of the list.
 */
const settlingMs = 1000;

/** Ends the keeping of the poster last jumped to at the top, if it is still kept. */
let stopKeeping: (() => void) | undefined;

/**
 * Keeps poster at the top, clear of the banner, while the list around it settles: its cards are laid
 * out only near the view (content-visibility), so those drawn by the jump take their own heights a
 * frame after it, and Firefox keeps some other card in place as they do, carrying the poster down
 * the page (the owner, Firefox, 2026-11-11: the ring started at the page's top and moved down onto
 * the poster). Each time the list changes size, before it is drawn, the poster goes back to the top;
 * it stops once the person goes on, focus leaves the poster, or the list has had time to settle.
 */
function keepAtTop(poster: HTMLElement): void {
  stopKeeping?.();
  const list = poster.closest("ul, ol");
  if (list === null || typeof ResizeObserver === "undefined") {
    return;
  }
  const resizes = new ResizeObserver(() => {
    if (document.activeElement !== poster) {
      stop();
      return;
    }
    const off = poster.getBoundingClientRect().top - clearOfTheBannerPx();
    if (Math.abs(off) > 1) {
      window.scrollBy({ top: off });
    }
  });
  const timer = window.setTimeout(stop, settlingMs);
  function stop(): void {
    resizes.disconnect();
    window.clearTimeout(timer);
    for (const event of goingOnEvents) {
      window.removeEventListener(event, stop, { capture: true });
    }
    stopKeeping = undefined;
  }
  for (const event of goingOnEvents) {
    window.addEventListener(event, stop, { capture: true, passive: true });
  }
  resizes.observe(list);
  stopKeeping = stop;
}

/**
 * Shows the poster jumpedTo names in list at the top, if it is drawn; what is left to do: jumpedTo
 * itself while it is not drawn yet, the poster shown while the page's end stopped it short and the
 * list has more to draw (which the caller draws), undefined once done or once focus went on from it.
 */
function showJumpedTo(
  jumpedTo: JumpedTo,
  list: HTMLElement | null,
  hasMore: boolean,
): JumpedTo | undefined {
  const { place, shown } = jumpedTo;
  if (shown !== undefined && document.activeElement !== shown) {
    // Focus went on from the poster before the page after it came.
    return undefined;
  }
  const poster = list?.querySelectorAll<HTMLElement>(".poster-grid > li > a")[place];
  if (poster === undefined) {
    return jumpedTo;
  }
  return showAtTop(poster) && hasMore ? { place, shown: poster } : undefined;
}

/**
 * How many entries a jump draws past the one it found, at once: rows enough below it to scroll it to
 * the view's top.
 */
const drawnPastAJump = 60;

/**
 * Shows the poster a jump found once it is drawn, at each drawing of the list, drawing the rows
 * after it while the page's end stops it short of the top and the person has not gone on from it;
 * a way to say which poster, by its place in the list's items.
 */
function useJumpedToShown<T>(
  pages: Pages<T>,
  listRef: RefObject<HTMLElement | null>,
): (place: number) => void {
  const { drawThrough, loadedSoFar, held } = pages;
  // The poster to focus once it is drawn, undefined for none.
  const toFocusRef = useRef<JumpedTo | undefined>(undefined);
  const [found, setFound] = useState(0);
  useEffect(() => {
    const goOn = (): void => {
      if (toFocusRef.current?.shown !== undefined) {
        toFocusRef.current = undefined;
      }
    };
    for (const event of goingOnEvents) {
      window.addEventListener(event, goOn, { capture: true, passive: true });
    }
    return () => {
      for (const event of goingOnEvents) {
        window.removeEventListener(event, goOn, { capture: true });
      }
    };
  }, []);
  useEffect(() => {
    const jumpedTo = toFocusRef.current;
    if (jumpedTo === undefined) {
      return;
    }
    const drawn = loadedSoFar.state === "loaded" ? loadedSoFar.value.items.length : 0;
    const left = showJumpedTo(jumpedTo, listRef.current, drawn < (held?.length ?? 0));
    toFocusRef.current = left;
    if (left?.shown !== undefined) {
      drawThrough(drawn + drawnPastAJump);
    }
  }, [found, loadedSoFar, held, listRef, drawThrough]);
  return useCallback((place: number) => {
    toFocusRef.current = { place };
    setFound((count) => count + 1);
  }, []);
}

/**
 * Moves a list by name or file name to the first title under the letter jump names or one after it,
 * # to the first of all; Z to A, to the first under it or one before it (placeOfLetter): it finds
 * the place among the entries the list holds, whole (D475), with no request, draws the list through
 * it at once, and focuses its poster at the top of the view, so a remote goes on from there; where
 * no title is under the letter or one after it, the last. Where the list's drawn end is too soon
 * below the poster to scroll it to the top, it draws more and shows it there again, until it is at
 * the top or the list ends (the owner, 2026-11-02), unless the person went on from it in between. A
 * jump pressed before the list is read lands once it is.
 */
export function useLetterJump<T extends { readonly letter?: string | undefined }>(props: {
  readonly isZToA?: boolean | undefined;
  readonly jump: LetterJump | undefined;
  readonly pages: Pages<T>;
  readonly listRef: RefObject<HTMLElement | null>;
}): void {
  const { jump, listRef, pages } = props;
  const isZToA = props.isZToA === true;
  const showWhenDrawn = useJumpedToShown(pages, listRef);
  // Read through a ref, so the list read again does not jump again: only a letter, or the list's
  // first read, does.
  const heldRef = useLatest(pages.held);
  const isHeld = pages.held !== undefined;
  const { drawThrough } = pages;
  useEffect(() => {
    const held = heldRef.current;
    if (jump === undefined || held === undefined || held.length === 0) {
      return;
    }
    const found = placeOfLetter(held, jump.letter, isZToA);
    const place = found < 0 ? held.length - 1 : found;
    drawThrough(place + 1 + drawnPastAJump);
    showWhenDrawn(place);
  }, [jump, isHeld, isZToA, drawThrough, showWhenDrawn, heldRef]);
}

/**
 * A letter jump counted again, for the same letter pressed twice in a row to jump again, and a way
 * to clear it: a list remounted by a sort change starts with none, not the last letter pressed in
 * another order.
 */
export function useLetterJumpState(): [
  LetterJump | undefined,
  (letter: string) => void,
  () => void,
] {
  const [jump, setJump] = useState<LetterJump | undefined>(undefined);
  const onLetter = useCallback((letter: string) => {
    setJump((last) => ({ letter, count: (last?.count ?? 0) + 1 }));
  }, []);
  const clear = useCallback(() => {
    setJump(undefined);
  }, []);
  return [jump, onLetter, clear];
}

/**
 * Clears a letter jump when key changes: a list remounted under another library, sort, text or chip
 * starts with none, not the last letter pressed in another one; shared by Films, Series and the
 * Libraries page alike.
 */
export function useResetJumpOnChange(key: string, clear: () => void): void {
  const lastKeyRef = useRef(key);
  useEffect(() => {
    if (lastKeyRef.current !== key) {
      lastKeyRef.current = key;
      clear();
    }
  }, [key, clear]);
}
