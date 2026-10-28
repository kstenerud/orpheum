// SPDX-License-Identifier: MPL-2.0

/**
 * A service worker of the relay as released up to 106f0198 (test-pull-relay.ts), for the browser-mode
 * tests of a page newer than its worker: it takes over at once as sw.ts does, and asks the page that
 * made each load under the API to fetch it, pulling the body a piece at a time. Registered by those
 * tests alone, as a module; never built into the app.
 */

import { mediaPathPrefix, networkFetch } from "@orpheum/sdk";

import { pullWorkerResponse } from "./test-pull-relay.ts";

/** A load the worker is asked to answer. */
interface PullFetchEvent {
  readonly request: Request;
  readonly clientId: string;
  respondWith(response: Promise<Response>): void;
}

/** An install or activate, which the worker can hold open. */
interface PullExtendableEvent {
  waitUntil(work: Promise<unknown>): void;
}

/** The worker's scope, as far as this one uses it. */
interface PullWorkerScope {
  readonly clients: {
    get(
      id: string,
    ): Promise<{ postMessage(message: unknown, transfer: Transferable[]): void } | undefined>;
    claim(): Promise<void>;
  };
  skipWaiting(): Promise<void>;
  addEventListener(type: "fetch", listener: (event: PullFetchEvent) => void): void;
  addEventListener(
    type: "install" | "activate",
    listener: (event: PullExtendableEvent) => void,
  ): void;
}

function isPullWorkerScope(scope: unknown): scope is PullWorkerScope {
  return (
    typeof scope === "object" && scope !== null && "clients" in scope && "skipWaiting" in scope
  );
}

/** Asks the page that made the load to fetch it, and answers with the body it pulls. */
async function pulledThroughThePage(
  worker: PullWorkerScope,
  event: PullFetchEvent,
): Promise<Response> {
  const page = await worker.clients.get(event.clientId);
  if (page === undefined) {
    throw new Error("the page that made the load is gone");
  }
  const { port1, port2 } = new MessageChannel();
  const answered = pullWorkerResponse(port1);
  page.postMessage(
    { kind: "orpheum-fetch", url: event.request.url, method: event.request.method, headers: [] },
    [port2],
  );
  return await answered;
}

const scope: unknown = globalThis;
if (isPullWorkerScope(scope)) {
  scope.addEventListener("install", (event) => {
    event.waitUntil(scope.skipWaiting());
  });
  scope.addEventListener("activate", (event) => {
    event.waitUntil(scope.clients.claim());
  });
  scope.addEventListener("fetch", (event) => {
    // The page's own load, answered from the network as sw.ts answers it (answeredUnderALoader): a
    // page Firefox opened past its worker was its worker's in name, and none of its loads reached it.
    if (event.request.mode === "navigate") {
      event.respondWith(networkFetch(event.request));
      return;
    }
    if (new URL(event.request.url).pathname.startsWith(mediaPathPrefix)) {
      event.respondWith(pulledThroughThePage(scope, event));
    }
  });
}
