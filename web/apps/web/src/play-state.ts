// SPDX-License-Identifier: MPL-2.0

import {
  framesShownOf,
  linkSpreadOf,
  PlayReporter,
  playerFoundUnplayableEvent,
  ReportRefused,
  reportEveryMs,
  steadinessOf,
  takeLinkNotes,
  unplayableOf,
  watchStalls,
  watchStandingStill,
  type PlayReport,
} from "@orpheum/player";
import type { Api, components } from "@orpheum/sdk";
import { useEffect, useRef, useState, type RefObject } from "react";

import { leavePlayback, sayInterestOf, sendInterest } from "./interest.ts";
import { MovingDown, type LadderOffer } from "./moving-down.ts";
import { ApiProblem, throwIfRefused } from "./problem.ts";
import { isPlaybackGone, mayPass, type ReopenAt } from "./reopen.ts";
import { useServices, type PathMoves } from "./services.tsx";
import { hasMedia, positionMs, reportVideoEvents, watchPage } from "./play-state-video-events.ts";

/**
 * Where a report goes: the playback its media token names, by the session's own authorization, or
 * by the headers given, as a Watch Together viewer's membership (D247 §2).
 */
export interface ReportTo {
  readonly mediaToken: string;
  readonly headers?: Readonly<Record<string, string>>;
  /** The page is going (hidden, or pagehide): the report is sent to survive it. */
  readonly isLeaving?: boolean;
}

/** What a report is answered with: the admin's stop, a lower quality to move to, or neither. */
export interface ReportAnswer {
  readonly stop?: { readonly message: string };
  readonly lowerQuality?: LadderOffer;
}

/** Under keepalive's 64 KiB body limit, in characters (the body is text; a margin is kept). */
const keepaliveBodyMost = 48_000;

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
  const answer = await api.POST("/playback/{token}/report", {
    params: { path: { token: to.mediaToken } },
    ...(to.headers === undefined ? {} : { headers: to.headers }),
    // keepalive lets the browser finish a report as the page goes, within its 64 KiB of body.
    keepalive: to.isLeaving === true && JSON.stringify(report).length <= keepaliveBodyMost,
    body: bodyOf(report),
  });
  try {
    throwIfRefused(answer);
    return answerOf(answer.data);
  } catch (cause) {
    if (cause instanceof ApiProblem && !mayPass(cause)) {
      throw new ReportRefused(cause);
    }
    throw cause;
  }
}

/** A report as the server takes it: its lists copied, and the ones it may lack left out. */
function bodyOf(report: PlayReport): components["schemas"]["ReportInputBody"] {
  const { stalls, standstills, pathMoves, ...rest } = report;
  return {
    ...rest,
    events: [...report.events],
    spans: [...report.spans],
    ...(stalls === undefined ? {} : { stalls: [...stalls] }),
    ...(standstills === undefined || standstills === null ? {} : { standstills: [...standstills] }),
    ...(pathMoves === undefined ? {} : { pathMoves: [...pathMoves] }),
  };
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

/**
 * What reporting tells the page: the server's answers, its refusal for good, and that it lost the
 * playback, which the page may open again from where it was instead (onGone true), and not be told
 * the refusal.
 */
interface ReportListener {
  readonly onStopped: (message: string) => void;
  readonly onLowerQuality: (offer: LadderOffer) => void;
  readonly onRefused: (cause: unknown) => void;
  readonly onGone: (whereNow: () => ReopenAt) => boolean;
}

/** What startReporting hands back: a report sent now, as when the player failed, and the stopping. */
interface Reporting {
  readonly reportNow: () => void;
  readonly stop: () => void;
}

/**
 * Reports a playing video's play state for the playback mediaToken names, from where it starts,
 * until the returned stop: each change, its stalls at once with the link's speed as readLinkKbps
 * measures it (D247 §2), where it is every reportEveryMs while playing, and when the page is
 * hidden. The admin's stop pauses the video and ends the reports (D243 §9).
 */
/**
 * Tells onGivenUp where the video stood when its player gave the film's own picture up as one its
 * device cannot play (D266), for the page to ask for the playback again from there, at the twin
 * (D323 §2), paused only where the person paused it: a video yet to show its first frame reads as
 * paused, and opened again paused it never started (the owner's test, 2026-11-03: Play on
 * The Gold Rush's page opened the player and did not play). Returns the stopping.
 */
export function whenOwnPictureGivenUp(
  video: HTMLVideoElement,
  onGivenUp: (at: ReopenAt) => void,
): () => void {
  const pause = watchPersonsPause(video);
  const listener = (event: Event): void => {
    const unplayable = unplayableOf(event);
    if (unplayable?.isFilmsOwnPicture === true) {
      onGivenUp({ positionMs: unplayable.positionMs, isPaused: pause.isPaused() });
    }
  };
  video.addEventListener(playerFoundUnplayableEvent, listener);
  return () => {
    pause.stop();
    video.removeEventListener(playerFoundUnplayableEvent, listener);
  };
}

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
 * Sends where video is every reportEveryMs while it plays, and at once when the page is hidden, from
 * where it last held media; returns the stopping.
 */
function reportWhilePlaying(
  video: HTMLVideoElement,
  reporter: PlayReporter,
  lastPositionMs: () => number,
): () => void {
  const ticker = setInterval(() => {
    if (!video.paused && hasMedia(video)) {
      reporter.flush(positionMs(video));
    }
  }, reportEveryMs);
  const flushWhenHidden = (): void => {
    if (document.visibilityState === "hidden") {
      reporter.flush(lastPositionMs(), true);
    }
  };
  const flushOnPageHide = (): void => {
    reporter.flush(lastPositionMs(), true);
  };
  document.addEventListener("visibilitychange", flushWhenHidden);
  window.addEventListener("pagehide", flushOnPageHide);
  return () => {
    clearInterval(ticker);
    document.removeEventListener("visibilitychange", flushWhenHidden);
    window.removeEventListener("pagehide", flushOnPageHide);
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

/**
 * Has reporter send, with every report, what video's player says of itself: its steadiness level
 * (D352 Q2), its link's spread and its notes on its link (bandwidth-sampling-2026-10-29.md P2, P6)
 * and the frames it showed.
 */
function reportPlayerOf(video: HTMLVideoElement, reporter: PlayReporter): void {
  reporter.reportSteadinessWith(() => steadinessOf(video));
  reporter.reportLinkSpreadWith(() => linkSpreadOf(video));
  reporter.reportLinkNotesWith(() => takeLinkNotes(video));
  reporter.reportFramesWith(() => framesShownOf(video));
}

function startReporting(
  video: HTMLVideoElement,
  playback: PlayStateOf & { readonly api: Api },
  listener: ReportListener,
): Reporting {
  const { api, mediaToken } = playback;
  let isStopped = false;
  const movingDown = new MovingDown({
    readLinkKbps: playback.readLinkKbps,
    askAgain: () => {
      reporter.flush(positionMs(video));
    },
    onMove: listener.onLowerQuality,
  });
  const reporter = new PlayReporter(async (report, options) => {
    if (isStopped) {
      return;
    }
    try {
      const answer = await sendReport(api, { mediaToken, ...options }, report);
      if (answer.stop !== undefined) {
        isStopped = true;
        video.pause();
        listener.onStopped(answer.stop.message);
      }
      movingDown.answered(answer.lowerQuality);
    } catch (cause) {
      if (cause instanceof ReportRefused && !loss.isReopened(cause.cause)) {
        listener.onRefused(cause.cause);
      }
      throw cause;
    }
  }, reportSeqCounter(mediaToken));
  const loss = watchForLoss(video, listener, () => videoEvents.lastPositionMs());
  reporter.measureLinkWith(playback.readLinkKbps);
  reportPlayerOf(video, reporter);
  const page = watchPage(video, reporter, playback.pathMoves);
  const videoEvents = reportVideoEvents(video, reporter, { ...playback, contexts: page.contexts });
  const stopWatching = watchForReport(video, reporter, movingDown);
  const stopTelling = reportWhilePlaying(video, reporter, videoEvents.lastPositionMs);
  const stopSaying = sayInterestOf(video, (said) => {
    sendInterest(api, mediaToken, said);
  });
  const stop = (): void => {
    loss.stop();
    stopSaying();
    // The player's page is gone, to anywhere but another film: its runs finish what it keeps ahead
    // (D309 §7). Another film's start, or Watching's move to it, says it was replaced.
    leavePlayback(api, mediaToken, "left");
    stopTelling();
    movingDown.stop();
    stopWatching();
    page.stop();
    videoEvents.stop();
    // The video may be emptied already: where it last was is where the person left it.
    reporter.recordAbandoned(videoEvents.lastPositionMs());
  };
  const reportNow = (): void => {
    reporter.flush(videoEvents.lastPositionMs());
  };
  return { reportNow, stop };
}

/**
 * Hands a playback the server lost, as it restarted, to the page to open again where it was, and
 * paused or playing as the person left it (the owner's test, 2026-10-18: "Press play to carry on"
 * could not be done, the video still playing). isReopened is whether the page took it; a video that
 * fails or is emptied stops too, which no person asked for, so only a pause of one holding media is
 * theirs.
 */
function watchForLoss(
  video: HTMLVideoElement,
  listener: Pick<ReportListener, "onGone">,
  lastPositionMs: () => number,
): { readonly isReopened: (cause: unknown) => boolean; readonly stop: () => void } {
  const pause = watchPersonsPause(video);
  return {
    isReopened: (cause) =>
      isPlaybackGone(cause) &&
      listener.onGone(() => ({ positionMs: lastPositionMs(), isPaused: pause.isPaused() })),
    stop: pause.stop,
  };
}

/**
 * Whether the person paused video, as against its never having played or having failed, emptied or
 * ended, none of which a person asks for: a pause of one holding media, until it plays again.
 */
function watchPersonsPause(video: HTMLVideoElement): {
  readonly isPaused: () => boolean;
  readonly stop: () => void;
} {
  let isPaused = false;
  const played = (): void => {
    isPaused = false;
  };
  const paused = (): void => {
    isPaused ||= hasMedia(video) && !video.ended;
  };
  video.addEventListener("play", played);
  video.addEventListener("pause", paused);
  return {
    isPaused: () => isPaused,
    stop: () => {
      video.removeEventListener("play", played);
      video.removeEventListener("pause", paused);
    },
  };
}

/** The playback whose play state is kept: its token, where it starts, and how its link is read. */
interface PlayStateOf {
  readonly mediaToken: string;
  readonly resumePositionMs: number;
  /** The link's speed as the playback measures it, undefined until it can say (D247 §1). */
  readonly readLinkKbps: () => number | undefined;
  /** The page's moves from one path to the core to another, where it makes any. */
  readonly pathMoves?: PathMoves | undefined;
  /**
   * Told that the server lost the playback, and how to read where it is: true when the page opens
   * it again from there, false to be told the refusal. Undefined tells it.
   */
  readonly onGone?: ((whereNow: () => ReopenAt) => boolean) | undefined;
  /**
   * Told where the video stood when its player gave the film's own picture up, for the page to ask
   * for the playback again from there, at the twin (D323 §2); undefined keeps playing below it.
   */
  readonly onOwnPictureGivenUp?: ((at: ReopenAt) => void) | undefined;
  /**
   * What failed in the player, undefined while nothing has: a failure is reported on at once, so a
   * playback the server lost before the video played, which sends no report, is learned of.
   */
  readonly failure?: string | undefined;
}

/**
 * Keeps a playing video's play state on the server (play-state.md): it starts where the person
 * left the file, without asking (D188), and reports each change of state, its stalls (D247 §2),
 * and where it is every ten seconds while playing and when the page is hidden, until the playback
 * ends. It returns why the server refused the reports for good, once it has, so the person is told
 * that where they are is no longer kept; the admin's message when they stopped the playback, which
 * pauses the video and ends the reports (D243 §9); and the lower quality a stall started, once it
 * is ready and the player still cannot keep up (D253); each undefined until then. A playback the
 * server lost goes to onGone, where it was, instead of being a refusal, when the page opens it again.
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
  const { pathMoves } = useServices();
  const [lowerQuality, setLowerQuality] = useState<LadderOffer | undefined>(undefined);
  // The link is read at each report, by what the latest render handed in.
  const linkReader = useRef(playback.readLinkKbps);
  linkReader.current = playback.readLinkKbps;
  const goneHearer = useRef(playback.onGone);
  goneHearer.current = playback.onGone;
  const reportNow = useRef<(() => void) | undefined>(undefined);
  useEffect(() => {
    const video = videoRef.current;
    if (video === null) {
      return undefined;
    }
    const reporting = startReporting(
      video,
      { api, mediaToken, resumePositionMs, readLinkKbps: () => linkReader.current(), pathMoves },
      {
        onStopped: setStopMessage,
        onLowerQuality: setLowerQuality,
        onRefused: setRefusal,
        onGone: (whereNow) => goneHearer.current?.(whereNow) ?? false,
      },
    );
    reportNow.current = reporting.reportNow;
    return () => {
      reportNow.current = undefined;
      reporting.stop();
    };
  }, [api, videoRef, mediaToken, resumePositionMs, pathMoves]);
  const { failure } = playback;
  useEffect(() => {
    if (failure !== undefined) {
      reportNow.current?.();
    }
  }, [failure]);
  const ownPictureHearer = useRef(playback.onOwnPictureGivenUp);
  ownPictureHearer.current = playback.onOwnPictureGivenUp;
  useEffect(() => {
    const video = videoRef.current;
    return video === null
      ? undefined
      : whenOwnPictureGivenUp(video, (at) => {
          ownPictureHearer.current?.(at);
        });
  }, [videoRef, mediaToken]);
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
    const reporter = new PlayReporter(async (report, options) => {
      const answer = await sendReport(api, { ...to, ...options }, report);
      movingDown.answered(answer.lowerQuality);
    }, reportSeqCounter(mediaToken));
    reporter.measureLinkWith(() => linkReader.current());
    reportPlayerOf(video, reporter);
    const stopWatching = watchForReport(video, reporter, movingDown);
    return () => {
      movingDown.stop();
      stopWatching();
    };
  }, [api, videoRef, mediaToken, headers]);
  return lowerQuality;
}
