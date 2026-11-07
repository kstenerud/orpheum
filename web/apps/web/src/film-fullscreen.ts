// SPDX-License-Identifier: MPL-2.0

import { useEffect } from "react";

/** The class that has a player's stage fill the browser's window where full screen was refused. */
export const fillsWindowClass = "film-player__stage--fills-window";

/** Who follows whether a stage fills the window (followFillsWindow). */
const fillListeners = new Set<() => void>();

/**
 * Has a player's stage fill the window, or no longer, and tells whoever follows it (useIsFullScreen):
 * the one way the fills-window class is set, so what follows it never misses a change.
 */
export function setFillsWindow(stage: Element, isFilling: boolean): void {
  if (stage.classList.contains(fillsWindowClass) === isFilling) {
    return;
  }
  // A stage the page keeps filling the window (keepFillingTheWindow) fills it until the page lets go.
  if (!isFilling && stage.hasAttribute(keptFillingAttribute)) {
    return;
  }
  stage.classList.toggle(fillsWindowClass, isFilling);
  for (const listener of fillListeners) {
    listener();
  }
}

/** The attribute of a stage its page keeps filling the window (keepFillingTheWindow). */
const keptFillingAttribute = "data-fills-window-kept";

/**
 * Has a player's stage fill the window for as long as its page wants it, whatever full screen does:
 * the Play page's player is the whole window on every device (ui-audit-2026-11-04.md §3, the
 * player), its controls over the picture, so leaving the browser's full screen (⤢, Escape, a Fire
 * TV's Back) leaves the film filling the window, never the page under it. Returns the letting go,
 * which the stage's page calls as it goes.
 */
export function keepFillingTheWindow(stage: Element): () => void {
  stage.setAttribute(keptFillingAttribute, "");
  setFillsWindow(stage, true);
  return () => {
    stage.removeAttribute(keptFillingAttribute);
    setFillsWindow(stage, false);
  };
}

/** Follows a stage's filling the window or no longer (setFillsWindow); returns the stopping. */
export function followFillsWindow(listener: () => void): () => void {
  fillListeners.add(listener);
  return () => {
    fillListeners.delete(listener);
  };
}

/** Whether a player's stage on the page fills the window, where full screen was refused. */
export function isAStageFillingTheWindow(page: Document): boolean {
  return page.querySelector(`.${fillsWindowClass}`) !== null;
}

/**
 * Puts a player's stage full screen, as a film opens on a television or in a browser that is itself
 * full screen; where the browser refuses, the stage fills the window instead, which in a browser
 * already full screen looks the same. Safari refuses a request not made in the press itself, and
 * the player opens a moment after the press that opened it (the owner's Mac, 2026-10-25: Safari full
 * screen, the film did not fill it), and may not answer at all, so a request unanswered in time
 * counts as refused. A stage its page keeps filling the window (keepFillingTheWindow) puts the whole
 * page full screen instead, which looks the same and outlives the stage: the Play page's next or
 * previous episode is the page drawn again, a new stage in place of the one full screen, which the
 * browser left as it went (the owner, 2026-11-05: "Pressing next or previous episode while in
 * fullscreen removes fullscreen"), and full screen asked again as the new stage shows, no press
 * behind it, is refused. The page stays, as a cast's receiver's does (askPageFullScreen).
 */
export function fillTheScreen(stage: HTMLElement, document: Document): void {
  if (document.fullscreenElement instanceof Element || pagesAskedFullScreen.has(document)) {
    return;
  }
  const fillWindow = (): void => {
    setFillsWindow(stage, true);
  };
  const asked = stage.hasAttribute(keptFillingAttribute) ? document.documentElement : stage;
  if (typeof asked.requestFullscreen !== "function") {
    fillWindow();
    return;
  }
  asked.requestFullscreen().catch(fillWindow);
  // A browser that neither grants nor refuses in time is taken to have refused. The answer is read
  // without the page's globals, which a page torn down before the wait ends no longer has.
  setTimeout(() => {
    if ((document.fullscreenElement ?? null) === null) {
      fillWindow();
    }
  }, fullscreenAnswerWithinMs);
}

/**
 * A full-screen button's press, or a double click on the film: out of full screen it puts the
 * player's stage full screen (fillTheScreen), from the press the Fullscreen API needs; in full screen
 * it leaves, as the page's own asking (leaveFullScreenAsked), so a cast's receiver keeps its cast, and
 * the stage goes back into the page, no longer filling the window where the browser once refused full
 * screen: its banner shows again (the owner's S23, 2026-10-30: the button changed nothing). A stage
 * filling the window out of full screen asks for it at the press, which the browser grants (a Fire TV
 * whose player opened out of full screen, 2026-10-27).
 */
export function toggleFullScreen(stage: Element | null | undefined, page: Document): void {
  if (page.fullscreenElement instanceof Element) {
    leaveFullScreenAsked(page);
    if (stage instanceof Element) {
      setFillsWindow(stage, false);
    }
    return;
  }
  if (!(stage instanceof HTMLElement)) {
    return;
  }
  // An iPhone has no element full screen (WebKit bug 206854): its film goes into iOS's own player,
  // which the press allows, rather than filling the window under Safari's bars (the owner, 2026-11-01).
  const video = stage.querySelector("video");
  if (typeof stage.requestFullscreen !== "function" && video !== null && hasOwnFullScreen(video)) {
    video.webkitEnterFullscreen();
    return;
  }
  fillTheScreen(stage, page);
}

/** A video that has its own full screen, as an iPhone's alone does where requestFullscreen is missing. */
interface WebKitVideo {
  readonly webkitEnterFullscreen: () => void;
}

/** Whether the video has the iPhone's own full screen. */
function hasOwnFullScreen(video: HTMLVideoElement): video is HTMLVideoElement & WebKitVideo {
  return "webkitEnterFullscreen" in video && typeof video.webkitEnterFullscreen === "function";
}

/**
 * Moves full screen from the video to its player's stage wherever the browser puts the video itself
 * full screen: the browser's own controls on the video (Watch Together's guest with the rights to
 * play or seek, D271 §4) have a full-screen button of their own, which asks for the video, and a
 * video full screen shows nothing of the page over it (the owner, Firefox on Android, 2026-11-07:
 * Android's own full screen, none of our controls). The video's is left and the stage asked for
 * (fillTheScreen), filling the window where the browser refuses, the press that asked having been
 * spent on the video's. An iPhone's own player is no full screen of the page's (WebKit bug 206854)
 * and is left alone. Returns the stopping.
 */
export function keepFullScreenOnTheStage(video: HTMLVideoElement, page: Document): () => void {
  const changed = (): void => {
    const stage = video.closest(".film-player__stage");
    if (page.fullscreenElement !== video || !(stage instanceof HTMLElement)) {
      return;
    }
    page.exitFullscreen().then(
      () => {
        fillTheScreen(stage, page);
      },
      () => undefined,
    );
  };
  page.addEventListener("fullscreenchange", changed);
  return () => {
    page.removeEventListener("fullscreenchange", changed);
  };
}

/** The pages that asked for themselves full screen, until the browser answers (askPageFullScreen). */
const pagesAskedFullScreen = new WeakSet<Document>();

/**
 * Asks for the whole page full screen, from a press, and answers whether the browser put it there.
 * Until it answers no player's stage asks for its own (fillTheScreen): a cast's receiver asked as its
 * Use was pressed, the Play page opening at once, and the stage, asked a moment later, became the
 * full-screen element, which the next video's Play page took away, leaving full screen (D364 §3, §5;
 * the cast test, 2026-10-27). The page held full screen across every video of the session.
 */
export async function askPageFullScreen(page: Document): Promise<boolean> {
  pagesAskedFullScreen.add(page);
  try {
    await page.documentElement.requestFullscreen();
    return true;
  } catch {
    return false;
  } finally {
    pagesAskedFullScreen.delete(page);
  }
}

/** The pages that asked to leave full screen, until the browser says it changed. */
const leavingAsked = new WeakSet<Document>();

/**
 * Leaves full screen at the page's own asking, which isFullScreenLeaveAsked says until the browser's
 * next fullscreenchange has been heard: an input layer offers Back where the browser leaves full
 * screen unasked (a Fire TV's Back, Escape; input-layer.tsx), and the page's own leaving is not that.
 */
export function leaveFullScreenAsked(page: Document): void {
  leavingAsked.add(page);
  const forget = (): void => {
    leavingAsked.delete(page);
  };
  page.addEventListener("fullscreenchange", forget, { once: true });
  page.exitFullscreen().catch(() => {
    page.removeEventListener("fullscreenchange", forget);
    forget();
  });
}

/**
 * Leaves full screen, at the page's own asking, as the component calling it is drawn: the cast's
 * remote, a page and no film, which a cast started from full screen leaves it for (D440 §5). The Play
 * page's player puts the whole page full screen (fillTheScreen, keepFillingTheWindow), which outlives
 * the player the remote takes the place of, so the remote would show full screen unless left.
 */
export function useLeavesFullScreen(): void {
  useEffect(() => {
    if (document.fullscreenElement instanceof Element) {
      leaveFullScreenAsked(document);
    }
  }, []);
}

/** Whether the full screen being left now is one the page asked to leave (leaveFullScreenAsked). */
export function isFullScreenLeaveAsked(page: Document): boolean {
  return leavingAsked.has(page);
}

/**
 * How long a browser may take to put the stage full screen before it fills the window instead, in
 * milliseconds: a full-screen change takes a fraction of a second. A justified constant.
 */
export const fullscreenAnswerWithinMs = 1000;
