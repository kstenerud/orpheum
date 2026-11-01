// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createLiveConnection } from "./live-connection.ts";
import { unreachedRetryMs } from "./rtc-reopen.ts";
import { createTogetherConnection, reconnectWaitMs } from "./together-connection.ts";
import type { TogetherSocketListeners } from "./together.ts";

// A page on a core's own HTTPS name, its Wi-Fi off and on, in Firefox on the desktop, which fires no
// `online` event (Mozilla bug 654579): its sockets' tries fail to connect while the network is away,
// and the page is back only as one of them connects, so the wait between tries must stay short.

/** The sockets a connection opens, each of which the test opens or lets fail to connect. */
function fakeSockets(): {
  readonly opened: TogetherSocketListeners[];
  readonly open: (listeners: TogetherSocketListeners) => { send: () => void; close: () => void };
} {
  const opened: TogetherSocketListeners[] = [];
  return {
    opened,
    open: (listeners) => {
      opened.push(listeners);
      return { send: () => undefined, close: () => undefined };
    },
  };
}

const monotonicMs = (): number => 1000;

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("a session connection whose tries fail to connect, as with no network", () => {
  it("tries again after unreachedRetryMs each time, its growing wait started over, with no online event", async () => {
    const sockets = fakeSockets();
    const connection = createTogetherConnection({
      credential: "c",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    // Eight tries that never open, a minute and more: the growing wait would be 30 s by now.
    for (let tries = 1; tries <= 8; tries += 1) {
      sockets.opened.at(-1)!.onClose(1006);
      await vi.advanceTimersByTimeAsync(unreachedRetryMs - 1);
      expect(sockets.opened).toHaveLength(tries);
      await vi.advanceTimersByTimeAsync(1);
      expect(sockets.opened).toHaveLength(tries + 1);
    }
    expect(unreachedRetryMs).toBeLessThan(reconnectWaitMs(4));
    // The server answers again: the next try opens, and a later drop of a socket that reached the
    // server waits the first growing wait, the count started over.
    sockets.opened.at(-1)!.onOpen();
    sockets.opened.at(-1)!.onClose(1006);
    const count = sockets.opened.length;
    await vi.advanceTimersByTimeAsync(reconnectWaitMs(0));
    expect(sockets.opened).toHaveLength(count + 1);
    connection.close();
  });
});

describe("a live connection whose tries fail to connect, as with no network", () => {
  it("tries again after unreachedRetryMs each time, and a drop after reaching the server grows from the first wait", async () => {
    const sockets = fakeSockets();
    const connection = createLiveConnection({
      sessionToken: "session",
      openSocket: sockets.open,
      monotonicMs,
    });
    connection.open();
    for (let tries = 1; tries <= 8; tries += 1) {
      sockets.opened.at(-1)!.onClose(1006);
      await vi.advanceTimersByTimeAsync(unreachedRetryMs - 1);
      expect(sockets.opened).toHaveLength(tries);
      await vi.advanceTimersByTimeAsync(1);
      expect(sockets.opened).toHaveLength(tries + 1);
    }
    sockets.opened.at(-1)!.onOpen();
    sockets.opened.at(-1)!.onClose(1006);
    const count = sockets.opened.length;
    await vi.advanceTimersByTimeAsync(reconnectWaitMs(0));
    expect(sockets.opened).toHaveLength(count + 1);
    connection.close();
  });
});
