// SPDX-License-Identifier: MPL-2.0

import { PlayReporter, reportEveryMs, seekTo, type PlayReport } from "@orpheum/player";
import type { Api } from "@orpheum/sdk";
import { useEffect, type RefObject } from "react";

import { throwIfRefused } from "./problem.ts";

/** Sends one play report for the playback the media token names. */
export async function sendReport(api: Api, mediaToken: string, report: PlayReport): Promise<void> {
  throwIfRefused(
    await api.POST("/playback/{token}/report", {
      params: { path: { token: mediaToken } },
      body: { ...report, events: [...report.events], spans: [...report.spans] },
    }),
  );
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
function seqCounter(mediaToken: string): () => number {
  return () => {
    const next = (lastSeqByToken.get(mediaToken) ?? 0) + 1;
    lastSeqByToken.set(mediaToken, next);
    return next;
  };
}

/** What reportVideoEvents hands back: where the video last was while it held media, and how to stop. */
interface Watching {
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
): Watching {
  let isResuming = resumePositionMs > 0;
  let lastMs = resumePositionMs;
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
        reporter.recordSeeked(lastMs, positionMs(video));
      }
    }),
    timeupdate: whileItHoldsMedia(() => {
      lastMs = positionMs(video);
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
    lastPositionMs: () => lastMs,
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
 * ten seconds while playing and when the page is hidden, until the playback ends.
 */
export function usePlayState(
  api: Api,
  videoRef: RefObject<HTMLVideoElement | null>,
  playback: { readonly mediaToken: string; readonly resumePositionMs: number },
): void {
  const { mediaToken, resumePositionMs } = playback;
  useEffect(() => {
    const video = videoRef.current;
    if (video === null) {
      return undefined;
    }
    const reporter = new PlayReporter(
      (report) => sendReport(api, mediaToken, report),
      seqCounter(mediaToken),
    );
    const watching = reportVideoEvents(video, reporter, resumePositionMs);
    const ticker = setInterval(() => {
      if (!video.paused && hasMedia(video)) {
        reporter.flush(positionMs(video));
      }
    }, reportEveryMs);
    const hidden = (): void => {
      if (document.visibilityState === "hidden") {
        reporter.flush(watching.lastPositionMs());
      }
    };
    document.addEventListener("visibilitychange", hidden);
    return () => {
      clearInterval(ticker);
      document.removeEventListener("visibilitychange", hidden);
      watching.stop();
      // The video may be emptied already: where it last was is where the person left it.
      reporter.recordAbandoned(watching.lastPositionMs());
    };
  }, [api, videoRef, mediaToken, resumePositionMs]);
}
