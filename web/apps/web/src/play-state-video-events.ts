// SPDX-License-Identifier: MPL-2.0

import {
  eventContextOf,
  seekTo,
  skippedByOf,
  watchMediaSession,
  type MediaSessionAction,
  type PlayEventContext,
  type PlayReporter,
  type SkippedBy,
} from "@orpheum/player";
import { playerMovedItselfEvent, startGateOf, type components } from "@orpheum/sdk";

import { decisionTimesOf, forgetDecisionTimes } from "./decision-times.ts";
import { seekAskedEvent, soughtByOf } from "./seek-ahead.ts";
import { startGateFieldsOf, timeSeeks } from "./seek-timing.ts";
import { soundOf } from "./startup-sound.ts";
import type { PathMoves } from "./services.tsx";
import { monotonicMs } from "./use-together.ts";

/** The video's position, in milliseconds. */
export function positionMs(video: HTMLVideoElement): number {
  return video.currentTime * 1000;
}

/**
 * Whether the video holds media. A video being torn down is emptied first, and then says it is
 * paused at 0:00, which is nothing the person watched.
 */
export function hasMedia(video: HTMLVideoElement): boolean {
  return video.readyState !== HTMLMediaElement.HAVE_NOTHING;
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
  const times = decisionTimesOf(mediaToken);
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
      forgetDecisionTimes(mediaToken);
      const playingAtMs = performance.now();
      // Typed, as a spread's fields are not checked against the body they are spread into.
      const preload: Pick<components["schemas"]["StartupBody"], "preloadSteps"> =
        times.preloadSteps === undefined || times.preloadSteps === ""
          ? {}
          : { preloadSteps: times.preloadSteps };
      reporter.recordStartup({
        decidedAfterMs: sinceAsked(times.decidedMs),
        firstDataAfterMs: sinceAsked(firstDataAtMs ?? playingAtMs),
        playingAfterMs: sinceAsked(playingAtMs),
        ...preload,
        sound: soundOf(video),
        ...startGateFieldsOf(video),
      });
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

/** What a start played on from: the video's play, a wait for data, or a seek. */
type StartedAfter = NonNullable<PlayEventContext["startedAfter"]>;

/** What moved a video, as the seek it starts says (seekMovers). */
interface SeekMovers {
  readonly skippedBy?: SkippedBy | undefined;
  readonly soughtBy?: string | undefined;
}

/**
 * What the next seek of video was moved by, heard before its seeking: what hls.js said it moved the
 * video over, and what on the page asked it (seek-ahead.ts), a step back of a cast's receiver no one
 * meant having said nothing of what asked it (staging, 2026-10-29, file 5680). take() gives them
 * and clears them, for the seek that heard them alone.
 */
function seekMovers(video: HTMLVideoElement): {
  readonly take: () => SeekMovers;
  readonly stop: () => void;
} {
  let movers: SeekMovers = {};
  const onMovedItself = (event: Event): void => {
    movers = { ...movers, skippedBy: skippedByOf(event) };
  };
  const onSeekAsked = (event: Event): void => {
    movers = { ...movers, soughtBy: soughtByOf(event) };
  };
  video.addEventListener(playerMovedItselfEvent, onMovedItself);
  video.addEventListener(seekAskedEvent, onSeekAsked);
  return {
    take: () => {
      const taken = movers;
      movers = {};
      return taken;
    },
    stop: () => {
      video.removeEventListener(playerMovedItselfEvent, onMovedItself);
      video.removeEventListener(seekAskedEvent, onSeekAsked);
    },
  };
}

/**
 * What a pause or a seek of video says of where it came from (event-context.ts): the page, the last
 * media-session action, and what moved it, which the seek it starts takes and clears (seekMovers);
 * and what a start played on from, its play, a wait for data or a seek, and how long before. A Fire
 * TV said it started twice 0.6 s apart with 9 s loaded, and nothing said what came between (staging,
 * 2026-10-28). A wait a seek brings is the seek's.
 */
function eventContexts(
  video: HTMLVideoElement,
  lastSessionAction: () => MediaSessionAction | undefined,
): EventContexts & { readonly stop: () => void } {
  const movers = seekMovers(video);
  let playedOnFrom: { readonly startedAfter: StartedAfter; readonly atMs: number } | undefined;
  const heard = (startedAfter: StartedAfter) => (): void => {
    if (startedAfter !== "waiting" || playedOnFrom?.startedAfter !== "seeking") {
      playedOnFrom = { startedAfter, atMs: performance.now() };
    }
  };
  const before: Record<string, () => void> = {
    play: heard("play"),
    waiting: heard("waiting"),
    seeking: heard("seeking"),
  };
  for (const [event, listener] of Object.entries(before)) {
    video.addEventListener(event, listener);
  }
  const now = (known: SeekMovers): PlayEventContext =>
    eventContextOf(video, document, {
      isUserActive: "userActivation" in navigator ? navigator.userActivation.isActive : undefined,
      mediaSessionAction: lastSessionAction(),
      ...known,
    });
  return {
    paused: () => now({}),
    seeked: () => now(movers.take()),
    started: () => {
      const from = playedOnFrom;
      playedOnFrom = undefined;
      return {
        ...now({}),
        ...(from === undefined
          ? {}
          : {
              startedAfter: from.startedAfter,
              stoodMs: Math.max(0, Math.round(performance.now() - from.atMs)),
            }),
      };
    },
    stop: () => {
      movers.stop();
      for (const [event, listener] of Object.entries(before)) {
        video.removeEventListener(event, listener);
      }
    },
  };
}

/**
 * Watches the page for what the reports carry beside the video's events: the media session's
 * actions, which a pause they made says, and each path the page moves to (the owner, 2026-10-17).
 */
export function watchPage(
  video: HTMLVideoElement,
  reporter: PlayReporter,
  pathMoves: PathMoves | undefined,
): { readonly contexts: EventContexts; readonly stop: () => void } {
  const session = watchMediaSession(
    "mediaSession" in navigator ? navigator.mediaSession : undefined,
    video,
    monotonicMs,
  );
  const contexts = eventContexts(video, session.lastAction);
  const stopHearingMoves = pathMoves?.subscribe(() => {
    reporter.recordPathMove(pathMoves.to() ?? "another path");
  });
  return {
    contexts,
    stop: () => {
      stopHearingMoves?.();
      contexts.stop();
      session.stop();
    },
  };
}

/** The playback whose video's events are told: its token, where it starts, and its pauses' and seeks' contexts. */
export interface VideoEventsOf {
  readonly mediaToken: string;
  readonly resumePositionMs: number;
  readonly contexts: EventContexts;
}

/** What a pause and a seek say of where they came from, and a start of what it played on from (eventContexts). */
export interface EventContexts {
  readonly paused: () => PlayEventContext;
  readonly seeked: () => PlayEventContext;
  readonly started: () => PlayEventContext;
}

/** What reportVideoEvents hands back: where the video last was while it held media, and how to stop. */
export interface VideoEventReporting {
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
export function reportVideoEvents(
  video: HTMLVideoElement,
  reporter: PlayReporter,
  playback: VideoEventsOf,
): VideoEventReporting {
  const { resumePositionMs, contexts } = playback;
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
      // The start gate's own pause, holding a seek until enough is loaded, is no one's pause.
      if (!video.ended && !startGateOf(video).isOwnPause()) {
        reporter.recordPaused(positionMs(video), contexts.paused());
      }
    }),
    seeking: whileItHoldsMedia(() => {
      const context = contexts.seeked();
      // The seek that puts the player where the person left it is not theirs to log.
      if (isResuming) {
        isResuming = false;
      } else {
        reporter.recordSeeked(lastHeldPositionMs, positionMs(video), context);
        seekTimer.seeking(lastHeldPositionMs);
      }
    }),
    playing: whileItHoldsMedia(() => {
      reporter.recordStarted(positionMs(video), contexts.started());
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
