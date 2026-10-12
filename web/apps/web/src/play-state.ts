// SPDX-License-Identifier: MPL-2.0

import {
  PlayReporter,
  playerFoundUnplayableEvent,
  ReportRefused,
  reportEveryMs,
  seekTo,
  unplayableOf,
  watchStalls,
  watchStandingStill,
  type PlayReport,
} from "@orpheum/player";
import { chunkLoadedOf, playerLoadedChunkEvent, type Api, type components } from "@orpheum/sdk";
import { useEffect, useRef, useState, type RefObject } from "react";

import { MovingDown, type LadderOffer } from "./moving-down.ts";
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
  readonly lowerQuality?: LadderOffer;
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
  const { stalls, standstills, ...rest } = report;
  const answer = await api.POST("/playback/{token}/report", {
    params: { path: { token: to.mediaToken } },
    ...(to.headers === undefined ? {} : { headers: to.headers }),
    body: {
      ...rest,
      events: [...report.events],
      spans: [...report.spans],
      ...(stalls === undefined ? {} : { stalls: [...stalls] }),
      ...(standstills === undefined || standstills === null
        ? {}
        : { standstills: [...standstills] }),
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

/** When a playback's player asked how its file plays, and was answered, on the page's clock. */
interface DecisionTimes {
  readonly askedMs: number;
  readonly decidedMs: number;
}

/** How many playbacks' decision times a tab keeps, the newest: more than a person starts at once. */
export const decisionTimesKept = 8;

/**
 * Each playback's decision times, by media token, until its reporting takes them, the newest
 * decisionTimesKept.
 */
const decisionTimesByToken = new Map<string, DecisionTimes>();

/**
 * Keeps when a playback's player asked how its file plays and was answered, askedMs and decidedMs
 * from performance.now(), so its reports can say how long it took to start playing.
 */
export function noteDecisionTimes(mediaToken: string, askedMs: number, decidedMs: number): void {
  decisionTimesByToken.set(mediaToken, { askedMs, decidedMs });
  // A film page's warm-up that is never played is never told: the oldest are let go of, so the
  // tab keeps a few at most (the M4 closing audit, 2026-10-08).
  for (const oldest of decisionTimesByToken.keys()) {
    if (decisionTimesByToken.size <= decisionTimesKept) {
      break;
    }
    decisionTimesByToken.delete(oldest);
  }
}

/** When the playback mediaToken names was asked for and decided, until its start is told. */
export function decisionTimesOf(mediaToken: string): DecisionTimes | undefined {
  return decisionTimesByToken.get(mediaToken);
}

/**
 * Tells the reporter, at the video's first playing, how long the playback mediaToken names took to
 * start: from asking how the file plays to the answer, to the first frame's data, and to playing.
 * Its decision times are forgotten once told, so a player made again for it, as a move to a lower
 * quality makes one, tells nothing more; one whose times are not known tells nothing. It returns the
 * listeners to add to the video.
 */
function startupListeners(
  video: HTMLVideoElement,
  reporter: PlayReporter,
  mediaToken: string,
): Record<string, () => void> {
  const times = decisionTimesByToken.get(mediaToken);
  if (times === undefined) {
    return {};
  }
  let firstDataAtMs: number | undefined;
  let isTold = false;
  const sinceAsked = (ms: number): number => Math.max(0, Math.round(ms - times.askedMs));
  return {
    loadeddata: () => {
      firstDataAtMs ??= performance.now();
    },
    playing: () => {
      if (isTold || !hasMedia(video)) {
        return;
      }
      isTold = true;
      decisionTimesByToken.delete(mediaToken);
      const playingAtMs = performance.now();
      reporter.recordStartup({
        decidedAfterMs: sinceAsked(times.decidedMs),
        firstDataAfterMs: sinceAsked(firstDataAtMs ?? playingAtMs),
        playingAfterMs: sinceAsked(playingAtMs),
      });
    },
  };
}

/**
 * Times each of the person's seeks, from its last seeking (a drag seeks again and again) to playing,
 * or, for a paused video, to being able to play, and tells the reporter (the owner, 2026-10-12: a
 * jump that felt slow could not be measured): with the report the started event sends, or at once
 * when paused, with when its first chunks of picture and sound arrived, to say which it waited on.
 * Its listeners go on the video before the started event's.
 */
function timeSeeks(
  video: HTMLVideoElement,
  reporter: Pick<PlayReporter, "recordSeekReady" | "flush">,
): {
  readonly seeking: (fromMs: number) => void;
  readonly listeners: Record<string, (event: Event) => void>;
} {
  let pending:
    | { readonly fromMs: number; atMs: number; pictureAfterMs?: number; soundAfterMs?: number }
    | undefined;
  const ready = (): boolean => {
    if (pending === undefined || video.seeking) {
      return false;
    }
    const toMs = positionMs(video);
    reporter.recordSeekReady({
      fromPositionMs: Math.max(0, Math.round(pending.fromMs)),
      toPositionMs: Math.max(0, Math.round(toMs)),
      readyAfterMs: Math.max(0, Math.round(performance.now() - pending.atMs)),
      ...(video.paused ? { isPaused: true } : {}),
      ...(pending.pictureAfterMs === undefined ? {} : { pictureAfterMs: pending.pictureAfterMs }),
      ...(pending.soundAfterMs === undefined ? {} : { soundAfterMs: pending.soundAfterMs }),
    });
    pending = undefined;
    return true;
  };
  return {
    listeners: {
      playing: () => {
        ready();
      },
      canplay: () => {
        if (video.paused && ready()) {
          reporter.flush(positionMs(video));
        }
      },
      [playerLoadedChunkEvent]: (event: Event) => {
        const loaded = chunkLoadedOf(event);
        if (pending === undefined || loaded === undefined) {
          return;
        }
        const afterMs = Math.max(0, Math.round(performance.now() - pending.atMs));
        if (loaded.track === "video") {
          pending.pictureAfterMs ??= afterMs;
        } else {
          pending.soundAfterMs ??= afterMs;
        }
      },
    },
    seeking: (fromMs) => {
      const atMs = performance.now();
      if (pending === undefined) {
        pending = { fromMs, atMs };
      } else {
        pending.atMs = atMs;
      }
    },
  };
}

/** Wraps a video's listener to hear only events from a video that holds media. */
function whileHoldingMedia(video: HTMLVideoElement): (listener: () => void) => () => void {
  return (listener) => (): void => {
    if (hasMedia(video)) {
      listener();
    }
  };
}

/** What reportVideoEvents hands back: where the video last was while it held media, and how to stop. */
interface VideoEventReporting {
  readonly lastPositionMs: () => number;
  readonly stop: () => void;
}

/**
 * Tells the reporter what the video does, and how long it took to start (startupListeners), and
 * starts it at the playback's resumePositionMs. Events from a video with no media are ignored, and
 * where it was when it last held some is kept: an emptied video is at 0:00, which the person did not
 * watch. A position told while seeking is not kept: Firefox tells of a seek's new position before
 * its seeking event, and every seek was logged as from where it went.
 */
function reportVideoEvents(
  video: HTMLVideoElement,
  reporter: PlayReporter,
  playback: { readonly mediaToken: string; readonly resumePositionMs: number },
): VideoEventReporting {
  const { resumePositionMs } = playback;
  let isResuming = resumePositionMs > 0;
  let lastHeldPositionMs = resumePositionMs;
  const seekTimer = timeSeeks(video, reporter);
  const whileItHoldsMedia = whileHoldingMedia(video);
  const listeners: Record<string, () => void> = {
    loadedmetadata: () => {
      if (isResuming) {
        seekTo(video, resumePositionMs);
      }
    },
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
        seekTimer.seeking(lastHeldPositionMs);
      }
    }),
    playing: whileItHoldsMedia(() => {
      reporter.recordStarted(positionMs(video));
    }),
    timeupdate: whileItHoldsMedia(() => {
      lastHeldPositionMs = video.seeking ? lastHeldPositionMs : positionMs(video);
    }),
    ended: whileItHoldsMedia(() => {
      reporter.recordCompleted(positionMs(video));
    }),
  };
  // Start-up and seek times are recorded first, for the report the started event sends.
  const added = [
    ...Object.entries(startupListeners(video, reporter, playback.mediaToken)),
    ...Object.entries(seekTimer.listeners),
    ...Object.entries(listeners),
  ];
  for (const [event, listener] of added) {
    video.addEventListener(event, listener);
  }
  if (isResuming && video.readyState >= HTMLMediaElement.HAVE_METADATA) {
    seekTo(video, resumePositionMs);
  }
  return {
    lastPositionMs: () => lastHeldPositionMs,
    stop: () => {
      for (const [event, listener] of added) {
        video.removeEventListener(event, listener);
      }
    },
  };
}

/** What reporting tells the page: the server's answers, and its refusal for good. */
interface ReportListener {
  readonly onStopped: (message: string) => void;
  readonly onLowerQuality: (offer: LadderOffer) => void;
  readonly onRefused: (cause: unknown) => void;
}

/**
 * Reports a playing video's play state for the playback mediaToken names, from where it starts,
 * until the returned stop: each change, its stalls at once with the link's speed as readLinkKbps
 * measures it (D247 §2), where it is every reportEveryMs while playing, and when the page is
 * hidden. The admin's stop pauses the video and ends the reports (D243 §9).
 */
/**
 * Reports each quality the video's player gives up as one its device cannot play, at once (D266).
 * Returns the stopping.
 */
export function tellUnplayable(
  video: EventTarget,
  reporter: Pick<PlayReporter, "recordUnplayable">,
): () => void {
  const listener = (event: Event): void => {
    const unplayable = unplayableOf(event);
    if (unplayable !== undefined) {
      reporter.recordUnplayable(unplayable, unplayable.positionMs);
    }
  };
  video.addEventListener(playerFoundUnplayableEvent, listener);
  return () => {
    video.removeEventListener(playerFoundUnplayableEvent, listener);
  };
}

/**
 * Reports what the video tells about its playing, at once: each stall, which may move it to a lower
 * quality (D247 §2), each quality its player gives up as unplayable here (D266), and each place it
 * keeps its position while asked to play, with what its video shows there. Returns the stopping.
 */
function watchForReport(
  video: HTMLVideoElement,
  reporter: PlayReporter,
  movingDown: Pick<MovingDown, "stalled">,
): () => void {
  const stopWatchingStalls = watchStalls(video, (stall) => {
    movingDown.stalled();
    reporter.recordStall(stall.positionMs, video);
  });
  const stopTellingUnplayable = tellUnplayable(video, reporter);
  const stopWatchingStill = watchStandingStill(video, (still) => {
    reporter.recordStandstill(still);
  });
  return () => {
    stopWatchingStill();
    stopWatchingStalls();
    stopTellingUnplayable();
  };
}

function startReporting(
  video: HTMLVideoElement,
  playback: PlayStateOf & { readonly api: Api },
  listener: ReportListener,
): () => void {
  const { api, mediaToken } = playback;
  let isStopped = false;
  const movingDown = new MovingDown({
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
  const videoEvents = reportVideoEvents(video, reporter, playback);
  const stopWatching = watchForReport(video, reporter, movingDown);
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
    stopWatching();
    document.removeEventListener("visibilitychange", flushWhenHidden);
    videoEvents.stop();
    // The video may be emptied already: where it last was is where the person left it.
    reporter.recordAbandoned(videoEvents.lastPositionMs());
  };
}

/** The playback whose play state is kept: its token, where it starts, and how its link is read. */
interface PlayStateOf {
  readonly mediaToken: string;
  readonly resumePositionMs: number;
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
  readonly lowerQuality: LadderOffer | undefined;
} {
  const { mediaToken, resumePositionMs } = playback;
  const [refusal, setRefusal] = useState<unknown>(undefined);
  const [stopMessage, setStopMessage] = useState<string | undefined>(undefined);
  const [lowerQuality, setLowerQuality] = useState<LadderOffer | undefined>(undefined);
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
      { api, mediaToken, resumePositionMs, readLinkKbps: () => linkReader.current() },
      { onStopped: setStopMessage, onLowerQuality: setLowerQuality, onRefused: setRefusal },
    );
  }, [api, videoRef, mediaToken, resumePositionMs]);
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
  playback: ReportTo & { readonly readLinkKbps: () => number | undefined },
): LadderOffer | undefined {
  const { mediaToken, headers } = playback;
  const [lowerQuality, setLowerQuality] = useState<LadderOffer | undefined>(undefined);
  const linkReader = useRef(playback.readLinkKbps);
  linkReader.current = playback.readLinkKbps;
  useEffect(() => {
    const video = videoRef.current;
    if (video === null) {
      return undefined;
    }
    const to: ReportTo = headers === undefined ? { mediaToken } : { mediaToken, headers };
    const movingDown = new MovingDown({
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
    const stopWatching = watchForReport(video, reporter, movingDown);
    return () => {
      movingDown.stop();
      stopWatching();
    };
  }, [api, videoRef, mediaToken, headers]);
  return lowerQuality;
}
