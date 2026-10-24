// SPDX-License-Identifier: MPL-2.0

/**
 * How late a video's picture reaches the screen, as the browser says of each frame it shows
 * (requestVideoFrameCallback): from the frame's handing to the compositor to when it is expected on
 * screen, and how far it is behind the sound the element plays then. The owner's TCL TV, in TV Bro,
 * showed the picture about half a second after its sound, and nothing measured it (2026-10-24).
 */
export interface FrameTiming {
  /** How many frames the figures are the middle of. */
  readonly frames: number;
  /** From the frame's handing to the compositor to its expected display, in milliseconds. */
  readonly displayAfterMs: number;
  /** How far the picture on screen is behind the sound playing then, in milliseconds. */
  readonly behindSoundMs: number;
}

/** How many of the latest frames a timing is the middle of. */
export const framesTimed = 48;

/** The middle of values, which a frame or two held up by a seek or a stall does not move. */
function median(values: readonly number[]): number {
  const sorted = [...values].sort((a, b) => a - b);
  return sorted[Math.floor(sorted.length / 2)] ?? 0;
}

/**
 * Times each frame video shows while it plays, and tells onTiming the middle of the latest
 * framesTimed every time that many more have been shown; it returns the stop. A browser without
 * requestVideoFrameCallback is told as undefined, once.
 */
export function timeFrames(
  video: HTMLVideoElement,
  onTiming: (timing: FrameTiming | undefined) => void,
): () => void {
  if (typeof video.requestVideoFrameCallback !== "function") {
    onTiming(undefined);
    return () => undefined;
  }
  let isStopped = false;
  const displayAfter: number[] = [];
  const behind: number[] = [];
  let sinceTold = 0;
  const onFrame = (nowMs: number, frame: VideoFrameCallbackMetadata): void => {
    if (isStopped) {
      return;
    }
    if (!video.paused && !video.seeking) {
      // The sound playing when the frame is on screen: where the element is now, and the time
      // until the frame shows.
      const soundAtS = video.currentTime + (frame.expectedDisplayTime - nowMs) / 1000;
      displayAfter.push(frame.expectedDisplayTime - frame.presentationTime);
      behind.push((soundAtS - frame.mediaTime) * 1000);
      displayAfter.splice(0, Math.max(0, displayAfter.length - framesTimed));
      behind.splice(0, Math.max(0, behind.length - framesTimed));
      sinceTold += 1;
      if (sinceTold >= framesTimed) {
        sinceTold = 0;
        onTiming({
          frames: behind.length,
          displayAfterMs: Math.round(median(displayAfter)),
          behindSoundMs: Math.round(median(behind)),
        });
      }
    }
    video.requestVideoFrameCallback(onFrame);
  };
  video.requestVideoFrameCallback(onFrame);
  return () => {
    isStopped = true;
  };
}
