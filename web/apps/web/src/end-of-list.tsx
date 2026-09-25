// SPDX-License-Identifier: MPL-2.0

import { useEffect, useRef, type JSX, type ReactNode } from "react";

/**
 * The end of a list that has more: it asks for more as it comes into view, and holds a button
 * that asks too, for a keyboard, a screen reader, or a browser without IntersectionObserver.
 */
export function EndOfList(props: {
  readonly onVisible: () => void;
  readonly children: ReactNode;
}): JSX.Element {
  const endRef = useRef<HTMLDivElement>(null);
  const { onVisible } = props;
  useEffect(() => {
    const target = endRef.current;
    if (target === null || typeof IntersectionObserver === "undefined") {
      return;
    }
    const watcher = new IntersectionObserver((seen) => {
      if (seen.some((entry) => entry.isIntersecting)) {
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
      {props.children}
    </div>
  );
}
