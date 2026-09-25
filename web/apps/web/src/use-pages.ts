// SPDX-License-Identifier: MPL-2.0

import { useEffect, useState } from "react";

import { failure, type Resource } from "./use-resource.ts";

/** One page of a list from the server, and the cursor of the next; "" on the last. */
export interface Page<T> {
  readonly items: readonly T[];
  readonly next: string;
}

/** A list read a page at a time: the pages so far, whether more is loading, and a way to ask. */
export interface Pages<T> {
  readonly list: Resource<Page<T>>;
  readonly isLoadingMore: boolean;
  readonly more: () => void;
}

/** The pages so far with the next one added. */
export function appended<T>(sofar: Page<T>, page: Page<T>): Page<T> {
  return { items: [...sofar.items, ...page.items], next: page.next };
}

/**
 * Reads a list a page at a time from load. A different list is a different component: key it by
 * what load reads, so it starts again from the first page.
 */
export function usePages<T>(load: (cursor: string) => Promise<Page<T>>): Pages<T> {
  const [list, setList] = useState<Resource<Page<T>>>({ state: "loading" });
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  useEffect(() => {
    let isCurrent = true;
    load("").then(
      (first) => {
        if (isCurrent) {
          setList({ state: "loaded", value: first });
        }
      },
      (cause: unknown) => {
        if (isCurrent) {
          setList(failure(cause));
        }
      },
    );
    return () => {
      isCurrent = false;
    };
  }, [load]);
  const more = (): void => {
    if (list.state !== "loaded" || list.value.next === "" || isLoadingMore) {
      return;
    }
    const sofar = list.value;
    setIsLoadingMore(true);
    load(sofar.next).then(
      (page) => {
        setIsLoadingMore(false);
        setList({ state: "loaded", value: appended(sofar, page) });
      },
      (cause: unknown) => {
        setIsLoadingMore(false);
        setList(failure(cause));
      },
    );
  };
  return { list, isLoadingMore, more };
}
