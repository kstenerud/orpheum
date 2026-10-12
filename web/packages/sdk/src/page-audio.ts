// SPDX-License-Identifier: MPL-2.0

/**
 * The page's audio session, where the browser has one (Safari's navigator.audioSession, iOS 16.4 and
 * later): "playback" while a film plays, as a video app's, which the ring/silent switch does not mute;
 * "auto" while a microphone is open, which makes it a call. And its heal: iOS ends Safari's GPU
 * process freely while Safari is in the background, the web process keeps the category it last sent
 * and never sends it again, and the tab's sound is stuck in the switch-muted ambient category while
 * every API says playback, until the session is set to another type and back (WebKit bug 323104; the
 * owner's iPhone, 2026-10-12: silent with the switch on until Safari was quit;
 * design/research/ios-silent-switch-2026-10-12.md).
 */

/** What the page's audio session is told it plays. */
export type PageAudio = "auto" | "playback";

/** The type the page means its session to have, which a heal sets again. */
let intended: PageAudio = "playback";

/** Sets the browser's audio session to type, where it has one; elsewhere nothing. */
function applyType(type: PageAudio | "ambient"): void {
  const session: unknown =
    typeof navigator === "undefined" ? undefined : Reflect.get(navigator, "audioSession");
  if (typeof session === "object" && session !== null) {
    Reflect.set(session, "type", type);
  }
}

/** Tells the page's audio session what it plays, and keeps it as what a heal sets again. */
export function setPageAudio(type: PageAudio): void {
  intended = type;
  applyType(type);
}

/**
 * Heals a session stuck in the switch-muted category: set to "ambient", then, a tick later, to what
 * the page means, which sends the category to the GPU process anew (WebKit bug 323104). Setting the
 * same type again sends nothing, and heals nothing.
 */
export function healPageAudio(): void {
  applyType("ambient");
  setTimeout(() => {
    applyType(intended);
  }, 0);
}

/** A page that says when it is shown again: a browser's document. */
export interface ShownAgainPage {
  readonly visibilityState: DocumentVisibilityState;
  addEventListener(type: "visibilitychange", listener: () => void): void;
  removeEventListener(type: "visibilitychange", listener: () => void): void;
}

/**
 * Keeps the page's audio session as a video app's from now, and heals it each time the page is shown
 * again, as after Safari was in the background, when its GPU process may have ended. Returns the
 * stopping.
 */
export function keepPageAudio(page: ShownAgainPage): () => void {
  setPageAudio("playback");
  const onVisibility = (): void => {
    if (page.visibilityState === "visible") {
      healPageAudio();
    }
  };
  page.addEventListener("visibilitychange", onVisibility);
  return () => {
    page.removeEventListener("visibilitychange", onVisibility);
  };
}
