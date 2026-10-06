// SPDX-License-Identifier: MPL-2.0

import {
  parseTogetherMessage,
  togetherProtocolVersion,
  type CannotKeepUp,
  type GroupView,
  type OpenTogetherSocket,
  type RosterMember,
  type TogetherEndReason,
  type TogetherServerMessage,
  type TogetherSocket,
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
  readonly isHost: boolean;
  readonly group: GroupView | undefined;
  /** The monotonic time at which group arrived: a playing group has moved on from it since. */
  readonly groupReceivedAtMs: number;
  readonly roster: readonly RosterMember[];
  /** Why it ended, when the server ended it; undefined while it goes on or when closed here. */
  readonly endReason: TogetherEndReason | undefined;
  /** The shortest round trip a ping has measured on this connection, in ms. */
  readonly minimumRttMs: number | undefined;
  /** The last command the server refused, by its code (cut-a §5.2). */
  readonly lastRejectionCode: string | undefined;
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
  readonly openSocket: OpenTogetherSocket;
  /** A monotonic clock in ms (performance.now): no clock is shared with the server (D66). */
  readonly monotonicMs: () => number;
}

const startingSnapshot: TogetherSnapshot = {
  status: "connecting",
  memberId: undefined,
  isHost: false,
  group: undefined,
  groupReceivedAtMs: 0,
  roster: [],
  endReason: undefined,
  minimumRttMs: undefined,
  lastRejectionCode: undefined,
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
        isHost: message.isHost,
        group: message.group,
        groupReceivedAtMs: nowMs,
        roster: message.roster,
        minimumRttMs: undefined,
      };
    case "group":
      if (isSameTimeline(current.group, message.group)) {
        // News of a group that plays on as it was, "continuing without" someone, moves no one: the
        // timeline stays the one first heard, so each message's own delay on the way does not nudge
        // every player. Re-based on each, a host on a tethered link was nudged every few seconds
        // while a viewer kept catching up, and Firefox's sound drifted ahead of its picture
        // (staging, 2026-10-06).
        return { ...current, group: { ...message.group, positionMs: current.group.positionMs } };
      }
      return { ...current, group: message.group, groupReceivedAtMs: nowMs };
    case "roster":
      return { ...current, roster: message.roster };
    case "rejected":
      return { ...current, lastRejectionCode: message.code };
    case "pong": {
      const rttMs = Math.max(0, nowMs - message.tMono);
      return { ...current, minimumRttMs: Math.min(rttMs, current.minimumRttMs ?? rttMs) };
    }
    case "ended":
      return { ...current, status: "ended", endReason: message.reason };
  }
}

/** The seq of the group as a message tells it, or undefined when it tells none. */
function seqOf(message: TogetherServerMessage): number | undefined {
  return message.type === "welcome" || message.type === "group" ? message.seq : undefined;
}

/** The hello a playback device opens with (cut-a §5.1). */
function helloText(options: TogetherConnectionOptions): string {
  return JSON.stringify({
    v: togetherProtocolVersion,
    type: "hello",
    credential: options.credential,
    ...(options.sessionToken === undefined ? {} : { sessionToken: options.sessionToken }),
    role: "playback",
    canSetRate: true,
  });
}

/** One device's connection, as connectTogether describes it. */
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

class SessionConnection implements TogetherConnection, TellsCannotKeepUp {
  readonly #options: TogetherConnectionOptions;
  readonly #listeners = new Set<() => void>();
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

  readonly open = (): void => {
    clearTimeout(this.#reconnectTimer);
    this.#stopTimers();
    this.#socket?.close();
    this.#change(startingSnapshot);
    this.#attempt = 0;
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

  /** Gives up on a socket nothing is heard on, as if it closed: it connects again after a wait. */
  #dropDeadSocket(): void {
    this.#generation += 1; // its close, whenever it comes, is not heard
    this.#socket?.close();
    this.#reconnectLater();
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
    this.#seq = seqOf(message) ?? this.#seq;
    this.#change(afterMessage(this.#snapshot, message, this.#now()));
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
    this.#reconnectLater();
  }

  /** Says it is connecting again, and does after a wait that grows with each failure. */
  #reconnectLater(): void {
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
 * A device's connection to its session as a playback device, which connects when opened: says hello, keeps the group, the roster
 * and the shortest round trip, pings, tells the group where the player is on every change and
 * every minute, and connects again with a growing wait after a drop, until the session ends for it
 * or it is closed. A hello the server refuses before any welcome ends it as "gone". Every number
 * it sends is whole, as the server reads them.
 */
export function createTogetherConnection(
  options: TogetherConnectionOptions,
): TogetherConnection & TellsCannotKeepUp {
  return new SessionConnection(options);
}
