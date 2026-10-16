// SPDX-License-Identifier: MPL-2.0

import { useCallback, useEffect, useRef } from "react";

/**
 * Watches a list's items and tells showRange the first and last in view as a person scrolls, so
 * the list's poll reads only their pages (usePages, D306 §7). It returns a ref for each item, by
 * its place in the list. Where the browser has no IntersectionObserver it says nothing, and the
 * poll reads the first page.
 */
export function useItemsInView(
  showRange: (first: number, last: number) => void,
): (index: number) => (element: Element | null) => void {
  const placeOf = useRef(new Map<Element, number>());
  const inView = useRef(new Set<number>());
  const watcher = useRef<IntersectionObserver | undefined>(undefined);
  const elementAt = useRef(new Map<number, Element>());
  useEffect(() => {
    if (typeof IntersectionObserver === "undefined") {
      return undefined;
    }
    const observer = new IntersectionObserver((entries) => {
      for (const entry of entries) {
        const place = placeOf.current.get(entry.target);
        if (place === undefined) {
          continue;
        }
        if (entry.isIntersecting) {
          inView.current.add(place);
        } else {
          inView.current.delete(place);
        }
      }
      if (inView.current.size > 0) {
        showRange(Math.min(...inView.current), Math.max(...inView.current));
      }
    });
    watcher.current = observer;
    for (const element of placeOf.current.keys()) {
      observer.observe(element);
    }
    return () => {
      observer.disconnect();
      watcher.current = undefined;
    };
  }, [showRange]);
  // One ref a place, kept, so React does not call it again with each render.
  const refs = useRef(new Map<number, (element: Element | null) => void>());
  return useCallback((index: number) => {
    const kept = refs.current.get(index);
    if (kept !== undefined) {
      return kept;
    }
    const ref = (element: Element | null): void => {
      const before = elementAt.current.get(index);
      if (before !== undefined && before !== element) {
        watcher.current?.unobserve(before);
        placeOf.current.delete(before);
        inView.current.delete(index);
        elementAt.current.delete(index);
      }
      if (element !== null && before !== element) {
        placeOf.current.set(element, index);
        elementAt.current.set(index, element);
        watcher.current?.observe(element);
      }
    };
    refs.current.set(index, ref);
    return ref;
  }, []);
}
