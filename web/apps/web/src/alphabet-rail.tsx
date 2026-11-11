// SPDX-License-Identifier: MPL-2.0

import { reportFailure } from "@orpheum/sdk";
import { useCallback, useEffect, useRef, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { clearOfTheBannerPx, focusAndShow } from "./focus-navigation.ts";
import { filtersKey, filtersQuery, kindsQuery, type ListedKind } from "./list-filters.ts";
import { useBackgroundChangesPoller } from "./polling.ts";
import type { ListFilters } from "./router.ts";
import { dataOrThrow } from "./problem.ts";
import { useRovingTabStop } from "./roving-tab-stop.ts";
import { useServices } from "./services.tsx";
import type { Pages } from "./use-pages.ts";
import { useResource } from "./use-resource.ts";

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
 * title in the list begins with, as the server names them (GET /items/letters); every letter,
 * undefined, for a list that has not asked. A list Z to A lists its letters Z to A, # last (D366 §4).
 * It is one Tab stop, as a poster grid is (useRovingTabStop; the owner, 2026-11-11): Tab enters it at
 * the letter last focused and leaves it with the next, the arrows moving from letter to letter.
 */
export function AlphabetRail(props: {
  readonly onLetter: (letter: string) => void;
  readonly letters?: readonly string[] | undefined;
  readonly isZToA?: boolean | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const given =
    props.letters === undefined
      ? railLetters
      : railLetters.filter((letter) => props.letters?.includes(letter) === true);
  const shown = props.isZToA === true ? [...given].reverse() : given;
  const railRef = useRef<HTMLOListElement>(null);
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
          <button
            type="button"
            className="alphabet-rail__letter"
            onClick={() => {
              props.onLetter(letter);
            }}
          >
            {letter}
          </button>
        </li>
      ))}
    </ol>
  );
}

/**
 * The name a jump to a letter asks the list to start from (`/items?from=`), none for the list's
 * start. A to Z, the letter itself, # the start; Z to A, where the server starts at the last title
 * before the name, the letter after it, Z the start and # A (D366 §4).
 */
export function fromForLetter(letter: string, isZToA: boolean): { readonly from?: string } {
  if (!isZToA) {
    return letter === "#" ? {} : { from: letter };
  }
  const next = railLetters[railLetters.indexOf(letter) + 1];
  return next === undefined ? {} : { from: next };
}

/**
 * The alphabet rail's letters for a list, by its kind, library and sort: only those some title in
 * it begins with (D349 §9), as the server names them (`GET /items/letters`), asked for again
 * with the list's own background poll (D237); undefined while that first answer is loading, for
 * AlphabetRail's own letters prop. Films, Series and the Libraries page all ask this one way.
 */
export function useAlphabetRailLetters(list: {
  readonly kind: ListedKind;
  readonly library: number | undefined;
  readonly sort: "name" | "fileName";
  /** The list's search, "" for none: only letters some matching title begins with. */
  readonly search: string;
  /** The list's chips (Films and Series): only letters some title they keep begins with. */
  readonly filters?: ListFilters | undefined;
}): readonly string[] | undefined {
  const { kind, library, sort, search, filters } = list;
  const { api } = useServices();
  const [letters] = useResource(
    `alphabet-rail-letters/${kind}/${String(library)}/${sort}/${search}/${filtersKey(filters)}`,
    async () => {
      const body = dataOrThrow(
        await api.GET("/items/letters", {
          params: {
            query: {
              kind: kindsQuery(kind),
              sort,
              ...(library === undefined ? {} : { library }),
              ...(search === "" ? {} : { query: search }),
              ...(filters === undefined ? {} : filtersQuery(filters)),
            },
          },
        }),
      );
      return body.letters ?? [];
    },
    useBackgroundChangesPoller(),
  );
  return letters.state === "loaded" ? letters.value : undefined;
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
 * list has more (which the caller loads), undefined once done or once focus went on from it.
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
  const posters = list?.querySelectorAll<HTMLElement>(".poster-grid > li > a");
  const poster = place < 0 ? posters?.[posters.length - 1] : posters?.[place];
  if (poster === undefined) {
    return jumpedTo;
  }
  return showAtTop(poster) && hasMore ? { place, shown: poster } : undefined;
}

/**
 * Shows the poster a jump found once it is drawn, at each drawing of the list, loading the page
 * after it while the page's end stops it short of the top and the person has not gone on from it;
 * a way to say which poster, by its place in the list's items, the last's for -1.
 */
function useJumpedToShown<T>(
  pages: Pages<T>,
  listRef: RefObject<HTMLElement | null>,
): (place: number) => void {
  const { loadMore, loadedSoFar } = pages;
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
    const hasMore = loadedSoFar.state === "loaded" && loadedSoFar.value.nextCursor !== "";
    const left = showJumpedTo(jumpedTo, listRef.current, hasMore);
    toFocusRef.current = left;
    if (left?.shown !== undefined) {
      loadMore();
    }
  }, [found, loadedSoFar, listRef, loadMore]);
  return useCallback((place: number) => {
    toFocusRef.current = { place };
    setFound((count) => count + 1);
  }, []);
}

/**
 * Moves a list by name or file name to the first title at or after the letter jump names, as the
 * server sorts them (`from`), # to the first of all; Z to A, to the first of the letter's titles in
 * that order (fromForLetter): it loads the list's pages up to it, as
 * scrolling would, and focuses its poster at the top of the view, so a remote goes on from there;
 * where no title comes at or after the letter, the last. Where the poster is in the last page
 * loaded and the page ends too soon below it to scroll it to the top, it loads the next page and
 * shows it there again, until it is at the top or the list ends (the owner, 2026-11-02), unless the
 * person went on from it in between. The newest jump wins over one still loading.
 */
export function useLetterJump<T extends { readonly id: number }>(props: {
  readonly kind: ListedKind;
  readonly sort: "name" | "fileName";
  readonly isZToA?: boolean | undefined;
  readonly library: number | undefined;
  /** The list's search, "" for none: a jump finds its letter among what the search lists. */
  readonly search?: string;
  /** The list's chips: a jump finds its letter among what they keep. */
  readonly filters?: ListFilters | undefined;
  readonly jump: LetterJump | undefined;
  readonly pages: Pages<T>;
  readonly listRef: RefObject<HTMLElement | null>;
}): void {
  const { api } = useServices();
  const { kind, sort, library, jump, listRef } = props;
  const { genre, decade, isUnwatchedOnly } = props.filters ?? {};
  const [search, isZToA] = [props.search ?? "", props.isZToA === true];
  const { loadUntil } = props.pages;
  const showWhenDrawn = useJumpedToShown(props.pages, listRef);
  useEffect(() => {
    if (jump === undefined) {
      return undefined;
    }
    let isCurrent = true;
    const query = {
      kind: kindsQuery(kind),
      sort,
      limit: 1,
      ...(isZToA ? { order: "descending" as const } : {}),
      ...(library === undefined ? {} : { library }),
      ...(search === "" ? {} : { query: search }),
      ...filtersQuery({ genre, decade, isUnwatchedOnly }),
      ...fromForLetter(jump.letter, isZToA),
    };
    api
      .GET("/items", { params: { query } })
      .then(async (answer) => {
        const first = dataOrThrow(answer).items?.[0];
        const place = await loadUntil((item) => item.id === first?.id);
        if (isCurrent) {
          showWhenDrawn(place ?? -1);
        }
      })
      .catch(reportFailure);
    return () => {
      isCurrent = false;
    };
  }, [
    api,
    kind,
    sort,
    isZToA,
    library,
    search,
    genre,
    decade,
    isUnwatchedOnly,
    jump,
    loadUntil,
    showWhenDrawn,
  ]);
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
