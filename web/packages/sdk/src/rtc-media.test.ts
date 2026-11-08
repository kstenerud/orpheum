// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  answerLoad,
  askPathOf,
  isPathMessage,
  isPathQueryMessage,
  responseFromPort,
  relayedAheadBytes,
  type FetchMessage,
  type PortLike,
} from "./rtc-media.ts";
import { carrierText, mediaCarrierOf } from "./media-carrier.ts";

/** A data channel's message: the size of a body's piece over the WebRTC path. */
const piece = 64 * 1024;

const aLoad: FetchMessage = {
  kind: "orpheum-fetch",
  url: "https://app.example/api/v1/chunks/x",
  method: "GET",
  headers: [],
};

/** Lets the ports' messages and the streams' reads run their course. */
async function settle(): Promise<void> {
  for (let turn = 0; turn < 50; turn += 1) {
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
}

/**
 * A body of count pieces, each filled with its index, made only as read: how much was read, whether
 * it was cancelled, and the pieces it made.
 */
function piecesOf(count: number): {
  stream: ReadableStream<Uint8Array>;
  read: () => number;
  cancelled: () => boolean;
  made: Uint8Array[];
} {
  const made: Uint8Array[] = [];
  let isCancelled = false;
  const stream = new ReadableStream<Uint8Array>(
    {
      pull: (controller) => {
        if (made.length === count) {
          controller.close();
          return;
        }
        const next = new Uint8Array(piece).fill(made.length % 256);
        made.push(next);
        controller.enqueue(next);
      },
      cancel: () => {
        isCancelled = true;
      },
    },
    { highWaterMark: 0 },
  );
  return {
    stream,
    read: () => made.length * piece,
    cancelled: () => isCancelled,
    made,
  };
}

/**
 * The messages that reach port, as a test playing the other end hears them: the next, waited for in
 * order, and every one so far.
 */
function heardOn(port: MessagePort): { next: () => Promise<unknown>; all: () => unknown[] } {
  const heard: unknown[] = [];
  let taken = 0;
  let waiting: (() => void) | undefined;
  port.addEventListener("message", (event: MessageEvent<unknown>) => {
    heard.push(event.data);
    const next = waiting;
    waiting = undefined;
    next?.();
  });
  port.start();
  return {
    next: async () => {
      while (taken === heard.length) {
        await new Promise<void>((resolve) => {
          waiting = resolve;
        });
      }
      taken += 1;
      return heard[taken - 1];
    },
    all: () => [...heard],
  };
}

/** The page's end of a port, counting each message the worker sends it. */
function countingIncoming(port: MessagePort, counted: () => void): PortLike {
  return {
    postMessage: (message, transfer) => {
      port.postMessage(message, transfer ?? []);
    },
    addEventListener: (type, listener) => {
      port.addEventListener(type, (event) => {
        counted();
        listener(event);
      });
    },
    start: () => {
      port.start();
    },
  };
}

describe("media over the WebRTC path", () => {
  it("carries a load from the worker to the page and its body back", async () => {
    const { port1, port2 } = new MessageChannel();
    const heard: Request[] = [];
    const pieces = ["orph", "eum"].map((text) => new TextEncoder().encode(text));
    const load: FetchMessage = {
      kind: "orpheum-fetch",
      url: "https://app.example/api/v1/bytes/media/x",
      method: "GET",
      headers: [["Range", "bytes=0-"]],
    };
    void answerLoad(load, port2, {
      send: (request) => {
        heard.push(request);
        return Promise.resolve(
          new Response(new Blob(pieces).stream(), {
            status: 206,
            headers: { "Content-Range": "bytes 0-6/7" },
          }),
        );
      },
      token: "t",
      viewer: "v",
    });
    const response = await responseFromPort(port1);
    expect(response.status).toBe(206);
    expect(response.headers.get("content-range")).toBe("bytes 0-6/7");
    expect(await response.text()).toBe("orpheum");
    expect(heard[0]?.headers.get("Range")).toBe("bytes=0-");
    expect(heard[0]?.headers.get("Authorization")).toBe("Bearer t");
    // A viewer's membership beside the page's own sign-in, which no cookie reaches across origins.
    expect(heard[0]?.headers.get("Orpheum-Viewer")).toBe("v");
    port1.close();
  });

  it("sends a body ahead of the worker's reader, not a piece for each message the busy page answers", async () => {
    const { port1, port2 } = new MessageChannel();
    const source = piecesOf(128);
    let toThePage = 0;
    void answerLoad(
      aLoad,
      countingIncoming(port2, () => (toThePage += 1)),
      {
        send: () => Promise.resolve(new Response(source.stream)),
        token: undefined,
        viewer: undefined,
      },
    );
    const body = new Uint8Array(await (await responseFromPort(port1)).arrayBuffer());
    // In order, every piece.
    expect(body.byteLength).toBe(128 * piece);
    for (let index = 0; index < 128; index += 1) {
      expect(body[index * piece]).toBe(index % 256);
    }
    // 8 MiB took one message for each 64 KiB piece, a round trip through the page each, which at a
    // Fire TV's 30 ms carried 17 Mb/s (staging, 2026-10-28); granted a share of the window at a
    // time, it takes one for each 256 KiB of what was granted: the body and the window ahead of it.
    expect(toThePage).toBeLessThanOrEqual(
      (128 * piece + relayedAheadBytes) / (relayedAheadBytes / 4),
    );
    port1.close();
  });

  it("grants a reader slower than the page room a share of the window at a time, not a piece at a time", async () => {
    const { port1, port2 } = new MessageChannel();
    const source = piecesOf(64);
    let toThePage = 0;
    void answerLoad(
      aLoad,
      countingIncoming(port2, () => (toThePage += 1)),
      {
        send: () => Promise.resolve(new Response(source.stream)),
        token: undefined,
        viewer: undefined,
      },
    );
    const reader = (await responseFromPort(port1)).body?.getReader();
    let read = 0;
    for (;;) {
      // The window fills while the reader waits, so each piece it takes frees one piece's room.
      await settle();
      const next = await reader?.read();
      if (next === undefined || next.done) {
        break;
      }
      read += next.value.byteLength;
    }
    expect(read).toBe(64 * piece);
    // A piece's room granted for each piece taken was a message for each, as pulling was (Firefox's
    // real worker, 35 for 64 pieces, 2026-10-28).
    expect(toThePage).toBeLessThanOrEqual(
      (64 * piece + relayedAheadBytes) / (relayedAheadBytes / 4),
    );
    port1.close();
  });

  it("sends no more than its window ahead of a reader that takes nothing", async () => {
    const { port1, port2 } = new MessageChannel();
    const source = piecesOf(1000);
    void answerLoad(aLoad, port2, {
      send: () => Promise.resolve(new Response(source.stream)),
      token: undefined,
      viewer: undefined,
    });
    await responseFromPort(port1);
    await settle();
    expect(source.read()).toBeGreaterThanOrEqual(relayedAheadBytes);
    expect(source.read()).toBeLessThanOrEqual(relayedAheadBytes + 2 * piece);
    port1.close();
  });

  it("moves each piece's own buffer to the worker rather than copying it", async () => {
    const { port1, port2 } = new MessageChannel();
    const source = piecesOf(4);
    void answerLoad(aLoad, port2, {
      send: () => Promise.resolve(new Response(source.stream)),
      token: undefined,
      viewer: undefined,
    });
    expect((await (await responseFromPort(port1)).arrayBuffer()).byteLength).toBe(4 * piece);
    // A moved buffer is gone from the page.
    expect(source.made.map((made) => made.byteLength)).toEqual([0, 0, 0, 0]);
    port1.close();
  });

  it("stops the page's load when the worker's reader gives the body up, as a seek does", async () => {
    const { port1, port2 } = new MessageChannel();
    const source = piecesOf(1000);
    void answerLoad(aLoad, port2, {
      send: () => Promise.resolve(new Response(source.stream)),
      token: undefined,
      viewer: undefined,
    });
    const reader = (await responseFromPort(port1)).body?.getReader();
    expect((await reader?.read())?.value?.[0]).toBe(0);
    await reader?.cancel();
    await settle();
    expect(source.cancelled()).toBe(true);
    port1.close();
  });

  // hls.js giving a chunk up before its head came left the page's request holding its connection's
  // room until an answer a stuck connection never sent (the owner's Chrome, staging, 2026-10-29).
  it("aborts the page's load when the worker's load is given up before its head came", async () => {
    const { port1, port2 } = new MessageChannel();
    const sent: Request[] = [];
    void answerLoad(aLoad, port2, {
      send: (request) => {
        sent.push(request);
        return new Promise<Response>(() => undefined); // its head never comes
      },
      token: undefined,
      viewer: undefined,
    });
    const givenUp = new AbortController();
    void responseFromPort(port1, givenUp.signal).catch(() => undefined);
    await settle();
    expect(sent[0]?.signal.aborted).toBe(false);
    givenUp.abort();
    await settle();
    expect(sent[0]?.signal.aborted).toBe(true);
    port1.close();
  });

  // A WebRTC request's body fails with its signal's reason as the worker's cancel aborts it, so the
  // body's cancel that follows rejects with it: left unhandled, Chrome's console read "Uncaught (in
  // promise) AbortError: signal is aborted without reason at MessagePort" after a film page's warm-up
  // was dropped (the owner, the production NAS, 2026-11-01).
  it("leaves no rejection unhandled when the worker gives up a body its abort has failed", async () => {
    const unhandled: unknown[] = [];
    const heard = (reason: unknown): void => {
      unhandled.push(reason);
    };
    // The test runner's Node, whose types this browser package does not take.
    const { process } = globalThis as unknown as {
      readonly process: {
        on(event: "unhandledRejection", listener: (reason: unknown) => void): void;
        off(event: "unhandledRejection", listener: (reason: unknown) => void): void;
      };
    };
    process.on("unhandledRejection", heard);
    const { port1, port2 } = new MessageChannel();
    void answerLoad(aLoad, port2, {
      send: (request) => {
        const body = new ReadableStream<Uint8Array>({
          start: (controller) => {
            controller.enqueue(new Uint8Array(piece));
            request.signal.addEventListener("abort", () => {
              controller.error(request.signal.reason);
            });
          },
        });
        return Promise.resolve(new Response(body));
      },
      token: undefined,
      viewer: undefined,
    });
    const reader = (await responseFromPort(port1)).body?.getReader();
    await reader?.read();
    await reader?.cancel();
    await settle();
    process.off("unhandledRejection", heard);
    expect(unhandled).toEqual([]);
    port1.close();
  });

  it("fails the body when the page's load fails partway", async () => {
    const { port1, port2 } = new MessageChannel();
    let sent = 0;
    const failing = new ReadableStream<Uint8Array>({
      pull: (controller) => {
        sent += 1;
        if (sent > 2) {
          controller.error(new Error("the channel closed"));
          return;
        }
        controller.enqueue(new Uint8Array(piece));
      },
    });
    void answerLoad(aLoad, port2, {
      send: () => Promise.resolve(new Response(failing)),
      token: undefined,
      viewer: undefined,
    });
    const response = await responseFromPort(port1);
    await expect(response.arrayBuffer()).rejects.toThrow("the channel closed");
    port1.close();
  });

  it("passes a 425 on with its body and Retry-After, its head first", async () => {
    const { port1, port2 } = new MessageChannel();
    void answerLoad(aLoad, port2, {
      send: () =>
        Promise.resolve(
          new Response("not ready", { status: 425, headers: { "Retry-After": "1" } }),
        ),
      token: undefined,
      viewer: undefined,
    });
    const response = await responseFromPort(port1);
    expect(response.status).toBe(425);
    expect(response.headers.get("Retry-After")).toBe("1");
    expect(await response.text()).toBe("not ready");
    port1.close();
  });

  // skipWaiting and clients.claim hand a page still open to a newer worker, and a reloaded page is
  // controlled by the worker before until the newer one claims it: each end must still carry the
  // body for the other. A relay that spoke only its own protocol hung every load, and staging's
  // pages stayed black (2026-10-28). Real workers of each release: apps/web/src/sw.browser.test.ts.

  it("pulls a piece at a time from a page whose head says nothing of credit, as a page of a release before", async () => {
    const { port1, port2 } = new MessageChannel();
    const fromTheWorker = heardOn(port2);
    const answered = responseFromPort(port1);
    port2.postMessage({ kind: "head", status: 200, headers: [] });
    const reader = (await answered).body?.getReader();
    const first = reader?.read();
    expect(await fromTheWorker.next()).toBe("pull");
    const data = new Uint8Array([7]).buffer;
    port2.postMessage({ kind: "chunk", data }, [data]);
    expect((await first)?.value).toEqual(new Uint8Array([7]));
    const last = reader?.read();
    expect(await fromTheWorker.next()).toBe("pull");
    port2.postMessage({ kind: "end" });
    expect((await last)?.done).toBe(true);
    // Never a credit, which such a page does not answer.
    expect(fromTheWorker.all()).toEqual(["pull", "pull"]);
    port1.close();
  });

  it("says in its head that it sends as the worker grants room, and answers a bare pull with one piece, as a worker of a release before asks", async () => {
    const { port1, port2 } = new MessageChannel();
    const fromThePage = heardOn(port1);
    const source = piecesOf(2);
    void answerLoad(aLoad, port2, {
      send: () => Promise.resolve(new Response(source.stream)),
      token: undefined,
      viewer: undefined,
    });
    expect(await fromThePage.next()).toMatchObject({ kind: "head", status: 200, relay: "credit" });
    port1.postMessage("pull");
    expect(await fromThePage.next()).toMatchObject({ kind: "chunk" });
    await settle();
    // One piece for one pull: such a worker takes each reply as the answer to its pull.
    expect(fromThePage.all()).toHaveLength(2);
    port1.postMessage("pull");
    port1.postMessage("pull");
    expect(await fromThePage.next()).toMatchObject({ kind: "chunk" });
    expect(await fromThePage.next()).toEqual({ kind: "end" });
    port1.close();
  });

  it("answers a worker of a release before's pull of a body the page's load has none of", async () => {
    const { port1, port2 } = new MessageChannel();
    const fromThePage = heardOn(port1);
    void answerLoad(aLoad, port2, {
      send: () => Promise.resolve(new Response(null, { status: 200 })),
      token: undefined,
      viewer: undefined,
    });
    expect(await fromThePage.next()).toMatchObject({ kind: "head" });
    port1.postMessage("pull");
    expect(await fromThePage.next()).toEqual({ kind: "end" });
    port1.close();
  });

  it("fails a load the page could not make", async () => {
    const { port1, port2 } = new MessageChannel();
    void answerLoad(
      { kind: "orpheum-fetch", url: "https://app.example/api/v1/x", method: "GET", headers: [] },
      port2,
      {
        send: () => Promise.reject(new Error("the channel closed")),
        token: undefined,
        viewer: undefined,
      },
    );
    await expect(responseFromPort(port1)).rejects.toThrow("the channel closed");
    port1.close();
  });

  it("knows a page's word on its path from anything else", () => {
    expect(isPathMessage({ kind: "orpheum-path", path: "webrtc" })).toBe(true);
    expect(isPathMessage({ kind: "orpheum-path", path: "pigeon" })).toBe(false);
    expect(isPathMessage("webrtc")).toBe(false);
  });

  it("asks a page its path, as a worker started again must: a page carrying its media says so, any other says nothing", async () => {
    const carrying = {
      postMessage: (message: unknown, transfer: Transferable[]) => {
        const [port] = transfer;
        if (isPathQueryMessage(message) && port instanceof MessagePort) {
          port.postMessage({ kind: "orpheum-path", path: "webrtc" });
        }
      },
    };
    expect(await askPathOf(carrying)).toBe("webrtc");
    const silent = { postMessage: () => undefined };
    // A silence is no answer, which a worker does not keep (the owner's Mac, 2026-10-29).
    expect(await askPathOf(silent, 20)).toBeUndefined();
  });
});

/** A page's service worker container, as far as the page's side of the path uses it. */
interface FakeWorkers {
  readonly container: EventTarget & {
    controller: { state: string; postMessage: (message: unknown) => void } | null;
    readonly ready: Promise<{ readonly active: { postMessage: (message: unknown) => void } }>;
    register: () => Promise<void>;
    startMessages: () => void;
  };
  /** What each worker the page posted to was told, by the worker's name. */
  readonly told: Map<string, unknown[]>;
  /** A worker of that name, which keeps what the page tells it. */
  readonly worker: (name: string) => { state: string; postMessage: (message: unknown) => void };
}

function fakeWorkers(): FakeWorkers {
  const told = new Map<string, unknown[]>();
  const worker = (name: string): { state: string; postMessage: (message: unknown) => void } => ({
    state: "activated",
    postMessage: (message) => {
      told.set(name, [...(told.get(name) ?? []), message]);
    },
  });
  const container = Object.assign(new EventTarget(), {
    controller: null as { state: string; postMessage: (message: unknown) => void } | null,
    ready: Promise.resolve({ active: worker("active") }),
    register: () => Promise.resolve(),
    startMessages: () => undefined,
  });
  return { container, told, worker };
}

/** The worker's question on a page's path, sent to container, and the page's answer, if any. */
async function pathAnswerFrom(container: EventTarget): Promise<unknown> {
  const { port1, port2 } = new MessageChannel();
  const heard = heardOn(port1);
  container.dispatchEvent(
    new MessageEvent("message", { data: { kind: "orpheum-path-query" }, ports: [port2] }),
  );
  await settle();
  port1.close();
  return heard.all()[0];
}

describe("the page's side of its path, to its service worker", () => {
  // Each test its own copy of the module, whose page state (the path it carries media over) the
  // rest of the file's must not see.
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("says its media goes over HTTP until it carries it, so a worker keeps the answer, not a silence", async () => {
    const media = await import("./rtc-media.ts");
    const { container, worker } = fakeWorkers();
    container.controller = worker("active");
    vi.stubGlobal("navigator", { serviceWorker: container });
    media.sayMediaGoesOverHttp(container);
    expect(await pathAnswerFrom(container)).toEqual({ kind: "orpheum-path", path: "http" });

    await media.carryMediaOver(() => Promise.reject(new Error("unused")), noCredentials, "WebRTC");
    // Carrying, only carryMediaOver's answer comes, once.
    const { port1, port2 } = new MessageChannel();
    const heard = heardOn(port1);
    container.dispatchEvent(
      new MessageEvent("message", { data: { kind: "orpheum-path-query" }, ports: [port2] }),
    );
    await settle();
    expect(heard.all()).toEqual([{ kind: "orpheum-path", path: "webrtc" }]);
    port1.close();
  });

  it("reports each different note the worker gives on its path once", async () => {
    const media = await import("./rtc-media.ts");
    const { container } = fakeWorkers();
    const reported: string[] = [];
    media.reportPathNotes(container, (note) => {
      reported.push(note);
    });
    for (const note of ["silent", "silent", "no page"]) {
      container.dispatchEvent(
        new MessageEvent("message", { data: { kind: "orpheum-path-note", note } }),
      );
    }
    container.dispatchEvent(new MessageEvent("message", { data: { kind: "orpheum-path" } }));
    expect(reported).toEqual(["silent", "no page"]);
  });

  it("says its path again to each worker that takes it over", async () => {
    const media = await import("./rtc-media.ts");
    const { container, told, worker } = fakeWorkers();
    container.controller = worker("before");
    vi.stubGlobal("navigator", { serviceWorker: container });
    await media.carryMediaOver(() => Promise.reject(new Error("unused")), noCredentials, "WebRTC");
    container.controller = worker("after");
    container.dispatchEvent(new Event("controllerchange"));
    const said = { kind: "orpheum-path", path: "webrtc" };
    expect(told.get("before")).toEqual([said]);
    expect(told.get("after")).toEqual([said]);
  });

  it("fails, not waits for ever, when no live worker controls the page once asked to (Chrome's reload as a worker activates)", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    const media = await import("./rtc-media.ts");
    const control = await import("./worker-control.ts");
    const { container, told, worker } = fakeWorkers();
    // The page committed under the worker before, which the browser then stopped.
    container.controller = { ...worker("stopped"), state: "redundant" };
    vi.stubGlobal("navigator", { serviceWorker: container });
    const carried = media.carryMediaOver(
      () => Promise.reject(new Error("unused")),
      noCredentials,
      "WebRTC",
    );
    const failed = expect(carried).rejects.toBeInstanceOf(control.PageNotControlled);
    await vi.advanceTimersByTimeAsync(control.controlWaitMs);
    await failed;
    expect(told.get("active")).toEqual([{ kind: "orpheum-claim" }]);
    const { stepsSoFar } = await import("./steps.ts");
    expect(stepsSoFar().at(-1)).toMatch(/service worker: it did not take the page over in time$/);
  });

  // Firefox's private window has no navigator.serviceWorker; a browser may refuse the registration.
  it("fails as a browser that runs no service worker where it has none or refuses it", async () => {
    const media = await import("./rtc-media.ts");
    const control = await import("./worker-control.ts");
    const send = (): Promise<Response> => Promise.reject(new Error("unused"));
    vi.stubGlobal("navigator", {});
    await expect(media.carryMediaOver(send, noCredentials, "WebRTC")).rejects.toBeInstanceOf(
      control.NoServiceWorker,
    );
    const { stepsSoFar } = await import("./steps.ts");
    expect(stepsSoFar().at(-1)).toMatch(/service worker: the browser has none$/);
    const { container } = fakeWorkers();
    container.register = () => Promise.reject(new DOMException("refused", "SecurityError"));
    vi.stubGlobal("navigator", { serviceWorker: container });
    await expect(media.carryMediaOver(send, noCredentials, "WebRTC")).rejects.toBeInstanceOf(
      control.NoServiceWorker,
    );
    expect(stepsSoFar().at(-1)).toMatch(/service worker: the browser refused it$/);
  });

  it("carries its media once the worker it asked claims it", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    const media = await import("./rtc-media.ts");
    const control = await import("./worker-control.ts");
    const { container, told, worker } = fakeWorkers();
    vi.stubGlobal("navigator", { serviceWorker: container });
    const carried = media.carryMediaOver(
      () => Promise.reject(new Error("unused")),
      noCredentials,
      "WebRTC",
    );
    await vi.advanceTimersByTimeAsync(control.controlWaitMs / 2);
    container.controller = worker("active");
    container.dispatchEvent(new Event("controllerchange"));
    await carried;
    expect(told.get("active")).toContainEqual({ kind: "orpheum-path", path: "webrtc" });
    // Each step it took is told in what happened, should a later one fail.
    const { stepsSoFar } = await import("./steps.ts");
    expect(stepsSoFar().map((step) => step.replace(/^\d+ ms /, ""))).toEqual([
      "service worker: registering (the page is not controlled)",
      "service worker: ready",
      "service worker: it controls the page",
    ]);
  });
});

const noCredentials = (): { token: undefined; viewer: undefined } => ({
  token: undefined,
  viewer: undefined,
});

describe("what carries a load, for a stuck chunk's log (the owner, 2026-10-29)", () => {
  it("is http while the page carries no media itself, and names a WebRTC connection's state", () => {
    expect(mediaCarrierOf("https://app.example/api/v1/bytes/media/chunks/t/00001.m4s")).toBe(
      "http",
    );
    expect(carrierText({ connection: 2, upMs: 41_400, underWay: 3, sinceDeliveredMs: 5512 })).toBe(
      "WebRTC connection 2, up 41 s, 3 under way, last bytes 5512 ms ago",
    );
    expect(
      carrierText({ connection: 1, upMs: 900, underWay: 1, sinceDeliveredMs: undefined }),
    ).toBe("WebRTC connection 1, up 1 s, 1 under way, no bytes yet");
    // One the transport ended itself says why, for a loader waiting on a dead connection.
    expect(
      carrierText({
        connection: 3,
        upMs: 60_000,
        underWay: 2,
        sinceDeliveredMs: 40_000,
        ended: "nothing came for 40000 ms, nor over its connection, which was replaced",
      }),
    ).toBe(
      "WebRTC connection 3, up 60 s, 2 under way, last bytes 40000 ms ago; ended: nothing came for 40000 ms, nor over its connection, which was replaced",
    );
  });

  it("answers from its own state where the loader's lookup comes back to this copy, as when the loader and the client are one build (the owner's Chrome offline, staging, 2026-10-29; VQ133)", async () => {
    vi.resetModules();
    const media = await import("./media-carrier.ts");
    const chunk = "https://client.example/api/v1/bytes/media/chunks/t/00001.m4s";
    // One build's loader hands the client the copy they share: its lookup is mediaCarrierOf itself,
    // which asked itself until the stack ran out at each chunk that failed to load.
    media.readMediaCarrierFrom(media.mediaCarrierOf);
    for (let failed = 0; failed < 100; failed += 1) {
      expect(media.mediaCarrierOf(chunk)).toBe("http");
    }
    // A lookup by way of another function that comes back here is answered from here too.
    media.readMediaCarrierFrom((url) => `the loader's word: ${media.mediaCarrierOf(url)}`);
    expect(media.mediaCarrierOf(chunk)).toBe("the loader's word: http");
  });
});
