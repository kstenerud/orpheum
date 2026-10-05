// SPDX-License-Identifier: MPL-2.0

import { apiPrefix } from "./client.ts";

/** The version of the session WebSocket's messages, every message's v (cut-a §5.1). */
export const togetherProtocolVersion = 2;

/** The group's play state (watch-together.md, Sync): Waiting is paused until every player is ready. */
export type GroupState = "Idle" | "Waiting" | "Paused" | "Playing";

/** The group as the server tells it: what plays, where, how fast, and the named event that made it so. */
export interface GroupView {
  readonly state: GroupState;
  readonly positionMs: number;
  readonly rate: number;
  /** What plays; 0 while nothing does, every screen black (D246 §5). */
  readonly fileId: number;
  /** The change, as the server names it: "paused", "seeked from 723000", "buffering" (D188). */
  readonly event: string | undefined;
  /** The member whose command or state caused the event; undefined when no one did. */
  readonly byMemberId: number | undefined;
}

/** One device of a member in the roster, and its health. */
export interface RosterDevice {
  readonly id: number;
  readonly role: string;
  readonly isBuffering: boolean;
}

/** One member in the roster: the name they chose, whether they host, and their devices. */
export interface RosterMember {
  readonly id: number;
  readonly name: string;
  readonly isHost: boolean;
  readonly devices: readonly RosterDevice[];
}

/**
 * Why a session ended for this device: the host ended it, the host removed this member, the
 * member left, or the server no longer knows the membership at all ("gone").
 */
export type TogetherEndReason = "hostEnded" | "ejected" | "left" | "gone";

/** A message the server sends, parsed (cut-a §5.1); undefined for one this client does not use. */
export type TogetherServerMessage =
  | {
      readonly type: "welcome";
      readonly seq: number;
      readonly memberId: number;
      readonly isHost: boolean;
      readonly group: GroupView;
      readonly roster: readonly RosterMember[];
    }
  | { readonly type: "group"; readonly seq: number; readonly group: GroupView }
  | { readonly type: "roster"; readonly roster: readonly RosterMember[] }
  | { readonly type: "rejected"; readonly basedOnSeq: number; readonly code: string }
  | { readonly type: "pong"; readonly tMono: number }
  | { readonly type: "ended"; readonly reason: TogetherEndReason };

/** A value read from outside, as a record of unknown fields; undefined when it is not an object. */
function recordOf(value: unknown): Readonly<Record<string, unknown>> | undefined {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? Object.fromEntries(Object.entries(value))
    : undefined;
}

/** A number field, or fallback when it is absent or not a number (the server omits zeros). */
function numberIn(record: Readonly<Record<string, unknown>>, key: string, fallback = 0): number {
  const value = record[key];
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

/** A string field, or undefined. */
function stringIn(record: Readonly<Record<string, unknown>>, key: string): string | undefined {
  const value = record[key];
  return typeof value === "string" ? value : undefined;
}

const groupStates: readonly GroupState[] = ["Idle", "Waiting", "Paused", "Playing"];

/** The group in a message, or undefined when it has none a client can follow. */
function parseGroup(value: unknown): GroupView | undefined {
  const record = recordOf(value);
  const state = groupStates.find((each) => each === record?.["state"]);
  if (record === undefined || state === undefined) {
    return undefined;
  }
  const byMemberId = numberIn(record, "byMemberId");
  return {
    state,
    positionMs: numberIn(record, "positionMs"),
    rate: numberIn(record, "rate", 1),
    fileId: numberIn(record, "fileId"),
    event: stringIn(record, "event"),
    byMemberId: byMemberId === 0 ? undefined : byMemberId,
  };
}

/** The roster in a message: every member it names well enough to show. */
function parseRoster(value: unknown): RosterMember[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.flatMap((each: unknown) => {
    const record = recordOf(each);
    const name = record === undefined ? undefined : stringIn(record, "name");
    if (record === undefined || name === undefined) {
      return [];
    }
    const devices = Array.isArray(record["devices"]) ? record["devices"] : [];
    return [
      {
        id: numberIn(record, "id"),
        name,
        isHost: record["isHost"] === true,
        devices: devices.flatMap((device: unknown) => {
          const fields = recordOf(device);
          return fields === undefined
            ? []
            : [
                {
                  id: numberIn(fields, "id"),
                  role: stringIn(fields, "role") ?? "",
                  isBuffering: fields["isBuffering"] === true,
                },
              ];
        }),
      },
    ];
  });
}

const endReasons: readonly TogetherEndReason[] = ["hostEnded", "ejected", "left"];

/** The parsed form of one of the messages a client follows the group by. */
function parseFollowed(
  record: Readonly<Record<string, unknown>>,
  group: GroupView | undefined,
): TogetherServerMessage | undefined {
  switch (record["type"]) {
    case "welcome":
      return group === undefined
        ? undefined
        : {
            type: "welcome",
            seq: numberIn(record, "seq"),
            memberId: numberIn(record, "memberId"),
            isHost: record["isHost"] === true,
            group,
            roster: parseRoster(record["roster"]),
          };
    case "group":
      return group === undefined
        ? undefined
        : { type: "group", seq: numberIn(record, "seq"), group };
    case "roster":
      return { type: "roster", roster: parseRoster(record["roster"]) };
    default:
      return undefined;
  }
}

/** One message the server sent, as text, parsed; undefined for one not understood or not used. */
export function parseTogetherMessage(text: string): TogetherServerMessage | undefined {
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
    case "rejected":
      return {
        type: "rejected",
        basedOnSeq: numberIn(record, "basedOnSeq"),
        code: stringIn(record, "code") ?? "",
      };
    case "pong":
      return { type: "pong", tMono: numberIn(record, "tMono") };
    case "ended": {
      const reason = endReasons.find((each) => each === record["reason"]) ?? "gone";
      return { type: "ended", reason };
    }
    default:
      return parseFollowed(record, parseGroup(record["group"]));
  }
}

/** What a socket's owner hears from it. */
export interface TogetherSocketListeners {
  readonly onOpen: () => void;
  readonly onMessage: (text: string) => void;
  /** The socket closed, with the close frame's code (1008: the server refused the hello). */
  readonly onClose: (code: number) => void;
}

/** An open session WebSocket, as the connection uses it. */
export interface TogetherSocket {
  readonly send: (text: string) => void;
  readonly close: () => void;
}

/** Opens a session WebSocket whose events go to listeners: the browser's, or a test's stand-in. */
export type OpenTogetherSocket = (listeners: TogetherSocketListeners) => TogetherSocket;

/**
 * The session WebSocket of the server at origin (GET /together/ws), over the browser's WebSocket:
 * wss: where the page is https:. Nothing secret is in its address; the credential goes in the
 * hello (D154).
 */
export function browserTogetherSocket(origin: string): OpenTogetherSocket {
  return (listeners) => {
    const address = new URL(`${apiPrefix}/together/ws`, origin);
    address.protocol = address.protocol === "https:" ? "wss:" : "ws:";
    const socket = new WebSocket(address);
    socket.addEventListener("open", listeners.onOpen);
    socket.addEventListener("message", (event: MessageEvent<unknown>) => {
      if (typeof event.data === "string") {
        listeners.onMessage(event.data);
      }
    });
    socket.addEventListener("close", (event) => {
      listeners.onClose(event.code);
    });
    return {
      send: (text) => {
        if (socket.readyState === WebSocket.OPEN) {
          socket.send(text);
        }
      },
      close: () => {
        socket.close();
      },
    };
  };
}
