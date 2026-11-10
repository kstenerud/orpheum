// SPDX-License-Identifier: MPL-2.0

import {
  createElement,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
  type ReactElement,
} from "react";

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
 * also hands the element to the list's own watcher, when it has one (useItemsInView, for its pages),
 * and the interest, an element drawing nothing that the grid draws beside its tiles to say them.
 */
export function useTilesInView(
  ids: readonly number[],
  listWatchAt?: WatchAt,
): { readonly watchAt: WatchAt; readonly interest: ReactElement } {
  // The range in view is kept outside the grid, and only TilesInterest, drawing nothing, is drawn
  // again as it changes: as state of the grid's own, each row scrolled drew the whole grid again
  // (web architecture audit, 2026-11-10).
  const [range] = useState(rangeInView);
  const tileWatchAt = useItemsInView(range.show);
  const watchAt = useMemo(() => {
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
  return { watchAt, interest: createElement(TilesInterest, { ids, range }) };
}

/** The first and last places in view. */
interface Range {
  readonly first: number;
  readonly last: number;
}

/** The range in view, kept for TilesInterest to read and be told of as it changes. */
interface RangeInView {
  readonly show: (first: number, last: number) => void;
  readonly shown: () => Range | undefined;
  readonly subscribe: (changed: () => void) => () => void;
}

/** A range in view, none yet. */
function rangeInView(): RangeInView {
  let shown: Range | undefined;
  const listeners = new Set<() => void>();
  return {
    show: (first, last) => {
      if (shown?.first === first && shown.last === last) {
        return;
      }
      shown = { first, last };
      for (const changed of listeners) {
        changed();
      }
    },
    shown: () => shown,
    subscribe: (changed) => {
      listeners.add(changed);
      return () => {
        listeners.delete(changed);
      };
    },
  };
}

/** Says the tiles of ids in range's places, drawn again alone as the range changes. */
function TilesInterest(props: {
  readonly ids: readonly number[];
  readonly range: RangeInView;
}): null {
  const range = useSyncExternalStore(props.range.subscribe, props.range.shown, props.range.shown);
  useTilesInterest(range === undefined ? [] : props.ids.slice(range.first, range.last + 1));
  return null;
}

/**
 * Says the server's work for the pictures of itemIds' tiles comes first while they are in view (D362
 * §4, D324): once they settle, again every interestSaidEverySeconds while the tab is visible, and
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
        void api.DELETE("/interest/tiles", { keepalive: true }).catch(() => undefined);
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
        void api.PUT("/interest/tiles", { body: { items: ids } }).catch(() => undefined);
      }
    };
    const onVisibility = (): void => {
      if (document.visibilityState === "visible") {
        say();
      } else if (hasSaid.current) {
        hasSaid.current = false;
        void api.DELETE("/interest/tiles", { keepalive: true }).catch(() => undefined);
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
