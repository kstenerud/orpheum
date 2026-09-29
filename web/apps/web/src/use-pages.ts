// SPDX-License-Identifier: MPL-2.0

import { useEffect, useState } from "react";

import type { Resource } from "./use-resource.ts";

/** One page of a list from the server, and the cursor of the next; "" on the last. */
export interface Page<T> {
  readonly items: readonly T[];
  readonly nextCursor: string;
}

/** A list read a page at a time: the pages so far, whether more is loading, and a way to ask. */
export interface Pages<T> {
  readonly pagesSoFar: Resource<Page<T>>;
  readonly isLoadingMore: boolean;
  readonly loadMore: () => void;
}

/** The pages so far with the next one added. */
export function appendPage<T>(loadedSoFar: Page<T>, page: Page<T>): Page<T> {
  return { items: [...loadedSoFar.items, ...page.items], nextCursor: page.nextCursor };
}

/**
 * Reads a list a page at a time from load. A different list is a different component: key it by
 * what load reads, so it starts again from the first page.
 */
export function usePages<T>(load: (cursor: string) => Promise<Page<T>>): Pages<T> {
  const [pagesSoFar, setPagesSoFar] = useState<Resource<Page<T>>>({ state: "loading" });
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  useEffect(() => {
    let isCurrent = true;
    load("").then(
      (first) => {
        if (isCurrent) {
          setPagesSoFar({ state: "loaded", value: first });
        }
      },
      (cause: unknown) => {
        if (isCurrent) {
          setPagesSoFar({ state: "failed", cause });
        }
      },
    );
    return () => {
      isCurrent = false;
    };
  }, [load]);
  const loadMore = (): void => {
    if (pagesSoFar.state !== "loaded" || pagesSoFar.value.nextCursor === "" || isLoadingMore) {
      return;
    }
    const loadedSoFar = pagesSoFar.value;
    setIsLoadingMore(true);
    load(loadedSoFar.nextCursor).then(
      (page) => {
        setIsLoadingMore(false);
        setPagesSoFar({ state: "loaded", value: appendPage(loadedSoFar, page) });
      },
      (cause: unknown) => {
        setIsLoadingMore(false);
        setPagesSoFar({ state: "failed", cause });
      },
    );
  };
  return { pagesSoFar, isLoadingMore, loadMore };
}
