// SPDX-License-Identifier: MPL-2.0

/** The class that has a player's stage fill the browser's window where full screen was refused. */
export const fillsWindowClass = "film-player__stage--fills-window";

/**
 * Puts a player's stage full screen, as a film opens on a television or in a browser that is itself
 * full screen; where the browser refuses, the stage fills the window instead, which in a browser
 * already full screen looks the same. Safari refuses a request not made in the press itself, and
 * the player opens a moment after the press that opened it (the owner's Mac, 2026-10-25: Safari full
 * screen, the film did not fill it), and may not answer at all, so a request unanswered in time
 * counts as refused.
 */
export function fillTheScreen(stage: HTMLElement, document: Document): void {
  if (document.fullscreenElement instanceof Element || pagesAskedFullScreen.has(document)) {
    return;
  }
  const fillWindow = (): void => {
    stage.classList.add(fillsWindowClass);
  };
  if (typeof stage.requestFullscreen !== "function") {
    fillWindow();
    return;
  }
  stage.requestFullscreen().catch(fillWindow);
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
    stage?.classList.remove(fillsWindowClass);
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
 * Shows a video full screen from a press: by the standard way where the browser has it, else in the
 * iPhone's own player (WebKit bug 206854). Watch Together's viewer, whose film has no player bar.
 */
export function enterVideoFullScreen(video: HTMLVideoElement | null): void {
  if (video === null) {
    return;
  }
  if (typeof video.requestFullscreen === "function") {
    void video.requestFullscreen().catch(() => undefined);
  } else if (hasOwnFullScreen(video)) {
    video.webkitEnterFullscreen();
  }
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
 * next fullscreenchange has been heard: a cast's receiver takes control back where the browser leaves
 * full screen unasked (a Fire TV's Back, Escape; cast-receiver.tsx), and its own leaving, Stop
 * casting or the film's end, is not that.
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

/** Whether the full screen being left now is one the page asked to leave (leaveFullScreenAsked). */
export function isFullScreenLeaveAsked(page: Document): boolean {
  return leavingAsked.has(page);
}

/**
 * How long a browser may take to put the stage full screen before it fills the window instead, in
 * milliseconds: a full-screen change takes a fraction of a second. A justified constant.
 */
export const fullscreenAnswerWithinMs = 1000;
