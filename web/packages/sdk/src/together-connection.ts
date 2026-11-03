// SPDX-License-Identifier: MPL-2.0

import type { SoundChoice } from "./playback-sound.ts";
import { unreachedRetryMs } from "./rtc-reopen.ts";
import { ShownCheck } from "./shown-check.ts";
import { whenPageChanges } from "./shown-page.ts";
import { expectedPositionMs } from "./together-drift.ts";
import { stateFlagsOf, type PlayerNote, type PlayerState } from "./together-notes.ts";
import {
  afterMessage,
  expectedGroupPositionMs,
  helloText,
  isSameTimeline,
  seqOf,
  startingSnapshot,
  wholePlayback,
  type ConnectionStatus,
  type TogetherConnectionOptions,
  type TogetherSnapshot,
} from "./together-connection-snapshot.ts";
import {
  parseTogetherMessage,
  togetherProtocolVersion,
  type CannotKeepUp,
  type Standing,
  type TogetherServerMessage,
  type TogetherSocket,
  type VoiceSignal,
} from "./together.ts";

export { expectedGroupPositionMs };
export type { ConnectionStatus, TogetherConnectionOptions, TogetherSnapshot };

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
 * How many pings a device sends a second apart once a new timeline arrives, so a short round trip
 * places it early (D268 §1, as Jellyfin pings each second three times before it pings each minute).
 */
export const youngPings = 3;
export const youngPingEveryMs = 1000;

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

/**
 * The wait before the next try to connect after one failed, and the attempt count the wait after
 * that grows from. A try whose socket never opened reached no server: the network is away, and the
 * next try, after unreachedRetryMs, is how the page hears it back where the browser says nothing
 * (Firefox on the desktop fires no `online` event), the growing wait starting over. Any other failure
 * grows the wait.
 */
export function nextReconnect(
  attempt: number,
  wasUnreached: boolean,
): { readonly waitMs: number; readonly attempt: number } {
  return wasUnreached
    ? { waitMs: unreachedRetryMs, attempt: 0 }
    : { waitMs: reconnectWaitMs(attempt), attempt: attempt + 1 };
}

export type { PlayerNote, PlayerState, Press, VoiceCallStep } from "./together-notes.ts";

/** A command only the host sends (D246 §8): play, pause, seek, put a film on, or stop. */
export type GroupCommand =
  | { readonly command: "play" }
  | { readonly command: "pause" }
  | { readonly command: "seek"; readonly positionMs: number }
  | { readonly command: "changeItem"; readonly fileId: number; readonly positionMs: number }
  | { readonly command: "stop" };

/** A device's connection to its session (cut-a §5.1). */
export interface TogetherConnection {
  readonly snapshot: () => TogetherSnapshot;
  /** Calls listener whenever the snapshot changes; returns the unsubscribing. */
  readonly subscribe: (listener: () => void) => () => void;
  /** Names where the player's state is read from, or that there is no player now. */
  readonly setPlayer: (read: (() => PlayerState) | undefined) => void;
  /** Tells the group where the player is now: on every change of it. */
  readonly reportState: () => void;
  /**
   * Sends the host's command, based on the group as last told; false, and nothing sent, while not
   * connected (it is not held for the reconnect, which could play it long after the press).
   */
  readonly command: (command: GroupCommand) => boolean;
  /** Tells the server one thing its player did, for its log; dropped while not connected. */
  readonly note: (note: PlayerNote) => void;
  /** Connects, from the start: after creating it, or again after closing it. */
  readonly open: () => void;
  /** Closes the connection, and connects no more until opened again. */
  readonly close: () => void;
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
 * A connection that carries a person's choices of their film's sound from a device of theirs with no
 * player, a cast's remote, to their devices that play it, which carry each out as their own press
 * (playback-sound.ts); the server relays them to the person's own devices alone.
 */
export interface CarriesSoundChoices {
  /** Sends choice to the person's devices that play the film; dropped while not connected. */
  readonly chooseSound: (choice: SoundChoice) => void;
  /** Calls listener with each choice made on another of the person's devices; returns the unsubscribing. */
  readonly subscribeToSoundChoices: (listener: (choice: SoundChoice) => void) => () => void;
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

/**
 * A connection that can tell the session what its page waits on before its film can play, its
 * person's speakers or a tap, which everyone's roster shows (Standing): apart from
 * TogetherConnection, as only a viewer's page asks either.
 */
export interface TellsStanding {
  /** Says, from now on, what the page waits on; "" once it waits on nothing. */
  readonly setStanding: (standing: Standing) => void;
}

/**
 * How a socket was lost, and since when: it closed, nothing was heard on it for its pings, nothing
 * within shownAnswerWithinMs of its page shown again, or its page was left.
 */
interface Lost {
  readonly atMs: number;
  readonly why: "closed" | "silent" | "shown" | "back" | "left";
}

class SessionConnection
  implements
    TogetherConnection,
    TellsCannotKeepUp,
    TellsStanding,
    RelaysVoiceSignals,
    CarriesSoundChoices
{
  readonly #options: TogetherConnectionOptions;
  readonly #listeners = new Set<() => void>();
  readonly #voiceSignalListeners = new Set<(fromDeviceId: number, signal: VoiceSignal) => void>();
  readonly #soundChoiceListeners = new Set<(choice: SoundChoice) => void>();
  #snapshot = startingSnapshot;
  #seq = 0;
  #attempt = 0;
  #socket: TogetherSocket | undefined;
  #readPlayer: (() => PlayerState) | undefined;
  #cannotKeepUp: CannotKeepUp | undefined;
  #standing: Standing = "";
  #timers: ReturnType<typeof setInterval>[] = [];
  #reconnectTimer: ReturnType<typeof setTimeout> | undefined;
  /** Which socket is the current one: each open and reconnect makes a new one. */
  #generation = 0;
  /** How many of a young timeline's pings are still to be sent (#pingYoung). */
  #youngPingsLeft = 0;
  /** Pings sent on the current socket since anything was last heard on it. */
  #unansweredPings = 0;
  /** When, on the monotonic clock, and how the last socket was lost, until a welcome again. */
  #lost: Lost | undefined;
  /** The check of the socket as the page is shown again, whose silence drops it as dead. */
  readonly #shownCheck = new ShownCheck({
    nowMs: () => this.#now(),
    ping: (tMono) => {
      this.#send({ type: "ping", tMono });
    },
    alive: (roundTripMs) => {
      this.note({ kind: "shownAgain", why: "alive", roundTripMs });
    },
    dead: (atMs, why) => {
      this.#dropDeadSocket({ atMs, why });
    },
  });
  /** Ends hearing the page hidden and shown, left and returned, and the path back; undefined while closed. */
  #stopHearingPage: (() => void) | undefined;
  /** Whether its page was left, its socket given up, until it returns from the back/forward cache. */
  #isLeft = false;

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

  readonly setStanding = (standing: Standing): void => {
    if (standing !== this.#standing) {
      this.#standing = standing;
      this.#send({ type: "standing", standing });
    }
  };

  readonly reportState = (): void => {
    const state = this.#readPlayer?.();
    if (state !== undefined) {
      this.#send({
        type: "state",
        fileId: state.fileId,
        positionMs: Math.max(0, Math.round(state.positionMs)),
        tMono: this.#now(),
        playing: state.isPlaying,
        buffering: state.isBuffering,
        rttMs: this.#snapshot.minimumRttMs ?? 0,
        ...(state.readyOnMs === undefined ? {} : { readyOnMs: state.readyOnMs }),
        ...(state.loadsFaster === undefined ? {} : { loadsFaster: state.loadsFaster }),
        ...(state.playback === undefined ? {} : { playback: wholePlayback(state.playback) }),
        ...stateFlagsOf(state),
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

  readonly command = (command: GroupCommand): boolean => {
    const position =
      "positionMs" in command ? { positionMs: Math.max(0, Math.round(command.positionMs)) } : {};
    return this.#send({ type: "command", ...command, ...position, basedOnSeq: this.#seq });
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

  readonly chooseSound = (choice: SoundChoice): void => {
    this.#send({ type: "sound", sound: choice });
  };

  readonly subscribeToSoundChoices = (listener: (choice: SoundChoice) => void): (() => void) => {
    this.#soundChoiceListeners.add(listener);
    return () => this.#soundChoiceListeners.delete(listener);
  };

  readonly subscribeToVoiceSignals = (
    listener: (fromDeviceId: number, signal: VoiceSignal) => void,
  ): (() => void) => {
    this.#voiceSignalListeners.add(listener);
    return () => this.#voiceSignalListeners.delete(listener);
  };

  readonly open = (): void => {
    this.#cancelReconnect();
    this.#stopTimers();
    this.#socket?.close();
    this.#change(startingSnapshot);
    this.#attempt = 0;
    this.#lost = undefined; // a fresh connection, not one connecting again
    this.#isLeft = false;
    this.#stopHearingPage?.();
    this.#stopHearingPage = whenPageChanges(this.#options, {
      shownAgain: (why) => {
        this.#shownAgain(why);
      },
      left: () => {
        this.#leftPage();
      },
      returned: () => {
        this.#returned();
      },
    });
    this.#connect();
  };

  readonly close = (): void => {
    this.#generation += 1;
    this.#cancelReconnect();
    this.#stopHearingPage?.();
    this.#stopHearingPage = undefined;
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

  /** Sends message while connected; whether it went. */
  #send(message: Readonly<Record<string, unknown>>): boolean {
    if (this.#snapshot.status !== "connected") {
      return false;
    }
    this.#socket?.send(JSON.stringify({ v: togetherProtocolVersion, ...message }));
    return true;
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
   * One of a young timeline's pings, while any are left: only once the last is answered, so pings a
   * second apart on a link slower than that never count as unanswered (D268 §1).
   */
  readonly #pingYoung = (): void => {
    if (this.#youngPingsLeft > 0 && this.#unansweredPings === 0) {
      this.#youngPingsLeft -= 1;
      this.#ping();
    }
  };

  /**
   * After a message: a new timeline is pinged for at once and youngPings times in all, so a
   * measured round trip places it early; each pong of the timeline is noted, with where it said
   * the group is against where it was expected, and whether it placed it anew (D264, D268 §1).
   */
  #tellPlacing(message: TogetherServerMessage, before: TogetherSnapshot, nowMs: number): void {
    const after = this.#snapshot;
    // A voice device plays nothing, so places nothing: its placings filled the log beside its
    // player's (the acceptance's log, 2026-10-07).
    if (after.group?.state !== "Playing" || this.#options.role === "panel") {
      return;
    }
    if (message.type !== "pong") {
      if (after.groupReceivedAtMs !== before.groupReceivedAtMs) {
        this.#youngPingsLeft = youngPings;
        this.#pingYoung();
      }
      return;
    }
    if (message.group === undefined || !isSameTimeline(before.group, message.group)) {
      return;
    }
    const rttMs = Math.max(0, nowMs - message.tMono);
    const saysMs = expectedPositionMs(message.group, {
      receivedAtMs: nowMs,
      nowMs,
      transitMs: rttMs / 2,
    });
    this.note({
      kind: "ponged",
      roundTripMs: Math.round(rttMs),
      aheadOfExpectedMs: Math.round(saysMs - (expectedGroupPositionMs(before, nowMs) ?? saysMs)),
      why: after.groupReceivedAtMs === before.groupReceivedAtMs ? "kept" : "placed",
    });
  }

  /**
   * Gives up on a socket nothing is heard on, as if it closed: it connects again after a wait, lost
   * as its pings found it, or as its page shown again found it.
   */
  #dropDeadSocket(lost: Lost = { atMs: this.#now(), why: "silent" }): void {
    this.#generation += 1; // its close, whenever it comes, is not heard
    this.#socket?.close();
    this.#reconnectLater(lost);
  }

  /**
   * The page was shown again, where a lock, a sleep or the background may have killed its socket
   * unseen, or the way to the core may work again (pathBack), where a lost network may have: one waiting to connect again connects now, and a connected one is pinged, and dropped
   * as dead unless it answers within shownAnswerWithinMs, the one way a socket is dropped as dead
   * (#dropDeadSocket), sooner than its pings would.
   */
  #shownAgain(why: "shown" | "back"): void {
    if (this.#isLeft) {
      return; // its return from the back/forward cache connects it
    }
    if (this.#snapshot.status === "reconnecting" && this.#reconnectTimer !== undefined) {
      this.#cancelReconnect();
      this.#connect();
      return;
    }
    if (this.#snapshot.status === "connected") {
      this.#shownCheck.start(why);
    }
  }

  /**
   * The page was left, perhaps into the back/forward cache, where its socket would stay open and the
   * core count the device present, leading, until its silence limit: the socket is closed at once, as
   * closing the tab closes it, so the lead passes (D372 §5), and nothing connects until it returns.
   */
  #leftPage(): void {
    if (this.#snapshot.status === "ended" || this.#isLeft) {
      return;
    }
    this.#isLeft = true;
    this.#generation += 1; // its close, whenever it comes, is not heard
    this.#cancelReconnect();
    this.#stopTimers();
    this.#socket?.close();
    this.#socket = undefined;
    this.#lost ??= { atMs: this.#now(), why: "left" };
    this.#change({ ...this.#snapshot, status: "reconnecting" });
  }

  /** The page came back from the back/forward cache: it connects again at once, if it was left. */
  #returned(): void {
    if (!this.#isLeft) {
      return;
    }
    this.#isLeft = false;
    this.#attempt = 0;
    this.#connect();
  }

  #stopTimers(): void {
    for (const timer of this.#timers) {
      clearInterval(timer);
    }
    this.#timers = [];
    this.#shownCheck.stop();
  }

  /** Stops a wait to connect again, if one is under way. */
  #cancelReconnect(): void {
    clearTimeout(this.#reconnectTimer);
    this.#reconnectTimer = undefined;
  }

  #welcomed(): void {
    this.#attempt = 0;
    this.#ping();
    this.reportState();
    // A new connection is a new device to the server, which knows nothing of what the page waits on.
    if (this.#standing !== "") {
      this.#send({ type: "standing", standing: this.#standing });
    }
    const lost = this.#lost;
    this.#lost = undefined;
    if (lost !== undefined) {
      this.note({ kind: "reconnected", offlineMs: this.#now() - lost.atMs, why: lost.why });
    }
    this.#stopTimers();
    this.#timers = [
      setInterval(this.#ping, pingEveryMs),
      setInterval(this.#pingYoung, youngPingEveryMs),
      setInterval(this.reportState, stateEveryMs),
    ];
  }

  #hear(text: string): void {
    this.#unansweredPings = 0;
    const message = parseTogetherMessage(text);
    if (message === undefined || this.#snapshot.status === "ended") {
      return;
    }
    if (this.#handedOn(message)) {
      return;
    }
    if (message.type === "pong") {
      this.#shownCheck.heardPong(message.tMono);
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

  /**
   * Hands a message relayed from another device to its listeners, voice signalling or a sound choice,
   * which change nothing of the session; whether it was one.
   */
  #handedOn(message: TogetherServerMessage): boolean {
    if (message.type === "rtc") {
      for (const listener of this.#voiceSignalListeners) {
        listener(message.fromDeviceId, message.signal);
      }
      return true;
    }
    if (message.type === "sound") {
      for (const listener of this.#soundChoiceListeners) {
        listener(message.choice);
      }
      return true;
    }
    return false;
  }

  /** The socket closed: the session is gone, or the device connects again after a wait. */
  #closed(code: number, wasWelcomed: boolean, wasOpened: boolean): void {
    this.#stopTimers();
    if (this.#snapshot.status === "ended") {
      return;
    }
    if (!wasWelcomed && code === policyViolationCode) {
      this.#change({ ...this.#snapshot, status: "ended", endReason: "gone" });
      return;
    }
    this.#reconnectLater({ atMs: this.#now(), why: "closed" }, !wasOpened);
  }

  /**
   * Says it is connecting again, and does after a wait that grows with each failure (nextReconnect:
   * not after a socket that never opened); lost is how, kept from the first loss until a welcome.
   */
  #reconnectLater(lost: Lost, wasUnreached = false): void {
    this.#lost ??= lost;
    this.#stopTimers();
    this.#change({ ...this.#snapshot, status: "reconnecting" });
    const next = nextReconnect(this.#attempt, wasUnreached);
    this.#attempt = next.attempt;
    this.#reconnectTimer = setTimeout(() => {
      this.#reconnectTimer = undefined;
      this.#connect();
    }, next.waitMs);
  }

  /** Opens a socket; what an earlier one, closed since, still says is not heard. */
  #connect(): void {
    this.#generation += 1;
    this.#unansweredPings = 0;
    const generation = this.#generation;
    const isCurrent = (): boolean => generation === this.#generation;
    let wasWelcomed = false;
    let wasOpened = false;
    this.#socket = this.#options.openSocket({
      onOpen: () => {
        if (isCurrent()) {
          wasOpened = true;
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
          this.#closed(code, wasWelcomed, wasOpened);
        }
      },
    });
  }
}

/**
 * A device's connection to its session in its role, which connects when opened: says hello, keeps
 * the group, the roster and the shortest round trip, pings, tells the group where the player is on
 * every change and every minute, relays voice signalling, and connects again with a growing wait
 * after a drop, or at once as its page is shown again on a socket that does not answer within
 * shownAnswerWithinMs, until the session ends for it or it is closed. A hello the server refuses before
 * any welcome ends it as "gone". Every number it sends is whole, as the server reads them.
 */
export function createTogetherConnection(
  options: TogetherConnectionOptions,
): TogetherConnection &
  TellsCannotKeepUp &
  TellsStanding &
  RelaysVoiceSignals &
  CarriesSoundChoices {
  return new SessionConnection(options);
}
