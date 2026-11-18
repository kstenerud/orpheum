// SPDX-License-Identifier: MPL-2.0

/**
 * The receiver's own idle close (the plan file's "A film played to its end with nothing next goes
 * back to its page", its cast part; Google's design checklist for a receiver): CAF's own idle timeout
 * is off, as it cannot see our player, which replaces CAF's (caf.ts), so the receiver times itself by
 * its video. With nothing playing it closes after idleClosesAfterMs; with a film paused, or ended
 * with no other put on, after pausedClosesAfterMs (the owner, 2026-11-18). Playing, it stays.
 */

/** How long the receiver stays with nothing playing before it closes: 5 minutes. */
export const idleClosesAfterMs = 5 * 60_000;

/** How long the receiver stays with its film paused before it closes: 20 minutes. */
export const pausedClosesAfterMs = 20 * 60_000;

/** How the receiver's video stands, for its idle close. */
export type Standing = "playing" | "paused" | "idle";

/** The video's events after which how it stands is read again. */
const readAgainAfter = ["loadstart", "play", "playing", "pause", "ended", "emptied"] as const;

/**
 * How video stands: playing; paused, a film on it stopped or played to its end; or idle, nothing on
 * it, as before any film or once one is taken off.
 */
export function standingOf(video: HTMLVideoElement): Standing {
  if (video.currentSrc === "" && video.readyState === 0) {
    return "idle";
  }
  return video.paused || video.ended ? "paused" : "playing";
}

/**
 * Has the receiver close, by close, once video has stood idle for idleClosesAfterMs or paused for
 * pausedClosesAfterMs, timed from when it came to stand so; returns the watch's end. Read at once:
 * a receiver started with nothing on it closes too.
 */
export function closesWhenIdle(video: HTMLVideoElement, close: () => void): () => void {
  let standing: Standing | undefined;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const readAgain = (): void => {
    const now = standingOf(video);
    if (now === standing) {
      return; // timed from when it came to stand so, not from each event since
    }
    standing = now;
    clearTimeout(timer);
    timer =
      now === "playing"
        ? undefined
        : setTimeout(close, now === "idle" ? idleClosesAfterMs : pausedClosesAfterMs);
  };
  for (const type of readAgainAfter) {
    video.addEventListener(type, readAgain);
  }
  readAgain();
  return () => {
    clearTimeout(timer);
    for (const type of readAgainAfter) {
      video.removeEventListener(type, readAgain);
    }
  };
}
