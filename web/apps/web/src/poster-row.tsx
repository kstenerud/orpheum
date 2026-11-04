// SPDX-License-Identifier: MPL-2.0

import "./poster-row.css";

import { useEffect, useRef, useState, type JSX, type ReactNode, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { arrivalMark } from "./focus-navigation.ts";
import { Icon } from "./icon.tsx";
import { IconButton } from "./icon-button.tsx";

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
 * the list a remote moves along by Left and Right (data-focus-container) and Tab walks, scrolling it;
 * and, for a mouse with no sideways wheel, an arrow at each end where there is more that way (the
 * owner, 2026-11-04). The arrows show only on a device whose pointer hovers and is fine, while the
 * pointer is over the row or focus is in it (poster-row.css); they are out of Tab's order and a
 * remote's (IconButton's isPointerOnly), so a phone and a television are as they were. listRef,
 * given, is the list's, for a row that scrolls itself to a poster (SeasonRow).
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
        className={props.className === undefined ? "poster-row" : `poster-row ${props.className}`}
        aria-label={props.label}
        data-focus-container=""
        {...(props.isArrival === true ? arrivalMark : {})}
      >
        {props.children}
      </ul>
      {ends.hasMoreBack ? (
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
      {ends.hasMoreOn ? (
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
 * Whether the list has more out of view either way, kept as it scrolls, as its box changes size and
 * as its posters come and go.
 */
function useRowEnds(listRef: RefObject<HTMLUListElement | null>): RowEnds {
  const [ends, setEnds] = useState<RowEnds>({ hasMoreBack: false, hasMoreOn: false });
  useEffect(() => {
    const list = listRef.current;
    if (list === null) {
      return undefined;
    }
    const measure = (): void => {
      const next = rowEndsOf(list);
      setEnds((shown) =>
        shown.hasMoreBack === next.hasMoreBack && shown.hasMoreOn === next.hasMoreOn ? shown : next,
      );
    };
    list.addEventListener("scroll", measure, { passive: true });
    const resizes = typeof ResizeObserver === "undefined" ? undefined : new ResizeObserver(measure);
    resizes?.observe(list);
    const changes =
      typeof MutationObserver === "undefined" ? undefined : new MutationObserver(measure);
    changes?.observe(list, { childList: true });
    // The first measure, once the list is laid out, where no observer reports one.
    const frame = requestAnimationFrame(measure);
    return () => {
      cancelAnimationFrame(frame);
      list.removeEventListener("scroll", measure);
      resizes?.disconnect();
      changes?.disconnect();
    };
  }, [listRef]);
  return ends;
}

/** Whether a list has more out of view before its scroll position, and after what it shows. */
function rowEndsOf(list: HTMLElement): RowEnds {
  const position = Math.abs(list.scrollLeft);
  return {
    hasMoreBack: position > slackPixels,
    hasMoreOn: position + list.clientWidth < list.scrollWidth - slackPixels,
  };
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
 * scroll-snap would put one; at once for a person who asked for less motion.
 */
function scrollRowAPage(list: HTMLElement, direction: RowDirection): void {
  const style = getComputedStyle(list);
  const paddingStart = pixelsOf(style.scrollPaddingLeft);
  const paddingEnd = pixelsOf(style.scrollPaddingRight);
  const box = list.getBoundingClientRect();
  const viewStart = box.left + list.clientLeft + paddingStart;
  const viewEnd = box.left + list.clientLeft + list.clientWidth - paddingEnd;
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
    left: list.scrollLeft + target.left - viewStart,
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
