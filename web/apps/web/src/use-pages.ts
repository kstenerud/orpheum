// SPDX-License-Identifier: MPL-2.0

import { reportFailure } from "@orpheum/sdk";
import { startTransition, useCallback, useEffect, useRef, useState } from "react";

import type { AskOutcome, Poller } from "./polling.ts";

import type { Resource } from "./use-resource.ts";

/** One page of a list from the server, and the cursor of the next; "" on the last. */
export interface Page<T> {
  readonly items: readonly T[];
  readonly nextCursor: string;
  /**
   * How many items the whole list holds, as its first page says it, where its server counts them:
   * a view sizes the list by it before the other pages come (the owner, 2026-11-11).
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
 * A list read a page at a time: what is loaded so far, merged into one page, whether more is
 * loading, a way to ask, and a way to say which of its items are in view, whose pages a poll reads.
 */
export interface Pages<T> {
  readonly loadedSoFar: Resource<Page<T>>;
  readonly isLoadingMore: boolean;
  readonly loadMore: () => void;
  /**
   * Loads page after page, as loadMore does, until isFound finds an item among those loaded or the
   * list ends; the item's place in loadedSoFar's items, or undefined where none is found.
   */
  readonly loadUntil: (isFound: (item: T) => boolean) => Promise<number | undefined>;
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

/** The pages read, merged into one: their items in order, an id seen twice kept the first time. */
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
  return { items, nextCursor: pages.at(-1)?.nextCursor ?? "", total: pages[0]?.total };
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
  pages: ReadPage<T>[] | undefined;
  hasFailed: boolean;
  isLoadingMore: boolean;
  /** The page more being loaded, which a load until something is found waits for. */
  loading?: Promise<unknown> | undefined;
  /** The items in view, by their place in the merged items; undefined before the view says. */
  range: { first: number; last: number } | undefined;
}

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
  if (shown.isLoadingMore || (before === undefined && !shown.hasFailed)) {
    answered("unchanged");
    return;
  }
  const read: Promise<ReadPage<T>[]> =
    shown.hasFailed || before === undefined
      ? loadAgain(load, before?.reduce((count, page) => count + page.items.length, 0) ?? 0)
      : readPagesInView(load, before, pagesToRead(shown, before));
  read.then(
    (now) => {
      if (shown.isLoadingMore) {
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
 * is loading, which it says through setIsLoadingMore.
 */
function useLoadMore<T>(
  list: PolledList<T> & { readonly fail: (cause: unknown) => void },
  setIsLoadingMore: (isLoadingMore: boolean) => void,
): () => void {
  const { load, show, fail } = list;
  const state = list.shown.current;
  return (): void => {
    const pages = state.pages;
    const cursor = pages?.at(-1)?.nextCursor ?? "";
    if (pages === undefined || cursor === "" || state.isLoadingMore) {
      return;
    }
    setIsLoadingMore(true);
    state.isLoadingMore = true;
    state.loading = load(cursor).then(
      (page) => {
        state.isLoadingMore = false;
        // A transition, so the page's new tiles render in slices a key press can run between: at
        // once, 60 posters took 110–140 ms on the Fire TV, and a held arrow's presses waited behind
        // them (2026-11-11). The list stays loading more until they are shown.
        startTransition(() => {
          setIsLoadingMore(false);
          show([...pages, { cursor, ...page }]);
        });
      },
      (cause: unknown) => {
        setIsLoadingMore(false);
        state.isLoadingMore = false;
        fail(cause);
      },
    );
  };
}

/**
 * How a list loads until it finds an item: a page more at a time after the last read, each one
 * shown as it comes, as loadMore loads it, while a poll skips its turn; after the page more already
 * loading, if one is. The found item's place among the merged items, or undefined when the list
 * ends, or a read fails, without it.
 */
function useLoadUntil<T>(
  list: PolledList<T> & {
    readonly fail: (cause: unknown) => void;
    readonly idOf: { readonly current: ((item: T) => unknown) | undefined };
  },
  setIsLoadingMore: (isLoadingMore: boolean) => void,
): (isFound: (item: T) => boolean) => Promise<number | undefined> {
  const { load, show, fail, idOf } = list;
  const state = list.shown.current;
  return useCallback(
    async (isFound: (item: T) => boolean): Promise<number | undefined> => {
      await state.loading?.catch(() => undefined);
      for (;;) {
        const pages = state.pages;
        if (pages === undefined) {
          return undefined;
        }
        const place = merged(pages, idOf.current).items.findIndex(isFound);
        const cursor = pages.at(-1)?.nextCursor ?? "";
        if (place >= 0 || cursor === "") {
          return place >= 0 ? place : undefined;
        }
        setIsLoadingMore(true);
        state.isLoadingMore = true;
        try {
          const page = await load(cursor);
          show([...pages, { cursor, ...page }]);
        } catch (cause: unknown) {
          fail(cause);
          return undefined;
        } finally {
          setIsLoadingMore(false);
          state.isLoadingMore = false;
        }
      }
    },
    [load, show, fail, idOf, state, setIsLoadingMore],
  );
}

/** A list as its poller reads and shows it: how to load a page, what is shown, and how to show it. */
interface PolledList<T> {
  readonly load: (cursor: string) => Promise<Page<T>>;
  readonly shown: { readonly current: Shown<T> };
  readonly show: (pages: ReadPage<T>[]) => void;
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
          show(now);
        }
        answered(outcome);
      });
    });
  }, [load, poller, shown, show]);
}

/**
 * Reads a list a page at a time from load. A different list is a different component: key it by
 * what load reads, so it starts again from the first page. Given idOf, an item read twice, as a
 * list changing between two pages' reads may give it, is shown once.
 *
 * Given a poller, it also asks again for the pages in view, as showRange says them, for as long as
 * the component shows it, backing off while nothing changes (D237): titles a scan finds, and the
 * posters made for them, appear without a reload, and a poll reads no more than pagesReadByAPoll
 * however far the list was scrolled (D306 §7). It skips its turn while a page more is loading.
 */
export function usePages<T>(
  load: (cursor: string) => Promise<Page<T>>,
  poller?: Poller,
  idOf?: (item: T) => unknown,
): Pages<T> {
  const [loadedSoFar, setLoadedSoFar] = useState<Resource<Page<T>>>({ state: "loading" });
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  // What is shown and whether a page more is loading, as the poll, which outlives renders, reads them.
  const shown = useRef<Shown<T>>({
    pages: undefined,
    hasFailed: false,
    isLoadingMore: false,
    range: undefined,
  });
  // Read through a ref, so a function made anew at each render does not read the list again.
  const idOfNow = useRef(idOf);
  idOfNow.current = idOf;
  const show = useCallback((pages: ReadPage<T>[]): void => {
    shown.current.pages = pages;
    shown.current.hasFailed = false;
    setLoadedSoFar({ state: "loaded", value: merged(pages, idOfNow.current) });
  }, []);
  const fail = useCallback((cause: unknown): void => {
    shown.current.hasFailed = true;
    setLoadedSoFar({ state: "failed", cause });
  }, []);
  useEffect(() => {
    let isCurrent = true;
    load("").then(
      (first) => {
        if (isCurrent) {
          show([{ cursor: "", ...first }]);
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
  usePagesPolling(poller, { load, shown, show });
  const loadMore = useLoadMore({ load, shown, show, fail }, setIsLoadingMore);
  const loadUntil = useLoadUntil({ load, shown, show, fail, idOf: idOfNow }, setIsLoadingMore);
  const showRange = useCallback((first: number, last: number): void => {
    shown.current.range = { first, last };
  }, []);
  // A failure its poller reads again after says so: the poller, not the read, decides it.
  const shownSoFar =
    loadedSoFar.state === "failed" && poller !== undefined
      ? { ...loadedSoFar, willAskAgain: true }
      : loadedSoFar;
  return { loadedSoFar: shownSoFar, isLoadingMore, loadMore, loadUntil, showRange };
}
