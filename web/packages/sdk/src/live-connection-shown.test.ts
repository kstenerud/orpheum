// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createLiveConnection } from "./live-connection.ts";
import { shownAnswerWithinMs } from "./shown-check.ts";
import { deadAfterUnansweredPings, pingEveryMs, reconnectWaitMs } from "./together-connection.ts";
import type { TogetherSocketListeners } from "./together.ts";
import { FakeLeftPage, FakePage } from "./test-follow.ts";

/** The server's end of each live socket: what was opened, sent, and closed from here. */
interface LiveSockets {
  readonly opened: TogetherSocketListeners[];
  readonly sent: Record<string, unknown>[];
  readonly closedHere: number[];
  readonly open: (listeners: TogetherSocketListeners) => {
    send: (text: string) => void;
    close: () => void;
  };
}

function liveSockets(): LiveSockets {
  const sockets: LiveSockets = {
    opened: [],
    sent: [],
    closedHere: [],
    open: (listeners) => {
      sockets.opened.push(listeners);
      const index = sockets.opened.length - 1;
      return {
        send: (text) => {
          sockets.sent.push(JSON.parse(text) as Record<string, unknown>);
        },
        close: () => {
          sockets.closedHere.push(index);
        },
      };
    },
  };
  return sockets;
}

let monotonicNowMs = 0;
const monotonicMs = (): number => monotonicNowMs;

beforeEach(() => {
  vi.useFakeTimers();
  monotonicNowMs = 1000;
});

afterEach(() => {
  vi.useRealTimers();
});

/** A live connection on page and leftPage, its first socket greeted and told the devices. */
function connectedLive(
  sockets: LiveSockets,
  page: FakePage,
  leftPage?: FakeLeftPage,
): ReturnType<typeof createLiveConnection> {
  const connection = createLiveConnection({
    sessionToken: "session",
    openSocket: sockets.open,
    monotonicMs,
    page,
    leftPage,
  });
  connection.open();
  sockets.opened[0]!.onOpen();
  sockets.opened[0]!.onMessage(JSON.stringify({ type: "devices", deviceIds: [3] }));
  return connection;
}

describe("a page's live connection, its page shown again (the session connection's check, one path)", () => {
  it("shown again, pings at once and connects again when nothing answers within shownAnswerWithinMs, long before its pings would", () => {
    const sockets = liveSockets();
    const page = new FakePage();
    const connection = connectedLive(sockets, page);
    page.hide(); // the phone locked, its socket dying unseen
    monotonicNowMs = 36_000;
    page.show();
    expect(sockets.sent.at(-1)).toEqual({ type: "ping", tMono: 36_000 });
    vi.advanceTimersByTime(shownAnswerWithinMs - 1);
    expect(connection.snapshot().isConnected).toBe(true);
    vi.advanceTimersByTime(1);
    expect(shownAnswerWithinMs * 10).toBeLessThan(pingEveryMs * deadAfterUnansweredPings);
    expect(connection.snapshot().isConnected).toBe(false);
    expect(sockets.closedHere).toEqual([0]);
    vi.advanceTimersByTime(reconnectWaitMs(0));
    expect(sockets.opened).toHaveLength(2);
  });

  it("shown again on a socket that answers, keeps it", () => {
    const sockets = liveSockets();
    const page = new FakePage();
    const connection = connectedLive(sockets, page);
    page.hide();
    monotonicNowMs = 36_000;
    page.show();
    sockets.opened[0]!.onMessage(JSON.stringify({ type: "pong", tMono: 36_000 }));
    vi.advanceTimersByTime(shownAnswerWithinMs * 2);
    expect(connection.snapshot().isConnected).toBe(true);
    expect(sockets.opened).toHaveLength(1);
    expect(sockets.closedHere).toEqual([]);
  });

  it("shown again while waiting to connect again, connects at once", () => {
    const sockets = liveSockets();
    const page = new FakePage();
    connectedLive(sockets, page);
    sockets.opened[0]!.onClose(1006);
    page.hide();
    page.show();
    expect(sockets.opened).toHaveLength(2);
  });
});

describe("a page's live connection, its page left (the owner, 2026-10-29)", () => {
  it("left, closes its socket at once and connects nothing until the page returns from the back/forward cache", () => {
    const sockets = liveSockets();
    const page = new FakePage();
    const leftPage = new FakeLeftPage();
    const connection = connectedLive(sockets, page, leftPage);
    leftPage.leave();
    expect(sockets.closedHere).toEqual([0]);
    expect(connection.snapshot().isConnected).toBe(false);
    sockets.opened[0]!.onClose(1000);
    page.show();
    vi.advanceTimersByTime(60_000);
    expect(sockets.opened).toHaveLength(1);
    leftPage.show();
    expect(sockets.opened).toHaveLength(2);
    connection.close();
    leftPage.leave();
    leftPage.show();
    expect(sockets.opened).toHaveLength(2);
  });
});
