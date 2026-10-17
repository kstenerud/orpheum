// SPDX-License-Identifier: MPL-2.0

/**
 * What was true of a player's page as its video paused or moved, sent with the event, which says where
 * the pause or the move came from: the page hidden or its window unfocused, the person's own gesture,
 * a media-session action (a media key, the desktop's controls), or hls.js moving the video itself
 * over a hole or a stall; and what the picture's and the sound's buffers held. Two players' 0.2 s
 * pauses and one 0.25 s jump forward were heard as hiccups, and nothing could say why (the owner,
 * 2026-10-17).
 */

import { diagnosisOf } from "./diagnosis.ts";
import { bufferedTextMost, type PlayEventContext } from "./report.ts";

/** What hls.js moved the video over by itself: a hole in what it loaded, or a stall it nudged on. */
export type SkippedBy = NonNullable<PlayEventContext["skippedBy"]>;

/** A media-session action the page answered. */
export type MediaSessionAction = NonNullable<PlayEventContext["mediaSessionAction"]>;

/** The page a video is on, as far as an event's context reads it. */
export interface ContextPage {
  readonly visibilityState: DocumentVisibilityState;
  hasFocus(): boolean;
}

/** The context of a pause or a move of video on page, with what else is known of it. */
export function eventContextOf(
  video: Pick<HTMLMediaElement, "readyState" | "error">,
  page: ContextPage,
  known: {
    readonly isUserActive?: boolean | undefined;
    readonly mediaSessionAction?: MediaSessionAction | undefined;
    readonly skippedBy?: SkippedBy | undefined;
  },
): PlayEventContext {
  const diagnosis = diagnosisOf(video);
  return {
    ...(page.visibilityState === "hidden" ? { isPageHidden: true } : {}),
    ...(page.hasFocus() ? { hasFocus: true } : {}),
    ...(known.isUserActive === true ? { isUserActive: true } : {}),
    ...(known.mediaSessionAction === undefined
      ? {}
      : { mediaSessionAction: known.mediaSessionAction }),
    ...(known.skippedBy === undefined ? {} : { skippedBy: known.skippedBy }),
    readyState: video.readyState,
    ...(diagnosis === undefined
      ? {}
      : {
          videoBuffered: diagnosis.videoBuffered.slice(-bufferedTextMost),
          audioBuffered: diagnosis.audioBuffered.slice(-bufferedTextMost),
        }),
  };
}

/** What hls.js said it moved the video over, from the event it moved it by; undefined for any other. */
export function skippedByOf(event: Event): SkippedBy | undefined {
  const detail: unknown = event instanceof CustomEvent ? event.detail : undefined;
  return detail === "hole" || detail === "nudge" ? detail : undefined;
}

/** How recent an action must be to be the cause of the pause or play that follows it. */
export const mediaSessionActionLastsMs = 1000;

/** The media session a page answers actions on, as far as it is used. */
export interface ActionSession {
  setActionHandler(action: MediaSessionAction, handler: (() => void) | null): void;
}

/**
 * Answers media's play and pause actions on session as the browser would by itself, playing or
 * pausing it, and remembers the last, so a pause a media key or the desktop's controls made says so;
 * returns the action of the last mediaSessionActionLastsMs, and the stopping.
 */
export function watchMediaSession(
  session: ActionSession | undefined,
  media: Pick<HTMLMediaElement, "play" | "pause">,
  nowMs: () => number,
): { readonly lastAction: () => MediaSessionAction | undefined; readonly stop: () => void } {
  let last: { readonly action: MediaSessionAction; readonly atMs: number } | undefined;
  const answer = (action: MediaSessionAction): void => {
    last = { action, atMs: nowMs() };
    if (action === "play") {
      media.play().catch(() => undefined);
    } else {
      media.pause();
    }
  };
  try {
    session?.setActionHandler("play", () => {
      answer("play");
    });
    session?.setActionHandler("pause", () => {
      answer("pause");
    });
  } catch {
    // A browser that offers no such action keeps its own answer, and says none.
  }
  return {
    lastAction: () =>
      last === undefined || nowMs() - last.atMs > mediaSessionActionLastsMs
        ? undefined
        : last.action,
    stop: () => {
      try {
        session?.setActionHandler("play", null);
        session?.setActionHandler("pause", null);
      } catch {
        // As above.
      }
    },
  };
}
