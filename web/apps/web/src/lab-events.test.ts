// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  eventSender,
  eventsPerRequestAtMost,
  newRunName,
  type EventSending,
  type LabEventsBody,
  type LabInputRunStart,
  type NotedEvent,
} from "./lab-events.ts";

/** The page as a Fire TV opens it. */
const start: LabInputRunStart = {
  viewportWidth: 960,
  viewportHeight: 540,
  devicePixelRatio: 2,
  mode: "geometric",
  hasMediaSession: true,
  hasFullscreen: true,
  hasCaptureStream: true,
};

/** A key going down at atMs. */
function key(name: string, atMs: number): NotedEvent {
  return { kind: "key", button: "other", key: name, code: name, keyCode: 0, atMs };
}

/** A stand-in server whose every request waits until the test answers it. */
function heldServer(): {
  readonly requests: { readonly body: LabEventsBody; readonly isLeaving: boolean }[];
  readonly answer: (index: number, isTaken: boolean) => Promise<void>;
  readonly sendings: EventSending[];
  readonly post: (body: LabEventsBody, isLeaving: boolean) => Promise<void>;
} {
  const requests: { body: LabEventsBody; isLeaving: boolean }[] = [];
  const settlers: { resolve: () => void; reject: (cause: Error) => void }[] = [];
  return {
    requests,
    sendings: [],
    post: async (body, isLeaving) => {
      await new Promise<void>((resolve, reject) => {
        requests.push({ body, isLeaving });
        settlers.push({ resolve, reject });
      });
    },
    answer: async (index, isTaken) => {
      const settler = settlers[index];
      if (isTaken) {
        settler?.resolve();
      } else {
        settler?.reject(new Error("refused"));
      }
      // The sender's own handling of the answer, then.
      await Promise.resolve();
      await Promise.resolve();
    },
  };
}

/** The keys a request carried, with their places. */
function keysOf(body: LabEventsBody | undefined): string[] {
  return (body?.events ?? []).map((event) => `${String(event.seq)}:${event.key ?? ""}`);
}

describe("the button test's events on their way to the log", () => {
  it("sends the first at once, and what comes while it is on its way next, together and in order", async () => {
    const server = heldServer();
    const sender = eventSender({
      run: "K3J7QW2A",
      start,
      post: server.post,
      onSending: (sending) => {
        server.sendings.push(sending);
      },
    });
    sender.note(key("ArrowDown", 10));
    expect(server.requests).toHaveLength(1);
    expect(keysOf(server.requests[0]?.body)).toEqual(["0:ArrowDown"]);
    expect(server.requests[0]?.body.start).toEqual(start);
    expect(server.requests[0]?.body.run).toBe("K3J7QW2A");

    sender.note(key("ArrowRight", 20));
    sender.note(key("Enter", 30));
    expect(server.requests).toHaveLength(1);

    await server.answer(0, true);
    expect(server.requests).toHaveLength(2);
    expect(keysOf(server.requests[1]?.body)).toEqual(["1:ArrowRight", "2:Enter"]);
    expect(server.requests[1]?.body.start).toBeUndefined();
    await server.answer(1, true);
    expect(server.sendings.at(-1)).toEqual({ run: "K3J7QW2A", sentCount: 3, hasFailed: false });
  });

  it("keeps a refused request's events and its start for the next event, and says it failed", async () => {
    const server = heldServer();
    const sender = eventSender({
      run: "K3J7QW2A",
      start,
      post: server.post,
      onSending: (sending) => {
        server.sendings.push(sending);
      },
    });
    sender.note(key("ArrowDown", 10));
    await server.answer(0, false);
    expect(server.sendings.at(-1)).toEqual({ run: "K3J7QW2A", sentCount: 0, hasFailed: true });
    expect(server.requests).toHaveLength(1);

    sender.note(key("ArrowUp", 20));
    expect(keysOf(server.requests[1]?.body)).toEqual(["0:ArrowDown", "1:ArrowUp"]);
    expect(server.requests[1]?.body.start).toEqual(start);
    await server.answer(1, true);
    expect(server.sendings.at(-1)).toEqual({ run: "K3J7QW2A", sentCount: 2, hasFailed: false });
  });

  it("sends what waits as the page leaves, kept alive, beside the request on its way", () => {
    const server = heldServer();
    const sender = eventSender({
      run: "K3J7QW2A",
      start,
      post: server.post,
      onSending: () => undefined,
    });
    sender.note(key("ArrowDown", 10));
    sender.note(key("Enter", 20));
    sender.sendAsLeaving();
    expect(server.requests.map((request) => request.isLeaving)).toEqual([false, true]);
    expect(keysOf(server.requests[1]?.body)).toEqual(["1:Enter"]);
  });

  it("splits a long wait's events into requests the server takes", async () => {
    const server = heldServer();
    const sender = eventSender({
      run: "K3J7QW2A",
      start,
      post: server.post,
      onSending: () => undefined,
    });
    sender.note(key("ArrowDown", 0));
    for (let index = 0; index < eventsPerRequestAtMost + 1; index += 1) {
      sender.note(key("ArrowRight", index));
    }
    await server.answer(0, true);
    expect(server.requests[1]?.body.events).toHaveLength(eventsPerRequestAtMost);
    await server.answer(1, true);
    expect(keysOf(server.requests[2]?.body)).toEqual([
      `${String(eventsPerRequestAtMost + 1)}:ArrowRight`,
    ]);
  });

  it("names a run by eight of base32's characters, as the server takes", () => {
    expect(newRunName()).toMatch(/^[A-Z2-7]{8}$/u);
  });
});
