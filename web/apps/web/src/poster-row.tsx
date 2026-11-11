// SPDX-License-Identifier: MPL-2.0

import "./poster-row.css";

import {
  useEffect,
  useRef,
  useState,
  useSyncExternalStore,
  type JSX,
  type ReactNode,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import { arrivalMark } from "./focus-navigation.ts";
import { Icon } from "./icon.tsx";
import { IconButton } from "./icon-button.tsx";
import { useRovingTabStop } from "./roving-tab-stop.ts";

/** Which way an arrow scrolls a row: back to its start, or on to its end. */
type RowDirection = "back" | "on";

/** Whether a row has more out of view before what it shows, and after it. */
interface RowEnds {
  readonly hasMoreBack: boolean;
  readonly hasMoreOn: boolean;
}

/** A pixel's slack, for a scroll position a browser keeps as a fraction. */
const slackPixels = 1;

/**
 * A row of posters that scrolls sideways (Home's rows, a series' seasons), the one way to draw one:
 * the list a remote moves along by Left and Right (data-focus-container), scrolling it, and Tab
 * enters and leaves in one step (useRovingTabStop); and, for a mouse with no sideways wheel, an
 * arrow at each end where there is more that way (the owner, 2026-11-04). The arrows show only on a device whose pointer hovers and is fine, while the
 * pointer is over the row or focus is in it (poster-row.css); they are out of Tab's order and a
 * remote's (IconButton's isPointerOnly), so a phone and a television are as they were. On every
 * device, each end with more beyond it fades out, the focused poster always clear of the fade
 * (poster-row.css; D189). listRef, given, is the list's, for a row that scrolls itself to a poster (SeasonRow).
 */
export function PosterRow(props: {
  readonly className?: string | undefined;
  /** The list's name, where no heading names it. */
  readonly label?: string | undefined;
  readonly listRef?: RefObject<HTMLUListElement | null> | undefined;
  /** Whether its first poster is where focus goes as its page opens (focus-navigation.ts). */
  readonly isArrival?: boolean | undefined;
  /** The posters, each an `li`. */
  readonly children: ReactNode;
}): JSX.Element {
  const { t } = useTranslation();
  const ownRef = useRef<HTMLUListElement>(null);
  const listRef = props.listRef ?? ownRef;
  const ends = useRowEnds(listRef);
  const arrows = useIsMousePointer() ? ends : noEnds;
  useRovingTabStop(listRef);
  const scroll = (direction: RowDirection): void => {
    const list = listRef.current;
    if (list !== null) {
      scrollRowAPage(list, direction);
    }
  };
  return (
    <div className="poster-row__frame">
      <ul
        ref={listRef}
        className={rowClassOf(ends, props.className)}
        aria-label={props.label}
        data-focus-container=""
        {...(props.isArrival === true ? arrivalMark : {})}
      >
        {props.children}
      </ul>
      {arrows.hasMoreBack ? (
        <span className="poster-row__arrow poster-row__arrow--back">
          <IconButton
            label={t("posterRow.scrollBack")}
            icon={<RowArrowIcon direction="back" />}
            isPointerOnly
            onClick={() => {
              scroll("back");
            }}
          />
        </span>
      ) : null}
      {arrows.hasMoreOn ? (
        <span className="poster-row__arrow poster-row__arrow--on">
          <IconButton
            label={t("posterRow.scrollOn")}
            icon={<RowArrowIcon direction="on" />}
            isPointerOnly
            onClick={() => {
              scroll("on");
            }}
          />
        </span>
      ) : null}
    </div>
  );
}

/**
 * The list's classes: an edge fade at each end it has more beyond (poster-row.css; the owner,
 * 2026-11-11: "Let's try it and see"), and the row's own.
 */
function rowClassOf(ends: RowEnds, own: string | undefined): string {
  return [
    "poster-row",
    ends.hasMoreBack ? "poster-row--more-back" : undefined,
    ends.hasMoreOn ? "poster-row--more-on" : undefined,
    own,
  ]
    .filter((name) => name !== undefined)
    .join(" ");
}

/** An arrow's chevron, pointing the way it scrolls. */
function RowArrowIcon(props: { readonly direction: RowDirection }): JSX.Element {
  return (
    <Icon className="poster-row__arrow-icon">
      <path
        d={props.direction === "back" ? "M15 4.5 7.5 12l7.5 7.5" : "M9 4.5 16.5 12 9 19.5"}
        fill="none"
        stroke="currentColor"
        strokeWidth="2.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </Icon>
  );
}

/**
 * The devices the arrows show on, as poster-row.css's own media query: one whose pointer hovers and
 * is fine, a mouse's. A justified constant, as phoneWidthQuery is: a stylesheet's query.
 */
const mousePointerQuery = "(hover: hover) and (pointer: fine)";

/** No more either way: a row that fits, and the arrows on a device they show on none of. */
const noEnds: RowEnds = { hasMoreBack: false, hasMoreOn: false };

/**
 * Whether the device's pointer is a mouse's (mousePointerQuery), kept as it changes: a tablet's
 * keyboard and trackpad attached; false where the page cannot ask.
 */
function useIsMousePointer(): boolean {
  return useSyncExternalStore(watchMousePointer, isMousePointerNow, () => false);
}

/** Tells changed as the device's pointer becomes a mouse's or stops being one. */
function watchMousePointer(changed: () => void): () => void {
  if (typeof globalThis.matchMedia !== "function") {
    return () => undefined;
  }
  const query = globalThis.matchMedia(mousePointerQuery);
  query.addEventListener("change", changed);
  return () => {
    query.removeEventListener("change", changed);
  };
}

/** Whether the device's pointer is a mouse's now. */
function isMousePointerNow(): boolean {
  return (
    typeof globalThis.matchMedia === "function" && globalThis.matchMedia(mousePointerQuery).matches
  );
}

/** How much of an end poster shows for the row to count it in view: all of it, but a pixel's rounding. */
const wholeShown = 0.98;

/**
 * Whether the list has more out of view either way: whether its first poster and its last are
 * wholly in view, watched by an IntersectionObserver, which says only when one comes into view or
 * leaves it, so a scroll on a television measures nothing frame by frame (web architecture audit,
 * 2026-11-10). The arrows (a mouse's) and the edge fades (every device's) both read it.
 */
function useRowEnds(listRef: RefObject<HTMLUListElement | null>): RowEnds {
  const [ends, setEnds] = useState<RowEnds>(noEnds);
  useEffect(() => {
    const list = listRef.current;
    if (list === null || typeof IntersectionObserver === "undefined") {
      return undefined;
    }
    const whole = new Map<Element, boolean>();
    const watched: { first: Element | null; last: Element | null } = { first: null, last: null };
    const show = (): void => {
      const next = {
        hasMoreBack: watched.first !== null && whole.get(watched.first) === false,
        hasMoreOn: watched.last !== null && whole.get(watched.last) === false,
      };
      setEnds((shown) =>
        shown.hasMoreBack === next.hasMoreBack && shown.hasMoreOn === next.hasMoreOn ? shown : next,
      );
    };
    const sight = new IntersectionObserver(
      (records) => {
        for (const record of records) {
          whole.set(record.target, record.intersectionRatio >= wholeShown);
        }
        show();
      },
      { root: list, threshold: [wholeShown] },
    );
    // The posters at the ends, watched afresh as posters come and go.
    const watchEnds = (): void => {
      const [first, last] = [list.firstElementChild, list.lastElementChild];
      for (const was of [watched.first, watched.last]) {
        if (was !== null && was !== first && was !== last) {
          sight.unobserve(was);
          whole.delete(was);
        }
      }
      for (const end of [first, last]) {
        if (end !== null && !whole.has(end)) {
          sight.observe(end);
        }
      }
      watched.first = first;
      watched.last = last;
      show();
    };
    watchEnds();
    const changes =
      typeof MutationObserver === "undefined" ? undefined : new MutationObserver(watchEnds);
    changes?.observe(list, { childList: true });
    return () => {
      sight.disconnect();
      changes?.disconnect();
    };
  }, [listRef]);
  return ends;
}

/** A computed length in pixels; none, as for "auto", is 0. */
function pixelsOf(length: string): number {
  const pixels = parseFloat(length);
  return Number.isNaN(pixels) ? 0 : pixels;
}

/**
 * Scrolls a row by about its width less one poster, which stays in view at the other end for where
 * the row was: on, the last poster wholly in view comes to the start; back, the row moves so the
 * first wholly in view ends at the end. Each lands with a poster at the start, where the row's
 * scroll-snap would put one, clear of the edge fade (its scroll-padding); at once for a person who
 * asked for less motion. What is in view counts the posters under the fades: the row's box less its
 * padding, the focus ring's room.
 */
function scrollRowAPage(list: HTMLElement, direction: RowDirection): void {
  const style = getComputedStyle(list);
  const box = list.getBoundingClientRect();
  const boxStart = box.left + list.clientLeft;
  const viewStart = boxStart + pixelsOf(style.paddingLeft);
  const viewEnd = boxStart + list.clientWidth - pixelsOf(style.paddingRight);
  const posters = [...list.children].map((poster) => poster.getBoundingClientRect());
  const target =
    direction === "on"
      ? posterBroughtOn(posters, viewStart, viewEnd)
      : posterBroughtBack(posters, viewStart, viewEnd);
  if (target === undefined) {
    return;
  }
  const isReduced =
    typeof matchMedia === "function" && matchMedia("(prefers-reduced-motion: reduce)").matches;
  list.scrollTo({
    left: list.scrollLeft + target.left - (boxStart + pixelsOf(style.scrollPaddingLeft)),
    behavior: isReduced ? "instant" : "smooth",
  });
}

/** Scrolling on: the last poster wholly in view, else the one after the first in view. */
function posterBroughtOn(
  posters: readonly DOMRect[],
  viewStart: number,
  viewEnd: number,
): DOMRect | undefined {
  const firstShown = posters.findIndex((poster) => poster.right > viewStart + slackPixels);
  const lastWhole = posters.findLastIndex((poster) => poster.right <= viewEnd + slackPixels);
  const index = lastWhole > firstShown ? lastWhole : firstShown + 1;
  return posters[index];
}

/**
 * Scrolling back: the first poster that, at the start, keeps the first wholly in view now in view
 * at the end; the one before that, where only it fits.
 */
function posterBroughtBack(
  posters: readonly DOMRect[],
  viewStart: number,
  viewEnd: number,
): DOMRect | undefined {
  const firstWhole = posters.findIndex((poster) => poster.left >= viewStart - slackPixels);
  const kept = posters[firstWhole];
  if (kept === undefined) {
    return undefined;
  }
  const width = viewEnd - viewStart;
  const index = posters.findIndex((poster) => poster.left >= kept.right - width - slackPixels);
  return posters[index < firstWhole ? index : Math.max(0, firstWhole - 1)];
}
