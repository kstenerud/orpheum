// SPDX-License-Identifier: MPL-2.0

import {
  PlayReporter,
  ReportRefused,
  reportEveryMs,
  seekTo,
  watchStalls,
  type PlayReport,
} from "@orpheum/player";
import type { Api, components } from "@orpheum/sdk";
import { useEffect, useRef, useState, type RefObject } from "react";

import { MovingDown, type LowerQuality } from "./moving-down.ts";
import { ApiProblem, throwIfRefused } from "./problem.ts";

/** The codes of refusals that pass: a report refused with one of these goes again with the next. */
const transientRefusalCodes: ReadonlySet<string> = new Set([
  "internal",
  "migrating",
  "rate_limited",
]);

/**
 * Where a report goes: the playback its media token names, by the session's own authorization, or
 * by the headers given, as a Watch Together viewer's membership (D247 §2).
 */
export interface ReportTo {
  readonly mediaToken: string;
  readonly headers?: Readonly<Record<string, string>>;
}

/** What a report is answered with: the admin's stop, a lower quality to move to, or neither. */
export interface ReportAnswer {
  readonly stop?: { readonly message: string };
  readonly lowerQuality?: LowerQuality;
}

/**
 * Sends one play report for the playback the media token names, and answers what the server says:
 * that the playback was stopped (D243 §9), or a lower quality to move to (D247 §2). A refusal that
 * sending again cannot change (the playback is gone, the session is not allowed it) rejects with
 * ReportRefused, so the reporter stops; a failure that may pass (no answer, a server busy or
 * updating) rejects as it came, so the report goes again with the next.
 */
export async function sendReport(
  api: Api,
  to: ReportTo,
  report: PlayReport,
): Promise<ReportAnswer> {
  const { stalls, ...rest } = report;
  const answer = await api.POST("/playback/{token}/report", {
    params: { path: { token: to.mediaToken } },
    ...(to.headers === undefined ? {} : { headers: to.headers }),
    body: {
      ...rest,
      events: [...report.events],
      spans: [...report.spans],
      ...(stalls === undefined ? {} : { stalls: [...stalls] }),
    },
  });
  try {
    throwIfRefused(answer);
    return answerOf(answer.data);
  } catch (cause) {
    if (cause instanceof ApiProblem && !transientRefusalCodes.has(cause.code)) {
      throw new ReportRefused(cause);
    }
    throw cause;
  }
}

/** A report's answer as the page reads it: what the server said, and nothing it did not. */
function answerOf(data: components["schemas"]["ReportOutputBody"] | undefined): ReportAnswer {
  const stop = data?.stop ?? undefined;
  const lowerQuality = data?.lowerQuality ?? undefined;
  return {
    ...(stop === undefined ? {} : { stop }),
    ...(lowerQuality === undefined ? {} : { lowerQuality }),
  };
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

/** What reporting tells the page: the server's answers, and its refusal for good. */
interface ReportListener {
  readonly onStopped: (message: string) => void;
  readonly onLowerQuality: (offer: LowerQuality) => void;
  readonly onRefused: (cause: unknown) => void;
}

/**
 * Reports a playing video's play state for the playback mediaToken names, from where it starts,
 * until the returned stop: each change, its stalls at once with the link's speed as readLinkKbps
 * measures it (D247 §2), where it is every reportEveryMs while playing, and when the page is
 * hidden. The admin's stop pauses the video and ends the reports (D243 §9).
 */
function startReporting(
  video: HTMLVideoElement,
  playback: PlayStateOf & { readonly api: Api },
  listener: ReportListener,
): () => void {
  const { api, mediaToken } = playback;
  let isStopped = false;
  const movingDown = new MovingDown({
    filmKbps: playback.filmKbps,
    readLinkKbps: playback.readLinkKbps,
    askAgain: () => {
      reporter.flush(positionMs(video));
    },
    onMove: listener.onLowerQuality,
  });
  const reporter = new PlayReporter(async (report) => {
    if (isStopped) {
      return;
    }
    try {
      const answer = await sendReport(api, { mediaToken }, report);
      if (answer.stop !== undefined) {
        isStopped = true;
        video.pause();
        listener.onStopped(answer.stop.message);
      }
      movingDown.answered(answer.lowerQuality);
    } catch (cause) {
      if (cause instanceof ReportRefused) {
        listener.onRefused(cause.cause);
      }
      throw cause;
    }
  }, reportSeqCounter(mediaToken));
  reporter.measureLinkWith(playback.readLinkKbps);
  const videoEvents = reportVideoEvents(video, reporter, playback.resumePositionMs);
  const stopWatchingStalls = watchStalls(video, (stall) => {
    movingDown.stalled();
    reporter.recordStall(stall.positionMs);
  });
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
    movingDown.stop();
    stopWatchingStalls();
    document.removeEventListener("visibilitychange", flushWhenHidden);
    videoEvents.stop();
    // The video may be emptied already: where it last was is where the person left it.
    reporter.recordAbandoned(videoEvents.lastPositionMs());
  };
}

/**
 * The playback whose play state is kept: its token, where it starts, the film's own rate, and how
 * its link is read.
 */
interface PlayStateOf {
  readonly mediaToken: string;
  readonly resumePositionMs: number;
  /** The film's rate as it plays, against which a stalled player weighs its link (D253 §2). */
  readonly filmKbps: number;
  /** The link's speed as the playback measures it, undefined until it can say (D247 §1). */
  readonly readLinkKbps: () => number | undefined;
}

/**
 * Keeps a playing video's play state on the server (play-state.md): it starts where the person
 * left the file, without asking (D188), and reports each change of state, its stalls (D247 §2),
 * and where it is every ten seconds while playing and when the page is hidden, until the playback
 * ends. It returns why the server refused the reports for good, once it has, so the person is told
 * that where they are is no longer kept; the admin's message when they stopped the playback, which
 * pauses the video and ends the reports (D243 §9); and the lower quality a stall started, once it
 * is ready and the player still cannot keep up (D253); each undefined until then.
 */
export function usePlayState(
  api: Api,
  videoRef: RefObject<HTMLVideoElement | null>,
  playback: PlayStateOf,
): {
  readonly refusal: unknown;
  readonly stopMessage: string | undefined;
  readonly lowerQuality: LowerQuality | undefined;
} {
  const { mediaToken, resumePositionMs, filmKbps } = playback;
  const [refusal, setRefusal] = useState<unknown>(undefined);
  const [stopMessage, setStopMessage] = useState<string | undefined>(undefined);
  const [lowerQuality, setLowerQuality] = useState<LowerQuality | undefined>(undefined);
  // The link is read at each report, by what the latest render handed in.
  const linkReader = useRef(playback.readLinkKbps);
  linkReader.current = playback.readLinkKbps;
  useEffect(() => {
    const video = videoRef.current;
    if (video === null) {
      return undefined;
    }
    return startReporting(
      video,
      { api, mediaToken, resumePositionMs, filmKbps, readLinkKbps: () => linkReader.current() },
      { onStopped: setStopMessage, onLowerQuality: setLowerQuality, onRefused: setRefusal },
    );
  }, [api, videoRef, mediaToken, resumePositionMs, filmKbps]);
  return { refusal, stopMessage, lowerQuality };
}

/**
 * Reports a viewer's stalls, and the link's speed, on the playback its membership started, by the
 * headers that carry it (D247 §2): nothing of what a viewer watches is kept (D243 §2), so nothing
 * else is said, and the video is not moved, since it follows the group. It returns the lower quality
 * a stall started, once it is ready and the player still cannot keep up (D253), undefined until
 * then.
 */
export function useStallReports(
  api: Api,
  videoRef: RefObject<HTMLVideoElement | null>,
  playback: ReportTo & {
    readonly filmKbps: number;
    readonly readLinkKbps: () => number | undefined;
  },
): LowerQuality | undefined {
  const { mediaToken, headers, filmKbps } = playback;
  const [lowerQuality, setLowerQuality] = useState<LowerQuality | undefined>(undefined);
  const linkReader = useRef(playback.readLinkKbps);
  linkReader.current = playback.readLinkKbps;
  useEffect(() => {
    const video = videoRef.current;
    if (video === null) {
      return undefined;
    }
    const to: ReportTo = headers === undefined ? { mediaToken } : { mediaToken, headers };
    const movingDown = new MovingDown({
      filmKbps,
      readLinkKbps: () => linkReader.current(),
      askAgain: () => {
        reporter.flush(positionMs(video));
      },
      onMove: setLowerQuality,
    });
    const reporter = new PlayReporter(async (report) => {
      const answer = await sendReport(api, to, report);
      movingDown.answered(answer.lowerQuality);
    }, reportSeqCounter(mediaToken));
    reporter.measureLinkWith(() => linkReader.current());
    const stopWatching = watchStalls(video, (stall) => {
      movingDown.stalled();
      reporter.recordStall(stall.positionMs);
    });
    return () => {
      movingDown.stop();
      stopWatching();
    };
  }, [api, videoRef, mediaToken, headers, filmKbps]);
  return lowerQuality;
}
