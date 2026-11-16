// SPDX-License-Identifier: MPL-2.0

import {
  discoveryWaitMs,
  isPathNoteMessage,
  workerProbeAnswer,
  workerProbePath,
} from "@orpheum/sdk";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { sha256Of } from "./client-files.ts";
import { keptImagesCache, keptImagesMost } from "./kept-images.ts";
import {
  answer,
  answeringLate,
  askWaitMs,
  load,
  nameFiles,
  pageOf,
  patientWaitMs,
  script,
  settledOver,
  startWorker,
  tell,
  type FakeCache,
  type FakePage,
  type Worker,
  useVitest,
} from "./test-service-worker.ts";

useVitest(vi);

describe("the service worker under a loader (D308 §6)", () => {
  let stores: Map<string, FakeCache>;
  let sha256: string;

  beforeEach(async () => {
    stores = new Map();
    sha256 = await sha256Of(script.slice().buffer);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("sends a page and its assets to the network as they would go, until a loader names the client's files", async () => {
    const worker = await startWorker(
      stores,
      [],
      vi.fn(() => Promise.resolve(answer("the page"))),
    );
    expect(await (await load(worker, "/", { by: "p", mode: "navigate" }))?.text()).toBe("the page");
    expect(await (await load(worker, "/assets/client-main.js", { by: "p" }))?.text()).toBe(
      "the page",
    );
    expect(worker.network).toHaveBeenCalledTimes(2);
  });

  it("answers the probe itself, never the network, so a page knows its worker is not bypassed (agent-failures A51)", async () => {
    const worker = await startWorker(
      stores,
      [],
      vi.fn(() => Promise.resolve(answer("the discovery's 404", 404))),
    );
    await nameFiles(worker, [{ path: "/assets/client-main.js", sha256 }]);
    const probed = await load(worker, workerProbePath, { by: "p", mode: "same-origin" });
    expect(await probed?.text()).toBe(workerProbeAnswer);
    expect(worker.network).not.toHaveBeenCalled();
  });

  it("answers a named file fetched over the page's path when its hash is the signed one, and keeps it by that hash", async () => {
    const send = vi.fn(() => Promise.resolve(answer(script)));
    const page = pageOf("p", "webrtc", send);
    const worker = await startWorker(
      stores,
      [page],
      vi.fn<(request: Request) => Promise<Response>>(),
    );
    expect(await nameFiles(worker, [{ path: "assets/client-main.js", sha256 }])).toBe("kept");
    await tell(worker, { kind: "orpheum-path", path: "webrtc" }, { source: "p" });

    const first = await load(worker, "/assets/client-main.js", { by: "p" });
    expect(new Uint8Array(await (first ?? Response.error()).arrayBuffer())).toEqual(script);
    const again = await load(worker, "/assets/client-main.js", { by: "p" });
    expect(new Uint8Array(await (again ?? Response.error()).arrayBuffer())).toEqual(script);

    expect(page.asked).toEqual(["/assets/client-main.js"]);
    expect(worker.network).not.toHaveBeenCalled();
  });

  it("refuses a file whose bytes are not the ones the core signed, and keeps none", async () => {
    const page = pageOf("p", "webrtc", () => Promise.resolve(answer("console.log('another');")));
    const worker = await startWorker(
      stores,
      [page],
      vi.fn<(request: Request) => Promise<Response>>(),
    );
    expect(await nameFiles(worker, [{ path: "assets/client-main.js", sha256 }])).toBe("kept");
    await tell(worker, { kind: "orpheum-path", path: "webrtc" }, { source: "p" });

    expect((await load(worker, "/assets/client-main.js", { by: "p" }))?.status).toBe(502);
    expect((await load(worker, "/assets/client-main.js", { by: "p" }))?.status).toBe(502);
    expect(page.asked).toHaveLength(2);
  });

  it("asks the page again, once, for a file its first fetch lost, so a first visit's dropped file still loads (the owner's Firefox, 2026-11-01)", async () => {
    const send = vi
      .fn<(request: Request) => Promise<Response>>()
      .mockResolvedValueOnce(answer("the connection dropped it", 503))
      .mockResolvedValue(answer(script));
    const page = pageOf("p", "webrtc", send);
    const worker = await startWorker(
      stores,
      [page],
      vi.fn<(request: Request) => Promise<Response>>(),
    );
    expect(await nameFiles(worker, [{ path: "assets/client-main.js", sha256 }])).toBe("kept");
    await tell(worker, { kind: "orpheum-path", path: "webrtc" }, { source: "p" });

    const answered = await load(worker, "/assets/client-main.js", { by: "p" });
    expect(answered?.status).toBe(200);
    expect(new Uint8Array(await (answered ?? Response.error()).arrayBuffer())).toEqual(script);
    expect(page.asked).toEqual(["/assets/client-main.js", "/assets/client-main.js"]);
  });

  it("takes a booting page's file from the network, kept only when it is the signed one", async () => {
    const page = pageOf("p", "http", () => Promise.reject(new Error("no path yet")));
    const network = vi.fn(() => Promise.resolve(answer(script)));
    const worker = await startWorker(stores, [page], network);
    expect(await nameFiles(worker, [{ path: "assets/client-main.js", sha256 }])).toBe("kept");

    expect((await load(worker, "/assets/client-main.js", { by: "p" }))?.status).toBe(200);
    expect((await load(worker, "/assets/client-main.js", { by: "p" }))?.status).toBe(200);
    expect(network).toHaveBeenCalledTimes(1);
  });

  it("fetches a client's file the discovery has not through the page that asked for it, though it said its path late (the owner's Mac, 2026-10-29)", async () => {
    let path: "http" | "webrtc" = "http";
    const send = vi.fn(() => Promise.resolve(answer(script)));
    const page: FakePage = {
      ...pageOf("p", "http", send),
      postMessage(message, transfer) {
        pageOf("p", path, send).postMessage(message, transfer);
        // Asked once, the page is busy mounting the client; asked again, it says its path.
        path = "webrtc";
      },
    };
    const network = vi.fn(() => Promise.resolve(answer("404 page not found", 404)));
    const worker = await startWorker(stores, [page], network);
    expect(await nameFiles(worker, [{ path: "assets/client-main.js", sha256 }])).toBe("kept");

    const answered = await load(worker, "/assets/client-main.js?client", { by: "p" });
    expect(answered?.status).toBe(200);
    expect(new Uint8Array(await (answered ?? Response.error()).arrayBuffer())).toEqual(script);
    expect(send).toHaveBeenCalledTimes(1);
  });

  it("answers a client's file no page can fetch over its path as such, never with the discovery's 404", async () => {
    const page = pageOf("p", "http", () => Promise.reject(new Error("no path")));
    const network = vi.fn(() => Promise.resolve(answer("404 page not found", 404)));
    const worker = await startWorker(stores, [page], network);
    expect(await nameFiles(worker, [{ path: "assets/client-main.js", sha256 }])).toBe("kept");

    const answered = await load(worker, "/assets/client-main.js?client", { by: "p" });
    expect(answered?.status).toBe(502);
    expect(await answered?.text()).toContain("no page here carries a path to the server");
  });

  it("remembers the named files when the browser starts it again", async () => {
    const page = pageOf("p", "webrtc", () => Promise.resolve(answer(script)));
    const first = await startWorker(
      stores,
      [page],
      vi.fn<(request: Request) => Promise<Response>>(),
    );
    expect(await nameFiles(first, [{ path: "assets/client-main.js", sha256 }])).toBe("kept");
    await tell(first, { kind: "orpheum-path", path: "webrtc" }, { source: "p" });
    await load(first, "/assets/client-main.js", { by: "p" });

    const again = await startWorker(stores, [], vi.fn<(request: Request) => Promise<Response>>());
    const kept = await load(again, "/assets/client-main.js", { by: "p" });
    expect(new Uint8Array(await (kept ?? Response.error()).arrayBuffer())).toEqual(script);
    expect(again.network).not.toHaveBeenCalled();
  });

  it("finds a client's file the loader named to another worker of the origin, never answering it with the discovery's 404 (the owner's Chrome, 2026-10-29)", async () => {
    const page = pageOf("p", "webrtc", () => Promise.resolve(answer(script)));
    const network = vi.fn(() => Promise.resolve(answer("404 page not found", 404)));
    // This worker read the list, then empty, before the loader named the files to another.
    const reading = await startWorker(stores, [page], network);
    expect((await load(reading, "/assets/loader-main.js", { by: "p" }))?.status).toBe(404);
    const told = await startWorker(stores, [page], network);
    expect(await nameFiles(told, [{ path: "assets/client-main.js", sha256 }])).toBe("kept");

    const answered = await load(reading, "/assets/client-main.js", { by: "p" });
    expect(answered?.status).toBe(200);
    expect(new Uint8Array(await (answered ?? Response.error()).arrayBuffer())).toEqual(script);
    expect(page.asked).toEqual(["/assets/client-main.js"]);
  });

  it("opens the loader's page as its origin answers now, and as kept when the origin fails", async () => {
    const pages: (string | Error | number)[] = [
      "the loader",
      "the loader, updated",
      new Error("offline"),
      400,
    ];
    const network = vi.fn(() => {
      const next = pages.shift() ?? "";
      if (next instanceof Error) {
        return Promise.reject(next);
      }
      return Promise.resolve(
        typeof next === "number" ? new Response("refused", { status: next }) : answer(next),
      );
    });
    const worker = await startWorker(stores, [], network);
    expect(await nameFiles(worker, [{ path: "assets/client-main.js", sha256 }])).toBe("kept");

    expect(await (await load(worker, "/households", { by: "p", mode: "navigate" }))?.text()).toBe(
      "the loader",
    );
    // An update is opened at once, not on the visit after (the production NAS, 2026-10-19).
    expect(await (await load(worker, "/", { by: "p", mode: "navigate" }))?.text()).toBe(
      "the loader, updated",
    );
    // The origin unreachable, or refusing as another core's cookies can make it: the page as kept.
    for (let failure = 0; failure < 2; failure += 1) {
      expect(await (await load(worker, "/", { by: "p", mode: "navigate" }))?.text()).toBe(
        "the loader, updated",
      );
    }
  });

  it("opens the loader's page as kept when its origin has not answered within discoveryWaitMs, and keeps the late answer for the visit after (D486)", async () => {
    const lateMs = discoveryWaitMs * 4;
    const pages: (() => Promise<Response>)[] = [
      () => Promise.resolve(answer("the loader")),
      () =>
        new Promise((resolve) => {
          setTimeout(() => {
            resolve(answer("the loader, updated"));
          }, lateMs);
        }),
      // Lie-fi: a link up that carries nothing.
      () => new Promise<Response>(() => undefined),
    ];
    const network = vi.fn(() =>
      (pages.shift() ?? (() => Promise.reject(new Error("a fourth load"))))(),
    );
    const worker = await startWorker(stores, [], network);
    expect(await nameFiles(worker, [{ path: "assets/client-main.js", sha256 }])).toBe("kept");
    expect(await (await load(worker, "/", { by: "p", mode: "navigate" }))?.text()).toBe(
      "the loader",
    );
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });

    const slow = load(worker, "/", { by: "p", mode: "navigate" });
    expect(await settledOver(slow, discoveryWaitMs - 200)).toBeUndefined();
    expect(await (await settledOver(slow, 400))?.text()).toBe("the loader");
    await settledOver(Promise.all(worker.kept), lateMs);

    const stuck = load(worker, "/", { by: "p", mode: "navigate" });
    expect(await (await settledOver(stuck, discoveryWaitMs + 200))?.text()).toBe(
      "the loader, updated",
    );
  });

  it("waits out a late origin for the loader's page when it has none kept (D486)", async () => {
    const lateMs = discoveryWaitMs * 4;
    const network = vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          setTimeout(() => {
            resolve(answer("the loader"));
          }, lateMs);
        }),
    );
    const worker = await startWorker(stores, [], network);
    expect(await nameFiles(worker, [{ path: "assets/client-main.js", sha256 }])).toBe("kept");
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });

    const first = load(worker, "/", { by: "p", mode: "navigate" });
    expect(await settledOver(first, discoveryWaitMs + 200)).toBeUndefined();
    expect(await (await settledOver(first, lateMs))?.text()).toBe("the loader");
  });

  it("answers the loader's own files as kept, and asks the origin for them again behind", async () => {
    const files = ["body{}", "body{color:red}"];
    const network = vi.fn(() => Promise.resolve(answer(files.shift() ?? "")));
    const worker = await startWorker(stores, [], network);
    expect(await nameFiles(worker, [{ path: "assets/client-main.js", sha256 }])).toBe("kept");

    expect(await (await load(worker, "/assets/loader-a1b2.css", { by: "p" }))?.text()).toBe(
      "body{}",
    );
    expect(await (await load(worker, "/assets/loader-a1b2.css", { by: "p" }))?.text()).toBe(
      "body{}",
    );
    await Promise.all(worker.kept);
    expect(network).toHaveBeenCalledTimes(2);
  });

  it("sends a page's media through it when started again, though it answers late: its client origin serves none", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    const send = vi.fn(() => Promise.resolve(new Response("a chunk", { status: 200 })));
    const first = await startWorker(stores, [], vi.fn<(request: Request) => Promise<Response>>());
    expect(await nameFiles(first, [{ path: "assets/client-main.js", sha256 }])).toBe("kept");
    // Started again, by the browser or DevTools, it knows no page's path, and this page is too busy
    // to answer the question within the second a load's ordinary question waits.
    const page = answeringLate(pageOf("p", "unanswered", send), askWaitMs + 500);
    const network = vi.fn<(request: Request) => Promise<Response>>();
    const again = await startWorker(stores, [page], network);
    for (const chunk of ["00170", "00171"]) {
      const answered = await settledOver(
        load(again, `/api/v1/bytes/media/chunks/t/q400k360p/${chunk}.m4s`, { by: "p" }),
        patientWaitMs,
      );
      expect(await answered?.text()).toBe("a chunk");
    }
    expect(page.asked).toEqual([
      "/api/v1/bytes/media/chunks/t/q400k360p/00170.m4s",
      "/api/v1/bytes/media/chunks/t/q400k360p/00171.m4s",
    ]);
    expect(network).not.toHaveBeenCalled();
  });

  // hls.js giving a chunk up before its head came left the page's request over WebRTC holding its
  // connection's room (the owner's Chrome, staging, 2026-10-29).
  it("cancels the page's load of a chunk the page gave up before its head came", async () => {
    const sent: Request[] = [];
    const send = (request: Request): Promise<Response> => {
      sent.push(request);
      return new Promise<Response>(() => undefined); // its head never comes
    };
    const worker = await startWorker(
      stores,
      [pageOf("p", "webrtc", send)],
      vi.fn<(request: Request) => Promise<Response>>(),
    );
    const givenUp = new AbortController();
    void load(worker, "/api/v1/bytes/media/chunks/t/q400k360p/00002.m4s", {
      by: "p",
      signal: givenUp.signal,
    })?.catch(() => undefined);
    await vi.waitFor(() => {
      expect(sent).toHaveLength(1);
    });
    expect(sent[0]?.signal.aborted).toBe(false);
    givenUp.abort();
    await vi.waitFor(() => {
      expect(sent[0]?.signal.aborted).toBe(true);
    });
  });

  it("answers a media load no page carries a path for as such, never sending it to the client origin's 404", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    // A page still finding its core: it answers neither a question nor a load.
    const page = pageOf("p", "unanswered", () => Promise.reject(new Error("no path yet")));
    const finding: FakePage = {
      ...page,
      postMessage(message, transfer) {
        if (isPathNoteMessage(message)) {
          page.postMessage(message, transfer);
        }
      },
    };
    const network = vi.fn(() => Promise.resolve(answer("404 page not found", 404)));
    const worker = await startWorker(stores, [finding], network);
    expect(await nameFiles(worker, [{ path: "assets/client-main.js", sha256 }])).toBe("kept");

    const answered = await settledOver(
      load(worker, "/api/v1/bytes/art/a1/portrait-m.webp", { by: "p" }),
      patientWaitMs,
    );
    expect(answered?.status).toBe(502);
    expect(await answered?.text()).toContain("no page here carried a path to the server");
    expect(network).not.toHaveBeenCalled();
    expect(page.notes).toEqual([
      `the service worker could not load a media file: no page here carried a path to the server within ${String(patientWaitMs)} ms`,
    ]);
  });

  it("takes a media load for a loader's, though it read no client's files before the loader named them to another worker (the owner's Mac, 2026-10-29)", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    const page = pageOf("p", "unanswered", () => Promise.reject(new Error("no path yet")));
    const network = vi.fn(() => Promise.resolve(answer("404 page not found", 404)));
    // This worker read the list, then empty, before the loader named the files to another.
    const reading = await startWorker(stores, [page], network);
    expect((await load(reading, "/assets/loader-main.js", { by: "p" }))?.status).toBe(404);
    const told = await startWorker(stores, [page], network);
    expect(await nameFiles(told, [{ path: "assets/client-main.js", sha256 }])).toBe("kept");
    network.mockClear();

    const answered = await settledOver(
      load(reading, "/api/v1/bytes/art/a1/portrait-m.webp?again=6", { by: "p" }),
      patientWaitMs,
    );
    expect(answered?.status).toBe(502);
    expect(network).not.toHaveBeenCalled();
  });

  it("sends a page's media through it though it was finding the core when a file was asked", async () => {
    let path: "http" | "webrtc" = "http";
    const send = vi.fn(() => Promise.resolve(new Response("a chunk", { status: 200 })));
    const page: FakePage = {
      ...pageOf("p", "http", send),
      postMessage(message, transfer) {
        pageOf("p", path, send).postMessage(message, transfer);
      },
    };
    const network = vi.fn(() => Promise.resolve(answer(script)));
    const worker = await startWorker(stores, [page], network);
    expect(await nameFiles(worker, [{ path: "assets/client-main.js", sha256 }])).toBe("kept");
    expect((await load(worker, "/assets/client-main.js", { by: "p" }))?.status).toBe(200);
    path = "webrtc";
    const answered = await load(worker, "/api/v1/bytes/media/chunks/t/q400k360p/00001.m4s", {
      by: "p",
    });
    expect(await answered?.text()).toBe("a chunk");
    expect(network).toHaveBeenCalledTimes(1);
  });

  it("leaves another origin's loads alone", async () => {
    const worker = await startWorker(stores, [], vi.fn<(request: Request) => Promise<Response>>());
    expect(await nameFiles(worker, [{ path: "assets/client-main.js", sha256 }])).toBe("kept");
    let wasAnswered = false;
    worker.listeners.get("fetch")?.({
      request: new Request("https://app.orpheum-app.com/assets/client-main.js"),
      clientId: "p",
      respondWith: () => {
        wasAnswered = true;
      },
      waitUntil: () => undefined,
    });
    expect(wasAnswered).toBe(false);
  });
});

describe("the service worker at a core's own origin", () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("keeps no silence for a path: a page that said nothing is asked again, and its media goes through it once it answers", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    let path: "unanswered" | "webrtc" = "unanswered";
    const send = vi.fn(() => Promise.resolve(new Response("a chunk", { status: 200 })));
    const notes: string[] = [];
    const page: FakePage = {
      ...pageOf("p", "unanswered", send),
      notes,
      postMessage(message, transfer) {
        if (isPathNoteMessage(message)) {
          notes.push(message.note);
        }
        pageOf("p", path, send).postMessage(message, transfer);
      },
    };
    const network = vi.fn(() => Promise.resolve(new Response("from the network")));
    const worker = await startWorker(new Map(), [page], network);
    const chunk = "/api/v1/bytes/media/chunks/t/q400k360p/00001.m4s";

    // Still finding its core, the page says nothing: this load goes to the network, and the page is
    // told why.
    const first = await settledOver(load(worker, chunk, { by: "p" }), askWaitMs);
    expect(await first?.text()).toBe("from the network");
    expect(notes).toEqual([
      `the service worker sent a media load to the network: asked its path, the page said nothing within ${String(askWaitMs)} ms`,
    ]);

    path = "webrtc";
    const next = await settledOver(load(worker, chunk, { by: "p" }), askWaitMs);
    expect(await next?.text()).toBe("a chunk");
    expect(network).toHaveBeenCalledTimes(1);
  });
});

describe("the service worker's kept images (D485)", () => {
  /** An image's address that names its content, as artUrl makes it. */
  const poster = `/api/v1/bytes/art/${"ab".repeat(32)}/portrait-m.webp`;
  const posterBytes = new Uint8Array([82, 73, 70, 70]);

  function image(status = 200): Response {
    return new Response(status === 200 ? posterBytes : null, {
      status,
      headers: { "Content-Type": "image/webp" },
    });
  }

  /** What a page's load of path is answered, read whole, once whatever it kept behind is kept. */
  async function loaded(
    worker: Worker,
    path: string,
  ): Promise<{ readonly status: number; readonly bytes: Uint8Array } | undefined> {
    const answered = await load(worker, path, { by: "p" });
    const bytes = new Uint8Array((await answered?.arrayBuffer()) ?? new ArrayBuffer(0));
    await Promise.all(worker.kept);
    return answered === undefined ? undefined : { status: answered.status, bytes };
  }

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("keeps an image the page fetched over its path by its address, and answers it from there, a worker started again too", async () => {
    const stores = new Map<string, FakeCache>();
    const page = pageOf("p", "webrtc", () => Promise.resolve(image()));
    const network = vi.fn<(request: Request) => Promise<Response>>();
    const worker = await startWorker(stores, [page], network);
    await tell(worker, { kind: "orpheum-path", path: "webrtc" }, { source: "p" });

    for (const asked of [poster, poster]) {
      expect((await loaded(worker, asked))?.bytes).toEqual(posterBytes);
    }
    const again = await startWorker(stores, [page], network);
    expect((await loaded(again, poster))?.bytes).toEqual(posterBytes);

    expect(page.asked).toEqual([poster]);
    expect([...(stores.get(keptImagesCache)?.keys() ?? [])]).toEqual([poster]);
    expect(network).not.toHaveBeenCalled();
  });

  // A kept image is assumed to fail one day (D494): a page asks for an image again only once it failed
  // to show, so the kept one is dropped, fetched afresh and kept anew, and the page's log is told. A
  // half poster kept once was drawn as "No poster" at every load after (the owner's Firefox on
  // Android, staging, 2026-11-16).
  it("drops a kept image the page asks for again, fetches it afresh and tells the page's log", async () => {
    const stores = new Map<string, FakeCache>();
    const half = posterBytes.slice(0, 2);
    const answers = [half, posterBytes];
    const page = pageOf("p", "webrtc", () =>
      Promise.resolve(
        new Response(answers.shift() ?? null, { headers: { "Content-Type": "image/webp" } }),
      ),
    );
    const worker = await startWorker(
      stores,
      [page],
      vi.fn<(request: Request) => Promise<Response>>(),
    );
    await tell(worker, { kind: "orpheum-path", path: "webrtc" }, { source: "p" });

    expect((await loaded(worker, poster))?.bytes).toEqual(half);
    expect((await loaded(worker, `${poster}?again=1`))?.bytes).toEqual(posterBytes);
    expect((await loaded(worker, poster))?.bytes).toEqual(posterBytes);

    expect(page.asked).toEqual([poster, poster]);
    expect(page.notes).toEqual([
      `the service worker dropped a kept image the page could not show, and fetched it again: ${poster}`,
    ]);
  });

  it("keeps an image under a loader too, fetched through the page carrying its path", async () => {
    const stores = new Map<string, FakeCache>();
    const page = pageOf("p", "webrtc", () => Promise.resolve(image()));
    const worker = await startWorker(
      stores,
      [page],
      vi.fn<(request: Request) => Promise<Response>>(),
    );
    const sha256 = await sha256Of(script.slice().buffer);
    expect(await nameFiles(worker, [{ path: "assets/client-main.js", sha256 }])).toBe("kept");

    expect((await loaded(worker, poster))?.bytes).toEqual(posterBytes);
    expect((await loaded(worker, poster))?.bytes).toEqual(posterBytes);
    expect(page.asked).toEqual([poster]);
  });

  it("keeps no redirect to the nearest size, no answer a redirect brought, no failure and no image its address does not name", async () => {
    const stores = new Map<string, FakeCache>();
    // A direct path's fetch follows the 307 to the nearest size, and answers that one's 200.
    const followed = (): Response => {
      const answer = image();
      Object.defineProperty(answer, "redirected", { value: true });
      return answer;
    };
    const answers = new Map<string, () => Response>([
      [
        poster,
        () =>
          new Response(null, {
            status: 307,
            headers: [["Location", poster.replace("portrait-m", "portrait-s")]],
          }),
      ],
      [poster.replace("portrait-m", "landscape-s"), followed],
      [poster.replace("portrait-m", "logo-s"), () => image(404)],
      ["/api/v1/bytes/art/a1/portrait-m.webp", () => image()],
    ]);
    const page = pageOf("p", "webrtc", (request) =>
      Promise.resolve(answers.get(new URL(request.url).pathname)?.() ?? image(404)),
    );
    const worker = await startWorker(
      stores,
      [page],
      vi.fn<(request: Request) => Promise<Response>>(),
    );
    await tell(worker, { kind: "orpheum-path", path: "webrtc" }, { source: "p" });

    for (const asked of answers.keys()) {
      await loaded(worker, asked);
      await loaded(worker, asked);
    }
    expect(page.asked).toEqual([...answers.keys()].flatMap((asked) => [asked, asked]));
    expect(stores.get(keptImagesCache)?.size ?? 0).toBe(0);
  });

  it("keeps at most keptImagesMost images, the oldest kept going first", async () => {
    const stores = new Map<string, FakeCache>();
    const imageAt = (index: number): string =>
      `/api/v1/bytes/art/${index.toString(16).padStart(64, "0")}/portrait-s.webp`;
    // Kept by a worker before this one, which knows nothing of how many.
    stores.set(
      keptImagesCache,
      new Map(Array.from({ length: keptImagesMost }, (_, index) => [imageAt(index), image()])),
    );
    const page = pageOf("p", "webrtc", () => Promise.resolve(image()));
    const worker = await startWorker(
      stores,
      [page],
      vi.fn<(request: Request) => Promise<Response>>(),
    );
    await tell(worker, { kind: "orpheum-path", path: "webrtc" }, { source: "p" });

    const newest = imageAt(keptImagesMost);
    expect((await loaded(worker, newest))?.status).toBe(200);
    const kept = [...(stores.get(keptImagesCache)?.keys() ?? [])];
    expect(kept.length).toBeLessThan(keptImagesMost);
    expect(kept.at(-1)).toBe(newest);
    expect(kept).not.toContain(imageAt(0));
    expect(kept[0]).toBe(imageAt(keptImagesMost + 1 - kept.length));
  });
});
