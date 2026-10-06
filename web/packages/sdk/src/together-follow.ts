// SPDX-License-Identifier: MPL-2.0

import type { GroupView, SyncTuning } from "./together.ts";
import type { GroupCommand, PlayerState, TogetherConnection } from "./together-connection.ts";

/** How long one nudge of the rate lasts (watch-together.md, Sync: "for ~1 s"). */
export const nudgeForMs = 1000;

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
 * How much a player loads ahead of where it is, and then stops: hls.js's maxBufferLength (30 s),
 * which nothing here changes, and the most an iPhone's ManagedMediaSource lets it load
 * (playback-in-practice.md §1). A player catching up aims to have the tuning's percent of it loaded
 * when the group arrives (D250 §4).
 */
export const playerLoadsAheadMs = 30_000;

/**
 * The furthest a player catching up alone jumps ahead of the group to wait for it there: a person
 * waits paused for at most about a minute of film, however slow their link (D250 §4).
 */
export const waitAheadAtMostMs = 60_000;

/**
 * How the speed a player loads at is measured, as film loaded per second: over the last
 * loadWindowMs, from at least loadSpanAtLeastMs of samples, and only while it is loading, its buffer
 * short of what it fills to by loadSampleMarginMs, since a full buffer loads nothing and would read
 * as a link that stopped (D247 §4, amended 2026-10-06).
 */
export const loadWindowMs = 10_000;
export const loadSpanAtLeastMs = 3000;
export const loadSampleMarginMs = 5000;

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
 * none within tuning's leaveDriftUpToMs, a seek beyond its seekDriftBeyondMs, else a rate that
 * closes the gap over nudgeForMs, no further from the group's than its nudgeMostPercent: a larger
 * drift is closed by the nudges that follow (D250 §1).
 */
export function driftCorrection(
  drift: { readonly expectedMs: number; readonly actualMs: number; readonly groupRate: number },
  tuning: SyncTuning,
): DriftCorrection {
  const behindMs = drift.expectedMs - drift.actualMs;
  if (Math.abs(behindMs) <= tuning.leaveDriftUpToMs) {
    return { kind: "none" };
  }
  if (Math.abs(behindMs) > tuning.seekDriftBeyondMs) {
    return { kind: "seek", toMs: drift.expectedMs };
  }
  const most = tuning.nudgeMostPercent / 100;
  const factor = Math.min(1 + most, Math.max(1 - most, 1 + behindMs / nudgeForMs));
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
  /** How to follow it, as the welcome said (D250 §1). */
  readonly tuning: SyncTuning;
}

/** The player's position, in ms. */
function positionMsOf(media: FollowedMedia): number {
  return media.currentTime * 1000;
}

/** Whether the player is where the group is, near enough to play along without a seek. */
function isInStep(media: FollowedMedia, expectedMs: number, tuning: SyncTuning): boolean {
  return (
    media.readyState >= haveFutureData &&
    !media.seeking &&
    Math.abs(positionMsOf(media) - expectedMs) <= tuning.seekDriftBeyondMs
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
    driftCorrection(
      { expectedMs: where.expectedMs, actualMs: positionMsOf(media), groupRate: where.group.rate },
      where.tuning,
    ).kind === "seek";
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

/**
 * The event a player fires at its video as it loads a chunk: a ChunkLoaded in its detail, which a
 * following player tells the server for its log.
 */
export const playerLoadedChunkEvent = "orpheumplayerloadedchunk";

/** One chunk a player loaded, as playerLoadedChunkEvent tells it. */
export interface ChunkLoaded {
  readonly track: "video" | "audio";
  readonly loadMs: number;
  readonly kbps: number;
  /** The quality the chunk is of, in kbit/s. */
  readonly qualityKbps: number;
}

/**
 * The event a player fires at its video as it begins playing another quality: a QualitySwitched
 * in its detail, which a following player tells the server for its log.
 */
export const playerSwitchedQualityEvent = "orpheumplayerswitchedquality";

/** A quality a player began playing, as playerSwitchedQualityEvent tells it. */
export interface QualitySwitched {
  readonly qualityKbps: number;
  readonly estimateKbps: number;
}

/** The detail an event carries, when it is a CustomEvent whose detail has every one of fields. */
function detailWith(event: Event, fields: readonly string[]): Record<string, unknown> | undefined {
  if (!(event instanceof CustomEvent)) {
    return undefined;
  }
  const detail: unknown = event.detail;
  if (typeof detail !== "object" || detail === null) {
    return undefined;
  }
  const record: Record<string, unknown> = { ...detail };
  return fields.every((field) => field in record) ? record : undefined;
}

/** A playerLoadedChunkEvent's chunk, or undefined when the event carries none. */
function chunkLoadedOf(event: Event): ChunkLoaded | undefined {
  const detail = detailWith(event, ["track", "loadMs", "kbps", "qualityKbps"]);
  const { track, loadMs, kbps, qualityKbps } = detail ?? {};
  return (track === "video" || track === "audio") &&
    typeof loadMs === "number" &&
    typeof kbps === "number" &&
    typeof qualityKbps === "number"
    ? { track, loadMs, kbps, qualityKbps }
    : undefined;
}

/** A playerSwitchedQualityEvent's quality, or undefined when the event carries none. */
function qualitySwitchedOf(event: Event): QualitySwitched | undefined {
  const { qualityKbps, estimateKbps } = detailWith(event, ["qualityKbps", "estimateKbps"]) ?? {};
  return typeof qualityKbps === "number" && typeof estimateKbps === "number"
    ? { qualityKbps, estimateKbps }
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
  /**
   * Where a player catching up alone waits, paused and loading, for the playing group to arrive;
   * undefined while it follows as usual (D247 §4, amended 2026-10-06).
   */
  #waitAtMs: number | undefined = undefined;
  /** Whether the pause it made to wait ahead is still to be heard: a leading player's is no command. */
  #isOwnPausePending = false;
  /** The end of what it loaded, sampled while it loads: when, and to where, in ms. */
  #loadSamples: { readonly atMs: number; readonly endMs: number }[] = [];
  /** Film loaded per second of the link, as last measured; undefined until it has been. */
  #loadRatio: number | undefined = undefined;

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
    if (where.group.state === "Playing" && this.#isWaitingAhead(where)) {
      // A player that jumped ahead to catch up waits there, paused and loading, for the group.
      if (!this.#media.paused) {
        this.#isOwnPausePending = true;
        this.#media.pause();
      }
    } else if (where.group.state === "Playing") {
      this.#play(where);
    } else {
      this.#holdAt(where);
    }
    this.#hasCaughtUp ||= isInStep(this.#media, where.expectedMs, where.tuning);
  };

  /** One of the player's own events: a person's command when it leads, else a reason to apply. */
  readonly heard = (event: string): void => {
    this.#noteLoading(event);
    const isOwn = this.#isOwnMove(event);
    const where = this.#whereNow();
    const command =
      this.#options.isLeading && where !== undefined && !isOwn
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

  /** Whether event is of a seek or pause the follower made itself, not its person: no command. */
  #isOwnMove(event: string): boolean {
    const isOwnSeek = (event === "seeking" || event === "seeked") && this.#isOwnSeekPending;
    if (event === "seeked") {
      this.#isOwnSeekPending = false;
    }
    const isOwnPause = event === "pause" && this.#isOwnPausePending;
    if (event === "pause") {
      this.#isOwnPausePending = false;
    }
    return isOwnSeek || isOwnPause;
  }

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
    this.#sampleLoading();
    this.apply();
    if (this.#connection.snapshot().group?.state === "Waiting") {
      this.#connection.reportState();
    }
  };

  /** The browser loaded more: it moves nothing, and tells the group only if it made it ready. */
  readonly loaded = (): void => {
    this.#noteLoading("progress");
    this.#sampleLoading();
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

  /**
   * Samples how far the player has loaded, and from the samples of the last loadWindowMs measures
   * how much film it loads per second. A sample is taken only while it loads short of a full
   * buffer; a seek, or loading that stops, starts the samples again.
   */
  #sampleLoading(): void {
    const media = this.#media;
    const nowMs = this.#options.monotonicMs();
    const endMs = loadedEndMs(media);
    const isLoading =
      endMs !== undefined &&
      endMs < media.duration * 1000 &&
      endMs - positionMsOf(media) < playerLoadsAheadMs - loadSampleMarginMs;
    const last = this.#loadSamples.at(-1);
    if (!isLoading || (last !== undefined && endMs < last.endMs)) {
      this.#loadSamples = [];
      return;
    }
    this.#loadSamples = [
      ...this.#loadSamples.filter((sample) => nowMs - sample.atMs <= loadWindowMs),
      { atMs: nowMs, endMs },
    ];
    const first = this.#loadSamples[0];
    if (first !== undefined && nowMs - first.atMs >= loadSpanAtLeastMs) {
      this.#loadRatio = (endMs - first.endMs) / (nowMs - first.atMs);
    }
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
      tuning: snapshot.tuning,
    };
  }

  #seek(toMs: number, why: "catchUp" | "hold"): void {
    this.#connection.note({ kind: "sought", positionMs: positionMsOf(this.#media), toMs, why });
    this.#loadSamples = [];
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
    this.#correct(where);
  }

  /** Corrects a playing player's drift from the group: nudged away, or sought across. */
  #correct(where: GroupNow): void {
    const media = this.#media;
    const correction = driftCorrection(
      { expectedMs: where.expectedMs, actualMs: positionMsOf(media), groupRate: where.group.rate },
      where.tuning,
    );
    // A player stalled and refilling is not nudged: its position does not move, and a faster rate
    // only needs a faster link once it plays (the owner's airplane test, 2026-10-06, where a frozen
    // iPhone was nudged to 2x second after second).
    const isNudged = correction.kind === "nudge" && !this.#isRefilling;
    media.playbackRate = isNudged ? correction.rate : where.group.rate;
    if (isNudged) {
      this.#nudgeEndsAtMs = where.nowMs + nudgeForMs;
      this.#connection.note({
        kind: "nudged",
        positionMs: positionMsOf(media),
        driftMs: where.expectedMs - positionMsOf(media),
        rate: correction.rate,
      });
    } else if (correction.kind === "seek") {
      this.#catchUp(correction.toMs, where);
    }
  }

  /**
   * Whether a player that jumped ahead still waits there for the group: until the group is within
   * the tuning's leaveDriftUpToMs of it, or has passed it, or has sought back from it further
   * than it could have jumped. Then it follows as usual, playing on with what it loaded meanwhile.
   */
  #isWaitingAhead(where: GroupNow): boolean {
    const waitAtMs = this.#waitAtMs;
    if (waitAtMs === undefined) {
      return false;
    }
    const aheadMs = waitAtMs - where.expectedMs;
    const { leaveDriftUpToMs, seekDriftBeyondMs } = where.tuning;
    if (aheadMs > leaveDriftUpToMs && aheadMs <= waitAheadAtMostMs + seekDriftBeyondMs) {
      return true;
    }
    this.#waitAtMs = undefined;
    return false;
  }

  /**
   * Seeks to catch up with the group at toMs, ahead by catchUpLeadMs, and says it fell behind if it
   * had been in step. It waits there for the group, the host's player too: a host on a slow link
   * stalls as a viewer does (the owner, 2026-10-06), and its pause to wait is no command.
   */
  #catchUp(toMs: number, where: GroupNow): void {
    if (this.#hasCaughtUp) {
      this.#media.dispatchEvent(new Event(fellBehindEvent));
    }
    const targetMs = toMs + this.#catchUpLeadMs(where.group.rate, where.tuning);
    this.#seek(targetMs, "catchUp");
    this.#waitAtMs = targetMs;
  }

  /**
   * How far ahead of the group a catch-up seek aims: by how long its last seek took to load, and
   * then by how long its link takes, as measured, to load the tuning's catchUpBufferPercent of what
   * its player loads ahead, so the group finds its buffer that full (D250 §4); at most
   * waitAheadAtMostMs, and never less than the last load's time alone, which is all it aims by with
   * no measure yet.
   */
  #catchUpLeadMs(groupRate: number, tuning: SyncTuning): number {
    const aimMs = Math.min(this.#lastLoadMs, aimAheadAtMostMs) * groupRate;
    const ratio = this.#loadRatio;
    if (ratio === undefined) {
      return aimMs;
    }
    const fillMs = (playerLoadsAheadMs * tuning.catchUpBufferPercent) / 100;
    const fillWallMs = ratio > 0 ? fillMs / ratio : Number.POSITIVE_INFINITY;
    return Math.max(
      aimMs,
      Math.min(waitAheadAtMostMs, (this.#lastLoadMs + fillWallMs) * groupRate),
    );
  }

  /** Holds the player paused at the group's position: Paused, Waiting or Idle. */
  #holdAt(where: GroupNow): void {
    const media = this.#media;
    this.#waitAtMs = undefined; // a group that stops is followed where it stopped
    media.playbackRate = where.group.rate;
    if (!media.paused) {
      media.pause();
    }
    // While its own seek still loads, the browser's player moves it a little as it loads (hls.js
    // steps over a gap at the start of what it loaded): a seek for that would start the load again,
    // and again, and the player never loads at all (the owner's test, 2026-10-05, on a slow link).
    const holdAtMs = playableAt(media, where.expectedMs, where.tuning);
    const awayMs = Math.abs(positionMsOf(media) - holdAtMs);
    const isLoadingOwnSeek = this.#seekStartedAtMs !== undefined;
    // A player held just before the first frame it loaded, by however little, is moved onto it: a
    // paused player never steps over the gap, and one not on what it loaded is never ready. Both
    // players held at 0 with 0.04 s to 60 s loaded, a gap under the drift left alone, and held the
    // group up for good (staging, 2026-10-06).
    const isBeforeWhatItLoaded =
      !isLoadingOwnSeek && holdAtMs !== where.expectedMs && !isOnLoaded(media);
    const { leaveDriftUpToMs, seekDriftBeyondMs } = where.tuning;
    const isAway = awayMs > (isLoadingOwnSeek ? seekDriftBeyondMs : leaveDriftUpToMs);
    if ((isAway || isBeforeWhatItLoaded) && !media.seeking) {
      this.#seek(holdAtMs, "hold");
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
/** The end, in ms, of what the player loaded where it is, or undefined where it loaded nothing. */
function loadedEndMs(media: FollowedMedia): number | undefined {
  const { buffered, currentTime } = media;
  for (let index = 0; index < buffered.length; index++) {
    if (buffered.start(index) - 0.001 <= currentTime && currentTime <= buffered.end(index)) {
      return buffered.end(index) * 1000;
    }
  }
  return undefined;
}

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

function playableAt(media: FollowedMedia, expectedMs: number, tuning: SyncTuning): number {
  const { buffered } = media;
  const expectedS = expectedMs / 1000;
  for (let index = 0; index < buffered.length; index++) {
    const startS = buffered.start(index);
    if (startS <= expectedS && expectedS <= buffered.end(index)) {
      return expectedMs;
    }
    if (startS > expectedS && (startS - expectedS) * 1000 <= tuning.seekDriftBeyondMs) {
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
    [
      playerLoadedChunkEvent,
      (event: Event) => {
        const loaded = chunkLoadedOf(event);
        if (loaded !== undefined) {
          connection.note({ kind: "chunkLoaded", ...loaded });
        }
      },
    ] as const,
    [
      playerSwitchedQualityEvent,
      (event: Event) => {
        const switched = qualitySwitchedOf(event);
        if (switched !== undefined) {
          connection.note({ kind: "qualitySwitched", ...switched });
        }
      },
    ] as const,
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
