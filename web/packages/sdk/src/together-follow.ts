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
 * How long a leading player's pause waits to be sent: a browser's own controls pause the video while
 * its person drags the timeline, and the seek that follows comes within this. A pause with a seek
 * after it is that seek, not a pause for everyone (the owner's first test, 2026-10-05).
 */
export const scrubPauseWindowMs = 300;

/**
 * After a stall or a seek, a player is ready again only with this much of the film loaded ahead of
 * it. Ready at its first frame, a player whose link is slower than the film plays that frame and
 * stalls again, and a group that waits for it plays and freezes in turn without end (the owner's
 * first test, 2026-10-05). Alone, the browser waits for more itself; in a group, this is that wait.
 */
export const refillAheadMs = 10_000;

/** How often a following player checks its drift while the group plays. */
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
  const isAJump =
    driftCorrection({
      expectedMs: where.expectedMs,
      actualMs: positionMsOf(media),
      groupRate: where.group.rate,
    }).kind !== "none";
  return event === "seeked" && isAJump
    ? { command: "seek", positionMs: positionMsOf(media) }
    : undefined;
}

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
  /** Whether the last seek was the follower's own, so its seeked is not taken for a person's. */
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
  /** A leading player's pause not yet sent, which a seek that follows it at once withdraws. */
  #pendingPause: ReturnType<typeof setTimeout> | undefined = undefined;

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
    if (event === "seeking" && this.#pendingPause !== undefined) {
      // A pause and then a seek is a person dragging the timeline: the seek is what they meant.
      clearTimeout(this.#pendingPause);
      this.#pendingPause = undefined;
    }
    const isOwnSeek = event === "seeked" && this.#isOwnSeekPending;
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
    this.#connection.reportState();
  };

  /** Sends a leading player's command: a pause only once no seek has followed it. */
  #lead(command: GroupCommand): void {
    if (command.command !== "pause") {
      this.#send(command);
      return;
    }
    clearTimeout(this.#pendingPause);
    this.#pendingPause = setTimeout(() => {
      this.#pendingPause = undefined;
      this.#send(command);
    }, scrubPauseWindowMs);
  }

  /** The browser loaded more: it moves nothing, and tells the group only if it made it ready. */
  readonly loaded = (): void => {
    this.#noteLoading("progress");
    if (this.#isBuffering() !== this.#wasReportedBuffering) {
      this.#connection.reportState();
    }
  };

  /** Stops what is pending, as the follower stops. */
  readonly stop = (): void => {
    clearTimeout(this.#pendingPause);
  };

  /** Starts refilling at a stall or a seek, and ends it once enough is loaded ahead. */
  #noteLoading(event: string): void {
    const media = this.#media;
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
    this.#media.currentTime = Math.max(0, toMs) / 1000;
  }

  /** Plays along with a playing group, its drift nudged away or sought across. */
  #play(where: GroupNow): void {
    const media = this.#media;
    if (media.paused && !media.ended) {
      media.play().catch(() => undefined); // a refused play stays paused, and is tried again
    }
    if (where.nowMs < this.#nudgeEndsAtMs || media.seeking || media.ended) {
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
      this.#seek(correction.toMs);
    }
  }

  /** Holds the player paused at the group's position: Paused, Waiting or Idle. */
  #holdAt(where: GroupNow): void {
    const media = this.#media;
    media.playbackRate = where.group.rate;
    if (!media.paused) {
      media.pause();
    }
    const isAway = Math.abs(positionMsOf(media) - where.expectedMs) > leaveDriftUpToMs;
    if (isAway && !media.seeking) {
      this.#seek(where.expectedMs);
    }
  }
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
  ];
  for (const [event, listener] of listeners) {
    media.addEventListener(event, listener);
  }
  connection.setPlayer(follower.playerState);
  const unsubscribe = connection.subscribe(follower.groupChanged);
  const checker = setInterval(follower.apply, checkDriftEveryMs);
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
