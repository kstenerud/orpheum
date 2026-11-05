// SPDX-License-Identifier: MPL-2.0

import { apiPrefix } from "./client.ts";
import type { ChannelLike } from "./rtc-request.ts";
import type { OpenTogetherSocket } from "./together.ts";

/** The close code of a channel that ended without the core saying why: a WebSocket's "abnormal". */
const closedAbnormally = 1006;

/** The close code of an upgrade the core refused: a WebSocket's "internal error". */
const refusedUpgrade = 1011;

/**
 * The session WebSocket over a WebRTC data channel (D273 §8, plans/webrtc-transport.md W3): a request
 * asking to become a WebSocket, then each text message either way one message, and the core's binary
 * message its close (rtcapi.CloseMessage). openChannel opens the channel: RtcTransport.openChannel.
 */
export function rtcTogetherSocket(openChannel: () => ChannelLike): OpenTogetherSocket {
  return (listeners) => {
    let channel: ChannelLike;
    try {
      channel = openChannel();
    } catch {
      return droppedAtOnce(listeners);
    }
    let isOpen = false;
    let isClosed = false;
    const closeWith = (code: number): void => {
      if (isClosed) {
        return;
      }
      isClosed = true;
      listeners.onClose(code);
      channel.close();
    };
    const head = {
      method: "GET",
      path: `${apiPrefix}/socket`,
      headers: { upgrade: ["websocket"] },
    };
    channel.listen({
      open: () => {
        channel.send(JSON.stringify(head));
        channel.send(new ArrayBuffer(0));
      },
      message: (data) => {
        if (typeof data !== "string") {
          closeWith(closeCodeOf(data));
        } else if (isOpen) {
          listeners.onMessage(data);
        } else if (statusOf(data) === 101) {
          isOpen = true;
          listeners.onOpen();
        } else {
          closeWith(refusedUpgrade);
        }
      },
      close: () => {
        closeWith(closedAbnormally);
      },
      error: () => {
        closeWith(closedAbnormally);
      },
    });
    return {
      send: (text) => {
        if (isOpen && !isClosed) {
          channel.send(text);
        }
      },
      close: () => {
        isClosed = true;
        channel.close();
      },
    };
  };
}

/**
 * A socket that closes as one that dropped, after its caller has it: no connection to the core is
 * up yet, as for the moment after a network's loss while the transport opens their replacements,
 * and the connection tries again after its wait. Thrown, it ended a guest's tries for good (the
 * browser tier under load, 2026-10-18).
 */
function droppedAtOnce(
  listeners: Parameters<OpenTogetherSocket>[0],
): ReturnType<OpenTogetherSocket> {
  let isClosed = false;
  queueMicrotask(() => {
    if (!isClosed) {
      listeners.onClose(closedAbnormally);
    }
  });
  return {
    send: () => undefined,
    close: () => {
      isClosed = true;
    },
  };
}

/** The status of a response head, or undefined for one that is not. */
function statusOf(text: string): number | undefined {
  try {
    const parsed: unknown = JSON.parse(text);
    return typeof parsed === "object" &&
      parsed !== null &&
      "status" in parsed &&
      typeof parsed.status === "number"
      ? parsed.status
      : undefined;
  } catch {
    return undefined;
  }
}

/** The close code the core's close message names, else that of an abnormal close. */
function closeCodeOf(data: ArrayBuffer): number {
  try {
    const parsed: unknown = JSON.parse(new TextDecoder().decode(data));
    return typeof parsed === "object" &&
      parsed !== null &&
      "close" in parsed &&
      typeof parsed.close === "number"
      ? parsed.close
      : closedAbnormally;
  } catch {
    return closedAbnormally;
  }
}
