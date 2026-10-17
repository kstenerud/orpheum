// SPDX-License-Identifier: MPL-2.0

import { answerLoad, isFetchMessage, isPathQueryMessage, type PortLike } from "@orpheum/sdk";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { sha256Of } from "./client-files.ts";

/** The client origin the worker serves, as a loader's core's would be (D308). */
const origin = "https://abc.orpheum-client.com";

/** One cache of the worker's, keyed by path. */
type FakeCache = Map<string, Response>;

/** A cache's key as the worker names it: a path. */
function keyOf(key: RequestInfo | URL): string {
  if (typeof key === "string") {
    return key;
  }
  return key instanceof URL ? key.href : key.url;
}

/** The worker's caches, kept across a worker started again, as the browser keeps them. */
function cachesOf(stores: Map<string, FakeCache>): Pick<CacheStorage, "open"> {
  return {
    open: (name: string) => {
      const store = stores.get(name) ?? new Map<string, Response>();
      stores.set(name, store);
      const cache: Pick<Cache, "match" | "put"> = {
        match: (key: RequestInfo | URL) => Promise.resolve(store.get(keyOf(key))?.clone()),
        put: (key: RequestInfo | URL, response: Response) => {
          store.set(keyOf(key), response);
          return Promise.resolve();
        },
      };
      // The worker uses only match and put.
      return Promise.resolve(cache as Cache);
    },
  };
}

/** A page of the worker's: it answers each load the worker asks of it over its path with send. */
interface FakePage {
  readonly id: string;
  readonly type: "window";
  readonly asked: string[];
  postMessage(message: unknown, transfer: Transferable[]): void;
}

function pageOf(
  id: string,
  path: "webrtc" | "http",
  send: (request: Request) => Promise<Response>,
): FakePage {
  const asked: string[] = [];
  return {
    id,
    type: "window",
    asked,
    postMessage(message, transfer) {
      const [port] = transfer;
      if (!(port instanceof MessagePort)) {
        return;
      }
      if (isPathQueryMessage(message)) {
        port.postMessage({ kind: "orpheum-path", path });
      }
      if (isFetchMessage(message)) {
        asked.push(new URL(message.url).pathname);
        const answering: PortLike = port;
        void answerLoad(message, answering, { send, token: undefined, viewer: undefined });
      }
    },
  };
}

type Listener = (event: unknown) => void;

/** The worker, started against a scope of fakes: its listeners, and what each load was answered. */
interface Worker {
  readonly listeners: Map<string, Listener>;
  readonly network: ReturnType<typeof vi.fn<(request: Request) => Promise<Response>>>;
  readonly kept: Promise<unknown>[];
}

async function startWorker(
  stores: Map<string, FakeCache>,
  pages: readonly FakePage[],
  network: Worker["network"],
): Promise<Worker> {
  const listeners = new Map<string, Listener>();
  vi.stubGlobal("clients", {
    get: (id: string) => Promise.resolve(pages.find((page) => page.id === id)),
    matchAll: () => Promise.resolve(pages),
    claim: () => Promise.resolve(),
  });
  vi.stubGlobal("caches", cachesOf(stores));
  vi.stubGlobal("location", { origin });
  vi.stubGlobal("skipWaiting", () => Promise.resolve());
  vi.stubGlobal("addEventListener", (type: string, listener: Listener) => {
    listeners.set(type, listener);
  });
  vi.stubGlobal("fetch", network);
  vi.resetModules();
  await import("./sw.ts");
  return { listeners, network, kept: [] };
}

/** A load a page made, as the worker's fetch event hands it; navigate for a page's own. */
function load(
  worker: Worker,
  path: string,
  made: { readonly by: string; readonly mode?: RequestMode | "navigate" },
): Promise<Response> | undefined {
  const { by: clientId, mode = "no-cors" } = made;
  const request = new Request(`${origin}${path}`);
  Object.defineProperty(request, "mode", { value: mode });
  let answer: Promise<Response> | undefined;
  worker.listeners.get("fetch")?.({
    request,
    clientId,
    respondWith: (response: Promise<Response>) => {
      answer = response;
    },
    waitUntil: (work: Promise<unknown>) => {
      worker.kept.push(work);
    },
  });
  return answer;
}

/** A page's word to the worker: data, from the page source, with ports. */
async function tell(
  worker: Worker,
  data: unknown,
  from: { readonly source?: string; readonly ports?: MessagePort[] } = {},
): Promise<void> {
  const { source, ports = [] } = from;
  const work: Promise<unknown>[] = [];
  worker.listeners.get("message")?.({
    data,
    ports,
    source: source === undefined ? null : { id: source },
    waitUntil: (one: Promise<unknown>) => {
      work.push(one);
    },
  });
  await Promise.all(work);
}

/** The loader naming the client's files, answered once the worker has kept their list. */
async function nameFiles(
  worker: Worker,
  files: readonly { path: string; sha256: string }[],
): Promise<string> {
  const { port1, port2 } = new MessageChannel();
  const answered = new Promise<string>((resolve) => {
    port1.addEventListener("message", (event: MessageEvent<unknown>) => {
      resolve(String(event.data));
    });
    port1.start();
  });
  await tell(
    worker,
    { kind: "orpheum-client-files", files: files.map((file) => ({ ...file, size: 1 })) },
    { ports: [port2] },
  );
  // A worker that never answers fails at once, not at the test's own timeout.
  const word = await Promise.race([
    answered,
    new Promise<string>((resolve) => {
      setTimeout(() => {
        resolve("no answer");
      }, 1000);
    }),
  ]);
  port1.close();
  return word;
}

const script = new TextEncoder().encode("console.log('the client');");

function answer(bytes: Uint8Array<ArrayBuffer> | string, status = 200): Response {
  return new Response(bytes, { status, headers: { "Content-Type": "text/javascript" } });
}

describe("the service worker under a loader (D308 §6)", () => {
  let stores: Map<string, FakeCache>;
  let sha256: string;

  beforeEach(async () => {
    stores = new Map();
    sha256 = await sha256Of(script.slice().buffer);
  });

  afterEach(() => {
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

  it("takes a booting page's file from the network, kept only when it is the signed one", async () => {
    const page = pageOf("p", "http", () => Promise.reject(new Error("no path yet")));
    const network = vi.fn(() => Promise.resolve(answer(script)));
    const worker = await startWorker(stores, [page], network);
    expect(await nameFiles(worker, [{ path: "assets/client-main.js", sha256 }])).toBe("kept");

    expect((await load(worker, "/assets/client-main.js", { by: "p" }))?.status).toBe(200);
    expect((await load(worker, "/assets/client-main.js", { by: "p" }))?.status).toBe(200);
    expect(network).toHaveBeenCalledTimes(1);
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

  it("opens the loader's page as kept, with no wait on its origin, and refreshes it behind", async () => {
    const pages = ["the loader", "the loader, updated"];
    const network = vi.fn(() => Promise.resolve(answer(pages.shift() ?? "")));
    const worker = await startWorker(stores, [], network);
    expect(await nameFiles(worker, [{ path: "assets/client-main.js", sha256 }])).toBe("kept");

    expect(await (await load(worker, "/households", { by: "p", mode: "navigate" }))?.text()).toBe(
      "the loader",
    );
    expect(await (await load(worker, "/", { by: "p", mode: "navigate" }))?.text()).toBe(
      "the loader",
    );
    await Promise.all(worker.kept);
    expect(await (await load(worker, "/", { by: "p", mode: "navigate" }))?.text()).toBe(
      "the loader, updated",
    );
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
