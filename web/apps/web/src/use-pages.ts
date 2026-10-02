// SPDX-License-Identifier: MPL-2.0

import { reportFailure } from "@orpheum/sdk";
import { useCallback, useEffect, useRef, useState } from "react";

import type { Poller } from "./polling.ts";

import type { Resource } from "./use-resource.ts";

/** One page of a list from the server, and the cursor of the next; "" on the last. */
export interface Page<T> {
  readonly items: readonly T[];
  readonly nextCursor: string;
}

/**
 * A list read a page at a time: what is loaded so far, merged into one page, whether more is
 * loading, and a way to ask.
 */
export interface Pages<T> {
  readonly loadedSoFar: Resource<Page<T>>;
  readonly isLoadingMore: boolean;
  readonly loadMore: () => void;
}

/** The pages so far with the next one added. */
export function appendPage<T>(loadedSoFar: Page<T>, page: Page<T>): Page<T> {
  return { items: [...loadedSoFar.items, ...page.items], nextCursor: page.nextCursor };
}

/**
 * The first pages of a list again, from the start, until they hold at least shownCount items or the
 * list ends: what a person scrolled through, as the server has it now.
 */
export async function loadAgain<T>(
  load: (cursor: string) => Promise<Page<T>>,
  shownCount: number,
): Promise<Page<T>> {
  let pages = await load("");
  while (pages.items.length < shownCount && pages.nextCursor !== "") {
    pages = appendPage(pages, await load(pages.nextCursor));
  }
  return pages;
}

/**
 * Reads every page shown again and hands it to answered, unless a page more is loading or the read
 * fails: then what is shown stays, and answered hears nothing changed.
 */
function readAgain<T>(
  load: (cursor: string) => Promise<Page<T>>,
  shown: { readonly page: Page<T> | undefined; readonly isLoadingMore: boolean },
  answered: (now: Page<T> | undefined) => void,
): void {
  const before = shown.page;
  if (before === undefined || shown.isLoadingMore) {
    answered(before);
    return;
  }
  loadAgain(load, before.items.length).then(
    (now) => {
      answered(shown.isLoadingMore ? shown.page : now);
    },
    (cause: unknown) => {
      reportFailure(cause);
      answered(shown.page);
    },
  );
}

/**
 * While the component shows a list, reads every page shown again as the poller says, and shows what
 * changed (D237). Nothing without a poller.
 */
/** A list as its poller reads and shows it: how to load a page, what is shown, and how to show more. */
interface PolledList<T> {
  readonly load: (cursor: string) => Promise<Page<T>>;
  readonly shown: {
    readonly current: { readonly page: Page<T> | undefined; readonly isLoadingMore: boolean };
  };
  readonly show: (page: Page<T>) => void;
}

function usePagesPolling<T>(
  poller: Poller | undefined,
  { load, shown, show }: PolledList<T>,
): void {
  useEffect(() => {
    if (poller === undefined) {
      return undefined;
    }
    return poller.start((answered) => {
      readAgain(load, shown.current, (now) => {
        const isChanged =
          now !== undefined && JSON.stringify(now) !== JSON.stringify(shown.current.page);
        if (isChanged) {
          show(now);
        }
        answered(isChanged);
      });
    });
  }, [load, poller, shown, show]);
}

/**
 * Reads a list a page at a time from load. A different list is a different component: key it by
 * what load reads, so it starts again from the first page.
 *
 * Given a poller, it also asks again for every page shown so far for as long as the component shows
 * it, backing off while nothing changes (D237), and keeps what it shows meanwhile: titles a scan
 * finds, and the posters made for them, appear without a reload. It skips its turn while a page
 * more is loading.
 */
export function usePages<T>(load: (cursor: string) => Promise<Page<T>>, poller?: Poller): Pages<T> {
  const [loadedSoFar, setLoadedSoFar] = useState<Resource<Page<T>>>({ state: "loading" });
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  // What is shown and whether a page more is loading, as the poll, which outlives renders, reads them.
  const shown = useRef<{ page: Page<T> | undefined; isLoadingMore: boolean }>({
    page: undefined,
    isLoadingMore: false,
  });
  const show = useCallback((page: Page<T>): void => {
    shown.current.page = page;
    setLoadedSoFar({ state: "loaded", value: page });
  }, []);
  useEffect(() => {
    let isCurrent = true;
    load("").then(
      (first) => {
        if (isCurrent) {
          show(first);
        }
      },
      (cause: unknown) => {
        if (isCurrent) {
          setLoadedSoFar({ state: "failed", cause });
        }
      },
    );
    return () => {
      isCurrent = false;
    };
  }, [load, show]);
  usePagesPolling(poller, { load, shown, show });
  const loadMore = (): void => {
    if (loadedSoFar.state !== "loaded" || loadedSoFar.value.nextCursor === "" || isLoadingMore) {
      return;
    }
    const loaded = loadedSoFar.value;
    setIsLoadingMore(true);
    shown.current.isLoadingMore = true;
    load(loaded.nextCursor).then(
      (page) => {
        setIsLoadingMore(false);
        shown.current.isLoadingMore = false;
        const more = appendPage(loaded, page);
        shown.current.page = more;
        setLoadedSoFar({ state: "loaded", value: more });
      },
      (cause: unknown) => {
        setIsLoadingMore(false);
        shown.current.isLoadingMore = false;
        setLoadedSoFar({ state: "failed", cause });
      },
    );
  };
  return { loadedSoFar, isLoadingMore, loadMore };
}
