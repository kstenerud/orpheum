// SPDX-License-Identifier: MPL-2.0

import { expectedPositionMs } from "./together-drift.ts";
import type { VoiceCallStats } from "./together-voice-stats.ts";
import {
  parseTogetherMessage,
  startingTuning,
  togetherProtocolVersion,
  type CannotKeepUp,
  type DeviceRole,
  type GroupView,
  type OpenTogetherSocket,
  type RosterMember,
  type SyncTuning,
  type TogetherEndReason,
  type TogetherServerMessage,
  type TogetherSocket,
  type VoiceSignal,
} from "./together.ts";

/**
 * How often a device says where it is when nothing changes (cut-a §5.1: "on every change and every
 * 60 s"). Drift between clocks is tens of milliseconds over minutes (watch-together.md, Sync), so
 * a minute is plenty.
 */
export const stateEveryMs = 60_000;

/**
 * How often a device pings. The minimum round trip is the sample kept (D66); a ping every 15 s
 * finds the new one soon after a phone moves from Wi-Fi to cellular, at a cost of a few bytes.
 */
export const pingEveryMs = 15_000;

/**
 * How many pings in a row may go unanswered, nothing at all heard since, before the device takes
 * its socket for dead and connects again: a phone whose network dropped it may never be told its
 * socket closed, and kept missing the host's pauses and seeks long after the server had given up
 * on it at three (staging, 2026-10-06).
 */
export const deadAfterUnansweredPings = 2;

/** The first wait before connecting again after a drop, doubled on each failure up to the last. */
export const firstReconnectWaitMs = 500;

/** The longest wait between attempts to connect again: a drop is a state, not an ending. */
export const longestReconnectWaitMs = 30_000;

/** The WebSocket close code the server sends when it refuses a hello: no such membership. */
const policyViolationCode = 1008;

/** How long to wait before the attempt'th try to connect again (0 is the first). */
export function reconnectWaitMs(attempt: number): number {
  return Math.min(firstReconnectWaitMs * 2 ** attempt, longestReconnectWaitMs);
}

/** Where this device's player is, as it tells the group. */
export interface PlayerState {
  readonly positionMs: number;
  readonly isPlaying: boolean;
  /** Not ready to play along: loading, seeking, or not yet at the group's position. */
  readonly isBuffering: boolean;
}

/**
 * One thing a device's player did, which the server logs to say why a film jumped or froze (the
 * owner's test, 2026-10-06, where nothing said what a viewer's iPhone did between its jumps):
 * - nudged: played at rate for a moment, driftMs behind the group (ahead when negative);
 * - sought: moved itself from positionMs to toMs, why: "catchUp" (jumping ahead of a group it fell
 *   behind) or "hold" (to where a stopped group is);
 * - chunkLoaded: loaded one chunk of track ("video" or "audio") in loadMs, at kbps, of a quality
 *   of qualityKbps;
 * - qualitySwitched: began playing the quality of qualityKbps, its link estimated at estimateKbps;
 * - reconnected: connected again after offlineMs, why: "closed" (its socket closed) or "silent"
 *   (nothing was heard on it);
 * - voiceStats: a voice call with partnerDeviceId, as its receiver measured it (D262 §1);
 * - voiceCall: a voice call with partnerDeviceId took a step, why: VoiceCallStep, so a call that
 *   never connects says how far it got (a browser tier's run, 2026-10-07, where nothing did);
 * - placed: a pong with a roundTripMs shorter than any before placed the group anew, moving where
 *   it is expected by movedMs (ahead when positive), the wait on the way of what placed it before
 *   (D264);
 * - playedMuted: the browser would not start the film with sound at positionMs, for want of a recent
 *   tap, and it plays muted (playedMutedEvent);
 * - unmuted: the person's tap turned the sound on at positionMs (soundOnAtTap).
 */
export type PlayerNote =
  | {
      readonly kind: "nudged";
      readonly positionMs: number;
      readonly driftMs: number;
      readonly rate: number;
    }
  | {
      readonly kind: "sought";
      readonly positionMs: number;
      readonly toMs: number;
      readonly why: "catchUp" | "hold";
    }
  | {
      readonly kind: "chunkLoaded";
      readonly track: "video" | "audio";
      readonly loadMs: number;
      readonly kbps: number;
      readonly qualityKbps: number;
    }
  | {
      readonly kind: "qualitySwitched";
      readonly qualityKbps: number;
      readonly estimateKbps: number;
    }
  | { readonly kind: "reconnected"; readonly offlineMs: number; readonly why: "closed" | "silent" }
  | {
      readonly kind: "voiceStats";
      readonly voice: VoiceCallStats & { readonly partnerDeviceId: number };
    }
  | { readonly kind: "voiceCall"; readonly partnerDeviceId: number; readonly why: VoiceCallStep }
  | { readonly kind: "placed"; readonly roundTripMs: number; readonly movedMs: number }
  | { readonly kind: "playedMuted"; readonly positionMs: number }
  | { readonly kind: "unmuted"; readonly positionMs: number };

/**
 * A step a voice call takes: it sent its offer, sent its answer to one, took the answer to its
 * own, connected or failed; or a signal came from a device it has no call with, and was dropped.
 */
export type VoiceCallStep =
  "offered" | "answered" | "accepted" | "connected" | "failed" | "signalDropped";

/** A command only the host sends (D246 §8): play, pause, seek, put a film on, or stop. */
export type GroupCommand =
  | { readonly command: "play" }
  | { readonly command: "pause" }
  | { readonly command: "seek"; readonly positionMs: number }
  | { readonly command: "changeItem"; readonly fileId: number; readonly positionMs: number }
  | { readonly command: "stop" };

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
  readonly group: GroupView | undefined;
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
  /** How to follow the group, as the welcome said (D250 §1). */
  readonly tuning: SyncTuning;
}

/** A device's connection to its session (cut-a §5.1). */
export interface TogetherConnection {
  readonly snapshot: () => TogetherSnapshot;
  /** Calls listener whenever the snapshot changes; returns the unsubscribing. */
  readonly subscribe: (listener: () => void) => () => void;
  /** Names where the player's state is read from, or that there is no player now. */
  readonly setPlayer: (read: (() => PlayerState) | undefined) => void;
  /** Tells the group where the player is now: on every change of it. */
  readonly reportState: () => void;
  /** Sends the host's command, based on the group as last told. */
  readonly command: (command: GroupCommand) => void;
  /** Tells the server one thing its player did, for its log; dropped while not connected. */
  readonly note: (note: PlayerNote) => void;
  /** Connects, from the start: after creating it, or again after closing it. */
  readonly open: () => void;
  /** Closes the connection, and connects no more until opened again. */
  readonly close: () => void;
}

/** What a connection needs. */
export interface TogetherConnectionOptions {
  /** The membership credential, sent in the hello, never in the address (D154). */
  readonly credential: string;
  /** A signed-in person's session token, which makes the membership theirs (D243 §2). */
  readonly sessionToken?: string | undefined;
  /** What the device does for its member (D120), said in the hello: "playback" when not given. */
  readonly role?: DeviceRole | undefined;
  readonly openSocket: OpenTogetherSocket;
  /** A monotonic clock in ms (performance.now): no clock is shared with the server (D66). */
  readonly monotonicMs: () => number;
}

const startingSnapshot: TogetherSnapshot = {
  status: "connecting",
  memberId: undefined,
  deviceId: undefined,
  isHost: false,
  group: undefined,
  groupReceivedAtMs: 0,
  groupPlacedByRttMs: undefined,
  roster: [],
  endReason: undefined,
  minimumRttMs: undefined,
  lastRejectionCode: undefined,
  tuning: startingTuning,
};

/** Whether next keeps playing the film current plays, at its rate: the same timeline, told again. */
function isSameTimeline(current: GroupView | undefined, next: GroupView): current is GroupView {
  return (
    current?.state === "Playing" &&
    next.state === "Playing" &&
    current.fileId === next.fileId &&
    current.rate === next.rate
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
    : expectedPositionMs(group, {
        receivedAtMs: snapshot.groupReceivedAtMs,
        nowMs,
        transitMs: (snapshot.groupPlacedByRttMs ?? snapshot.minimumRttMs ?? 0) / 2,
      });
}

/** The snapshot after a group message: a new timeline, or the one it follows told again. */
function afterGroup(current: TogetherSnapshot, group: GroupView, nowMs: number): TogetherSnapshot {
  if (isSameTimeline(current.group, group)) {
    // News of a group that plays on as it was, "continuing without" someone, moves no one: the
    // timeline stays the one first heard, so each message's own delay on the way does not nudge
    // every player. Re-based on each, a host on a tethered link was nudged every few seconds
    // while a viewer kept catching up, and Firefox's sound drifted ahead of its picture
    // (staging, 2026-10-06).
    return { ...current, group: { ...group, positionMs: current.group.positionMs } };
  }
  return {
    ...current,
    group,
    groupReceivedAtMs: nowMs,
    groupPlacedByRttMs: undefined,
  };
}

/** The snapshot after a pong: its round trip measured, and the group placed anew by it (D264). */
function afterPong(
  current: TogetherSnapshot,
  message: Extract<TogetherServerMessage, { readonly type: "pong" }>,
  nowMs: number,
): TogetherSnapshot {
  const rttMs = Math.max(0, nowMs - message.tMono);
  const measured = { ...current, minimumRttMs: Math.min(rttMs, current.minimumRttMs ?? rttMs) };
  // A pong of the timeline followed places it anew when its round trip is the shortest yet:
  // its wait is bounded by what was measured, a group message's by nothing (D264).
  return message.group !== undefined &&
    isSameTimeline(current.group, message.group) &&
    rttMs < (current.groupPlacedByRttMs ?? Number.POSITIVE_INFINITY)
    ? {
        ...measured,
        group: { ...current.group, positionMs: message.group.positionMs },
        groupReceivedAtMs: nowMs,
        groupPlacedByRttMs: rttMs,
      }
    : measured;
}

/** The snapshot after one message from the server, and whether it was the welcome. */
function afterMessage(
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
        group: message.group,
        groupReceivedAtMs: nowMs,
        groupPlacedByRttMs: undefined,
        roster: message.roster,
        minimumRttMs: undefined,
        tuning: message.tuning,
      };
    case "group":
      return afterGroup(current, message.group, nowMs);
    case "roster":
      return { ...current, roster: message.roster };
    case "rejected":
      return { ...current, lastRejectionCode: message.code };
    case "pong":
      return afterPong(current, message, nowMs);
    case "ended":
      return { ...current, status: "ended", endReason: message.reason };
    case "rtc":
      return current; // voice signalling goes to its listeners, and changes nothing here
  }
}

/** The seq of the group as a message tells it, or undefined when it tells none. */
function seqOf(message: TogetherServerMessage): number | undefined {
  return message.type === "welcome" || message.type === "group" ? message.seq : undefined;
}

/**
 * The hello a device opens with, in its role (cut-a §5.1): one that plays says it can set its
 * player's rate; a voice device has no player.
 */
function helloText(options: TogetherConnectionOptions): string {
  const role = options.role ?? "playback";
  return JSON.stringify({
    v: togetherProtocolVersion,
    type: "hello",
    credential: options.credential,
    ...(options.sessionToken === undefined ? {} : { sessionToken: options.sessionToken }),
    role,
    ...(role === "voice" ? {} : { canSetRate: true }),
  });
}

/**
 * A connection that carries voice signalling between this device and another of the session's
 * (D193 §4), which the server relays to the device named.
 */
export interface RelaysVoiceSignals {
  /** Sends signal to the device toDeviceId; dropped while not connected. */
  readonly sendVoiceSignal: (toDeviceId: number, signal: VoiceSignal) => void;
  /** Calls listener with each signal another device sends this one; returns the unsubscribing. */
  readonly subscribeToVoiceSignals: (
    listener: (fromDeviceId: number, signal: VoiceSignal) => void,
  ) => () => void;
}

/**
 * A connection that can tell the group its player cannot keep up (D247 §8): apart from
 * TogetherConnection, which a group's follower needs, as only the page that plays the chunked
 * quality knows.
 */
export interface TellsCannotKeepUp {
  /**
   * Says, from now on, that even this device's lowest quality is beyond its link, or that it
   * keeps up again (undefined), which everyone in the session is told.
   */
  readonly setCannotKeepUp: (cannotKeepUp: CannotKeepUp | undefined) => void;
}

class SessionConnection implements TogetherConnection, TellsCannotKeepUp, RelaysVoiceSignals {
  readonly #options: TogetherConnectionOptions;
  readonly #listeners = new Set<() => void>();
  readonly #voiceSignalListeners = new Set<(fromDeviceId: number, signal: VoiceSignal) => void>();
  #snapshot = startingSnapshot;
  #seq = 0;
  #attempt = 0;
  #socket: TogetherSocket | undefined;
  #readPlayer: (() => PlayerState) | undefined;
  #cannotKeepUp: CannotKeepUp | undefined;
  #timers: ReturnType<typeof setInterval>[] = [];
  #reconnectTimer: ReturnType<typeof setTimeout> | undefined;
  /** Which socket is the current one: each open and reconnect makes a new one. */
  #generation = 0;
  /** Pings sent on the current socket since anything was last heard on it. */
  #unansweredPings = 0;
  /** When, on the monotonic clock, and how the last socket was lost, until a welcome again. */
  #lost: { readonly atMs: number; readonly why: "closed" | "silent" } | undefined;

  constructor(options: TogetherConnectionOptions) {
    this.#options = options;
  }

  readonly snapshot = (): TogetherSnapshot => this.#snapshot;

  readonly subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  readonly setPlayer = (read: (() => PlayerState) | undefined): void => {
    this.#readPlayer = read;
  };

  readonly setCannotKeepUp = (cannotKeepUp: CannotKeepUp | undefined): void => {
    this.#cannotKeepUp = cannotKeepUp;
    this.reportState();
  };

  readonly reportState = (): void => {
    const state = this.#readPlayer?.();
    if (state !== undefined) {
      this.#send({
        type: "state",
        positionMs: Math.max(0, Math.round(state.positionMs)),
        tMono: this.#now(),
        playing: state.isPlaying,
        buffering: state.isBuffering,
        rttMs: this.#snapshot.minimumRttMs ?? 0,
        ...(this.#cannotKeepUp === undefined
          ? {}
          : {
              cannotKeepUp: {
                lowestKbps: Math.round(this.#cannotKeepUp.lowestKbps),
                ...(this.#cannotKeepUp.linkKbps === undefined
                  ? {}
                  : { linkKbps: Math.round(this.#cannotKeepUp.linkKbps) }),
              },
            }),
      });
    }
  };

  readonly command = (command: GroupCommand): void => {
    const position =
      "positionMs" in command ? { positionMs: Math.max(0, Math.round(command.positionMs)) } : {};
    this.#send({ type: "command", ...command, ...position, basedOnSeq: this.#seq });
  };

  readonly note = (note: PlayerNote): void => {
    const whole = Object.fromEntries(
      Object.entries(note).map(([name, value]) => [
        name,
        typeof value === "number" && name !== "rate" ? Math.round(value) : value,
      ]),
    );
    this.#send({ type: "note", note: whole });
  };

  readonly sendVoiceSignal = (toDeviceId: number, signal: VoiceSignal): void => {
    this.#send({ type: "rtc", toDeviceId, ...signal });
  };

  readonly subscribeToVoiceSignals = (
    listener: (fromDeviceId: number, signal: VoiceSignal) => void,
  ): (() => void) => {
    this.#voiceSignalListeners.add(listener);
    return () => this.#voiceSignalListeners.delete(listener);
  };

  readonly open = (): void => {
    clearTimeout(this.#reconnectTimer);
    this.#stopTimers();
    this.#socket?.close();
    this.#change(startingSnapshot);
    this.#attempt = 0;
    this.#lost = undefined; // a fresh connection, not one connecting again
    this.#connect();
  };

  readonly close = (): void => {
    this.#generation += 1;
    clearTimeout(this.#reconnectTimer);
    this.#stopTimers();
    this.#change({ ...this.#snapshot, status: "ended" });
    this.#socket?.close();
  };

  #now(): number {
    return Math.round(this.#options.monotonicMs());
  }

  #change(next: TogetherSnapshot): void {
    this.#snapshot = next;
    for (const listener of this.#listeners) {
      listener();
    }
  }

  #send(message: Readonly<Record<string, unknown>>): void {
    if (this.#snapshot.status === "connected") {
      this.#socket?.send(JSON.stringify({ v: togetherProtocolVersion, ...message }));
    }
  }

  readonly #ping = (): void => {
    if (this.#unansweredPings >= deadAfterUnansweredPings) {
      this.#dropDeadSocket();
      return;
    }
    this.#unansweredPings += 1;
    this.#send({ type: "ping", tMono: Math.max(1, this.#now()) });
  };

  /**
   * After a message: a new timeline is pinged for at once, so a measured round trip places it
   * within one; a pong that placed the timeline anew is noted, with how far it moved it (D264).
   */
  #tellPlacing(message: TogetherServerMessage, before: TogetherSnapshot, nowMs: number): void {
    const after = this.#snapshot;
    // A voice device plays nothing, so places nothing: its placings filled the log beside its
    // player's (the acceptance's log, 2026-10-07).
    if (after.group?.state !== "Playing" || this.#options.role === "voice") {
      return;
    }
    if (message.type !== "pong") {
      if (after.groupReceivedAtMs !== before.groupReceivedAtMs) {
        this.#ping();
      }
      return;
    }
    if (after.groupPlacedByRttMs !== before.groupPlacedByRttMs) {
      const wasMs = expectedGroupPositionMs(before, nowMs) ?? 0;
      const isMs = expectedGroupPositionMs(after, nowMs) ?? 0;
      this.note({
        kind: "placed",
        roundTripMs: Math.round(after.groupPlacedByRttMs ?? 0),
        movedMs: Math.round(isMs - wasMs),
      });
    }
  }

  /** Gives up on a socket nothing is heard on, as if it closed: it connects again after a wait. */
  #dropDeadSocket(): void {
    this.#generation += 1; // its close, whenever it comes, is not heard
    this.#socket?.close();
    this.#reconnectLater("silent");
  }

  #stopTimers(): void {
    for (const timer of this.#timers) {
      clearInterval(timer);
    }
    this.#timers = [];
  }

  #welcomed(): void {
    this.#attempt = 0;
    this.#ping();
    this.reportState();
    const lost = this.#lost;
    this.#lost = undefined;
    if (lost !== undefined) {
      this.note({ kind: "reconnected", offlineMs: this.#now() - lost.atMs, why: lost.why });
    }
    this.#stopTimers();
    this.#timers = [
      setInterval(this.#ping, pingEveryMs),
      setInterval(this.reportState, stateEveryMs),
    ];
  }

  #hear(text: string): void {
    this.#unansweredPings = 0;
    const message = parseTogetherMessage(text);
    if (message === undefined || this.#snapshot.status === "ended") {
      return;
    }
    if (message.type === "rtc") {
      for (const listener of this.#voiceSignalListeners) {
        listener(message.fromDeviceId, message.signal);
      }
      return;
    }
    this.#seq = seqOf(message) ?? this.#seq;
    const before = this.#snapshot;
    const nowMs = this.#now();
    this.#change(afterMessage(before, message, nowMs));
    this.#tellPlacing(message, before, nowMs);
    if (message.type === "welcome") {
      this.#welcomed();
    } else if (message.type === "ended") {
      this.#stopTimers();
      this.#socket?.close();
    }
  }

  /** The socket closed: the session is gone, or the device connects again after a wait. */
  #closed(code: number, wasWelcomed: boolean): void {
    this.#stopTimers();
    if (this.#snapshot.status === "ended") {
      return;
    }
    if (!wasWelcomed && code === policyViolationCode) {
      this.#change({ ...this.#snapshot, status: "ended", endReason: "gone" });
      return;
    }
    this.#reconnectLater("closed");
  }

  /** Says it is connecting again, and does after a wait that grows with each failure. */
  #reconnectLater(why: "closed" | "silent"): void {
    this.#lost ??= { atMs: this.#now(), why };
    this.#stopTimers();
    this.#change({ ...this.#snapshot, status: "reconnecting" });
    this.#reconnectTimer = setTimeout(() => {
      this.#connect();
    }, reconnectWaitMs(this.#attempt));
    this.#attempt += 1;
  }

  /** Opens a socket; what an earlier one, closed since, still says is not heard. */
  #connect(): void {
    this.#generation += 1;
    this.#unansweredPings = 0;
    const generation = this.#generation;
    const isCurrent = (): boolean => generation === this.#generation;
    let wasWelcomed = false;
    this.#socket = this.#options.openSocket({
      onOpen: () => {
        if (isCurrent()) {
          this.#socket?.send(helloText(this.#options));
        }
      },
      onMessage: (text) => {
        if (isCurrent()) {
          this.#hear(text);
          wasWelcomed ||= this.#snapshot.status === "connected";
        }
      },
      onClose: (code) => {
        if (isCurrent()) {
          this.#closed(code, wasWelcomed);
        }
      },
    });
  }
}

/**
 * A device's connection to its session in its role, which connects when opened: says hello, keeps
 * the group, the roster and the shortest round trip, pings, tells the group where the player is on
 * every change and every minute, relays voice signalling, and connects again with a growing wait
 * after a drop, until the session ends for it or it is closed. A hello the server refuses before
 * any welcome ends it as "gone". Every number it sends is whole, as the server reads them.
 */
export function createTogetherConnection(
  options: TogetherConnectionOptions,
): TogetherConnection & TellsCannotKeepUp & RelaysVoiceSignals {
  return new SessionConnection(options);
}
