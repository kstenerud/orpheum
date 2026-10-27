// SPDX-License-Identifier: MPL-2.0

import { useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";

import { useItemsInView } from "./items-in-view.ts";
import { InterestSaidEveryContext } from "./page-interest.ts";
import { useServices } from "./services.tsx";

/** The most tiles one say names: the server's limit. */
const mostTilesSaid = 200;

/** How long the tiles in view stay put before they are said: a list scrolled through says where it stops. */
const tilesSettleMs = 500;

/** A ref callback for a list's item at its place. */
type WatchAt = (index: number) => (element: Element | null) => void;

/**
 * Watches a grid's tiles and says those in view (useTilesInterest), so the work their pictures need
 * comes first while they are seen (the owner, 2026-10-27: "viewing a library signals interest in
 * what's needed to display the tiles with images"; D362 §4). It returns a ref for each place, which
 * also hands the element to the list's own watcher, when it has one (useItemsInView, for its pages).
 */
export function useTilesInView(ids: readonly number[], listWatchAt?: WatchAt): WatchAt {
  const [range, setRange] = useState<{ readonly first: number; readonly last: number }>();
  const showRange = useCallback((first: number, last: number): void => {
    setRange((shown) => (shown?.first === first && shown.last === last ? shown : { first, last }));
  }, []);
  const tileWatchAt = useItemsInView(showRange);
  useTilesInterest(range === undefined ? [] : ids.slice(range.first, range.last + 1));
  return useMemo(() => {
    if (listWatchAt === undefined) {
      return tileWatchAt;
    }
    // One ref a place, kept, so React does not call it again with each render.
    const refs = new Map<number, (element: Element | null) => void>();
    return (index: number) => {
      let ref = refs.get(index);
      if (ref === undefined) {
        const forList = listWatchAt(index);
        const forTiles = tileWatchAt(index);
        ref = (element: Element | null): void => {
          forList(element);
          forTiles(element);
        };
        refs.set(index, ref);
      }
      return ref;
    };
  }, [listWatchAt, tileWatchAt]);
}

/**
 * Says the server's work for the pictures of itemIds' tiles comes first while they are in view (D362
 * §4, D324): once they settle, again every interest_said_every_seconds while the tab is visible, and
 * that none is shown when the list goes, the tab hides or the page unloads, the request kept alive
 * past the page. Tiles scrolled to replace those said before.
 */
export function useTilesInterest(itemIds: readonly number[]): void {
  const { api } = useServices();
  const everySeconds = useContext(InterestSaidEveryContext);
  const key = itemIds.slice(0, mostTilesSaid).join(",");
  const hasSaid = useRef(false);
  useEffect(() => {
    const leave = (): void => {
      if (hasSaid.current) {
        hasSaid.current = false;
        void api.DELETE("/tiles/interest", { keepalive: true }).catch(() => undefined);
      }
    };
    window.addEventListener("pagehide", leave);
    return () => {
      window.removeEventListener("pagehide", leave);
      leave();
    };
  }, [api]);
  useEffect(() => {
    if (key === "") {
      return undefined;
    }
    const ids = key.split(",").map(Number);
    const say = (): void => {
      if (document.visibilityState === "visible") {
        hasSaid.current = true;
        void api.PUT("/tiles/interest", { body: { itemIds: ids } }).catch(() => undefined);
      }
    };
    const onVisibility = (): void => {
      if (document.visibilityState === "visible") {
        say();
      } else if (hasSaid.current) {
        hasSaid.current = false;
        void api.DELETE("/tiles/interest", { keepalive: true }).catch(() => undefined);
      }
    };
    const settle = setTimeout(say, tilesSettleMs);
    const timer = setInterval(say, everySeconds * 1000);
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      clearTimeout(settle);
      clearInterval(timer);
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, [api, key, everySeconds]);
}
