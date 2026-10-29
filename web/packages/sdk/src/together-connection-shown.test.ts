// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { shownAnswerWithinMs } from "./shown-check.ts";
import {
  createTogetherConnection,
  deadAfterUnansweredPings,
  longestReconnectWaitMs,
  pingEveryMs,
  reconnectWaitMs,
} from "./together-connection.ts";
import type { TogetherSocketListeners } from "./together.ts";
import { FakePage } from "./test-follow.ts";

/** A stand-in for the server's end of each socket: what was opened, sent, and closed from here. */
function fakeSockets(): {
  readonly opened: TogetherSocketListeners[];
  readonly sent: Record<string, unknown>[];
  readonly closedHere: number[];
  readonly open: (listeners: TogetherSocketListeners) => {
    send: (text: string) => void;
    close: () => void;
  };
} {
  const opened: TogetherSocketListeners[] = [];
  const sent: Record<string, unknown>[] = [];
  const closedHere: number[] = [];
  return {
    opened,
    sent,
    closedHere,
    open: (listeners) => {
      opened.push(listeners);
      const index = opened.length - 1;
      return {
        send: (text) => {
          sent.push(JSON.parse(text) as Record<string, unknown>);
        },
        close: () => {
          closedHere.push(index);
        },
      };
    },
  };
}

/** A welcome of member 7's device 9, its session paused. */
const welcome = {
  v: 2,
  type: "welcome",
  seq: 4,
  sessionId: 1,
  memberId: 7,
  deviceId: 9,
  group: { state: "Paused", positionMs: 1000, rate: 1, fileId: 3 },
  roster: [{ id: 7, name: "Gran", devices: [{ id: 9, role: "video" }] }],
};

let monotonicNowMs = 0;
const monotonicMs = (): number => monotonicNowMs;

beforeEach(() => {
  vi.useFakeTimers();
  monotonicNowMs = 1000;
});

afterEach(() => {
  vi.useRealTimers();
});

describe("a connection whose page is shown again (the owner's VQ105, 2026-10-29)", () => {
  it("shown again, pings at once, and connects again when nothing answers within shownAnswerWithinMs (VQ105)", () => {
    const sockets = fakeSockets();
    const page = new FakePage();
    const connection = createTogetherConnection({
      credential: "c",
      openSocket: sockets.open,
      monotonicMs,
      page,
    });
    connection.open();
    sockets.opened[0]!.onOpen();
    sockets.opened[0]!.onMessage(JSON.stringify(welcome));
    page.hide(); // the phone locked, its socket dying unseen
    monotonicNowMs = 36_000;
    page.show();
    expect(sockets.sent.at(-1)).toEqual({ v: 2, type: "ping", tMono: 36_000 });
    vi.advanceTimersByTime(shownAnswerWithinMs - 1);
    expect(connection.snapshot().status).toBe("connected");
    vi.advanceTimersByTime(1); // long before its pings would find it dead
    expect(shownAnswerWithinMs * 10).toBeLessThan(pingEveryMs * deadAfterUnansweredPings);
    expect(connection.snapshot().status).toBe("reconnecting");
    expect(sockets.closedHere).toEqual([0]);
    monotonicNowMs = 36_000 + shownAnswerWithinMs + reconnectWaitMs(0);
    vi.advanceTimersByTime(reconnectWaitMs(0));
    sockets.opened[1]!.onOpen();
    sockets.opened[1]!.onMessage(JSON.stringify(welcome));
    expect(sockets.sent.find((each) => each["type"] === "note")?.["note"]).toEqual({
      kind: "reconnected",
      offlineMs: shownAnswerWithinMs + reconnectWaitMs(0),
      why: "shown",
    });
  });

  it("shown again on a socket that answers, keeps it and notes the round trip", () => {
    const sockets = fakeSockets();
    const page = new FakePage();
    const connection = createTogetherConnection({
      credential: "c",
      openSocket: sockets.open,
      monotonicMs,
      page,
    });
    connection.open();
    sockets.opened[0]!.onOpen();
    sockets.opened[0]!.onMessage(JSON.stringify(welcome));
    page.hide();
    monotonicNowMs = 36_000;
    page.show();
    monotonicNowMs = 36_080;
    sockets.opened[0]!.onMessage(JSON.stringify({ v: 2, type: "pong", tMono: 36_000 }));
    vi.advanceTimersByTime(shownAnswerWithinMs * 2);
    expect(connection.snapshot().status).toBe("connected");
    expect(sockets.opened).toHaveLength(1);
    expect(
      sockets.sent.filter((each) => each["type"] === "note").map((each) => each["note"]),
    ).toEqual([{ kind: "shownAgain", why: "alive", roundTripMs: 80 }]);
  });

  it("shown again while waiting to connect again, connects at once, and no more once closed", () => {
    const sockets = fakeSockets();
    const page = new FakePage();
    const connection = createTogetherConnection({
      credential: "c",
      openSocket: sockets.open,
      monotonicMs,
      page,
    });
    connection.open();
    sockets.opened[0]!.onClose(1006);
    vi.advanceTimersByTime(reconnectWaitMs(0));
    sockets.opened[1]!.onClose(1006); // its wait is now reconnectWaitMs(1)
    page.hide();
    page.show();
    expect(sockets.opened).toHaveLength(3);
    connection.close();
    page.hide();
    page.show();
    vi.advanceTimersByTime(longestReconnectWaitMs);
    expect(sockets.opened).toHaveLength(3);
  });
});

// The owner's Chrome, Wi-Fi off until its film stalled and on again: its media came back over a new
// WebRTC connection, and its session's socket waited out its growing wait, 28 s more, while the film
// played on out of step (staging, 2026-10-29; VQ130).
describe("a connection told the way to the core is back", () => {
  /** A path whose word that it is back the test gives, and how many listen to it. */
  function backPath(): {
    readonly pathBack: (listener: () => void) => () => void;
    readonly isBack: () => void;
    readonly listening: () => number;
  } {
    const listeners = new Set<() => void>();
    return {
      pathBack: (listener) => {
        listeners.add(listener);
        return () => listeners.delete(listener);
      },
      isBack: () => {
        for (const listener of listeners) {
          listener();
        }
      },
      listening: () => listeners.size,
    };
  }

  it("connects again at once while it waits out its wait, and hears no more once closed", () => {
    const sockets = fakeSockets();
    const path = backPath();
    const connection = createTogetherConnection({
      credential: "c",
      openSocket: sockets.open,
      monotonicMs,
      pathBack: path.pathBack,
    });
    connection.open();
    sockets.opened[0]!.onOpen();
    sockets.opened[0]!.onMessage(JSON.stringify(welcome));
    sockets.opened[0]!.onClose(1006); // the network went
    for (let attempt = 0; attempt < 6; attempt += 1) {
      vi.advanceTimersByTime(reconnectWaitMs(attempt));
      sockets.opened.at(-1)!.onClose(1006); // no connection to the core is up yet
    }
    expect(reconnectWaitMs(6)).toBe(longestReconnectWaitMs);
    const before = sockets.opened.length;
    path.isBack();
    expect(sockets.opened).toHaveLength(before + 1); // at once, not after its 30 s
    monotonicNowMs = 92_000;
    sockets.opened.at(-1)!.onOpen();
    sockets.opened.at(-1)!.onMessage(JSON.stringify(welcome));
    expect(connection.snapshot().status).toBe("connected");
    expect(sockets.sent.filter((each) => each["type"] === "note").at(-1)?.["note"]).toEqual({
      kind: "reconnected",
      offlineMs: 91_000,
      why: "closed",
    });
    connection.close();
    expect(path.listening()).toBe(0);
  });

  it("pings a socket it holds, and connects again when nothing answers within shownAnswerWithinMs", () => {
    const sockets = fakeSockets();
    const path = backPath();
    const connection = createTogetherConnection({
      credential: "c",
      openSocket: sockets.open,
      monotonicMs,
      pathBack: path.pathBack,
    });
    connection.open();
    sockets.opened[0]!.onOpen();
    sockets.opened[0]!.onMessage(JSON.stringify(welcome));
    monotonicNowMs = 20_000;
    path.isBack(); // a socket that never heard its connection close
    expect(sockets.sent.at(-1)).toEqual({ v: 2, type: "ping", tMono: 20_000 });
    vi.advanceTimersByTime(shownAnswerWithinMs);
    expect(connection.snapshot().status).toBe("reconnecting");
    vi.advanceTimersByTime(reconnectWaitMs(0));
    expect(sockets.opened).toHaveLength(2);
  });
});
