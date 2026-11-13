// SPDX-License-Identifier: MPL-2.0

import { reportFailure } from "@orpheum/sdk";
import { useCallback, useEffect, useRef, useState } from "react";
import { flushSync } from "react-dom";

import { inPieces } from "./in-pieces.ts";

import type { AskOutcome, Poller } from "./polling.ts";

import type { Remembered } from "./remembered.ts";

import type { Resource } from "./use-resource.ts";

/** One page of a list from the server, and the cursor of the next; "" on the last. */
export interface Page<T> {
  readonly items: readonly T[];
  readonly nextCursor: string;
  /**
   * How many items the whole list holds, as its pages say it, where its server counts them: a view
   * sizes the list by it before the other pages come, and each page read says where the list's end
   * now is (the owner, 2026-11-11).
   */
  readonly total?: number | undefined;
}

/**
 * How many items of a list have not come yet, as its total says: none once its last page has come,
 * and none where it says no total, or fewer than have come (a hint, as its pages may change).
 */
export function itemsToCome<T>(page: Page<T>): number {
  if (page.nextCursor === "" || page.total === undefined) {
    return 0;
  }
  return Math.max(0, page.total - page.items.length);
}

/**
 * A list read a page at a time, or whole: what is drawn so far, merged into one page, every item
 * read (held), whether more is loading or still being drawn, a way to ask for more, a way to draw
 * through a place at once, and a way to say which of its items are in view, whose pages a poll reads.
 */
export interface Pages<T> {
  readonly loadedSoFar: Resource<Page<T>>;
  /**
   * Every item read, drawn or not yet: a list read whole holds them all as its one answer comes,
   * though it draws them in pieces; undefined before the first read answers.
   */
  readonly held: readonly T[] | undefined;
  /** Whether a page more is loading, or what was read is still being drawn. */
  readonly isLoadingMore: boolean;
  readonly loadMore: () => void;
  /**
   * Draws at least the first count of the items held now, at once, where they are still being drawn
   * in pieces: an item a jump or Back goes to, and the rows after it.
   */
  readonly drawThrough: (count: number) => void;
  /** The view's first and last item in view, by their place in loadedSoFar's items. */
  readonly showRange: (first: number, last: number) => void;
}

/** The pages so far with the next one added. */
export function appendPage<T>(loadedSoFar: Page<T>, page: Page<T>): Page<T> {
  return {
    items: [...loadedSoFar.items, ...page.items],
    nextCursor: page.nextCursor,
    total: loadedSoFar.total,
  };
}

/**
 * The first pages of a list again, from the start, until they hold at least shownCount items or the
 * list ends: what a person scrolled through, as the server has it now. Each is kept with the cursor
 * it was read by.
 */
export async function loadAgain<T>(
  load: (cursor: string) => Promise<Page<T>>,
  shownCount: number,
): Promise<ReadPage<T>[]> {
  const pages = [{ cursor: "", ...(await load("")) }];
  let count = pages[0]?.items.length ?? 0;
  for (let last = pages[0]; last !== undefined && count < shownCount && last.nextCursor !== "";) {
    const cursor = last.nextCursor;
    last = { cursor, ...(await load(cursor)) };
    pages.push(last);
    count += last.items.length;
  }
  return pages;
}

/** A page as it was read: by its cursor, "" for the first. */
export interface ReadPage<T> extends Page<T> {
  readonly cursor: string;
}

/**
 * The pages read, merged into one: their items in order, an id seen twice kept the first time, and
 * the total the last page saying one says, the list as its latest page read found it.
 */
export function merged<T>(pages: readonly ReadPage<T>[], idOf?: (item: T) => unknown): Page<T> {
  const items: T[] = [];
  const seen = new Set<unknown>();
  for (const page of pages) {
    for (const item of page.items) {
      const id = idOf === undefined ? undefined : idOf(item);
      if (id !== undefined && seen.has(id)) {
        continue;
      }
      seen.add(id);
      items.push(item);
    }
  }
  return {
    items,
    nextCursor: pages.at(-1)?.nextCursor ?? "",
    total: pages.findLast((page) => page.total !== undefined)?.total,
  };
}

/**
 * How many pages a poll reads at most: those the view shows, as D306 §7 has it, so a poll's cost
 * stays the same however far a person scrolled. Pages scrolled past keep what they had, and are
 * read again once they are in view again.
 */
export const pagesReadByAPoll = 2;

/** Which pages hold the items from first to last, by their place among the pages' items. */
export function pagesHolding<T>(
  pages: readonly ReadPage<T>[],
  first: number,
  last: number,
): number[] {
  const holding: number[] = [];
  let start = 0;
  pages.forEach((page, index) => {
    const end = start + page.items.length - 1;
    if (end >= first && start <= last) {
      holding.push(index);
    }
    start = end + 1;
  });
  return holding;
}

/** What a list's poll reads against, as the poll, which outlives renders, sees it. */
interface Shown<T> {
  /** The pages read, drawn or not yet. */
  pages: ReadPage<T>[] | undefined;
  /** The pages read, merged: what held says. */
  held: Page<T> | undefined;
  /** How many of held's items are drawn. */
  drawn: number;
  /** How many items to draw at once at the drawing's next piece, as drawThrough asks. */
  drawnThrough: number;
  /** Whether held is being drawn in pieces. */
  isDrawing: boolean;
  hasFailed: boolean;
  isLoadingMore: boolean;
  /** The items in view, by their place in the merged items; undefined before the view says. */
  range: { first: number; last: number } | undefined;
  /** Whether the component still shows the list: a page more shown in pieces stops once it is not. */
  isMounted: boolean;
}

/**
 * How many of a page more's items each piece shows: one row of the widest grid, a few of a row's
 * tiles. A piece is a commit of its own, and pieces follow one another until the main thread is due
 * back (inPieces).
 */
const itemsPerPiece = 12;

/**
 * How many of a list's items its first drawing draws at once, the rest following in pieces: a
 * screen of posters and more, as its first page of 60 was before a list was read whole (D475).
 */
const drawnAtOnce = 60;

/** The pages a poll reads: those the view shows, or, before it says, the first; at most two. */
function pagesToRead<T>(shown: Shown<T>, pages: readonly ReadPage<T>[]): number[] {
  const inView =
    shown.range === undefined ? [0] : pagesHolding(pages, shown.range.first, shown.range.last);
  return (inView.length === 0 ? [0] : inView).slice(0, pagesReadByAPoll);
}

/**
 * Reads the pages in view again, each by the cursor it was read by, and hands answered what that
 * came to, and the pages to show when they changed. A list whose last read failed is read again
 * from the start, as far as it was shown. It skips its turn, unchanged, while the first read or a
 * page more is loading.
 */
function readAgain<T>(
  load: (cursor: string) => Promise<Page<T>>,
  shown: Shown<T>,
  answered: (outcome: AskOutcome, now?: ReadPage<T>[]) => void,
): void {
  const before = shown.pages;
  if (shown.isLoadingMore || shown.isDrawing || (before === undefined && !shown.hasFailed)) {
    answered("unchanged");
    return;
  }
  const read: Promise<ReadPage<T>[]> =
    shown.hasFailed || before === undefined
      ? loadAgain(load, before?.reduce((count, page) => count + page.items.length, 0) ?? 0)
      : readPagesInView(load, before, pagesToRead(shown, before));
  read.then(
    (now) => {
      if (shown.isLoadingMore || shown.isDrawing) {
        answered("unchanged");
      } else if (shown.hasFailed || JSON.stringify(now) !== JSON.stringify(before)) {
        answered("changed", now);
      } else {
        answered("unchanged");
      }
    },
    (cause: unknown) => {
      reportFailure(cause);
      answered("failed");
    },
  );
}

/** The pages with each of those at indexes read again by its own cursor. */
async function readPagesInView<T>(
  load: (cursor: string) => Promise<Page<T>>,
  pages: readonly ReadPage<T>[],
  indexes: readonly number[],
): Promise<ReadPage<T>[]> {
  const now = [...pages];
  for (const index of indexes) {
    const page = pages[index];
    if (page !== undefined) {
      now[index] = keepUnchanged(page, { cursor: page.cursor, ...(await load(page.cursor)) });
    }
  }
  return now;
}

/**
 * A page read again, each item that did not change the very one shown before, so a view that
 * draws rows by their identity draws again only what changed; the page shown before when nothing
 * did.
 */
function keepUnchanged<T>(before: ReadPage<T>, now: ReadPage<T>): ReadPage<T> {
  const beforeByText = new Map(before.items.map((item) => [JSON.stringify(item), item]));
  const items = now.items.map((item) => beforeByText.get(JSON.stringify(item)) ?? item);
  const isSame =
    items.length === before.items.length &&
    items.every((item, index) => item === before.items[index]) &&
    now.nextCursor === before.nextCursor &&
    now.total === before.total;
  return isSame ? before : { ...now, items };
}

/**
 * How a list asks for its next page: after the last page read, unless the list ended or a page more
 * is loading, which it says through setIsLoadingMore. The page is drawn in pieces, as show draws
 * it, and the list stays loading more, and its poll waits, until the whole page is drawn.
 */
function useLoadMore<T>(
  list: PolledList<T> & { readonly fail: (cause: unknown) => void },
  setIsLoadingMore: (isLoadingMore: boolean) => void,
): () => void {
  const { load, show, fail } = list;
  const state = list.shown.current;
  return useCallback((): void => {
    const pages = state.pages;
    const cursor = pages?.at(-1)?.nextCursor ?? "";
    if (pages === undefined || cursor === "" || state.isLoadingMore) {
      return;
    }
    setIsLoadingMore(true);
    state.isLoadingMore = true;
    load(cursor).then(
      async (page) => {
        await show([...pages, { cursor, ...page }]);
        state.isLoadingMore = false;
        setIsLoadingMore(false);
      },
      (cause: unknown) => {
        setIsLoadingMore(false);
        state.isLoadingMore = false;
        fail(cause);
      },
    );
  }, [load, show, fail, state, setIsLoadingMore]);
}

/** A list as its poller reads and shows it: how to load a page, what is shown, and how to show it. */
interface PolledList<T> {
  readonly load: (cursor: string) => Promise<Page<T>>;
  readonly shown: { readonly current: Shown<T> };
  readonly show: (pages: ReadPage<T>[]) => Promise<unknown>;
}

/**
 * While the component shows a list, reads the pages in view again as the poller says, and shows
 * what changed (D237, D306 §7); a list whose read failed is read again from the start. Nothing
 * without a poller.
 */
function usePagesPolling<T>(
  poller: Poller | undefined,
  { load, shown, show }: PolledList<T>,
): void {
  useEffect(() => {
    if (poller === undefined) {
      return undefined;
    }
    return poller.start((answered, signal) => {
      readAgain(load, shown.current, (outcome, now) => {
        // A read its poll gave up is no longer heard: a later read may already have shown more.
        if (now !== undefined && signal?.aborted !== true) {
          void show(now);
        }
        answered(outcome);
      });
    });
  }, [load, poller, shown, show]);
}

/**
 * Where a list read whole is kept in the tab, and under what: Back to Films, Series or a library's
 * page draws it from there at once, its focus put back, while it is read again (D475), as a film's
 * page is (useRemembered).
 */
export interface RememberedList<T> {
  readonly in: Remembered<ReadPage<T>[]>;
  readonly key: string;
}

/** How a list's items are told apart, and where it is kept in the tab, if it is. */
export interface PagesOptions<T> {
  /** What tells an item apart: one read twice, as a list changing between two pages' reads may give
   * it, is shown once. */
  readonly idOf?: (item: T) => unknown;
  readonly remembered?: RememberedList<T> | undefined;
}

/**
 * Reads load's first page, shown, or its failure, again whenever load is a different read; a list
 * kept in the tab is shown from there first, each item read again that did not change kept the one
 * shown.
 */
function useFirstPage<T>(
  load: (cursor: string) => Promise<Page<T>>,
  show: (pages: ReadPage<T>[]) => Promise<unknown>,
  fail: (cause: unknown) => void,
  kept: readonly ReadPage<T>[] | undefined,
): void {
  const keptRef = useRef(kept);
  useEffect(() => {
    let isCurrent = true;
    const before = keptRef.current;
    if (before !== undefined) {
      // After this commit: show draws its first piece at once, and its others as commits of their own.
      void Promise.resolve().then(() => (isCurrent ? show([...before]) : undefined));
    }
    load("").then(
      (first) => {
        if (isCurrent) {
          const now = { cursor: "", ...first };
          void show([
            before?.length === 1 && before[0] !== undefined ? keepUnchanged(before[0], now) : now,
          ]);
        }
      },
      (cause: unknown) => {
        if (isCurrent) {
          fail(cause);
        }
      },
    );
    return () => {
      isCurrent = false;
    };
  }, [load, show, fail]);
}

/** The first count of a page's items, the rest still to be drawn. */
function firstOf<T>(page: Page<T>, count: number): Page<T> {
  return count >= page.items.length ? page : { ...page, items: page.items.slice(0, count) };
}

/**
 * Reads a list from load: a page at a time, or whole where load leaves its limit out (D475). A
 * different list is a different component: key it by what load reads, so it starts again from the
 * first page. Given options' idOf, an item read twice, as a list changing between two pages' reads
 * may give it, is shown once. Given options' remembered, it is kept in the tab, and drawn from there
 * when the component shows it again, while it is read again.
 *
 * What is read is drawn in pieces past the items already drawn, each its own commit, the main
 * thread handed back every 50 ms (inPieces), a list's first drawing drawnAtOnce at once: at once,
 * 60 posters took 110–140 ms on the Fire TV, and a held arrow's presses waited behind them; as a
 * transition, the posters came only once Down was let go (2026-11-11). Every item read is held
 * while it is drawn, for a jump to find its place among them (held), and drawThrough draws through
 * that place at once.
 *
 * Given a poller, it also asks again for the pages in view, as showRange says them, for as long as
 * the component shows it, backing off while nothing changes (D237): titles a scan finds, and the
 * posters made for them, appear without a reload, and a poll reads no more than pagesReadByAPoll
 * however far the list was scrolled (D306 §7); a list read whole is one page, read whole. It skips
 * its turn while a page more is loading or what was read is being drawn.
 */
export function usePages<T>(
  load: (cursor: string) => Promise<Page<T>>,
  poller?: Poller,
  options: PagesOptions<T> = {},
): Pages<T> {
  const { remembered } = options;
  const [kept] = useState(() => remembered?.in.get(remembered.key));
  const [loadedSoFar, setLoadedSoFar] = useState<Resource<Page<T>>>(() =>
    kept === undefined
      ? { state: "loading" }
      : { state: "loaded", value: firstOf(merged(kept, options.idOf), drawnAtOnce) },
  );
  const [held, setHeld] = useState<readonly T[] | undefined>(undefined);
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const [isDrawing, setIsDrawing] = useState(false);
  // What is shown and whether a page more is loading, as the poll, which outlives renders, reads them.
  const shown = useRef<Shown<T>>({
    pages: undefined,
    held: undefined,
    drawn: loadedSoFar.state === "loaded" ? loadedSoFar.value.items.length : 0,
    drawnThrough: 0,
    isDrawing: false,
    hasFailed: false,
    isLoadingMore: false,
    range: undefined,
    isMounted: true,
  });
  // Read through a ref, so a function made anew at each render does not read the list again.
  const idOfNow = useRef(options.idOf);
  idOfNow.current = options.idOf;
  const rememberedNow = useRef(remembered);
  rememberedNow.current = remembered;
  const drawFirst = useCallback((count: number, isNow: boolean): void => {
    const state = shown.current;
    const page = state.held;
    if (page === undefined) {
      return;
    }
    state.drawn = Math.min(count, page.items.length);
    const draw = (): void => {
      setLoadedSoFar({ state: "loaded", value: firstOf(page, state.drawn) });
    };
    if (isNow) {
      flushSync(draw);
    } else {
      draw();
    }
  }, []);
  const show = useCallback(
    async (pages: ReadPage<T>[]): Promise<unknown> => {
      const state = shown.current;
      const page = merged(pages, idOfNow.current);
      state.pages = pages;
      state.held = page;
      state.hasFailed = false;
      setHeld(page.items);
      const keptIn = rememberedNow.current;
      keptIn?.in.set(keptIn.key, pages);
      if (state.isDrawing) {
        // The drawing under way draws what is held now.
        return undefined;
      }
      const from = Math.min(state.drawn, page.items.length);
      if (page.items.length - from <= 0) {
        drawFirst(page.items.length, false);
        return undefined;
      }
      const atOnce = from === 0 ? drawnAtOnce : 0;
      state.isDrawing = true;
      setIsDrawing(true);
      await inPieces({
        count: page.items.length - from,
        piece: itemsPerPiece,
        showUpTo: (count) => {
          const upTo = Math.max(from + count, atOnce, state.drawnThrough);
          if (upTo > state.drawn) {
            drawFirst(upTo, true);
          }
        },
        isCurrent: () => state.isMounted,
      });
      state.isDrawing = false;
      if (state.isMounted) {
        // Whatever came while it was drawn: a list read again as its kept one was drawn.
        drawFirst(state.held?.items.length ?? 0, false);
        setIsDrawing(false);
      }
      return undefined;
    },
    [drawFirst],
  );
  const fail = useCallback((cause: unknown): void => {
    shown.current.hasFailed = true;
    setLoadedSoFar({ state: "failed", cause });
  }, []);
  useEffect(() => {
    const state = shown.current;
    state.isMounted = true;
    return () => {
      state.isMounted = false;
    };
  }, []);
  useFirstPage(load, show, fail, kept);
  usePagesPolling(poller, { load, shown, show });
  const loadMore = useLoadMore({ load, shown, show, fail }, setIsLoadingMore);
  const drawThrough = useCallback(
    (count: number): void => {
      const state = shown.current;
      state.drawnThrough = Math.max(state.drawnThrough, count);
      if (state.isDrawing && count > state.drawn) {
        drawFirst(count, false);
      }
    },
    [drawFirst],
  );
  const showRange = useCallback((first: number, last: number): void => {
    shown.current.range = { first, last };
  }, []);
  // A failure its poller reads again after says so: the poller, not the read, decides it.
  const shownSoFar =
    loadedSoFar.state === "failed" && poller !== undefined
      ? { ...loadedSoFar, willAskAgain: true }
      : loadedSoFar;
  return {
    loadedSoFar: shownSoFar,
    held,
    isLoadingMore: isLoadingMore || isDrawing,
    loadMore,
    drawThrough,
    showRange,
  };
}
