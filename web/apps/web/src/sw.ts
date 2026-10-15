// SPDX-License-Identifier: MPL-2.0

/**
 * The service worker of the WebRTC path (plans/webrtc-transport.md W2b): while a page says its
 * requests go over WebRTC, the loads it makes by itself under the API (images, a video's bytes,
 * hls.js's chunks, subtitle tracks) are fetched by that page through its transport, which this
 * worker asks for over a message port. Every other load, and every page on HTTP, goes as it would.
 * Built on its own, self-contained (`vite build --mode sw`), as a classic worker every browser runs.
 */

import {
  isClaimMessage,
  isPathMessage,
  loadAfterAsking,
  mediaPathPrefix,
  responseFromPort,
} from "@orpheum/sdk";

/** A load the worker is asked to answer. */
interface FetchEventLike {
  readonly request: Request;
  readonly clientId: string;
  respondWith(response: Promise<Response>): void;
}

/** A page's message to the worker, and who sent it. */
interface MessageEventLike {
  readonly data: unknown;
  readonly source: { readonly id: string } | null;
  waitUntil(work: Promise<unknown>): void;
}

/** An install or activate, which the worker can hold open. */
interface ExtendableEventLike {
  waitUntil(work: Promise<unknown>): void;
}

/** A page the worker can post to. */
interface ClientLike {
  postMessage(message: unknown, transfer: Transferable[]): void;
}

/** The worker's global scope, as far as it uses it. */
interface WorkerScope {
  readonly clients: {
    get(id: string): Promise<ClientLike | undefined>;
    claim(): Promise<void>;
  };
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
  });
  scope.addEventListener("fetch", (event) => {
    const url = new URL(event.request.url);
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
  const { port1, port2 } = new MessageChannel();
  const headers: [string, string][] = [];
  event.request.headers.forEach((value, name) => {
    headers.push([name, value]);
  });
  const answered = responseFromPort(port1);
  page.postMessage(
    { kind: "orpheum-fetch", url: event.request.url, method: event.request.method, headers },
    [port2],
  );
  return await answered;
}
