// SPDX-License-Identifier: MPL-2.0

/**
 * The service worker of the WebRTC path (plans/webrtc-transport.md W2b): while a page says its
 * requests go over WebRTC, the loads it makes by itself under the API (images, a video's bytes,
 * hls.js's chunks, subtitle tracks) are fetched by that page through its transport, which this
 * worker asks for over a message port. Every other load, and every page on HTTP, goes as it would.
 * Built on its own, self-contained (`vite build --mode sw`), as a classic worker every browser runs.
 *
 * Once a discovery's loader has named the core's client's files to it (D308), it also answers each
 * of them, fetched by the page over its path and answered only when its SHA-256 is the one the
 * core's signed manifest names, kept by that hash; and it keeps the loader's own page and files, so
 * a page opens with no load to its origin at all, which another core's cookies cannot stop (§7). At a
 * core's own origin no loader ever names them, and those loads go as they would.
 */

import {
  askPathOf,
  isClaimMessage,
  isPathMessage,
  loadAfterAsking,
  mediaPathPrefix,
  networkFetch,
  responseFromPort,
} from "@orpheum/sdk";

import { clientAssetsPrefix, isClientFilesMessage, sha256Of } from "./client-files.ts";

/** A load the worker is asked to answer. */
interface FetchEventLike {
  readonly request: Request;
  readonly clientId: string;
  respondWith(response: Promise<Response>): void;
  waitUntil(work: Promise<unknown>): void;
}

/** A page's message to the worker, and who sent it. */
interface MessageEventLike {
  readonly data: unknown;
  readonly ports: readonly MessagePort[];
  readonly source: { readonly id: string } | null;
  waitUntil(work: Promise<unknown>): void;
}

/** An install or activate, which the worker can hold open. */
interface ExtendableEventLike {
  waitUntil(work: Promise<unknown>): void;
}

/** A page the worker can post to, or a worker of one. */
interface ClientLike {
  readonly id?: string;
  readonly type?: string;
  postMessage(message: unknown, transfer: Transferable[]): void;
}

/** The worker's global scope, as far as it uses it. */
interface WorkerScope {
  readonly clients: {
    get(id: string): Promise<ClientLike | undefined>;
    matchAll(options: { readonly type: "window" }): Promise<readonly ClientLike[]>;
    claim(): Promise<void>;
  };
  readonly caches: CacheStorage;
  readonly location: { readonly origin: string };
  skipWaiting(): Promise<void>;
  addEventListener(type: "fetch", listener: (event: FetchEventLike) => void): void;
  addEventListener(type: "message", listener: (event: MessageEventLike) => void): void;
  addEventListener(
    type: "install" | "activate",
    listener: (event: ExtendableEventLike) => void,
  ): void;
}

function isWorkerScope(scope: unknown): scope is WorkerScope {
  return (
    typeof scope === "object" && scope !== null && "clients" in scope && "skipWaiting" in scope
  );
}

/** The pages on the WebRTC path, by their client id. */
// Each page's word on its path, kept while the worker runs: the browser stops an idle worker, and one
// started again knows none, so a page it has no word from is asked (askPathOf). A guest's next film
// went to the page's own origin and was not found (staging, 2026-10-14).
const onWebrtc = new Set<string>();
const onHttp = new Set<string>();

/** The cache the core's client's files are kept in by their SHA-256, and their list by path. */
const clientFilesCache = "orpheum-client-files";
const clientFilesListKey = "/__orpheum/client-files";

/** The cache the loader's own page and files are kept in. */
const loaderCache = "orpheum-loader";

/** The client's files' hashes by path, as the loader last said; read back after a restart. */
let clientFiles: Map<string, string> | undefined;

const scope: unknown = globalThis;
if (isWorkerScope(scope)) {
  scope.addEventListener("install", (event) => {
    event.waitUntil(scope.skipWaiting());
  });
  scope.addEventListener("activate", (event) => {
    event.waitUntil(scope.clients.claim());
  });
  scope.addEventListener("message", (event) => {
    if (event.source !== null && isPathMessage(event.data)) {
      notePath(event.source.id, event.data.path);
    }
    if (isClaimMessage(event.data)) {
      event.waitUntil(scope.clients.claim());
    }
    if (isClientFilesMessage(event.data)) {
      clientFiles = new Map(event.data.files.map((file) => [file.path, file.sha256]));
      const [answer] = event.ports;
      // Told once kept, so the loader adds no script before the worker knows its file.
      event.waitUntil(
        keepClientFiles(scope, clientFiles).then(() => {
          answer?.postMessage("kept");
        }),
      );
    }
  });
  scope.addEventListener("fetch", (event) => {
    const url = new URL(event.request.url);
    if (url.origin === scope.location.origin && answeredUnderALoader(scope, event)) {
      return;
    }
    if (
      url.origin !== scope.location.origin ||
      !url.pathname.startsWith(mediaPathPrefix) ||
      onHttp.has(event.clientId)
    ) {
      return;
    }
    event.respondWith(
      onWebrtc.has(event.clientId) ? throughThePage(scope, event) : askedFirst(scope, event),
    );
  });
}

/** Keeps a page's word on its path. */
function notePath(clientId: string, path: "webrtc" | "http"): void {
  if (path === "webrtc") {
    onWebrtc.add(clientId);
    onHttp.delete(clientId);
  } else {
    onHttp.add(clientId);
    onWebrtc.delete(clientId);
  }
}

/** A load from a page the worker has no word from: the page is asked its path, then the load goes. */
async function askedFirst(worker: WorkerScope, event: FetchEventLike): Promise<Response> {
  return await loadAfterAsking({
    page: await worker.clients.get(event.clientId),
    request: event.request,
    throughThePage: () => throughThePage(worker, event),
    noted: (path) => {
      notePath(event.clientId, path);
    },
  });
}

/** Asks the page that made a load to fetch it through its transport, and answers with what it sends. */
async function throughThePage(worker: WorkerScope, event: FetchEventLike): Promise<Response> {
  const page = await worker.clients.get(event.clientId);
  if (page === undefined) {
    throw new Error("the page that made the load is gone");
  }
  return await fetchedBy(page, event.request);
}

/** Asks page to fetch request through its transport, and answers with what it sends. */
async function fetchedBy(page: ClientLike, request: Request): Promise<Response> {
  const { port1, port2 } = new MessageChannel();
  const headers: [string, string][] = [];
  request.headers.forEach((value, name) => {
    headers.push([name, value]);
  });
  const answered = responseFromPort(port1);
  page.postMessage({ kind: "orpheum-fetch", url: request.url, method: request.method, headers }, [
    port2,
  ]);
  return await answered;
}

/**
 * Takes a page's own load, or a file under its assets, which under a loader the worker answers
 * itself (D308): a file of the core's client by its hash, or the loader's own page or files as kept.
 * At the core's own origin, where no loader ever named a client's files, each goes as it would.
 * Whether it took the load.
 */
function answeredUnderALoader(worker: WorkerScope, event: FetchEventLike): boolean {
  const { pathname } = new URL(event.request.url);
  const isPage = event.request.mode === "navigate";
  if (!isPage && !pathname.startsWith(clientAssetsPrefix)) {
    return false;
  }
  event.respondWith(underALoader(worker, event, isPage ? "/" : pathname));
  return true;
}

/** The answer to a load answeredUnderALoader took, kept or fetched under key. */
async function underALoader(
  worker: WorkerScope,
  event: FetchEventLike,
  key: string,
): Promise<Response> {
  const files = await clientFilesOf(worker);
  if (files.size === 0) {
    return await networkFetch(event.request);
  }
  const sha256 = files.get(key.slice(1));
  return sha256 === undefined
    ? await keptLoaderFile(worker, event, key)
    : await clientFile(worker, event, sha256);
}

/** Keeps the client's files' list, so a worker started again still knows each file's hash. */
async function keepClientFiles(worker: WorkerScope, files: Map<string, string>): Promise<void> {
  const cache = await worker.caches.open(clientFilesCache);
  await cache.put(clientFilesListKey, new Response(JSON.stringify([...files])));
}

/** The client's files' hashes by path, as last kept. */
async function clientFilesOf(worker: WorkerScope): Promise<Map<string, string>> {
  if (clientFiles !== undefined) {
    return clientFiles;
  }
  const kept = await (await worker.caches.open(clientFilesCache)).match(clientFilesListKey);
  const pairs: unknown = kept === undefined ? [] : await kept.json();
  clientFiles = new Map(
    Array.isArray(pairs)
      ? pairs.filter(
          (pair): pair is [string, string] =>
            Array.isArray(pair) && typeof pair[0] === "string" && typeof pair[1] === "string",
        )
      : [],
  );
  return clientFiles;
}

/**
 * A page that carries a path to the core, which the worker can ask to fetch for the load clientId
 * made: it, or, for a worker of a page, a window that does. None while the page is still finding the
 * core, as a loader booting on a page the worker already knows the client's files for is.
 */
async function carryingPageFor(
  worker: WorkerScope,
  clientId: string,
): Promise<ClientLike | undefined> {
  if (onWebrtc.has(clientId)) {
    return await worker.clients.get(clientId);
  }
  const made = await worker.clients.get(clientId);
  const page =
    made?.type === "window" ? made : (await worker.clients.matchAll({ type: "window" }))[0];
  if (page?.id === undefined || onHttp.has(page.id)) {
    return undefined;
  }
  // A worker started again knows no page's path: the page is asked, and one finding the core says
  // nothing in time.
  const path = onWebrtc.has(page.id) ? "webrtc" : await askPathOf(page);
  notePath(page.id, path);
  return path === "webrtc" ? page : undefined;
}

/**
 * One of the core's client's files, as its signed manifest names it: kept by its SHA-256, else
 * fetched by the page over its path, and answered only when its SHA-256 is that one (D308 §6).
 */
async function clientFile(
  worker: WorkerScope,
  event: FetchEventLike,
  sha256: string,
): Promise<Response> {
  const cache = await worker.caches.open(clientFilesCache);
  const key = `/__orpheum/sha256/${sha256}`;
  const kept = await cache.match(key);
  if (kept !== undefined) {
    return kept;
  }
  const page = await carryingPageFor(worker, event.clientId);
  // A page with no path yet is the loader starting: its own file of the same name comes from the
  // discovery, kept only when it is the very file the manifest names, as a name made of its content
  // says it is.
  const fetched =
    page === undefined ? await networkFetch(event.request) : await fetchedBy(page, event.request);
  const bytes = await fetched.arrayBuffer();
  const isSigned = fetched.status === 200 && (await sha256Of(bytes)) === sha256;
  if (!isSigned && page !== undefined) {
    return new Response("this file is not the one the server signed", { status: 502 });
  }
  const answer = new Response(bytes, {
    status: fetched.status,
    headers: { "Content-Type": fetched.headers.get("Content-Type") ?? "application/octet-stream" },
  });
  if (isSigned) {
    await cache.put(key, answer.clone());
  }
  return answer;
}

/**
 * The loader's page or one of its files, as kept, the origin asked for it again behind it; the
 * origin's own answer while none is kept. A page so kept opens with no load to the origin, which
 * another core's cookies, set for the whole site, cannot stop (D308 §7).
 */
async function keptLoaderFile(
  worker: WorkerScope,
  event: FetchEventLike,
  key: string,
): Promise<Response> {
  const cache = await worker.caches.open(loaderCache);
  const fresh = networkFetch(event.request).then(
    async (answer) => {
      if (answer.ok) {
        await cache.put(key, answer.clone());
      }
      return answer;
    },
    () => undefined,
  );
  const kept = await cache.match(key);
  if (kept !== undefined) {
    event.waitUntil(fresh);
    return kept;
  }
  return (await fresh) ?? Response.error();
}
