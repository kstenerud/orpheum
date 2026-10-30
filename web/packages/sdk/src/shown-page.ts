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
 * Calls shown each time page is shown again ("shown"), if there is a page, and each time pathBack tells
 * that the way to the core may work again ("back"), if there is one: either may find a socket dead that died
 * unseen. Returns the stopping.
 */
export function whenShownOrBack(
  page: ShownPage | undefined,
  pathBack: ((listener: () => void) => () => void) | undefined,
  shown: (why: "shown" | "back") => void,
): () => void {
  const stopHearingPage = whenVisibilityChanges(page, (isHidden) => {
    if (!isHidden) {
      shown("shown");
    }
  });
  const stopHearingPath = pathBack?.(() => {
    shown("back");
  });
  return () => {
    stopHearingPage();
    stopHearingPath?.();
  };
}

/**
 * A page's window, which tells when its page is left (pagehide: closed, reloaded, or navigated away,
 * perhaps into the back/forward cache) and when it is shown (pageshow, persisted when it comes back
 * from that cache).
 */
export interface LeftPage {
  addEventListener(
    type: "pagehide" | "pageshow",
    listener: (event: Pick<PageTransitionEvent, "persisted">) => void,
  ): void;
  removeEventListener(
    type: "pagehide" | "pageshow",
    listener: (event: Pick<PageTransitionEvent, "persisted">) => void,
  ): void;
}

/**
 * Calls left each time the page is left, if there is a page, and returned each time it comes back from
 * the back/forward cache: a page navigated away may sit there with its sockets open, which the core
 * counts present until their silence limit (the owner, 2026-10-29). Returns the stopping.
 */
export function whenLeftOrReturned(
  page: LeftPage | undefined,
  left: () => void,
  returned: () => void,
): () => void {
  const onHide = (): void => {
    left();
  };
  const onShow = (event: Pick<PageTransitionEvent, "persisted">): void => {
    if (event.persisted) {
      returned();
    }
  };
  page?.addEventListener("pagehide", onHide);
  page?.addEventListener("pageshow", onShow);
  return () => {
    page?.removeEventListener("pagehide", onHide);
    page?.removeEventListener("pageshow", onShow);
  };
}

/** What a connection's page tells it of: shown again, the way to the core back, left and returned. */
export interface PageOfConnection {
  readonly page?: ShownPage | undefined;
  readonly pathBack?: ((listener: () => void) => () => void) | undefined;
  readonly leftPage?: LeftPage | undefined;
}

/** What a connection does as its page tells it so. */
export interface DoesAsPageChanges {
  /** The page is shown again, or the way to the core may work again: check the socket. */
  readonly shownAgain: (why: "shown" | "back") => void;
  /** The page is left: give the socket up at once, so the core counts the device gone. */
  readonly left: () => void;
  /** The page came back from the back/forward cache: connect again at once. */
  readonly returned: () => void;
}

/**
 * Has does hear its page shown again, the way to the core back, and the page left and returned: the
 * one way a page's sockets hear of their page, the session's and the live connection's alike (the
 * owner, 2026-10-29: one kind of decision, one path). Returns the stopping.
 */
export function whenPageChanges(of: PageOfConnection, does: DoesAsPageChanges): () => void {
  const stopShown = whenShownOrBack(of.page, of.pathBack, does.shownAgain);
  const stopLeft = whenLeftOrReturned(of.leftPage, does.left, does.returned);
  return () => {
    stopShown();
    stopLeft();
  };
}
