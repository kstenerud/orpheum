// SPDX-License-Identifier: MPL-2.0

import type { operations } from "./gen/api.ts";

/**
 * Media over the WebRTC path (plans/webrtc-transport.md W2b). What the browser loads by itself, an
 * image, a video's bytes, hls.js's chunks, a subtitle track, cannot use a data channel. A service
 * worker takes those loads while the page says it is on WebRTC and asks the page, which holds the
 * connections, to fetch each through its transport; the page sends the response back over a message
 * port, umi no tobira each time the service worker's stream asks, so a film never sits whole in memory.
 */

/** The page's word to its service worker on which path its requests take. */
export interface PathMessage {
  readonly kind: "orpheum-path";
  readonly path: "webrtc" | "http";
}

/**
 * The service worker's question to a page it has no word from, since a worker the browser stopped
 * while idle starts again knowing no page's path; the page answers on the port with its PathMessage.
 */
export interface PathQueryMessage {
  readonly kind: "orpheum-path-query";
}

/**
 * A page's request to the active worker to take it under its control: a page the worker did not
 * claim as it started, as Firefox loaded the guest's second page, is never controlled otherwise, and
 * its loads went past the worker to the page's own origin (the browser tier, 2026-10-14).
 */
export interface ClaimMessage {
  readonly kind: "orpheum-claim";
}

/** Whether a message is a page's request to be claimed. */
export function isClaimMessage(message: unknown): message is ClaimMessage {
  return (
    typeof message === "object" &&
    message !== null &&
    "kind" in message &&
    message.kind === "orpheum-claim"
  );
}

/** How long a worker waits for a page's answer to its PathQueryMessage before taking it for HTTP. */
export const pathQueryWaitMs = 1000;

/** The service worker's request to its page for one load. */
export interface FetchMessage {
  readonly kind: "orpheum-fetch";
  readonly url: string;
  readonly method: string;
  readonly headers: readonly (readonly [string, string])[];
}

/** What the page sends back on a load's port. */
export type ReplyMessage =
  | {
      readonly kind: "head";
      readonly status: number;
      readonly headers: readonly (readonly [string, string])[];
    }
  | { readonly kind: "chunk"; readonly data: ArrayBuffer }
  | { readonly kind: "end" }
  | { readonly kind: "error"; readonly message: string };

/** What the service worker sends on a load's port: give the next piece, or stop. */
export type PullMessage = "pull" | "cancel";

/** The paths whose loads go over the page's transport: the API and its bytes. */
export const mediaPathPrefix = "/api/v1/";

/** A port as the bridge uses it; a MessagePort, or a test's. */
/**
 * What a page's media loads are signed with: its own session's token, and a Watch Together viewer's
 * membership, which the core's viewer cookie carries on its own origin and no cookie reaches from
 * another (discovery-core.md K10).
 */
export interface MediaCredentials {
  readonly token: string | undefined;
  readonly viewer: string | undefined;
}

/** The headers a film's chunk is asked for with, as the API declares them. */
type ChunkHeaders = NonNullable<operations["getChunkFile"]["parameters"]["header"]>;

/**
 * The header a viewer's membership goes in (internal/core/api.ViewerHeaderName), checked against the
 * API's declaration, so a rename on the server fails the type check rather than the guest's film.
 */
export const viewerHeaderName = "Orpheum-Viewer" satisfies keyof ChunkHeaders;

export interface PortLike {
  postMessage(message: unknown, transfer?: Transferable[]): void;
  addEventListener(type: "message", listener: (event: MessageEvent<unknown>) => void): void;
  start(): void;
}

/** Whether a message is a page's word on which path its requests take. */
export function isPathMessage(message: unknown): message is PathMessage {
  return (
    typeof message === "object" &&
    message !== null &&
    "kind" in message &&
    message.kind === "orpheum-path" &&
    "path" in message &&
    (message.path === "webrtc" || message.path === "http")
  );
}

/** Whether a message is the service worker's question on the page's path. */
export function isPathQueryMessage(message: unknown): message is PathQueryMessage {
  return (
    typeof message === "object" &&
    message !== null &&
    "kind" in message &&
    message.kind === "orpheum-path-query"
  );
}

/**
 * The path a page says its loads take, asked on a port of a new channel: "webrtc" when the page
 * carries its own media, "http" when it says so, or gives no answer within waitMs, as a page that
 * never carried media gives none.
 */
export async function askPathOf(
  page: { postMessage(message: unknown, transfer: Transferable[]): void },
  waitMs: number = pathQueryWaitMs,
): Promise<PathMessage["path"]> {
  const { port1, port2 } = new MessageChannel();
  try {
    return await new Promise((resolve) => {
      const timer = setTimeout(() => {
        resolve("http");
      }, waitMs);
      port1.addEventListener("message", (event: MessageEvent<unknown>) => {
        clearTimeout(timer);
        resolve(isPathMessage(event.data) ? event.data.path : "http");
      });
      port1.start();
      page.postMessage({ kind: "orpheum-path-query" } satisfies PathQueryMessage, [port2]);
    });
  } finally {
    port1.close();
  }
}

/**
 * A load from a page a worker has no word from: the page, when there is one, is asked its path
 * (askPathOf), which noted keeps, and the load goes through it when it carries its media, else to
 * the network as the browser would send it.
 */
export async function loadAfterAsking(load: {
  readonly page: { postMessage(message: unknown, transfer: Transferable[]): void } | undefined;
  readonly request: Request;
  readonly throughThePage: () => Promise<Response>;
  readonly noted: (path: PathMessage["path"]) => void;
}): Promise<Response> {
  const path = load.page === undefined ? "http" : await askPathOf(load.page);
  load.noted(path);
  return path === "webrtc" ? await load.throughThePage() : await fetch(load.request);
}

/** Whether a message is the service worker's request for a load. */
export function isFetchMessage(message: unknown): message is FetchMessage {
  return (
    typeof message === "object" &&
    message !== null &&
    "kind" in message &&
    message.kind === "orpheum-fetch" &&
    "url" in message &&
    typeof message.url === "string"
  );
}

/**
 * The page's side: fetches one load the service worker asked for through send, signed in with
 * token unless the load names its own, and answers on port, the head at once and each piece of the
 * body when the service worker pulls.
 */
export async function answerLoad(
  message: FetchMessage,
  port: PortLike,
  load: { readonly send: (request: Request) => Promise<Response> } & MediaCredentials,
): Promise<void> {
  const headers = new Headers(message.headers.map(([name, value]) => [name, value]));
  if (load.token !== undefined && !headers.has("Authorization")) {
    headers.set("Authorization", `Bearer ${load.token}`);
  }
  if (load.viewer !== undefined && !headers.has(viewerHeaderName)) {
    headers.set(viewerHeaderName, load.viewer);
  }
  let response: Response;
  try {
    response = await load.send(new Request(message.url, { method: message.method, headers }));
  } catch (error: unknown) {
    port.postMessage({ kind: "error", message: String(error) } satisfies ReplyMessage);
    return;
  }
  const replied: [string, string][] = [];
  response.headers.forEach((value, name) => {
    replied.push([name, value]);
  });
  port.postMessage({
    kind: "head",
    status: response.status,
    headers: replied,
  } satisfies ReplyMessage);
  const reader = response.body?.getReader();
  port.addEventListener("message", (event) => {
    if (event.data === "cancel") {
      void reader?.cancel();
      return;
    }
    if (event.data !== "pull") {
      return;
    }
    if (reader === undefined) {
      port.postMessage({ kind: "end" } satisfies ReplyMessage);
      return;
    }
    reader.read().then(
      ({ done: isDone, value }) => {
        if (isDone) {
          port.postMessage({ kind: "end" } satisfies ReplyMessage);
          return;
        }
        const data = value.slice().buffer;
        port.postMessage({ kind: "chunk", data } satisfies ReplyMessage, [data]);
      },
      (error: unknown) => {
        port.postMessage({ kind: "error", message: String(error) } satisfies ReplyMessage);
      },
    );
  });
  port.start();
}

/**
 * The service worker's side: the response a load's port carries, resolved at its head, its body a
 * stream that pulls umi no tobira at a time from the page.
 */
export function responseFromPort(port: PortLike): Promise<Response> {
  return new Promise<Response>((resolve, reject) => {
    let deliver: ((reply: ReplyMessage) => void) | undefined;
    port.addEventListener("message", (event) => {
      const reply = asReply(event.data);
      if (reply === undefined) {
        return;
      }
      if (reply.kind === "head") {
        const body = hasBody(reply.status)
          ? bodyFrom(port, (next) => {
              deliver = next;
            })
          : null;
        resolve(
          new Response(body, {
            status: reply.status,
            headers: reply.headers.map(([name, value]) => [name, value]),
          }),
        );
        return;
      }
      if (reply.kind === "error" && deliver === undefined) {
        reject(new Error(reply.message));
        return;
      }
      deliver?.(reply);
    });
    port.start();
  });
}

/** The statuses whose response has no body. */
const bodylessStatuses = new Set([101, 204, 205, 304]);

/** A body pulled umi no tobira at a time over port; setDeliver hands it the replies after the head. */
function bodyFrom(
  port: PortLike,
  setDeliver: (deliver: (reply: ReplyMessage) => void) => void,
): ReadableStream<Uint8Array> {
  let waiting: ((reply: ReplyMessage) => void) | undefined;
  setDeliver((reply) => {
    const next = waiting;
    waiting = undefined;
    next?.(reply);
  });
  return new ReadableStream<Uint8Array>(
    {
      pull: async (controller) => {
        const reply = await new Promise<ReplyMessage>((resolve) => {
          waiting = resolve;
          port.postMessage("pull" satisfies PullMessage);
        });
        if (reply.kind === "chunk") {
          controller.enqueue(new Uint8Array(reply.data));
        } else if (reply.kind === "error") {
          controller.error(new Error(reply.message));
        } else {
          controller.close();
        }
      },
      cancel: () => {
        port.postMessage("cancel" satisfies PullMessage);
      },
    },
    { highWaterMark: 0 },
  );
}

/** A reply as the page sent it, or undefined for anything else. */
function asReply(data: unknown): ReplyMessage | undefined {
  if (typeof data !== "object" || data === null || !("kind" in data)) {
    return undefined;
  }
  switch (data.kind) {
    case "head":
      return asHead(data);
    case "chunk":
      return "data" in data && data.data instanceof ArrayBuffer
        ? { kind: "chunk", data: data.data }
        : undefined;
    case "end":
      return { kind: "end" };
    case "error":
      return asError(data);
    default:
      return undefined;
  }
}

/** An error reply, with the page's words when it gave some. */
function asError(data: object): ReplyMessage {
  return {
    kind: "error",
    message: "message" in data ? String(data.message) : "the page could not load it",
  };
}

/** A head reply, or undefined for one that is not. */
function asHead(data: object): ReplyMessage | undefined {
  if (!("status" in data) || typeof data.status !== "number") {
    return undefined;
  }
  const headers = "headers" in data && Array.isArray(data.headers) ? data.headers : [];
  return { kind: "head", status: data.status, headers: headers.filter(isHeaderPair) };
}

function isHeaderPair(value: unknown): value is [string, string] {
  return (
    Array.isArray(value) &&
    value.length === 2 &&
    typeof value[0] === "string" &&
    typeof value[1] === "string"
  );
}

/** Whether a status's response carries a body, which a Response refuses one for when it does not. */
export function hasBody(status: number): boolean {
  return !bodylessStatuses.has(status);
}

/** The service worker's own load of request, from the network: what it answers through no page. */
export function networkFetch(request: Request): Promise<Response> {
  return fetch(request);
}

/** Where the service worker is served, beside the app, so its scope is the whole app. */
export const serviceWorkerUrl = "/sw.js";

/**
 * Moves the loads the browser makes by itself onto send, the page's transport (WebRTC, or the core's
 * own name on another origin): registers the service worker, answers each load it asks for, signed
 * in with the session's token at the time, and tells it this page carries its media; resolves once
 * it controls the page, and says in the console that media now goes over name.
 */
export async function carryMediaOver(
  send: (request: Request) => Promise<Response>,
  credentials: () => MediaCredentials,
  name: string,
): Promise<void> {
  if (!("serviceWorker" in navigator)) {
    throw new Error("this browser runs no service worker, so media stays on HTTP");
  }
  const workers = navigator.serviceWorker;
  workers.addEventListener("message", (event: MessageEvent<unknown>) => {
    const [port] = event.ports;
    if (port !== undefined && isFetchMessage(event.data)) {
      void answerLoad(event.data, port, { send, ...credentials() });
    }
    if (port !== undefined && isPathQueryMessage(event.data)) {
      // The worker started again since this page said so, and asks.
      port.postMessage({ kind: "orpheum-path", path: "webrtc" } satisfies PathMessage);
    }
  });
  workers.startMessages();
  const announce = (): void => {
    workers.controller?.postMessage({ kind: "orpheum-path", path: "webrtc" } satisfies PathMessage);
  };
  const controlled = new Promise<void>((resolve) => {
    if (workers.controller !== null) {
      resolve();
      return;
    }
    workers.addEventListener("controllerchange", () => {
      resolve();
    });
  });
  workers.addEventListener("controllerchange", announce);
  await workers.register(serviceWorkerUrl);
  const ready = await workers.ready;
  if (workers.controller === null) {
    ready.active?.postMessage({ kind: "orpheum-claim" } satisfies ClaimMessage);
  }
  await controlled;
  announce();
  // eslint-disable-next-line no-console -- which path a tab's media takes is read in its console (D273 §7)
  console.info(`Orpheum: media now goes over ${name}`);
}
