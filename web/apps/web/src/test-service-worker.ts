// SPDX-License-Identifier: MPL-2.0

/**
 * The service worker's test bench (sw.test.ts): sw.ts started against a scope of fakes, its caches,
 * its pages and the network, and the loads and words a page hands it.
 */

import {
  answerLoad,
  isFetchMessage,
  isPathNoteMessage,
  isPathQueryMessage,
  type PortLike,
} from "@orpheum/sdk";
import type { vi as Vi } from "vitest";

/** The client origin the worker serves, as a loader's core's would be (D308). */
export const origin = "https://abc.client.example";

/** One cache of the worker's, keyed by path. */
export type FakeCache = Map<string, Response>;

/** A cache's key as the worker names it: a path, as a request at the worker's own origin names it too. */
export function keyOf(key: RequestInfo | URL): string {
  if (typeof key === "string") {
    return key;
  }
  const url = key instanceof URL ? key : new URL(key.url);
  return url.origin === origin ? `${url.pathname}${url.search}` : url.href;
}

/**
 * The worker's caches, kept across a worker started again, as the browser keeps them: each lists its
 * entries in the order they were kept, one kept again last.
 */
export function cachesOf(stores: Map<string, FakeCache>): Pick<CacheStorage, "open"> {
  return {
    open: (name: string) => {
      const store = stores.get(name) ?? new Map<string, Response>();
      stores.set(name, store);
      // The worker adds nothing by fetching it, nor reads more than one at a time.
      const unused = (): Promise<never> => Promise.reject(new Error("the worker uses none of it"));
      const cache: Cache = {
        add: unused,
        addAll: unused,
        matchAll: unused,
        match: (key: RequestInfo | URL) => Promise.resolve(store.get(keyOf(key))?.clone()),
        put: (key: RequestInfo | URL, response: Response) => {
          store.delete(keyOf(key));
          store.set(keyOf(key), response);
          return Promise.resolve();
        },
        keys: () =>
          Promise.resolve([...store.keys()].map((key) => new Request(new URL(key, origin)))),
        delete: (key: RequestInfo | URL) => Promise.resolve(store.delete(keyOf(key))),
      };
      return Promise.resolve(cache);
    },
  };
}

/** A page of the worker's: it answers each load the worker asks of it over its path with send. */
export interface FakePage {
  readonly id: string;
  readonly type: "window";
  readonly asked: string[];
  /** What the worker told the page of its media's path (PathNoteMessage). */
  readonly notes: string[];
  postMessage(message: unknown, transfer: Transferable[]): void;
}

export function pageOf(
  id: string,
  path: "webrtc" | "http" | "unanswered",
  send: (request: Request) => Promise<Response>,
): FakePage {
  const asked: string[] = [];
  const notes: string[] = [];
  return {
    id,
    type: "window",
    asked,
    notes,
    postMessage(message, transfer) {
      if (isPathNoteMessage(message)) {
        notes.push(message.note);
      }
      const [port] = transfer;
      if (!(port instanceof MessagePort)) {
        return;
      }
      if (isPathQueryMessage(message) && path !== "unanswered") {
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

/**
 * The test file's own vitest, which the test calls useVitest with once: production code may not
 * import vitest, and this file is not a test file (web/.dependency-cruiser.cjs, as test-choosing.ts).
 */
let vitest: typeof Vi | undefined;

/** Hands this file the test file's vi, before startWorker or settledOver. */
export function useVitest(vi: typeof Vi): void {
  vitest = vi;
}

function vi(): typeof Vi {
  if (vitest === undefined) {
    throw new Error("useVitest(vi) was not called by the test file");
  }
  return vitest;
}

export type Listener = (event: unknown) => void;

/** The worker, started against a scope of fakes: its listeners, and what each load was answered. */
export interface Worker {
  readonly listeners: Map<string, Listener>;
  readonly network: ReturnType<typeof Vi.fn<(request: Request) => Promise<Response>>>;
  readonly kept: Promise<unknown>[];
}

export async function startWorker(
  stores: Map<string, FakeCache>,
  pages: readonly FakePage[],
  network: Worker["network"],
): Promise<Worker> {
  const listeners = new Map<string, Listener>();
  vi().stubGlobal("clients", {
    get: (id: string) => Promise.resolve(pages.find((page) => page.id === id)),
    matchAll: () => Promise.resolve(pages),
    claim: () => Promise.resolve(),
  });
  vi().stubGlobal("caches", cachesOf(stores));
  vi().stubGlobal("location", { origin });
  vi().stubGlobal("skipWaiting", () => Promise.resolve());
  vi().stubGlobal("addEventListener", (type: string, listener: Listener) => {
    listeners.set(type, listener);
  });
  vi().stubGlobal("fetch", network);
  vi().resetModules();
  await import("./sw.ts");
  return { listeners, network, kept: [] };
}

/** A load a page made, as the worker's fetch event hands it; navigate for a page's own. */
export function load(
  worker: Worker,
  path: string,
  made: {
    readonly by: string;
    readonly mode?: RequestMode | "navigate";
    readonly signal?: AbortSignal;
  },
): Promise<Response> | undefined {
  const { by: clientId, mode = "no-cors", signal } = made;
  const request = new Request(`${origin}${path}`, { signal: signal ?? null });
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
export async function tell(
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
export async function nameFiles(
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

export const script: Uint8Array<ArrayBuffer> = new TextEncoder().encode(
  "console.log('the client');",
);

export function answer(bytes: Uint8Array<ArrayBuffer> | string, status = 200): Response {
  return new Response(bytes, { status, headers: { "Content-Type": "text/javascript" } });
}

/** How long the worker waits for a page's answer on its path: a load's question, and a patient one. */
export const askWaitMs = 1000;
export const patientWaitMs = 5000;

/** page, answering the worker's question on its path with "webrtc" only after afterMs. */
export function answeringLate(page: FakePage, afterMs: number): FakePage {
  return {
    ...page,
    postMessage(message, transfer) {
      const [port] = transfer;
      if (isPathQueryMessage(message) && port instanceof MessagePort) {
        setTimeout(() => {
          port.postMessage({ kind: "orpheum-path", path: "webrtc" });
        }, afterMs);
        return;
      }
      page.postMessage(message, transfer);
    },
  };
}

/**
 * What work settles to as the fake clock runs on for at most ms, undefined if it has not by then; a
 * port's message comes on the real event loop, so each step of the clock yields to it first.
 */
export async function settledOver<T>(
  work: Promise<T> | undefined,
  ms: number,
): Promise<T | undefined> {
  let settled: { readonly value: T } | undefined;
  let failure: { readonly cause: unknown } | undefined;
  void work?.then(
    (value) => {
      settled = { value };
    },
    (cause: unknown) => {
      failure = { cause };
    },
  );
  const step = 100;
  for (let ran = 0; settled === undefined && failure === undefined && ran <= ms; ran += step) {
    await new Promise<void>((resolve) => {
      setImmediate(resolve);
    });
    await vi().advanceTimersByTimeAsync(step);
  }
  if (failure !== undefined) {
    throw failure.cause;
  }
  return settled?.value;
}
