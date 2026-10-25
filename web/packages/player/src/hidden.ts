// SPDX-License-Identifier: MPL-2.0

/**
 * Pauses a film whose page is hidden (D287): an iPhone's page told it plays as a video app's, so the
 * silent switch does not mute it, also plays its sound on with Safari in the background, the picture
 * gone (the owner, 2026-10-12: "I've backgrounded the browser, but the audio from the movie is still
 * playing"). As YouTube's page does, and as iOS did before, the film waits; a film in
 * picture-in-picture, still watched, plays on. In a Watch Together group the pause is the browser's,
 * not its person's (followGroup's page), and the player rejoins the group as the page is shown again.
 * Only a handheld's: a computer's hidden tab is one of several open beside the film, which plays on
 * as a computer's video does elsewhere (the owner, Firefox, 2026-10-25: "Browser shouldn't auto-pause
 * playback when you switch to another tab").
 */

/** The page a film is on: whether it is hidden, its picture-in-picture video, and its events. */
export interface HiddenPage {
  readonly visibilityState: DocumentVisibilityState;
  readonly pictureInPictureElement?: Element | null;
  addEventListener(type: "visibilitychange", listener: () => void): void;
  removeEventListener(type: "visibilitychange", listener: () => void): void;
}

/** The film: whether it plays, Safari's presentation mode where it has one, and pausing it. */
export type PausedFilm = Pick<HTMLMediaElement, "paused" | "pause"> & {
  readonly webkitPresentationMode?: string;
};

/** What says whether the browser is a handheld's: its pointer, and its touch points. */
export interface HandheldSigns {
  readonly maxTouchPoints: number;
  // eslint-disable-next-line @typescript-eslint/naming-convention -- MediaQueryList's own name
  matchMedia(query: string): { readonly matches: boolean };
}

/**
 * Whether the browser is a phone's or a tablet's, where a hidden page is the browser gone to the
 * background: its main pointer is a finger, and it has touch. A computer with a touch screen keeps
 * a mouse or a trackpad as its main pointer.
 */
export function isHandheld(signs: HandheldSigns): boolean {
  return signs.maxTouchPoints > 0 && signs.matchMedia("(pointer: coarse)").matches;
}

/**
 * On a handheld (isHandheld), pauses media each time page is hidden while it plays, unless it is in
 * picture-in-picture; elsewhere does nothing. Returns the stopping.
 */
export function pauseWhenHidden(
  media: PausedFilm,
  page: HiddenPage,
  isOnHandheld: boolean,
): () => void {
  if (!isOnHandheld) {
    return () => undefined;
  }
  const onVisibility = (): void => {
    const isPictureInPicture =
      Object.is(page.pictureInPictureElement, media) ||
      media.webkitPresentationMode === "picture-in-picture";
    if (page.visibilityState === "hidden" && !media.paused && !isPictureInPicture) {
      media.pause();
    }
  };
  page.addEventListener("visibilitychange", onVisibility);
  return () => {
    page.removeEventListener("visibilitychange", onVisibility);
  };
}
