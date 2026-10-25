// SPDX-License-Identifier: MPL-2.0

import { apiPrefix } from "./client.ts";

/** The version of the session WebSocket's messages, every message's v (cut-a §5.1). */
export const togetherProtocolVersion = 2;

/** The group's play state (watch-together.md, Sync): Waiting is paused until every player is ready. */
export type GroupState = "Idle" | "Waiting" | "Paused" | "Playing";

/** What changed the group: the server's codes for its events (internal/core/together, EventKind). */
export type GroupEvent =
  | "played"
  | "paused"
  | "seeked"
  | "changedRate"
  | "changedItem"
  | "stopped"
  | "everyoneReady"
  | "continuedWithout"
  | "waitingFor";

const groupEvents: readonly GroupEvent[] = [
  "played",
  "paused",
  "seeked",
  "changedRate",
  "changedItem",
  "stopped",
  "everyoneReady",
  "continuedWithout",
  "waitingFor",
];

/** The group as the server tells it: what plays, where, how fast, and the named event that made it so. */
export interface GroupView {
  readonly state: GroupState;
  readonly positionMs: number;
  readonly rate: number;
  /** What plays; 0 while nothing does, every screen black (D246 §5). */
  readonly fileId: number;
  /** What changed, as a code the client words (D188); undefined for none, or one it does not know. */
  readonly event: GroupEvent | undefined;
  /** Where a seek jumped from; positionMs is where it jumped to. 0 for any other event. */
  readonly seekedFromMs: number;
  /** The member whose command or state caused the event; undefined when no one did. */
  readonly byMemberId: number | undefined;
  /**
   * For a group that is Playing, how long after the message was sent it starts moving from
   * positionMs, so every player starts at once (D268 §3); 0, or absent, once it moves.
   */
  readonly startsInMs?: number | undefined;
}

/**
 * A device whose link carries less than its lowest quality needs, as it said with both numbers
 * (D247 §8); linkKbps is undefined when it was not measured.
 */
export interface CannotKeepUp {
  readonly linkKbps: number | undefined;
  readonly lowestKbps: number;
}

/**
 * What a device does for its member (D120): plays the film, carries their voice, or both. A
 * voice device takes no part in the group's sync.
 */
export type DeviceRole = "playback" | "voice" | "both";

/** What a page waits on before its film can play, as it tells the session; "" for nothing. */
export type Standing = "" | "needsTap";

/**
 * Where a playing device stands with its group, as the server reads it: what its page waits on,
 * loading its first buffer, catching up, buffering, suspended, or watching; undefined for a voice
 * device, or one waiting on nothing while the session plays nothing.
 */
export type DeviceStatus =
  "needsTap" | "loading" | "catchingUp" | "buffering" | "suspended" | "watching";

const deviceStatuses: readonly DeviceStatus[] = [
  "needsTap",
  "loading",
  "catchingUp",
  "buffering",
  "suspended",
  "watching",
];

/** A roster device's status, or undefined when it names none this client knows. */
function deviceStatusIn(value: unknown): DeviceStatus | undefined {
  return deviceStatuses.find((status) => status === value);
}

/** One device of a member in the roster, and its health. */
export interface RosterDevice {
  readonly id: number;
  readonly role: string;
  readonly isBuffering: boolean;
  /** Its word that even its lowest quality is beyond its link; undefined while it keeps up. */
  readonly cannotKeepUp: CannotKeepUp | undefined;
  /** Where it stands with its group (DeviceStatus). */
  readonly status: DeviceStatus | undefined;
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
export type TogetherEndReason =
  | "hostEnded"
  | "ejected"
  | "movedAway"
  | "left"
  | "gone"
  | "uploadFull"
  /**
   * A cast's ends (D337): its receiver took control back, another cast replaced it, its remote
   * stopped it, or its film ended.
   */
  | "takenBack"
  | "replaced"
  | "stopped"
  | "filmEnded";

/** How a device follows its group, as the server's settings said when it was welcomed (D250 §1). */
export interface SyncTuning {
  /** How far off the group's position a player is left alone. */
  readonly leaveDriftUpToMs: number;
  /** The most a nudge plays faster or slower, as a percent of the group's rate. */
  readonly nudgeMostPercent: number;
  /** How far off a player seeks rather than nudges. */
  readonly seekDriftBeyondMs: number;
  /**
   * How far off the group's position a player that corrects by seeks alone is left alone: one
   * whose rate's changes cost it more than leaveDriftUpToMs (D268 §2).
   */
  readonly seekOnlyLeaveDriftUpToMs: number;
  /** How full a player catching up aims to have its buffer when the group reaches it, in percent. */
  readonly catchUpBufferPercent: number;
  /**
   * How long after it starts playing, finishes a seek or changes its rate a player's position is
   * not read for drift: Safari's stands still for up to 250 ms then (D250 §7).
   */
  readonly settleAfterChangeMs: number;
  /** Whether a nudged player's sound keeps its pitch (D250 §7). */
  readonly shouldNudgeKeepPitch: boolean;
  /**
   * Whether the browser cancels echo on the microphone for voice: headphones need none (D63), and
   * on Android it makes a call of it.
   */
  readonly shouldCancelMicrophoneEcho: boolean;
  /** Whether the browser's gain control and noise suppression work on the microphone for voice. */
  readonly shouldBrowserLevelMicrophone: boolean;
}

/** The tuning a device follows by before its welcome says, or where it says none: the settings' starting values. */
export const startingTuning: SyncTuning = {
  leaveDriftUpToMs: 150,
  nudgeMostPercent: 10,
  seekDriftBeyondMs: 3000,
  seekOnlyLeaveDriftUpToMs: 400,
  catchUpBufferPercent: 80,
  settleAfterChangeMs: 300,
  shouldNudgeKeepPitch: true,
  shouldCancelMicrophoneEcho: false,
  shouldBrowserLevelMicrophone: false,
};

/**
 * What a member may do to playback (D271 §4): the host everything, a guest what their link gives,
 * play and pause, and changing the position, each its own.
 */
export interface PlaybackRights {
  readonly canPlayPause: boolean;
  readonly canSeek: boolean;
}

/** The host's rights: every one. */
export const everyRight: PlaybackRights = { canPlayPause: true, canSeek: true };

/** No right: what a device has before its welcome says. */
export const noRight: PlaybackRights = { canPlayPause: false, canSeek: false };

/** A welcome's rights; one that says none gives none. */
function parseRights(value: unknown): PlaybackRights {
  const record = recordOf(value);
  return record === undefined
    ? noRight
    : { canPlayPause: record["canPlayPause"] === true, canSeek: record["canSeek"] === true };
}

/** A message the server sends, parsed (cut-a §5.1); undefined for one this client does not use. */
export type TogetherServerMessage =
  | {
      readonly type: "welcome";
      readonly seq: number;
      readonly memberId: number;
      /** This device's own id, as the roster lists it. */
      readonly deviceId: number;
      readonly isHost: boolean;
      /** What this device's member may do to playback (D271 §4). */
      readonly rights: PlaybackRights;
      readonly group: GroupView;
      readonly roster: readonly RosterMember[];
      readonly tuning: SyncTuning;
    }
  | { readonly type: "group"; readonly seq: number; readonly group: GroupView }
  | { readonly type: "roster"; readonly roster: readonly RosterMember[] }
  | { readonly type: "rejected"; readonly basedOnSeq: number; readonly code: string }
  | {
      readonly type: "pong";
      readonly tMono: number;
      /** Where the group is as the server answers (D264); undefined from a server that says not. */
      readonly group: GroupView | undefined;
    }
  | { readonly type: "ended"; readonly reason: TogetherEndReason }
  | { readonly type: "rtc"; readonly fromDeviceId: number; readonly signal: VoiceSignal };

/**
 * One piece of voice signalling between two devices, which the server relays without reading
 * (D193 §4): a session description, or one ICE candidate as its JSON text.
 */
export type VoiceSignal = { readonly sdp: string } | { readonly candidate: string };

/** A value read from outside, as a record of unknown fields; undefined when it is not an object. */
export function recordOf(value: unknown): Readonly<Record<string, unknown>> | undefined {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? Object.fromEntries(Object.entries(value))
    : undefined;
}

/** A number field, or fallback when it is absent or not a number (the server omits zeros). */
function numberIn(record: Readonly<Record<string, unknown>>, key: string, fallback = 0): number {
  const value = record[key];
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

/** A boolean field, or isAbsentTrue where it is none. */
function booleanIn(
  record: Readonly<Record<string, unknown>>,
  key: string,
  isAbsentTrue: boolean,
): boolean {
  const value = record[key];
  return typeof value === "boolean" ? value : isAbsentTrue;
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
    event: groupEvents.find((each) => each === record["event"]),
    seekedFromMs: numberIn(record, "seekedFromMs"),
    startsInMs: numberIn(record, "startsInMs"),
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
                  cannotKeepUp: cannotKeepUpIn(fields["cannotKeepUp"]),
                  status: deviceStatusIn(fields["status"]),
                },
              ];
        }),
      },
    ];
  });
}

/** A roster device's cannotKeepUp, or undefined when it has none or it is not its shape. */
function cannotKeepUpIn(value: unknown): CannotKeepUp | undefined {
  const fields = recordOf(value);
  if (fields === undefined || typeof fields["lowestKbps"] !== "number") {
    return undefined;
  }
  const link = fields["linkKbps"];
  return {
    linkKbps: typeof link === "number" && link > 0 ? link : undefined,
    lowestKbps: fields["lowestKbps"],
  };
}

const endReasons: readonly TogetherEndReason[] = [
  "hostEnded",
  "ejected",
  "movedAway",
  "left",
  "uploadFull",
  "takenBack",
  "replaced",
  "stopped",
  "filmEnded",
];

/** A welcome's tuning, each value the starting one where it says none. */
function parseTuning(value: unknown): SyncTuning {
  const record = recordOf(value) ?? {};
  return {
    leaveDriftUpToMs: numberIn(record, "leaveDriftUpToMs", startingTuning.leaveDriftUpToMs),
    nudgeMostPercent: numberIn(record, "nudgeMostPercent", startingTuning.nudgeMostPercent),
    seekDriftBeyondMs: numberIn(record, "seekDriftBeyondMs", startingTuning.seekDriftBeyondMs),
    seekOnlyLeaveDriftUpToMs: numberIn(
      record,
      "seekOnlyLeaveDriftUpToMs",
      startingTuning.seekOnlyLeaveDriftUpToMs,
    ),
    catchUpBufferPercent: numberIn(
      record,
      "catchUpBufferPercent",
      startingTuning.catchUpBufferPercent,
    ),
    settleAfterChangeMs: numberIn(
      record,
      "settleAfterChangeMs",
      startingTuning.settleAfterChangeMs,
    ),
    shouldNudgeKeepPitch: booleanIn(
      record,
      "shouldNudgeKeepPitch",
      startingTuning.shouldNudgeKeepPitch,
    ),
    shouldCancelMicrophoneEcho: booleanIn(
      record,
      "shouldCancelMicrophoneEcho",
      startingTuning.shouldCancelMicrophoneEcho,
    ),
    shouldBrowserLevelMicrophone: booleanIn(
      record,
      "shouldBrowserLevelMicrophone",
      startingTuning.shouldBrowserLevelMicrophone,
    ),
  };
}

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
            deviceId: numberIn(record, "deviceId"),
            isHost: record["isHost"] === true,
            rights: parseRights(record["rights"]),
            group,
            roster: parseRoster(record["roster"]),
            tuning: parseTuning(record["tuning"]),
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

/** The voice signalling an rtc message carries, or undefined when it carries none. */
function parseVoiceSignal(record: Readonly<Record<string, unknown>>): VoiceSignal | undefined {
  const sdp = stringIn(record, "sdp");
  if (sdp !== undefined && sdp !== "") {
    return { sdp };
  }
  const candidate = stringIn(record, "candidate");
  return candidate !== undefined && candidate !== "" ? { candidate } : undefined;
}

/** An rtc message: signalling from another device; undefined when it names none or says nothing. */
function parseRtc(record: Readonly<Record<string, unknown>>): TogetherServerMessage | undefined {
  const fromDeviceId = numberIn(record, "fromDeviceId");
  const signal = parseVoiceSignal(record);
  return fromDeviceId === 0 || signal === undefined
    ? undefined
    : { type: "rtc", fromDeviceId, signal };
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
      return { type: "pong", tMono: numberIn(record, "tMono"), group: parseGroup(record["group"]) };
    case "ended": {
      const reason = endReasons.find((each) => each === record["reason"]) ?? "gone";
      return { type: "ended", reason };
    }
    case "rtc":
      return parseRtc(record);
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
