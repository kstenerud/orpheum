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
  type PathNoteMessage,
} from "@orpheum/sdk";

import {
  clientAssetsPrefix,
  clientFilesCache,
  clientFilesListKey,
  isClientFilesMessage,
  keptClientFiles,
  sha256Of,
} from "./client-files.ts";

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

/** The cache the loader's own page and files are kept in. */
const loaderCache = "orpheum-loader";

/**
 * How long a page is waited for to say it carries its path, once the discovery has not the client's
 * file it asked for: a page that asks for one has reached its core, and may only be busy, so it gets
 * longer than a load's ordinary question.
 */
const patientPathQueryMs = 5000;

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
    // A page that said it is on HTTP goes past the worker only where no loader ever named a client's
    // files (the core's own origin): under a loader nothing of a page's media goes to the network.
    if (
      url.origin !== scope.location.origin ||
      !url.pathname.startsWith(mediaPathPrefix) ||
      (onHttp.has(event.clientId) && clientFiles?.size === 0)
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

/**
 * A load from a page the worker has no word from: the page is asked its path, then the load goes.
 * Under a loader every page carries its own media, its client origin serving none, so the load never
 * goes to the network: it goes through a page that carries its path, waited for as patiently as a
 * client's file is, or is answered as unfetched. A page that did not answer within a second was taken
 * for HTTP, kept so, and every load after went to the client origin, which answered 404 until hls.js
 * gave up (the owner's Chrome, its worker started again under DevTools' throttling, staging,
 * 2026-10-18), or every poster of a tab reloaded across a deploy (the owner's Mac, 2026-10-29).
 */
async function askedFirst(worker: WorkerScope, event: FetchEventLike): Promise<Response> {
  if (await isUnderALoader(worker)) {
    return await carriedUnderALoader(worker, event);
  }
  if (onHttp.has(event.clientId)) {
    return await networkFetch(event.request);
  }
  const page = await worker.clients.get(event.clientId);
  return await loadAfterAsking({
    page,
    request: event.request,
    throughThePage: () => throughThePage(worker, event),
    noted: (path) => {
      notePath(event.clientId, path);
    },
    told: (note) => {
      tellPage(page, note);
    },
  });
}

/**
 * Whether a loader ever named the client's files at this origin (D308): as this worker read them, or,
 * when it read none, as kept now, since a worker that first read the list before the loader kept it,
 * or started as a deploy replaced the one before, would otherwise hold that empty read for its life.
 */
async function isUnderALoader(worker: WorkerScope): Promise<boolean> {
  return (
    (await clientFilesOf(worker)).size > 0 ||
    (await clientFilesOf(worker, { isReadAgain: true })).size > 0
  );
}

/**
 * A media load under a loader: through the page carrying its path, asked as patiently as for a
 * client's file, else answered as unfetched, never with the discovery's 404, and the page that made
 * it told why.
 */
async function carriedUnderALoader(worker: WorkerScope, event: FetchEventLike): Promise<Response> {
  const page = await carryingPageFor(worker, event.clientId, patientPathQueryMs);
  if (page !== undefined) {
    return await fetchedBy(page, event.request);
  }
  const why = `no page here carried a path to the server within ${String(patientPathQueryMs)} ms`;
  tellPage(
    await worker.clients.get(event.clientId),
    `the service worker could not load a media file: ${why}`,
  );
  const { pathname } = new URL(event.request.url);
  return new Response(`${pathname} is the server's, and ${why} to fetch it over`, { status: 502 });
}

/** Tells page, when there is one, a note on its media's path, for its log (PathNoteMessage). */
function tellPage(page: ClientLike | undefined, note: string): void {
  page?.postMessage({ kind: "orpheum-path-note", note } satisfies PathNoteMessage, []);
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
  const answered = responseFromPort(port1, request.signal);
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
  const path = key.slice(1);
  let files = await clientFilesOf(worker);
  if (!files.has(path)) {
    // What this worker read may be older than what is kept: a worker started while the page's loader
    // named its client's files to the worker before it read the list first, and never heard them. It
    // answered every file of the client it had not kept with the discovery's 404, and each screen
    // that loads as it is first shown left the page black (the owner's Chrome, staging, 2026-10-29).
    files = await clientFilesOf(worker, { isReadAgain: true });
  }
  if (files.size === 0) {
    return await networkFetch(event.request);
  }
  const sha256 = files.get(path);
  return sha256 === undefined
    ? await keptLoaderFile(worker, event, key)
    : await clientFile(worker, event, sha256);
}

/** Keeps the client's files' list, so a worker started again still knows each file's hash. */
async function keepClientFiles(worker: WorkerScope, files: Map<string, string>): Promise<void> {
  const cache = await worker.caches.open(clientFilesCache);
  await cache.put(clientFilesListKey, new Response(JSON.stringify([...files])));
}

/**
 * The client's files' hashes by path, as last kept: read once, or read again when isReadAgain, since
 * a loader may have named them to another worker of this origin, which kept them, since.
 */
async function clientFilesOf(
  worker: WorkerScope,
  read: { readonly isReadAgain: boolean } = { isReadAgain: false },
): Promise<Map<string, string>> {
  if (clientFiles !== undefined && !read.isReadAgain) {
    return clientFiles;
  }
  clientFiles = await keptClientFiles(worker.caches);
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
  waitMs?: number,
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
  // nothing in time, which is not kept: it carries its media once it has found the core.
  const path = onWebrtc.has(page.id) ? "webrtc" : await askPathOf(page, waitMs);
  if (path === "webrtc") {
    notePath(page.id, path);
  }
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
  const { page, fetched } = await fetchedForClient(worker, event);
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
 * A file of the client's, as fetched: by the page that asked for it over its path, or, from a page
 * with no path yet, the loader starting, from the discovery, whose own file of the same name is
 * answered, kept only when it is the very file the manifest names, as a name made of its content
 * says it is. A file the discovery has not is the core's client's, which only the core serves: the
 * page asking for it has reached its core and carries its path, though it did not say so in time,
 * and is asked again, more patiently. Answered with the discovery's 404, the client never ran and the
 * page stayed black, saying nothing (the owner's Mac, staging, 2026-10-29); with no such page, the
 * answer says so. Its page is undefined where the discovery answered.
 */
async function fetchedForClient(
  worker: WorkerScope,
  event: FetchEventLike,
): Promise<{ readonly page: ClientLike | undefined; readonly fetched: Response }> {
  const page = await carryingPageFor(worker, event.clientId);
  if (page !== undefined) {
    return { page, fetched: await fetchedBy(page, event.request) };
  }
  const fromOrigin = await networkFetch(event.request).catch(() => undefined);
  if (fromOrigin?.status === 200) {
    return { page, fetched: fromOrigin };
  }
  const carrying = await carryingPageFor(worker, event.clientId, patientPathQueryMs);
  if (carrying === undefined) {
    const { pathname } = new URL(event.request.url);
    const unfetched = new Response(
      `${pathname} is a file of the server's client, and no page here carries a path to the server to fetch it over`,
      { status: 502 },
    );
    return { page: carrying, fetched: unfetched };
  }
  return { page: carrying, fetched: await fetchedBy(carrying, event.request) };
}

/**
 * The loader's page as the origin answers it now, or as kept when the origin fails: another core's
 * cookies, set for the whole site, can make every load to it fail, and the kept page still opens
 * (D308 §7). Kept first, the visit after an update ran the page before it, which for a household
 * whose server had gone said "This link no longer finds its server" until a reload (nas.example,
 * 2026-10-19); the owner took the wait of one load on opening instead. Its files, named by their
 * content, come as kept, the origin asked for them again behind.
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
  if (key === "/") {
    const answered = await fresh;
    if (answered?.ok === true) {
      return answered;
    }
    return (await cache.match(key)) ?? answered ?? Response.error();
  }
  const kept = await cache.match(key);
  if (kept !== undefined) {
    event.waitUntil(fresh);
    return kept;
  }
  return (await fresh) ?? Response.error();
}
