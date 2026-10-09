// SPDX-License-Identifier: MPL-2.0

import { apiPrefix } from "./client.ts";
import type { ChannelLike } from "./rtc-transport.ts";
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
    const channel = openChannel();
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
      path: `${apiPrefix}/together/ws`,
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
