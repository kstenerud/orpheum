// SPDX-License-Identifier: MPL-2.0

import "./end-of-list.css";

import { useCallback, useEffect, useRef, type JSX } from "react";
import { useTranslation } from "react-i18next";

/**
 * How far below the screen the end of a list asks for more: two screens, a held Down's second or so
 * on a TV, so the next page has come before focus reaches the last row. A best guess, measured by
 * nothing finer than the Fire TV's walk; a page is 60 posters, cheap to ask for early.
 */
const aheadOfView = "200%";

/**
 * The end of a list that has more: it asks for more as it nears the view, which a remote's or a
 * keyboard's focus moving down brings it into, and says so while more loads. It holds no button: a
 * "Show more" that loaded more unpressed confused, and on a remote focus left on it was lost when
 * more came, the next press jumping to the page's top (the owner, Fire TV, 2026-10-25).
 *
 * It asks while still aheadOfView below the screen, and again once a page has come while it is still
 * that near: asked only as it came into view, the next page was asked for on a TV's last row, and Down
 * from that row, finding nothing below in the grid until the page came, left it for the alphabet
 * rail beside it (the Fire TV's trace, 2026-11-11: Down from the 55th of 60 films took focus to "Y").
 */
export function EndOfList(props: {
  readonly onMore: () => void;
  readonly isLoadingMore: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const endRef = useRef<HTMLDivElement>(null);
  const end = useCallback(() => endRef.current, []);
  useAskedWhenNear(end, props.onMore, props.isLoadingMore);
  return (
    <div ref={endRef} className="end-of-list">
      <output>{props.isLoadingMore ? t("common.loading") : ""}</output>
    </div>
  );
}

/**
 * Asks onNear while what target finds is aheadOfView below the screen, and again once a page has come while it
 * is still that near, never while more is loading: EndOfList's, and a grid's first placeholder's,
 * which stands where the list's end will be once the next page comes (the owner, 2026-11-11).
 */
export function useAskedWhenNear(
  target: () => Element | null,
  onNear: () => void,
  isLoadingMore: boolean,
): void {
  useEffect(() => {
    const near = target();
    if (near === null || isLoadingMore || typeof IntersectionObserver === "undefined") {
      return;
    }
    // A new watcher says at once whether the end is near: a page come while it stays near asks again.
    const watcher = nearViewWatcher((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) {
        onNear();
      }
    }, near.ownerDocument);
    watcher.observe(near);
    return () => {
      watcher.disconnect();
    };
  }, [target, onNear, isLoadingMore]);
}

/**
 * A watcher of what comes within aheadOfView below the view of document: the document its root, so
 * the margin counts in a frame too, where the implicit root, the top window's, would see the frame
 * clip it first (a test's page is a frame); the top window's where a browser takes no document for a
 * root (Chromium before 81).
 */
function nearViewWatcher(
  tell: IntersectionObserverCallback,
  document: Document,
): IntersectionObserver {
  const rootMargin = `0px 0px ${aheadOfView} 0px`;
  try {
    return new IntersectionObserver(tell, { root: document, rootMargin });
  } catch {
    return new IntersectionObserver(tell, { rootMargin });
  }
}
