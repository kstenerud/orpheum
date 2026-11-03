// SPDX-License-Identifier: MPL-2.0

/**
 * The WebRTC path's media relay through a real service worker (sw.ts itself, registered as a module
 * from its source), in Chromium and Firefox: a page answers each load its worker asks for, and the
 * body comes back whole and in order, whichever release each end is of. skipWaiting and
 * clients.claim hand an open page to a newer worker, and a reloaded page is controlled by the older
 * one until the newer one claims it; a relay that spoke only its own protocol hung every load across
 * that, and staging's pages stayed black (2026-10-28).
 */

import {
  answerLoad,
  isFetchMessage,
  isPathQueryMessage,
  relayedAheadBytes,
  type FetchMessage,
} from "@orpheum/sdk";
import { afterEach, describe, expect, it } from "vitest";

import { pullPageAnswer } from "./test-pull-relay.ts";

/** A data channel's message: the size of a body's piece over the WebRTC path. */
const piece = 64 * 1024;

/** Pieces enough to outrun any window the relay keeps ahead of its reader. */
const pieceCount = 64;

/**
 * How long a relayed load may go with nothing of it come, its head or a piece of its body, before the
 * test takes it for hung. Measured from its last progress, not its start: the release before's pull
 * takes a round trip between page and worker for each of the 64 pieces, and in Firefox with the whole
 * browser-mode run beside it on a loaded machine each took about 63 ms, so the whole body took 5.3 s
 * (2026-11-03) while no piece waited past 1.2 s. A hung relay sends nothing at all.
 */
const hungAfterMs = 5000;

/** The pages and workers a test set up, taken down after it. */
const made: { frames: HTMLIFrameElement[]; registrations: ServiceWorkerRegistration[] } = {
  frames: [],
  registrations: [],
};

afterEach(async () => {
  for (const frame of made.frames.splice(0)) {
    frame.remove();
  }
  await Promise.all(made.registrations.splice(0).map((registration) => registration.unregister()));
});

/** script registered as a module service worker over scope, once it is active. */
async function activeWorker(script: string, scope: string): Promise<void> {
  const registration = await navigator.serviceWorker.register(script, { type: "module", scope });
  made.registrations.push(registration);
  const worker = registration.installing ?? registration.waiting ?? registration.active;
  if (worker === null) {
    throw new Error(`${script} did not install`);
  }
  await new Promise<void>((resolve, reject) => {
    const settled = (): void => {
      if (worker.state === "activated") {
        resolve();
      } else if (worker.state === "redundant") {
        reject(new Error(`${script} failed to install`));
      }
    };
    worker.addEventListener("statechange", settled);
    settled();
  });
}

/**
 * The scope of a test's worker, which is also its page's address: a file of the app's own, so the page
 * opens in every browser (Firefox opens Vite's empty 404 as an error page, of another origin), with a
 * query of the test's own, so each test's worker controls its page alone.
 */
function scopeNamed(name: string): string {
  return `/apps/web/src/sw.ts?page=${name}`;
}

/**
 * The page at scope, controlled by its worker, which answers each load the worker asks of it with
 * answer and says it carries its media over WebRTC.
 */
async function pageAnswering(
  scope: string,
  answer: (message: FetchMessage, port: MessagePort) => void,
): Promise<Window> {
  const frame = document.createElement("iframe");
  made.frames.push(frame);
  const loaded = new Promise((resolve) => {
    frame.addEventListener("load", resolve, { once: true });
  });
  frame.src = scope;
  document.body.append(frame);
  await loaded;
  const page = frame.contentWindow;
  const controller = page?.navigator.serviceWorker.controller;
  if (page === null || controller === null || controller === undefined) {
    throw new Error("the page under the worker's scope is not controlled by it");
  }
  const workers = page.navigator.serviceWorker;
  workers.addEventListener("message", (event: MessageEvent<unknown>) => {
    const [port] = event.ports;
    if (port !== undefined && isFetchMessage(event.data)) {
      answer(event.data, port);
    }
    if (port !== undefined && isPathQueryMessage(event.data)) {
      port.postMessage({ kind: "orpheum-path", path: "webrtc" });
    }
  });
  workers.startMessages();
  controller.postMessage({ kind: "orpheum-path", path: "webrtc" });
  return page;
}

/** What the page's transport answers: a 206 of pieceCount pieces, each its own buffer of its index. */
function piecesFromTheCore(): Promise<Response> {
  let sent = 0;
  const body = new ReadableStream<Uint8Array>(
    {
      pull: (controller) => {
        if (sent === pieceCount) {
          controller.close();
          return;
        }
        controller.enqueue(new Uint8Array(piece).fill(sent));
        sent += 1;
      },
    },
    { highWaterMark: 0 },
  );
  return Promise.resolve(
    new Response(body, { status: 206, headers: { "Content-Range": "bytes 0-1/2" } }),
  );
}

/**
 * The page's load of a chunk under the API, failing the test when it hangs: when nothing of it, its
 * head or a piece of its body, comes for hungAfterMs.
 */
async function loadedBy(page: Window): Promise<{ response: Response; body: Uint8Array }> {
  let stall: ReturnType<typeof setTimeout> | undefined;
  let hang: (error: Error) => void = () => undefined;
  const hung = new Promise<never>((_, reject) => {
    hang = reject;
  });
  const moved = (what: string): void => {
    clearTimeout(stall);
    stall = setTimeout(() => {
      hang(
        new Error(`the relayed load hung: nothing came in ${String(hungAfterMs)} ms after ${what}`),
      );
    }, hungAfterMs);
  };
  const load = (async () => {
    moved("its start");
    const response = await page.fetch("/api/v1/chunks/relayed");
    const reader = response.body?.getReader();
    if (reader === undefined) {
      throw new Error("the relayed load has no body");
    }
    const pieces: Uint8Array[] = [];
    moved("its head");
    for (let read = await reader.read(); !read.done; read = await reader.read()) {
      pieces.push(read.value);
      moved(`${String(pieces.length)} reads of its body`);
    }
    const body = new Uint8Array(pieces.reduce((length, read) => length + read.byteLength, 0));
    let at = 0;
    for (const read of pieces) {
      body.set(read, at);
      at += read.byteLength;
    }
    return { response, body };
  })();
  try {
    return await Promise.race([load, hung]);
  } finally {
    clearTimeout(stall);
  }
}

/** Expects the whole body, every piece in its place. */
function expectEveryPieceInOrder(loaded: { response: Response; body: Uint8Array }): void {
  expect(loaded.response.status).toBe(206);
  expect(loaded.response.headers.get("Content-Range")).toBe("bytes 0-1/2");
  expect(loaded.body.byteLength).toBe(pieceCount * piece);
  const firsts = Array.from({ length: pieceCount }, (_, index) => loaded.body[index * piece]);
  expect(firsts).toEqual(Array.from({ length: pieceCount }, (_, index) => index));
}

/** This release's page side, as carryMediaOver answers. */
function thisPage(message: FetchMessage, port: MessagePort): void {
  void answerLoad(message, port, { send: piecesFromTheCore, token: undefined, viewer: undefined });
}

/** The released page side, which answers each "pull" with one piece. */
function pagePulledFrom(message: FetchMessage, port: MessagePort): void {
  void pullPageAnswer(message, port, piecesFromTheCore);
}

describe("the media relay through a real service worker", () => {
  it("carries a page's load through this release's worker, the body ahead of its reader", async () => {
    await activeWorker("/apps/web/src/sw.ts", scopeNamed("this"));
    let fromTheWorker = 0;
    const page = await pageAnswering(scopeNamed("this"), (message, port) => {
      port.addEventListener("message", () => {
        fromTheWorker += 1;
      });
      thisPage(message, port);
    });
    expectEveryPieceInOrder(await loadedBy(page));
    // Room granted a share of the window at a time, not a pull for each of the 64 pieces, each a
    // round trip through the busy page: a Fire TV's 30 ms carried 17 Mb/s (staging, 2026-10-28).
    expect(fromTheWorker).toBeLessThanOrEqual(
      (pieceCount * piece + relayedAheadBytes) / (relayedAheadBytes / 4),
    );
  });

  it("carries the load of a page of the release before, which this worker took over", async () => {
    await activeWorker("/apps/web/src/sw.ts", scopeNamed("old-page"));
    const page = await pageAnswering(scopeNamed("old-page"), pagePulledFrom);
    expectEveryPieceInOrder(await loadedBy(page));
  });

  it("carries a page's load through a worker of the release before, as a reloaded page meets", async () => {
    await activeWorker("/apps/web/src/test-pull-worker.ts", scopeNamed("old-worker"));
    const page = await pageAnswering(scopeNamed("old-worker"), thisPage);
    expectEveryPieceInOrder(await loadedBy(page));
  });
});
