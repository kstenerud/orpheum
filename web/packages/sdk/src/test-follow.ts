// SPDX-License-Identifier: MPL-2.0

/**
 * Stand-ins for following a group in tests: a video element and a session's connection, as a test
 * sets and reads them. Not exported by the package (index.ts).
 */

import { type GroupView, noRight, startingTuning, type SyncTuning } from "./together.ts";
import type {
  GroupCommand,
  PlayerNote,
  PlayerState,
  TogetherConnection,
  TogetherSnapshot,
} from "./together-connection.ts";

/** A stand-in document: shown until a test hides it, telling its listeners as it does. */
export class FakePage {
  visibilityState: DocumentVisibilityState = "visible";
  readonly #listeners = new Set<() => void>();

  addEventListener(_type: "visibilitychange", listener: () => void): void {
    this.#listeners.add(listener);
  }

  removeEventListener(_type: "visibilitychange", listener: () => void): void {
    this.#listeners.delete(listener);
  }

  hide(): void {
    this.show("hidden");
  }

  show(state: DocumentVisibilityState = "visible"): void {
    this.visibilityState = state;
    for (const listener of this.#listeners) {
      listener();
    }
  }
}

/** A stand-in video element: its position, play state and events, as a test sets them. */
export class FakeMedia {
  currentTime = 0;
  playbackRate = 1;
  /* eslint-disable @typescript-eslint/naming-convention -- HTMLMediaElement's own names, which this stands in for */
  muted = false;
  autoplay = false;
  preservesPitch = true;
  paused = true;
  ended = false;
  seeking = false;
  /* eslint-enable @typescript-eslint/naming-convention -- the fake's own names follow */
  readyState = 4;
  networkState = 2; // NETWORK_LOADING
  duration = 7200;
  /** What is loaded, as [start, end] in seconds: all of it unless a test says otherwise. */
  loaded: [number, number][] = [[0, 7200]];
  /** Each event's listeners, in the order they were added, as an element keeps them. */
  readonly listeners = new Map<string, ((event: Event) => void)[]>();

  get buffered(): Pick<TimeRanges, "length" | "start" | "end"> {
    const { loaded } = this;
    return {
      length: loaded.length,
      start: (index) => loaded[index]?.[0] ?? 0,
      end: (index) => loaded[index]?.[1] ?? 0,
    };
  }

  play(): Promise<void> {
    this.paused = false;
    return Promise.resolve();
  }

  pause(): void {
    this.paused = true;
  }

  addEventListener(type: string, listener: (event: Event) => void): void {
    const listeners = this.listeners.get(type) ?? [];
    if (!listeners.includes(listener)) {
      this.listeners.set(type, [...listeners, listener]);
    }
  }

  removeEventListener(type: string, listener: (event: Event) => void): void {
    this.listeners.set(
      type,
      (this.listeners.get(type) ?? []).filter((each) => each !== listener),
    );
  }

  /** The browser firing an event at the element. */
  fire(type: string, event: Event = new Event(type)): void {
    for (const listener of this.listeners.get(type) ?? []) {
      listener(event);
    }
  }

  /** The events fired at the element by its page, by type. */
  readonly dispatched: string[] = [];

  dispatchEvent(event: Event): boolean {
    this.dispatched.push(event.type);
    this.fire(event.type, event);
    return true;
  }
}

/** What a stand-in connection was given: the commands, reports and notes, in order. */
interface Given {
  readonly commands: GroupCommand[];
  readonly reports: PlayerState[];
  readonly notes: PlayerNote[];
}

/** The snapshot a stand-in connection starts with: connected, its group as given. */
function snapshotOf(group: GroupView, tuning: SyncTuning, minimumRttMs?: number): TogetherSnapshot {
  return {
    status: "connected",
    memberId: 1,
    isHost: false,
    rights: noRight,
    group,
    groupReceivedAtMs: 0,
    groupPlacedByRttMs: undefined,
    roster: [],
    endReason: undefined,
    minimumRttMs,
    lastRejectionCode: undefined,
    answersHeard: 0,
    tuning,
  };
}

/** A stand-in connection whose group a test sets, keeping the commands and reports it is given. */
export function fakeConnection(
  group: GroupView,
  tuning: SyncTuning = startingTuning,
  minimumRttMs?: number,
): Given & {
  connection: TogetherConnection;
  setGroup: (next: GroupView, receivedAtMs: number, placedByRttMs?: number) => void;
  /** The connection changing with no answer to a command, as a pong or a roster does. */
  heardNoAnswer: () => void;
} {
  let snapshot = snapshotOf(group, tuning, minimumRttMs);
  let listener = (): void => undefined;
  let readPlayer: (() => PlayerState) | undefined;
  const given: Given = { commands: [], reports: [], notes: [] };
  return {
    ...given,
    setGroup: (next, receivedAtMs, placedByRttMs) => {
      snapshot = {
        ...snapshot,
        group: next,
        groupReceivedAtMs: receivedAtMs,
        groupPlacedByRttMs: placedByRttMs,
        groupSeq: (snapshot.groupSeq ?? 0) + 1,
        answersHeard: snapshot.answersHeard + 1,
      };
      listener();
    },
    heardNoAnswer: () => {
      listener();
    },
    connection: {
      snapshot: () => snapshot,
      subscribe: (heard) => {
        listener = heard;
        return () => undefined;
      },
      setPlayer: (read) => {
        readPlayer = read;
      },
      reportState: () => {
        const state = readPlayer?.();
        if (state !== undefined) {
          given.reports.push(state);
        }
      },
      command: (command) => {
        given.commands.push(command);
      },
      note: (note) => {
        given.notes.push(note);
      },
      open: () => undefined,
      close: () => undefined,
    },
  };
}

export const paused: GroupView = {
  state: "Paused",
  positionMs: 60_000,
  rate: 1,
  fileId: 5,
  event: undefined,
  seekedFromMs: 0,
  byMemberId: undefined,
};
