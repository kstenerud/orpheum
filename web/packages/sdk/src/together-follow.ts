// SPDX-License-Identifier: MPL-2.0

import type { GroupView } from "./together.ts";
import type { GroupCommand, PlayerState, TogetherConnection } from "./together-connection.ts";

/**
 * Drift up to this is left alone: below what a viewer notices between two screens, and inside the
 * noise of a monotonic sample taken a round trip late. watch-together.md's Sync: "nudge
 * playbackRate for ~1 s to absorb roughly 60–3000 ms of drift; seek beyond that". To be measured
 * in the device lab before it is relied on.
 */
export const leaveDriftUpToMs = 60;

/** Drift beyond this is sought across rather than nudged away (watch-together.md, Sync). */
export const seekDriftBeyondMs = 3000;

/** How long one nudge of the rate lasts (watch-together.md, Sync: "for ~1 s"). */
export const nudgeForMs = 1000;

/**
 * The slowest and fastest a nudge plays, as a factor of the group's rate: within these a film's
 * sound is still the film's. A drift too large to absorb in one nudge at these is absorbed by the
 * next ones. Our choice, to be measured in the device lab with the thresholds above.
 */
export const slowestNudgeFactor = 0.5;
export const fastestNudgeFactor = 2;

/**
 * How long a leading player's pause or seek waits to be sent: a browser's own controls pause the
 * video while its person drags the timeline, and seek at each step of the drag, each within this of
 * the last. A pause with a seek after it is that seek, not a pause for everyone, and a drag is one
 * seek, to where it ended (the owner's tests, 2026-10-05).
 */
export const scrubPauseWindowMs = 300;

/**
 * After a stall or a seek, a player is ready again only with this much of the film loaded ahead of
 * it. Ready at its first frame, a player whose link is slower than the film plays that frame and
 * stalls again, and a group that waits for it plays and freezes in turn without end (the owner's
 * first test, 2026-10-05). Alone, the browser waits for more itself; in a group, this is that wait.
 */
export const refillAheadMs = 10_000;

/**
 * The furthest a catch-up seek aims ahead of the group: by the time a seek has loaded the group has
 * moved on, so it aims ahead by how long the last one took to load, up to this (D247 §4,
 * Silo-Server's number).
 */
export const aimAheadAtMostMs = 10_000;

/**
 * How often a following player checks its drift while the group plays, and reports its state while
 * the group waits.
 */
export const checkDriftEveryMs = 1000;

/**
 * Where a group is at the monotonic time nowMs, having been told it at receivedAtMs: a playing
 * group has moved on at its rate since (D66: no clock is shared, only time since arrival counts).
 */
export function expectedPositionMs(group: GroupView, receivedAtMs: number, nowMs: number): number {
  return group.state === "Playing"
    ? group.positionMs + (nowMs - receivedAtMs) * group.rate
    : group.positionMs;
}

/** What to do about the distance between where a player is and where the group is. */
export type DriftCorrection =
  | { readonly kind: "none" }
  | { readonly kind: "nudge"; readonly rate: number }
  | { readonly kind: "seek"; readonly toMs: number };

/**
 * The correction for a player at actualMs while the group is at expectedMs playing at groupRate:
 * none within leaveDriftUpToMs, a seek beyond seekDriftBeyondMs, else a rate that closes the gap
 * over nudgeForMs, kept within the nudge factors.
 */
export function driftCorrection(drift: {
  readonly expectedMs: number;
  readonly actualMs: number;
  readonly groupRate: number;
}): DriftCorrection {
  const behindMs = drift.expectedMs - drift.actualMs;
  if (Math.abs(behindMs) <= leaveDriftUpToMs) {
    return { kind: "none" };
  }
  if (Math.abs(behindMs) > seekDriftBeyondMs) {
    return { kind: "seek", toMs: drift.expectedMs };
  }
  const factor = Math.min(
    fastestNudgeFactor,
    Math.max(slowestNudgeFactor, 1 + behindMs / nudgeForMs),
  );
  return { kind: "nudge", rate: drift.groupRate * factor };
}

/** The part of a video or audio element that following a group uses. */
export type FollowedMedia = Pick<
  HTMLMediaElement,
  | "currentTime"
  | "playbackRate"
  | "paused"
  | "ended"
  | "seeking"
  | "readyState"
  | "networkState"
  | "buffered"
  | "duration"
  | "play"
  | "pause"
  | "addEventListener"
  | "removeEventListener"
  | "dispatchEvent"
>;

/**
 * HTMLMediaElement.HAVE_METADATA, HAVE_FUTURE_DATA and NETWORK_IDLE, which a plain page may not
 * define.
 */
const haveMetadata = 1;
const haveFutureData = 3;
const networkIdle = 1;

/** How a player follows its group. */
export interface FollowOptions {
  /** The film this player plays: a group playing another is not followed. */
  readonly fileId: number;
  /** Whether this player's own play, pause and seeks are the group's (the host's; D246 §8). */
  readonly isLeading: boolean;
  readonly monotonicMs: () => number;
}

/** The group a player follows, where it is expected now, and when now is. */
interface GroupNow {
  readonly group: GroupView;
  readonly expectedMs: number;
  readonly nowMs: number;
}

/** The player's position, in ms. */
function positionMsOf(media: FollowedMedia): number {
  return media.currentTime * 1000;
}

/** Whether the player is where the group is, near enough to play along without a seek. */
function isInStep(media: FollowedMedia, expectedMs: number): boolean {
  return (
    media.readyState >= haveFutureData &&
    !media.seeking &&
    Math.abs(positionMsOf(media) - expectedMs) <= seekDriftBeyondMs
  );
}

/**
 * Whether the player has loaded enough ahead to play on after a stall or a seek: refillAheadMs, or
 * up to the film's end, or all the browser will load while it is paused, since the browser has
 * stopped loading and waiting longer gains nothing.
 */
function hasRefilled(media: FollowedMedia): boolean {
  if (media.readyState < haveFutureData) {
    return false;
  }
  if (media.networkState === networkIdle) {
    return true;
  }
  const { buffered, currentTime } = media;
  for (let index = 0; index < buffered.length; index++) {
    const endS = buffered.end(index);
    if (buffered.start(index) <= currentTime && currentTime <= endS) {
      return (endS - currentTime) * 1000 >= refillAheadMs || endS >= media.duration;
    }
  }
  return false;
}

/** The command a leading player's own event is, if any: its person's play, pause or seek. */
function commandOfEvent(
  event: string,
  media: FollowedMedia,
  where: GroupNow,
): GroupCommand | undefined {
  const { state } = where.group;
  if (event === "play" && state === "Paused") {
    return { command: "play" };
  }
  if (state === "Playing" && (event === "ended" || (event === "pause" && !media.ended))) {
    return { command: "pause" };
  }
  // A jump is past the seek threshold: the browser's player moves itself a fraction of a second to
  // step over a gap in what it loaded, and each such step, taken for its person's seek, made the
  // whole group wait and load again, without end on a slow link (the owner's test, 2026-10-05).
  const isAJump =
    driftCorrection({
      expectedMs: where.expectedMs,
      actualMs: positionMsOf(media),
      groupRate: where.group.rate,
    }).kind === "seek";
  // A seek is the group's as its person makes it, not once it has loaded: a host on a slow link
  // would otherwise hold the group up loading a place the group was never told of, and be left
  // behind (the owner's test, 2026-10-05).
  return event === "seeking" && isAJump
    ? { command: "seek", positionMs: positionMsOf(media) }
    : undefined;
}

/**
 * The event a player fires at its video as it moves the video itself, before the seeking that
 * follows: hls.js steps over a gap in what it loaded, or nudges a stalled video on. That seek is
 * the player's, not its person's, and a leading player does not send it to the group (the owner's
 * test, 2026-10-05, where each such step became a jump for everyone).
 */
export const playerMovedItselfEvent = "orpheumplayermoveditself";

/**
 * The event a following player fires at its video as it seeks to catch up with a group it fell
 * behind, having once been in step: a stall in all but name, which a player on a slow link never
 * otherwise counts, every load being a seek's (D247 §2; the owner's test, 2026-10-05).
 */
export const fellBehindEvent = "orpheumfellbehind";

/** The events a following player tells the group of, and is moved by. */
const followedEvents = [
  "loadedmetadata",
  "canplay",
  "playing",
  "waiting",
  "play",
  "pause",
  "seeking",
  "seeked",
  "ended",
] as const;

/** One player following its group, as followGroup describes it. */
class GroupFollower {
  readonly #media: FollowedMedia;
  readonly #connection: TogetherConnection;
  readonly #options: FollowOptions;
  /** Whether the last seek was the follower's own, so its seeking is not taken for a person's. */
  #isOwnSeekPending = false;
  #nudgeEndsAtMs = 0;
  /** Whether it has once been where the group is: until then it holds no one up (D246 §6). */
  #hasCaughtUp = false;
  /** Whether it has stalled or sought, and has not yet loaded enough ahead to play on. */
  #isRefilling = false;
  /** Whether the group was last told it buffers, so loading only tells it of a change. */
  #wasReportedBuffering: boolean | undefined = undefined;
  /**
   * Whether a command this player sent awaits the server's answer: until the group changes, the
   * player is left as its person put it, not pulled back to the group as it was.
   */
  #isAwaitingGroup = false;
  /** When its own last seek started, on the monotonic clock, until it has loaded; else undefined. */
  #seekStartedAtMs: number | undefined = undefined;
  /** How long its own last seek took to load. */
  #lastLoadMs = 0;
  /** A leading player's pause or seek not yet sent, which a seek that follows it at once replaces. */
  #pendingCommand: ReturnType<typeof setTimeout> | undefined = undefined;

  constructor(media: FollowedMedia, connection: TogetherConnection, options: FollowOptions) {
    this.#media = media;
    this.#connection = connection;
    this.#options = options;
  }

  /** The player's state as the group is told it. */
  readonly playerState = (): PlayerState => {
    const isBuffering = this.#isBuffering();
    this.#wasReportedBuffering = isBuffering;
    return { positionMs: positionMsOf(this.#media), isPlaying: !this.#media.paused, isBuffering };
  };

  /** The group changed: whatever this player asked for is answered. */
  readonly groupChanged = (): void => {
    this.#isAwaitingGroup = false;
    this.apply();
  };

  /** Puts the player where the group is now, when the group plays its film. */
  readonly apply = (): void => {
    const where = this.#whereNow();
    if (where === undefined || this.#isAwaitingGroup || this.#media.readyState < haveMetadata) {
      return;
    }
    if (where.group.state === "Playing") {
      this.#play(where);
    } else {
      this.#holdAt(where);
    }
    this.#hasCaughtUp ||= isInStep(this.#media, where.expectedMs);
  };

  /** One of the player's own events: a person's command when it leads, else a reason to apply. */
  readonly heard = (event: string): void => {
    this.#noteLoading(event);
    const isOwnSeek = (event === "seeking" || event === "seeked") && this.#isOwnSeekPending;
    if (event === "seeked") {
      this.#isOwnSeekPending = false;
    }
    const where = this.#whereNow();
    const command =
      this.#options.isLeading && where !== undefined && !isOwnSeek
        ? commandOfEvent(event, this.#media, where)
        : undefined;
    if (command === undefined) {
      this.apply();
    } else {
      this.#lead(command);
    }
    // While its person's command waits to be sent, the player's loading is of a place the group
    // has not been told of, and would be taken for a stall: it is told once the command is sent.
    if (this.#pendingCommand === undefined) {
      this.#connection.reportState();
    }
  };

  /**
   * Sends a leading player's command: a pause or a seek once no seek has followed it within
   * scrubPauseWindowMs, the last of a drag. Until then the player is left where its person put it.
   */
  #lead(command: GroupCommand): void {
    if (command.command !== "pause" && command.command !== "seek") {
      this.#send(command);
      return;
    }
    clearTimeout(this.#pendingCommand);
    this.#isAwaitingGroup = true;
    this.#pendingCommand = setTimeout(() => {
      this.#pendingCommand = undefined;
      this.#send(command);
    }, scrubPauseWindowMs);
  }

  /**
   * Each checkDriftEveryMs: applies the group, and while it waits tells it this player's state, by
   * which the server ends a wait whose time is up (D247 §4).
   */
  readonly tick = (): void => {
    this.apply();
    if (this.#connection.snapshot().group?.state === "Waiting") {
      this.#connection.reportState();
    }
  };

  /** The browser loaded more: it moves nothing, and tells the group only if it made it ready. */
  readonly loaded = (): void => {
    this.#noteLoading("progress");
    if (this.#pendingCommand === undefined && this.#isBuffering() !== this.#wasReportedBuffering) {
      this.#connection.reportState();
    }
  };

  /** The player moves the video itself: the seeking that follows is not its person's. */
  readonly playerMovedItself = (): void => {
    this.#isOwnSeekPending = true;
  };

  /** Stops what is pending, as the follower stops. */
  readonly stop = (): void => {
    clearTimeout(this.#pendingCommand);
  };

  /**
   * Starts refilling at a stall or a seek, and ends it once enough is loaded ahead; times how long
   * its own seek took to load.
   */
  #noteLoading(event: string): void {
    const media = this.#media;
    if (
      this.#seekStartedAtMs !== undefined &&
      media.readyState >= haveFutureData &&
      !media.seeking
    ) {
      this.#lastLoadMs = this.#options.monotonicMs() - this.#seekStartedAtMs;
      this.#seekStartedAtMs = undefined;
    }
    if (event === "waiting" || event === "seeking" || media.readyState < haveFutureData) {
      this.#isRefilling = true;
    } else if (this.#isRefilling && hasRefilled(media)) {
      this.#isRefilling = false;
    }
  }

  /** Whether the player holds the group up: not yet caught up, loading, or refilling. */
  #isBuffering(): boolean {
    const media = this.#media;
    return (
      !this.#hasCaughtUp ||
      media.readyState < haveFutureData ||
      media.seeking ||
      (this.#isRefilling && !hasRefilled(media))
    );
  }

  #send(command: GroupCommand): void {
    this.#isAwaitingGroup = true;
    this.#connection.command(command);
    this.#connection.reportState();
  }

  #whereNow(): GroupNow | undefined {
    const snapshot = this.#connection.snapshot();
    const { group } = snapshot;
    if (group?.fileId !== this.#options.fileId) {
      return undefined;
    }
    const nowMs = this.#options.monotonicMs();
    return {
      group,
      nowMs,
      expectedMs: expectedPositionMs(group, snapshot.groupReceivedAtMs, nowMs),
    };
  }

  #seek(toMs: number): void {
    this.#isOwnSeekPending = true;
    this.#seekStartedAtMs = this.#options.monotonicMs();
    this.#media.currentTime = Math.max(0, toMs) / 1000;
  }

  /** Plays along with a playing group, its drift nudged away or sought across. */
  #play(where: GroupNow): void {
    const media = this.#media;
    if (media.paused && !media.ended) {
      media.play().catch(() => undefined); // a refused play stays paused, and is tried again
    }
    // A player still loading is corrected by nothing: each correction would start its load again.
    if (
      where.nowMs < this.#nudgeEndsAtMs ||
      media.seeking ||
      media.ended ||
      media.readyState < haveFutureData
    ) {
      return;
    }
    const correction = driftCorrection({
      expectedMs: where.expectedMs,
      actualMs: positionMsOf(media),
      groupRate: where.group.rate,
    });
    media.playbackRate = correction.kind === "nudge" ? correction.rate : where.group.rate;
    if (correction.kind === "nudge") {
      this.#nudgeEndsAtMs = where.nowMs + nudgeForMs;
    } else if (correction.kind === "seek") {
      this.#catchUp(correction.toMs, where.group.rate);
    }
  }

  /**
   * Seeks to catch up with the group at toMs, aimed ahead by how long its last seek took to load,
   * and says it fell behind if it had been in step.
   */
  #catchUp(toMs: number, groupRate: number): void {
    if (this.#hasCaughtUp) {
      this.#media.dispatchEvent(new Event(fellBehindEvent));
    }
    this.#seek(toMs + Math.min(this.#lastLoadMs, aimAheadAtMostMs) * groupRate);
  }

  /** Holds the player paused at the group's position: Paused, Waiting or Idle. */
  #holdAt(where: GroupNow): void {
    const media = this.#media;
    media.playbackRate = where.group.rate;
    if (!media.paused) {
      media.pause();
    }
    // While its own seek still loads, the browser's player moves it a little as it loads (hls.js
    // steps over a gap at the start of what it loaded): a seek for that would start the load again,
    // and again, and the player never loads at all (the owner's test, 2026-10-05, on a slow link).
    const holdAtMs = playableAt(media, where.expectedMs);
    const awayMs = Math.abs(positionMsOf(media) - holdAtMs);
    const isLoadingOwnSeek = this.#seekStartedAtMs !== undefined;
    // A player held just before the first frame it loaded, by however little, is moved onto it: a
    // paused player never steps over the gap, and one not on what it loaded is never ready. Both
    // players held at 0 with 0.04 s to 60 s loaded, a gap under leaveDriftUpToMs, and held the
    // group up for good (staging, 2026-10-06).
    const isBeforeWhatItLoaded =
      !isLoadingOwnSeek && holdAtMs !== where.expectedMs && !isOnLoaded(media);
    const isAway = awayMs > (isLoadingOwnSeek ? seekDriftBeyondMs : leaveDriftUpToMs);
    if ((isAway || isBeforeWhatItLoaded) && !media.seeking) {
      this.#seek(holdAtMs);
    }
  }
}

/**
 * Where a paused player can be held at expectedMs: there, or, when that falls in a gap just
 * before what it loaded, within the seek threshold, the start of what it loaded. A copied picture
 * is cut at its source's keyframes, so its first loaded frame can come a moment after the group's
 * position; a paused player never steps over that gap as a playing one does, is never ready, and
 * held the group up for good (the owner's test, 2026-10-06, a copied HEVC picture on a Mac).
 */
/** Whether the player's position is on something it loaded, to within a millisecond. */
function isOnLoaded(media: FollowedMedia): boolean {
  const { buffered, currentTime } = media;
  for (let index = 0; index < buffered.length; index++) {
    if (buffered.start(index) - 0.001 <= currentTime && currentTime <= buffered.end(index)) {
      return true;
    }
  }
  return false;
}

function playableAt(media: FollowedMedia, expectedMs: number): number {
  const { buffered } = media;
  const expectedS = expectedMs / 1000;
  for (let index = 0; index < buffered.length; index++) {
    const startS = buffered.start(index);
    if (startS <= expectedS && expectedS <= buffered.end(index)) {
      return expectedMs;
    }
    if (startS > expectedS && (startS - expectedS) * 1000 <= seekDriftBeyondMs) {
      return startS * 1000;
    }
  }
  return expectedMs;
}

/**
 * Makes media follow the group while it plays options.fileId (watch-together.md, Sync): paused at
 * the group's position while it is Paused, Waiting or Idle; playing while it is Playing, its drift
 * nudged away by the rate or sought across. It tells the group its state on every change, and
 * counts as buffering until it has once been where the group is, so a player still loading holds
 * no one up (D246 §6), and after a stall or a seek until it has refillAheadMs loaded. A leading player's own play, pause and seek become commands; the
 * follower's own do not. Returns the stopping.
 */
export function followGroup(
  media: FollowedMedia,
  connection: TogetherConnection,
  options: FollowOptions,
): () => void {
  const follower = new GroupFollower(media, connection, options);
  const listeners = [
    ...followedEvents.map(
      (event) =>
        [
          event,
          () => {
            follower.heard(event);
          },
        ] as const,
    ),
    ["progress", follower.loaded] as const,
    [playerMovedItselfEvent, follower.playerMovedItself] as const,
  ];
  for (const [event, listener] of listeners) {
    media.addEventListener(event, listener);
  }
  connection.setPlayer(follower.playerState);
  const unsubscribe = connection.subscribe(follower.groupChanged);
  const checker = setInterval(follower.tick, checkDriftEveryMs);
  follower.apply();
  connection.reportState();
  return () => {
    clearInterval(checker);
    follower.stop();
    unsubscribe();
    connection.setPlayer(undefined);
    for (const [event, listener] of listeners) {
      media.removeEventListener(event, listener);
    }
  };
}
