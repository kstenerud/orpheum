// SPDX-License-Identifier: MPL-2.0

import { PlayReporter, reportEveryMs, seekTo, type PlayReport } from "@orpheum/player";
import type { Api } from "@orpheum/sdk";
import { useEffect, type RefObject } from "react";

/** Sends one play report for the playback the media token names. */
export async function sendReport(api: Api, mediaToken: string, report: PlayReport): Promise<void> {
  const { error } = await api.POST("/playback/{token}/report", {
    params: { path: { token: mediaToken } },
    body: { ...report, events: [...report.events], spans: [...report.spans] },
  });
  if (error !== undefined) {
    throw new Error(error.error.message);
  }
}

/** The video's position, in milliseconds. */
function positionMs(video: HTMLVideoElement): number {
  return video.currentTime * 1000;
}

/**
 * Tells the reporter what the video does, and starts it at resumePositionMs. Returns what
 * stops listening.
 */
function reportVideoEvents(
  video: HTMLVideoElement,
  reporter: PlayReporter,
  resumePositionMs: number,
): () => void {
  let lastMs = resumePositionMs;
  let isResuming = resumePositionMs > 0;
  const listeners: Record<string, () => void> = {
    loadedmetadata: () => {
      if (isResuming) {
        seekTo(video, resumePositionMs);
      }
    },
    playing: () => {
      reporter.started(positionMs(video));
    },
    pause: () => {
      if (!video.ended) {
        reporter.paused(positionMs(video));
      }
    },
    seeking: () => {
      // The seek that puts the player where the person left it is not theirs to log.
      if (isResuming) {
        isResuming = false;
      } else {
        reporter.seeked(lastMs, positionMs(video));
      }
    },
    timeupdate: () => {
      lastMs = positionMs(video);
    },
    ended: () => {
      reporter.completed(positionMs(video));
    },
  };
  for (const [event, listener] of Object.entries(listeners)) {
    video.addEventListener(event, listener);
  }
  if (isResuming && video.readyState >= HTMLMediaElement.HAVE_METADATA) {
    seekTo(video, resumePositionMs);
  }
  return () => {
    for (const [event, listener] of Object.entries(listeners)) {
      video.removeEventListener(event, listener);
    }
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
    const reporter = new PlayReporter((report) => sendReport(api, mediaToken, report));
    const stopListening = reportVideoEvents(video, reporter, resumePositionMs);
    const ticker = setInterval(() => {
      if (!video.paused) {
        reporter.flush(positionMs(video));
      }
    }, reportEveryMs);
    const hidden = (): void => {
      if (document.visibilityState === "hidden") {
        reporter.flush(positionMs(video));
      }
    };
    document.addEventListener("visibilitychange", hidden);
    return () => {
      clearInterval(ticker);
      document.removeEventListener("visibilitychange", hidden);
      stopListening();
      reporter.abandoned(positionMs(video));
    };
  }, [api, videoRef, mediaToken, resumePositionMs]);
}
