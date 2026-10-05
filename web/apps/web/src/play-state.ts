// SPDX-License-Identifier: MPL-2.0

import {
  PlayReporter,
  ReportRefused,
  reportEveryMs,
  seekTo,
  type PlayReport,
} from "@orpheum/player";
import type { Api } from "@orpheum/sdk";
import { useEffect, useState, type RefObject } from "react";

import { ApiProblem, throwIfRefused } from "./problem.ts";

/** The codes of refusals that pass: a report refused with one of these goes again with the next. */
const transientRefusalCodes: ReadonlySet<string> = new Set([
  "internal",
  "migrating",
  "rate_limited",
]);

/**
 * Sends one play report for the playback the media token names, and answers the admin's stop when
 * the server says the playback was stopped (D243 §9). A refusal that sending again
 * cannot change (the playback is gone, the session is not allowed it) rejects with ReportRefused,
 * so the reporter stops; a failure that may pass (no answer, a server busy or updating) rejects as
 * it came, so the report goes again with the next.
 */
export async function sendReport(
  api: Api,
  mediaToken: string,
  report: PlayReport,
): Promise<{ readonly message: string } | undefined> {
  const answer = await api.POST("/playback/{token}/report", {
    params: { path: { token: mediaToken } },
    body: { ...report, events: [...report.events], spans: [...report.spans] },
  });
  try {
    throwIfRefused(answer);
    return answer.data?.stop ?? undefined;
  } catch (cause) {
    if (cause instanceof ApiProblem && !transientRefusalCodes.has(cause.code)) {
      throw new ReportRefused(cause);
    }
    throw cause;
  }
}

/** The video's position, in milliseconds. */
function positionMs(video: HTMLVideoElement): number {
  return video.currentTime * 1000;
}

/**
 * Whether the video holds media. A video being torn down is emptied first, and then says it is
 * paused at 0:00, which is nothing the person watched.
 */
function hasMedia(video: HTMLVideoElement): boolean {
  return video.readyState !== HTMLMediaElement.HAVE_NOTHING;
}

/** The last report number of each playback, by media token, kept across the player's remounts. */
const lastSeqByToken = new Map<string, number>();

/** Numbers one playback's reports on from its last, however many reporters it has had. */
function reportSeqCounter(mediaToken: string): () => number {
  return () => {
    const next = (lastSeqByToken.get(mediaToken) ?? 0) + 1;
    lastSeqByToken.set(mediaToken, next);
    return next;
  };
}

/** What reportVideoEvents hands back: where the video last was while it held media, and how to stop. */
interface VideoEventReporting {
  readonly lastPositionMs: () => number;
  readonly stop: () => void;
}

/**
 * Tells the reporter what the video does, and starts it at resumePositionMs. Events from a video
 * with no media are ignored, and where it was when it last held some is kept: an emptied video is
 * at 0:00, which the person did not watch.
 */
function reportVideoEvents(
  video: HTMLVideoElement,
  reporter: PlayReporter,
  resumePositionMs: number,
): VideoEventReporting {
  let isResuming = resumePositionMs > 0;
  let lastHeldPositionMs = resumePositionMs;
  const whileItHoldsMedia = (listener: () => void) => (): void => {
    if (hasMedia(video)) {
      listener();
    }
  };
  const listeners: Record<string, () => void> = {
    loadedmetadata: () => {
      if (isResuming) {
        seekTo(video, resumePositionMs);
      }
    },
    playing: whileItHoldsMedia(() => {
      reporter.recordStarted(positionMs(video));
    }),
    pause: whileItHoldsMedia(() => {
      if (!video.ended) {
        reporter.recordPaused(positionMs(video));
      }
    }),
    seeking: whileItHoldsMedia(() => {
      // The seek that puts the player where the person left it is not theirs to log.
      if (isResuming) {
        isResuming = false;
      } else {
        reporter.recordSeeked(lastHeldPositionMs, positionMs(video));
      }
    }),
    timeupdate: whileItHoldsMedia(() => {
      lastHeldPositionMs = positionMs(video);
    }),
    ended: whileItHoldsMedia(() => {
      reporter.recordCompleted(positionMs(video));
    }),
  };
  for (const [event, listener] of Object.entries(listeners)) {
    video.addEventListener(event, listener);
  }
  if (isResuming && video.readyState >= HTMLMediaElement.HAVE_METADATA) {
    seekTo(video, resumePositionMs);
  }
  return {
    lastPositionMs: () => lastHeldPositionMs,
    stop: () => {
      for (const [event, listener] of Object.entries(listeners)) {
        video.removeEventListener(event, listener);
      }
    },
  };
}

/**
 * Keeps a playing video's play state on the server (play-state.md): it starts where the person
 * left the file, without asking (D188), and reports each change of state, and where it is every
 * ten seconds while playing and when the page is hidden, until the playback ends. It returns why
 * the server refused the reports for good, once it has, so the person is told that where they are
 * is no longer kept; and the admin's message when they stopped the playback, which pauses the
 * video and ends the reports (D243 §9); each undefined until then.
 */
export function usePlayState(
  api: Api,
  videoRef: RefObject<HTMLVideoElement | null>,
  playback: { readonly mediaToken: string; readonly resumePositionMs: number },
): { readonly refusal: unknown; readonly stopMessage: string | undefined } {
  const { mediaToken, resumePositionMs } = playback;
  const [refusal, setRefusal] = useState<unknown>(undefined);
  const [stopMessage, setStopMessage] = useState<string | undefined>(undefined);
  useEffect(() => {
    const video = videoRef.current;
    if (video === null) {
      return undefined;
    }
    let isStopped = false;
    const reporter = new PlayReporter(async (report) => {
      if (isStopped) {
        return;
      }
      try {
        const stop = await sendReport(api, mediaToken, report);
        if (stop !== undefined) {
          isStopped = true;
          video.pause();
          setStopMessage(stop.message);
        }
      } catch (cause) {
        if (cause instanceof ReportRefused) {
          setRefusal(cause.cause);
        }
        throw cause;
      }
    }, reportSeqCounter(mediaToken));
    const videoEvents = reportVideoEvents(video, reporter, resumePositionMs);
    const ticker = setInterval(() => {
      if (!video.paused && hasMedia(video)) {
        reporter.flush(positionMs(video));
      }
    }, reportEveryMs);
    const flushWhenHidden = (): void => {
      if (document.visibilityState === "hidden") {
        reporter.flush(videoEvents.lastPositionMs());
      }
    };
    document.addEventListener("visibilitychange", flushWhenHidden);
    return () => {
      clearInterval(ticker);
      document.removeEventListener("visibilitychange", flushWhenHidden);
      videoEvents.stop();
      // The video may be emptied already: where it last was is where the person left it.
      reporter.recordAbandoned(videoEvents.lastPositionMs());
    };
  }, [api, videoRef, mediaToken, resumePositionMs]);
  return { refusal, stopMessage };
}
