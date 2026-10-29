// SPDX-License-Identifier: MPL-2.0

/** A page that tells when it is hidden or shown again: a browser's document. */
export interface ShownPage {
  readonly visibilityState: DocumentVisibilityState;
  addEventListener(type: "visibilitychange", listener: () => void): void;
  removeEventListener(type: "visibilitychange", listener: () => void): void;
}

/** Calls changed each time page is hidden or shown, if there is a page; returns the stopping. */
export function whenVisibilityChanges(
  page: ShownPage | undefined,
  changed: (isHidden: boolean) => void,
): () => void {
  const onVisibility = (): void => {
    if (page !== undefined) {
      changed(page.visibilityState === "hidden");
    }
  };
  page?.addEventListener("visibilitychange", onVisibility);
  return () => {
    page?.removeEventListener("visibilitychange", onVisibility);
  };
}

/**
 * Calls shown each time page is shown again, if there is a page, and each time pathBack tells that
 * the way to the core may work again, if there is one: either may find a socket dead that died
 * unseen. Returns the stopping.
 */
export function whenShownOrBack(
  page: ShownPage | undefined,
  pathBack: ((listener: () => void) => () => void) | undefined,
  shown: () => void,
): () => void {
  const stopHearingPage = whenVisibilityChanges(page, (isHidden) => {
    if (!isHidden) {
      shown();
    }
  });
  const stopHearingPath = pathBack?.(shown);
  return () => {
    stopHearingPage();
    stopHearingPath?.();
  };
}
