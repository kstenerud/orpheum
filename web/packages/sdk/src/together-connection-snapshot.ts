// SPDX-License-Identifier: MPL-2.0

import { wireSound } from "./playback-sound.ts";
import type { LeftPage, ShownPage } from "./shown-page.ts";
import { expectedPositionMs } from "./together-drift.ts";
import {
  noRight,
  startingTuning,
  togetherProtocolVersion,
  type DeviceRole,
  type GroupView,
  type OpenTogetherSocket,
  type PlaybackReport,
  type PlaybackRights,
  type RosterMember,
  type SyncTuning,
  type TogetherEndReason,
  type TogetherServerMessage,
} from "./together.ts";

/**
 * How far apart two devices' clocks are taken to run, in parts per million: a placing ages by this
 * share of its age, so one a few minutes old gives way to a pong whose round trip is a little
 * longer (D268 §1; BBC's figure for DVB-CSS, which assumes 50 ppm of frequency error).
 */
export const assumedClockSkewPpm = 50;

/**
 * Whether the device is connected: connecting the first time, welcomed, connecting again after a
 * drop, or ended (the session is over for it, or it was closed).
 */
export type ConnectionStatus = "connecting" | "connected" | "reconnecting" | "ended";

/** What the connection knows of the session now. */
export interface TogetherSnapshot {
  readonly status: ConnectionStatus;
  readonly memberId: number | undefined;
  /**
   * This device's own id in the roster, as its welcome said, a new one on each connection;
   * undefined, or absent, until then.
   */
  readonly deviceId?: number | undefined;
  readonly isHost: boolean;
  /** What this device's member may do to playback, as the welcome said (D271 §4). */
  readonly rights: PlaybackRights;
  readonly group: GroupView | undefined;
  /**
   * The number of the group's change that group is, as the server counts them; absent until the
   * welcome. A player's word that it followed a change names it (the groupFollowed note).
   */
  readonly groupSeq?: number;
  /** The monotonic time at which group arrived: a playing group has moved on from it since. */
  readonly groupReceivedAtMs: number;
  /**
   * The round trip of the pong that last placed group, in ms: it arrived half of it after the
   * server said it (D264). Undefined while the group message that announced it places it, whose
   * wait on the way no one knows.
   */
  readonly groupPlacedByRttMs: number | undefined;
  readonly roster: readonly RosterMember[];
  /** Why it ended, when the server ended it; undefined while it goes on or when closed here. */
  readonly endReason: TogetherEndReason | undefined;
  /** The shortest round trip a ping has measured on this connection, in ms. */
  readonly minimumRttMs: number | undefined;
  /** The last command the server refused, by its code (cut-a §5.2). */
  readonly lastRejectionCode: string | undefined;
  /**
   * How many messages that answer a command, a group or a rejection, have been heard: a command
   * is answered once it moves on, and a pong or a roster is no answer.
   */
  readonly answersHeard: number;
  /** How to follow the group, as the welcome said (D250 §1). */
  readonly tuning: SyncTuning;
}

/** What a connection needs. */
export interface TogetherConnectionOptions {
  /** The membership credential, sent in the hello, never in the address (D154). */
  readonly credential: string;
  /**
   * The web client's script the page runs, said in the hello for the server's log, so a device's
   * report says which client made it (agent-failures A28); undefined for a development page.
   */
  readonly client?: string | undefined;
  /** A signed-in person's session token, which makes the membership theirs (D243 §2). */
  readonly sessionToken?: string | undefined;
  /**
   * The part of its member the device fulfils (D120, D338 §4), said in the hello: "video", the
   * video viewer, when not given.
   */
  readonly role?: DeviceRole | undefined;
  readonly openSocket: OpenTogetherSocket;
  /** A monotonic clock in ms (performance.now): no clock is shared with the server (D66). */
  readonly monotonicMs: () => number;
  /**
   * The page the device is on, whose showing again has the socket checked at once
   * (shownAnswerWithinMs): a locked phone, a sleeping computer or a tab in the background may come
   * back on a socket that died meanwhile. Absent, nothing is checked but by the pings.
   */
  readonly page?: ShownPage | undefined;
  /**
   * Tells its listener each time the way to the core may work again (RtcTransport.onBack, or the
   * browser online); returns its removal. Told, it does as when its page is shown again: the owner's
   * Chrome otherwise played on out of step for 28 s after its media came back (VQ130).
   */
  readonly pathBack?: ((listener: () => void) => () => void) | undefined;
  /**
   * The page's window, whose pagehide gives the socket up at once and whose pageshow from the
   * back/forward cache connects again: a Play page navigated away sat there with its socket open,
   * leading, until the core's silence limit (the owner, 2026-10-29). Absent, only closing the tab
   * gives it up.
   */
  readonly leftPage?: LeftPage | undefined;
}

/** The snapshot of a connection not yet opened. */
export const startingSnapshot: TogetherSnapshot = {
  status: "connecting",
  memberId: undefined,
  deviceId: undefined,
  isHost: false,
  rights: noRight,
  group: undefined,
  groupReceivedAtMs: 0,
  groupPlacedByRttMs: undefined,
  roster: [],
  endReason: undefined,
  minimumRttMs: undefined,
  lastRejectionCode: undefined,
  answersHeard: 0,
  tuning: startingTuning,
};

/**
 * Whether next keeps playing the film current plays, at its rate, from where it was: the same
 * timeline, told again. A seek or a film put on again is a new one, as a group that plays on through
 * them, with no leader to stand for, tells them (D371 §2).
 */
export function isSameTimeline(
  current: GroupView | undefined,
  next: GroupView,
): current is GroupView {
  return (
    current?.state === "Playing" &&
    next.state === "Playing" &&
    current.fileId === next.fileId &&
    current.rate === next.rate &&
    next.event !== "seeked" &&
    next.event !== "changedItem"
  );
}

/**
 * Where the group a snapshot follows is at the monotonic time nowMs: told at groupReceivedAtMs by a
 * message on its way half the round trip of the pong that placed it, or, placed by its group
 * message, half the shortest round trip measured (D66, D264); undefined while it follows none.
 */
export function expectedGroupPositionMs(
  snapshot: TogetherSnapshot,
  nowMs: number,
): number | undefined {
  const { group } = snapshot;
  return group === undefined
    ? undefined
    : expectedPositionMs(group, { ...placingOf(snapshot), nowMs });
}

/** A playback in whole numbers, as the server reads them. */
export function wholePlayback(playback: PlaybackReport): PlaybackReport {
  const { qualityKbps, linkKbps, sound } = playback;
  return {
    delivery: playback.delivery,
    ...(qualityKbps === undefined ? {} : { qualityKbps: Math.round(qualityKbps) }),
    ...(linkKbps === undefined ? {} : { linkKbps: Math.round(linkKbps) }),
    ...(sound === undefined ? {} : { sound: wireSound(sound) }),
  };
}

/** When what placed a snapshot's group arrived, and how long it took to (D66, D264). */
function placingOf(snapshot: TogetherSnapshot): { receivedAtMs: number; transitMs: number } {
  return {
    receivedAtMs: snapshot.groupReceivedAtMs,
    transitMs: (snapshot.groupPlacedByRttMs ?? snapshot.minimumRttMs ?? 0) / 2,
  };
}

/** The snapshot after a group message: a new timeline, or the one it follows told again. */
export function afterGroup(
  current: TogetherSnapshot,
  group: GroupView,
  nowMs: number,
): TogetherSnapshot {
  if (isSameTimeline(current.group, group)) {
    // News of a group that plays on as it was, "continuing without" someone, moves no one: the
    // timeline stays the one first heard, so each message's own delay on the way does not nudge
    // every player. Re-based on each, a host on a tethered link was nudged every few seconds
    // while a viewer kept catching up, and Firefox's sound drifted ahead of its picture
    // (staging, 2026-10-06).
    return {
      ...current,
      group: { ...group, positionMs: current.group.positionMs },
    };
  }
  return {
    ...current,
    group,
    groupReceivedAtMs: nowMs,
    groupPlacedByRttMs: undefined,
  };
}

/**
 * Whether a pong of rttMs, at nowMs, places the group better than the placing a snapshot has: by
 * what each may be off now, half its round trip, and for the placing also its age times
 * assumedClockSkewPpm, so a placing ages and gives way (D264, D268 §1).
 */
export function isBetterPlacing(current: TogetherSnapshot, rttMs: number, nowMs: number): boolean {
  const placedRttMs = current.groupPlacedByRttMs;
  if (placedRttMs === undefined) {
    return true;
  }
  const ageMs = nowMs - current.groupReceivedAtMs;
  return rttMs / 2 < placedRttMs / 2 + (ageMs * assumedClockSkewPpm) / 1_000_000;
}

/** The snapshot after a pong: its round trip measured, and the group placed anew by it (D264). */
export function afterPong(
  current: TogetherSnapshot,
  message: Extract<TogetherServerMessage, { readonly type: "pong" }>,
  nowMs: number,
): TogetherSnapshot {
  const rttMs = Math.max(0, nowMs - message.tMono);
  const measured = { ...current, minimumRttMs: Math.min(rttMs, current.minimumRttMs ?? rttMs) };
  // A pong of the timeline followed places it anew when it is off by less than the placing now:
  // its wait is bounded by what was measured, a group message's by nothing (D264, D268 §1).
  return message.group !== undefined &&
    isSameTimeline(current.group, message.group) &&
    isBetterPlacing(current, rttMs, nowMs)
    ? {
        ...measured,
        group: { ...current.group, positionMs: message.group.positionMs },
        groupReceivedAtMs: nowMs,
        groupPlacedByRttMs: rttMs,
      }
    : measured;
}

/** The snapshot after one message from the server, and whether it was the welcome. */
export function afterMessage(
  current: TogetherSnapshot,
  message: TogetherServerMessage,
  nowMs: number,
): TogetherSnapshot {
  switch (message.type) {
    case "welcome":
      return {
        ...current,
        status: "connected",
        memberId: message.memberId,
        deviceId: message.deviceId,
        isHost: message.isHost,
        rights: message.rights,
        group: message.group,
        groupSeq: message.seq,
        groupReceivedAtMs: nowMs,
        groupPlacedByRttMs: undefined,
        roster: message.roster,
        minimumRttMs: undefined,
        tuning: message.tuning,
        answersHeard: current.answersHeard + 1,
      };
    case "group":
      return {
        ...afterGroup(current, message.group, nowMs),
        groupSeq: message.seq,
        answersHeard: current.answersHeard + 1,
      };
    case "roster":
      return { ...current, roster: message.roster };
    case "rejected":
      return {
        ...current,
        lastRejectionCode: message.code,
        answersHeard: current.answersHeard + 1,
      };
    case "pong":
      return afterPong(current, message, nowMs);
    case "ended":
      return { ...current, status: "ended", endReason: message.reason };
    case "rtc":
    case "sound":
      return current; // voice signalling and sound choices go to their listeners, changing nothing here
  }
}

/** The seq of the group as a message tells it, or undefined when it tells none. */
export function seqOf(message: TogetherServerMessage): number | undefined {
  return message.type === "welcome" || message.type === "group" ? message.seq : undefined;
}

/**
 * The hello a device opens with, in its role (cut-a §5.1): one that plays says it can set its
 * player's rate; a voice device has no player.
 */
export function helloText(options: TogetherConnectionOptions): string {
  const role = options.role ?? "video";
  return JSON.stringify({
    v: togetherProtocolVersion,
    type: "hello",
    channel: "together",
    credential: options.credential,
    ...(options.sessionToken === undefined ? {} : { sessionToken: options.sessionToken }),
    role,
    ...(options.client === undefined ? {} : { client: options.client }),
    ...(role === "panel" ? {} : { canSetRate: true }),
  });
}
