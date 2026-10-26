// SPDX-License-Identifier: MPL-2.0

/**
 * A signed-in page's live connection (D337 §2; plans/cast-to-device.md §3): a second socket to the
 * session socket's address, opened with a hello of its own, so it travels wherever that one does.
 * The core keeps which of a person's devices are connected now by it, tells each of their pages when
 * that changes, and hands a device a cast to receive on it.
 */

import { deadAfterUnansweredPings, pingEveryMs, reconnectWaitMs } from "./together-connection.ts";
import { recordOf, type OpenTogetherSocket, type TogetherSocket } from "./together.ts";

/** A cast handed to this device to receive: its session of one, the film, and who cast it. */
export interface CastToReceive {
  readonly castId: number;
  /** The cast session's credential, which the receiver's session socket says hello with (D88). */
  readonly membershipCredential: string;
  readonly fileId: number;
  /** Where the sender stood, in ms; undefined to start where the person left the film. */
  readonly positionMs: number | undefined;
  readonly fromDeviceId: number;
  readonly fromDeviceName: string;
}

/** What the live connection knows now. */
export interface LiveSnapshot {
  /** Whether it is connected now: told the devices once at least since it last opened a socket. */
  readonly isConnected: boolean;
  /** The person's devices connected now, this one among them, by id; empty until told. */
  readonly deviceIds: readonly number[];
  /** Those of deviceIds with an Orpheum tab in front, which a cast can go to (D338 §2). */
  readonly inFrontDeviceIds: readonly number[];
  /** The last cast handed to this device, until it is taken (takeReceive). */
  readonly receive: CastToReceive | undefined;
  /** Whether the server refused the sign-in, after which it connects no more. */
  readonly isRefused: boolean;
}

/** A page's live connection. */
export interface LiveConnection {
  readonly snapshot: () => LiveSnapshot;
  /** Calls listener whenever the snapshot changes; returns the unsubscribing. */
  readonly subscribe: (listener: () => void) => () => void;
  /** Says the cast handed over has been taken, so it is not taken again. */
  readonly takeReceive: () => void;
  /** Tells the server the page's tab came to the front, or went behind (D338 §2). */
  readonly showFront: (isInFront: boolean) => void;
  readonly open: () => void;
  readonly close: () => void;
}

/** What a live connection needs. */
export interface LiveConnectionOptions {
  /** The page's session token, sent in the hello, never in the address (D154). */
  readonly sessionToken: string;
  /** The web client's script the page runs, said in the hello for the server's log. */
  readonly client?: string | undefined;
  readonly openSocket: OpenTogetherSocket;
  readonly monotonicMs: () => number;
  /** Whether the page's tab is in front now, said in each hello (D338 §2); in front when not given. */
  readonly isInFront?: (() => boolean) | undefined;
}

/** The WebSocket close code the server sends when it refuses the hello: a sign-in it does not know. */
const policyViolationCode = 1008;

const startingSnapshot: LiveSnapshot = {
  isConnected: false,
  deviceIds: [],
  inFrontDeviceIds: [],
  receive: undefined,
  isRefused: false,
};

/** A record's number field, or undefined. */
function numberIn(record: Readonly<Record<string, unknown>>, name: string): number | undefined {
  const value = record[name];
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

/** A receive message's cast, or undefined for one malformed. */
function parseReceive(value: unknown): CastToReceive | undefined {
  const record = recordOf(value);
  if (record === undefined) {
    return undefined;
  }
  const castId = numberIn(record, "castId");
  const fileId = numberIn(record, "fileId");
  const fromDeviceId = numberIn(record, "fromDeviceId");
  const credential = record["membershipCredential"];
  const name = record["fromDeviceName"];
  if (
    castId === undefined ||
    fileId === undefined ||
    fromDeviceId === undefined ||
    typeof credential !== "string" ||
    credential === ""
  ) {
    return undefined;
  }
  return {
    castId,
    membershipCredential: credential,
    fileId,
    positionMs: numberIn(record, "positionMs"),
    fromDeviceId,
    fromDeviceName: typeof name === "string" ? name : "",
  };
}

/** One message the server sends on a live connection, or undefined for one this client ignores. */
export type LiveMessage =
  | {
      readonly type: "devices";
      readonly deviceIds: readonly number[];
      readonly inFrontDeviceIds: readonly number[];
    }
  | { readonly type: "receive"; readonly receive: CastToReceive }
  | { readonly type: "pong" };

/** A list of ids as a message carries it; empty for none or one malformed. */
function idsIn(ids: unknown): number[] {
  return Array.isArray(ids) ? ids.filter((id): id is number => typeof id === "number") : [];
}

/** Reads one message the server sent; undefined for one malformed or unknown. */
export function parseLiveMessage(text: string): LiveMessage | undefined {
  let parsed: unknown;
  try {
    parsed = JSON.parse(text);
  } catch {
    return undefined;
  }
  const record = recordOf(parsed);
  if (record === undefined) {
    return undefined;
  }
  switch (record["type"]) {
    case "devices":
      return {
        type: "devices",
        deviceIds: idsIn(record["deviceIds"]),
        inFrontDeviceIds: idsIn(record["inFrontDeviceIds"]),
      };
    case "receive": {
      const receive = parseReceive(record["receive"]);
      return receive === undefined ? undefined : { type: "receive", receive };
    }
    case "pong":
      return { type: "pong" };
    default:
      return undefined;
  }
}

class PageLiveConnection implements LiveConnection {
  readonly #options: LiveConnectionOptions;
  readonly #listeners = new Set<() => void>();
  #snapshot = startingSnapshot;
  #socket: TogetherSocket | undefined;
  #generation = 0;
  #attempt = 0;
  #unansweredPings = 0;
  #pinger: ReturnType<typeof setInterval> | undefined;
  #reconnectTimer: ReturnType<typeof setTimeout> | undefined;
  #isClosed = true;
  /** Whether the current socket has said hello, after which a move of the tab is said on it. */
  #isGreeted = false;

  constructor(options: LiveConnectionOptions) {
    this.#options = options;
  }

  readonly snapshot = (): LiveSnapshot => this.#snapshot;

  readonly subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  readonly takeReceive = (): void => {
    if (this.#snapshot.receive !== undefined) {
      this.#change({ ...this.#snapshot, receive: undefined });
    }
  };

  readonly showFront = (isInFront: boolean): void => {
    if (this.#isGreeted) {
      this.#send({ type: "front", ...(isInFront ? {} : { behind: true }) });
    } // else the hello says it
  };

  readonly open = (): void => {
    this.#isClosed = false;
    this.#attempt = 0;
    this.#connect();
  };

  readonly close = (): void => {
    this.#isClosed = true;
    this.#generation += 1;
    clearTimeout(this.#reconnectTimer);
    clearInterval(this.#pinger);
    this.#socket?.close();
    this.#change({ ...this.#snapshot, isConnected: false });
  };

  #change(next: LiveSnapshot): void {
    this.#snapshot = next;
    for (const listener of this.#listeners) {
      listener();
    }
  }

  #send(message: Readonly<Record<string, unknown>>): void {
    this.#socket?.send(JSON.stringify(message));
  }

  readonly #ping = (): void => {
    if (this.#unansweredPings >= deadAfterUnansweredPings) {
      this.#generation += 1; // its close, whenever it comes, is not heard
      this.#socket?.close();
      this.#reconnectLater();
      return;
    }
    this.#unansweredPings += 1;
    this.#send({ type: "ping", tMono: Math.max(1, Math.round(this.#options.monotonicMs())) });
  };

  #hear(text: string): void {
    this.#unansweredPings = 0;
    const message = parseLiveMessage(text);
    if (message === undefined) {
      return;
    }
    switch (message.type) {
      case "devices":
        this.#attempt = 0;
        this.#change({
          ...this.#snapshot,
          isConnected: true,
          deviceIds: message.deviceIds,
          inFrontDeviceIds: message.inFrontDeviceIds,
        });
        return;
      case "receive":
        this.#change({ ...this.#snapshot, receive: message.receive });
        return;
      case "pong":
        return;
    }
  }

  #closed(code: number): void {
    clearInterval(this.#pinger);
    if (this.#isClosed) {
      return;
    }
    if (code === policyViolationCode) {
      this.#change({ ...this.#snapshot, isConnected: false, isRefused: true });
      return;
    }
    this.#reconnectLater();
  }

  #reconnectLater(): void {
    clearInterval(this.#pinger);
    this.#change({ ...this.#snapshot, isConnected: false });
    clearTimeout(this.#reconnectTimer);
    this.#reconnectTimer = setTimeout(() => {
      this.#connect();
    }, reconnectWaitMs(this.#attempt));
    this.#attempt += 1;
  }

  #connect(): void {
    this.#generation += 1;
    this.#isGreeted = false;
    this.#unansweredPings = 0;
    clearInterval(this.#pinger);
    this.#socket?.close();
    const generation = this.#generation;
    const isCurrent = (): boolean => generation === this.#generation;
    this.#socket = this.#options.openSocket({
      onOpen: () => {
        if (isCurrent()) {
          this.#send({
            type: "live",
            sessionToken: this.#options.sessionToken,
            ...(this.#options.client === undefined ? {} : { client: this.#options.client }),
            ...((this.#options.isInFront?.() ?? true) ? {} : { behind: true }),
          });
          this.#isGreeted = true;
          this.#pinger = setInterval(this.#ping, pingEveryMs);
        }
      },
      onMessage: (text) => {
        if (isCurrent()) {
          this.#hear(text);
        }
      },
      onClose: (code) => {
        if (isCurrent()) {
          this.#closed(code);
        }
      },
    });
  }
}

/**
 * A page's live connection, which connects when opened: says hello with the session token and
 * whether its tab is in front, says so again as that changes, keeps the person's connected devices,
 * those in front, and the last cast handed to this device, pings as the session
 * socket does, and connects again with a growing wait after a drop, until closed or until the server
 * refuses the sign-in.
 */
export function createLiveConnection(options: LiveConnectionOptions): LiveConnection {
  return new PageLiveConnection(options);
}
