// SPDX-License-Identifier: MPL-2.0

/**
 * Stand-ins for a computer's page hidden behind another window while its film plays on (the owner,
 * 2026-10-28), for tests of the players that follow a group.
 */

/**
 * Makes video a playing video at atS seconds, loaded, that pause and play move; returns whether it
 * is paused now.
 */
export function playingVideo(video: HTMLVideoElement, atS: number): () => boolean {
  let isPaused = false;
  Object.defineProperty(video, "readyState", { value: 4, configurable: true });
  Object.defineProperty(video, "currentTime", { value: atS, writable: true, configurable: true });
  Object.defineProperty(video, "paused", { get: () => isPaused, configurable: true });
  /* eslint-disable no-param-reassign -- the stand-in's whole purpose */
  video.pause = () => {
    isPaused = true;
  };
  video.play = async () => {
    isPaused = false;
    await Promise.resolve();
  };
  /* eslint-enable no-param-reassign -- only these */
  return () => isPaused;
}

/** Hides the document, as a window covered by another is, telling its listeners; returns the showing. */
export function hideTheDocument(): () => void {
  Object.defineProperty(document, "visibilityState", { value: "hidden", configurable: true });
  document.dispatchEvent(new Event("visibilitychange"));
  return () => {
    Reflect.deleteProperty(document, "visibilityState");
  };
}
