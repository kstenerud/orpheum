// SPDX-License-Identifier: MPL-2.0

/**
 * Pauses a film whose page is hidden (D287): an iPhone's page told it plays as a video app's, so the
 * silent switch does not mute it, also plays its sound on with Safari in the background, the picture
 * gone (the owner, 2026-10-12: "I've backgrounded the browser, but the audio from the movie is still
 * playing"). As YouTube's page does, and as iOS did before, the film waits; a film in
 * picture-in-picture, still watched, plays on. In a Watch Together group the pause is the browser's,
 * not its person's (followGroup's page), and the player rejoins the group as the page is shown again.
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

/** Pauses media each time page is hidden while it plays, unless it is in picture-in-picture; returns the stopping. */
export function pauseWhenHidden(media: PausedFilm, page: HiddenPage): () => void {
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
