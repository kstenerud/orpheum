// SPDX-License-Identifier: MPL-2.0

import "./end-of-list.css";

import { useEffect, useRef, type JSX } from "react";
import { useTranslation } from "react-i18next";

/**
 * The end of a list that has more: it asks for more as it comes into view, which a remote's or a
 * keyboard's focus moving down brings it into, and says so while more loads. It holds no button: a
 * "Show more" that loaded more unpressed confused, and on a remote focus left on it was lost when
 * more came, the next press jumping to the page's top (the owner, Fire TV, 2026-10-25).
 */
export function EndOfList(props: {
  readonly onMore: () => void;
  readonly isLoadingMore: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const endRef = useRef<HTMLDivElement>(null);
  const { onMore: onVisible } = props;
  useEffect(() => {
    const target = endRef.current;
    if (target === null || typeof IntersectionObserver === "undefined") {
      return;
    }
    const watcher = new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) {
        onVisible();
      }
    });
    watcher.observe(target);
    return () => {
      watcher.disconnect();
    };
  }, [onVisible]);
  return (
    <div ref={endRef} className="end-of-list">
      <output>{props.isLoadingMore ? t("common.loading") : ""}</output>
    </div>
  );
}
