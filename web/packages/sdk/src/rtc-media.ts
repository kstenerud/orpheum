// SPDX-License-Identifier: MPL-2.0

import type { operations } from "./gen/api.ts";
import { carriesMediaOver, mediaPathNow } from "./media-carrier.ts";
import {
  controlledWithin,
  controlWaitMs,
  isControlled,
  PageNotControlled,
} from "./worker-control.ts";

/**
 * Media over the WebRTC path (plans/webrtc-transport.md W2b). What the browser loads by itself, an
 * image, a video's bytes, hls.js's chunks, a subtitle track, cannot use a data channel. A service
 * worker takes those loads while the page says it is on WebRTC and asks the page, which holds the
 * connections, to fetch each through its transport; the page sends the response back over a message
 * port, its pieces moved rather than copied, up to `relayedAheadBytes` ahead of what the service
 * worker's reader has taken, so a film never sits whole in memory and a busy page does not hold up
 * every piece: one piece for each round trip through the page, a Fire TV's 30 ms, carried 17 Mb/s.
 *
 * A page and its worker can be of different releases: skipWaiting and clients.claim hand a page
 * still open to a newer worker, and a reloaded page is controlled by the worker before until the
 * newer one claims it. So the page says in its head which way it sends a body (`relay: "credit"`),
 * and the worker grants room only to a page that says so, pulling a piece at a time from any other;
 * and the page answers a bare "pull" with one piece, as a worker of a release before asks. A relay
 * that spoke only its own protocol hung every load across that, and staging's pages stayed black
 * (2026-10-28).
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

/**
 * What the page sends back on a load's port. Its head says, in `relay`, that the page sends the body
 * as the worker grants it room; a page of a release before says nothing, and sends a piece for each
 * "pull".
 */
export type ReplyMessage =
  | {
      readonly kind: "head";
      readonly status: number;
      readonly headers: readonly (readonly [string, string])[];
      readonly relay?: "credit";
    }
  | { readonly kind: "chunk"; readonly data: ArrayBuffer }
  | { readonly kind: "end" }
  | { readonly kind: "error"; readonly message: string };

/**
 * What the service worker sends on a load's port: room for `bytes` more of the body, to a page whose
 * head said `relay: "credit"`; the next piece ("pull"), as a worker of a release before asks and to a
 * page that did not say so; or stop. A page sends while it has room, so it is at most a piece past
 * it, and one piece for each "pull".
 */
export type BodyRequestMessage =
  { readonly kind: "credit"; readonly bytes: number } | "pull" | "cancel";

/**
 * How far a load's body may run ahead of the service worker's reader, a justified constant: the
 * pieces in flight between the page and its worker and those the worker holds for its reader. One
 * piece at a time (64 KiB) for each round trip through the page carried a Fire TV's chunks at
 * 15–18 Mb/s (staging, 2026-10-28); a mebibyte at a round trip of 10 ms carries some 800 Mb/s, what
 * the data channel itself carries, and a few loads at once hold a few mebibytes, no film.
 */
export const relayedAheadBytes = 1024 * 1024;

/**
 * The share of `relayedAheadBytes` the worker grants at least at a time, so a reader's every piece
 * does not cost the busy page a message of its own: granted a piece's room for each piece taken, a
 * reader slower than the page (Firefox's, its worker under load) cost one for nearly every piece.
 */
const grantAtLeastShare = 4;

/** Whether a message is the service worker's grant of room for more of a body. */
function isCredit(message: unknown): message is { kind: "credit"; bytes: number } {
  return (
    typeof message === "object" &&
    message !== null &&
    "kind" in message &&
    message.kind === "credit" &&
    "bytes" in message &&
    typeof message.bytes === "number"
  );
}

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
 * carries its own media, "http" when it says its media goes over HTTP, or undefined when it gives no
 * answer within waitMs, as a page of a release before that never carried media, or one still finding
 * its core, gives none.
 */
export async function askPathOf(
  page: { postMessage(message: unknown, transfer: Transferable[]): void },
  waitMs: number = pathQueryWaitMs,
): Promise<PathMessage["path"] | undefined> {
  const { port1, port2 } = new MessageChannel();
  try {
    return await new Promise((resolve) => {
      const timer = setTimeout(() => {
        resolve(undefined);
      }, waitMs);
      port1.addEventListener("message", (event: MessageEvent<unknown>) => {
        clearTimeout(timer);
        resolve(isPathMessage(event.data) ? event.data.path : undefined);
      });
      port1.start();
      page.postMessage({ kind: "orpheum-path-query" } satisfies PathQueryMessage, [port2]);
    });
  } finally {
    port1.close();
  }
}

/**
 * The service worker's word to a page on a load of its media it sent to the network, or on the path
 * it keeps for the page, and why, for the page's log: where a tab's posters went, and why, was found
 * only by digging (the owner's Mac, staging, 2026-10-29).
 */
export interface PathNoteMessage {
  readonly kind: "orpheum-path-note";
  readonly note: string;
}

/** Whether a message is the service worker's note on a page's path. */
export function isPathNoteMessage(message: unknown): message is PathNoteMessage {
  return (
    typeof message === "object" &&
    message !== null &&
    "kind" in message &&
    message.kind === "orpheum-path-note" &&
    "note" in message &&
    typeof message.note === "string"
  );
}

/**
 * A load from a page a worker has no word from: the page, when there is one, is asked its path
 * (askPathOf), and the load goes through it when it carries its media, else to the network as the
 * browser would send it. Only the page's answer is kept, by noted: a silence kept as HTTP sent every
 * later load of a page under a loader, still finding its core when asked, to its client origin, which
 * serves no media and answered each poster 404 (the owner's Mac, staging, 2026-10-29). A page that
 * said nothing is asked again at its next load, and told, by told, why this one went to the network.
 */
export async function loadAfterAsking(load: {
  readonly page: { postMessage(message: unknown, transfer: Transferable[]): void } | undefined;
  readonly request: Request;
  readonly throughThePage: () => Promise<Response>;
  readonly noted: (path: PathMessage["path"]) => void;
  readonly told: (note: string) => void;
}): Promise<Response> {
  if (load.page === undefined) {
    return await networkFetch(load.request);
  }
  const path = await askPathOf(load.page);
  if (path === undefined) {
    load.told(
      `the service worker sent a media load to the network: asked its path, the page said nothing within ${String(pathQueryWaitMs)} ms`,
    );
    return await networkFetch(load.request);
  }
  load.noted(path);
  if (path === "http") {
    load.told("the service worker keeps the page on HTTP: the page said its media goes over HTTP");
    return await networkFetch(load.request);
  }
  return await load.throughThePage();
}

/**
 * Tells report, once each for the page's life, every different note the service worker gives the
 * page on its path (PathNoteMessage).
 */
export function reportPathNotes(
  workers: Pick<ServiceWorkerContainer, "addEventListener" | "startMessages">,
  report: (note: string) => void,
): void {
  const told = new Set<string>();
  workers.addEventListener("message", (event: MessageEvent<unknown>) => {
    if (isPathNoteMessage(event.data) && !told.has(event.data.note)) {
      told.add(event.data.note);
      report(event.data.note);
    }
  });
  workers.startMessages();
}

/**
 * Has this page answer its service worker's question on its path with "http" until it carries its
 * media (carryMediaOver), which answers "webrtc" from then on: a worker keeps only a page's answer, so
 * a page that said nothing would be asked, and waited for, at each load. Only a page on the core's own
 * origin says so; one under a loader says nothing until it carries its media, its origin serving none.
 */
export function sayMediaGoesOverHttp(
  workers: Pick<ServiceWorkerContainer, "addEventListener" | "startMessages">,
): void {
  workers.addEventListener("message", (event: MessageEvent<unknown>) => {
    const [port] = event.ports;
    if (port !== undefined && isPathQueryMessage(event.data) && mediaPathNow() === undefined) {
      port.postMessage({ kind: "orpheum-path", path: "http" } satisfies PathMessage);
    }
  });
  workers.startMessages();
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
 * token unless the load names its own, and answers on port: the head at once, saying it sends the
 * body as the worker grants room, and then the body, while the worker's room lasts, or a piece for
 * each "pull" of a worker of a release before.
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
  // A cancel before the head aborts the load: hls.js giving a chunk up before its head came left the
  // page's request holding its connection's room until its answer, which a stuck connection never sent.
  const asked = new AbortController();
  const relayed: { body?: BodyRelay } = {};
  port.addEventListener("message", (event) => {
    if (event.data === "cancel") {
      asked.abort();
      relayed.body?.cancel();
    } else if (relayed.body !== undefined) {
      relayAsked(relayed.body, event.data); // asked for only once the head is sent
    }
  });
  port.start();
  let response: Response;
  try {
    response = await load.send(
      new Request(message.url, { method: message.method, headers, signal: asked.signal }),
    );
  } catch (error: unknown) {
    port.postMessage({ kind: "error", message: String(error) } satisfies ReplyMessage);
    return;
  }
  const replied: [string, string][] = [];
  response.headers.forEach((value, name) => {
    replied.push([name, value]);
  });
  // A response with no body answers as an empty one: a worker of a release before pulls a body of
  // any status but its few bodyless ones, and is answered "end".
  relayed.body = new BodyRelay((response.body ?? emptyBody()).getReader(), port);
  if (asked.signal.aborted) {
    relayed.body.cancel();
  }
  port.postMessage({
    kind: "head",
    status: response.status,
    headers: replied,
    relay: "credit",
  } satisfies ReplyMessage);
}

/** Does as the service worker asked of relay: a piece for a "pull", more room for a grant. */
function relayAsked(relay: BodyRelay, data: unknown): void {
  if (data === "pull") {
    relay.pullOne();
  } else if (isCredit(data)) {
    relay.grant(data.bytes);
  }
}

/** A body of nothing. */
function emptyBody(): ReadableStream<Uint8Array> {
  return new ReadableStream<Uint8Array>({
    start: (controller) => {
      controller.close();
    },
  });
}

/**
 * A body the page relays to its service worker, one read at a time, until it ends, fails or the
 * worker stops it: while the worker's granted room lasts, or one piece for each "pull" of a worker
 * of a release before.
 */
class BodyRelay {
  private room = 0;
  private pullsOwed = 0;
  private isReading = false;
  private isOver = false;

  constructor(
    private readonly reader: ReadableStreamDefaultReader<Uint8Array>,
    private readonly port: PortLike,
  ) {}

  grant(bytes: number): void {
    this.room += bytes;
    void this.sendWhileAsked();
  }

  pullOne(): void {
    this.pullsOwed += 1;
    void this.sendWhileAsked();
  }

  cancel(): void {
    this.isOver = true;
    void this.reader.cancel();
  }

  private async sendWhileAsked(): Promise<void> {
    if (this.isReading) {
      return;
    }
    this.isReading = true;
    try {
      while ((this.room > 0 || this.pullsOwed > 0) && !this.isOver) {
        // A read the worker's cancel overtook ends done, or sends a piece the worker drops.
        const { done: isDone, value } = await this.reader.read();
        if (isDone) {
          this.isOver = true;
          this.port.postMessage({ kind: "end" } satisfies ReplyMessage);
          return;
        }
        const data = movableBufferOf(value);
        if (this.pullsOwed > 0) {
          this.pullsOwed -= 1;
        } else {
          this.room -= data.byteLength;
        }
        this.port.postMessage({ kind: "chunk", data } satisfies ReplyMessage, [data]);
      }
    } catch (error: unknown) {
      if (!this.isOver) {
        this.isOver = true;
        this.port.postMessage({ kind: "error", message: String(error) } satisfies ReplyMessage);
      }
    } finally {
      this.isReading = false;
    }
  }
}

/**
 * A piece's bytes as a buffer to move to the worker: its own buffer when it is the whole of one, as
 * a data channel's message is (the page's load holds the body's only reader, so nothing else reads
 * it), else a copy, so moving it takes nothing else with it.
 */
function movableBufferOf(piece: Uint8Array): ArrayBuffer {
  const { buffer } = piece;
  if (
    buffer instanceof ArrayBuffer &&
    piece.byteOffset === 0 &&
    piece.byteLength === buffer.byteLength
  ) {
    return buffer;
  }
  return piece.slice().buffer;
}

/**
 * The service worker's side: the response a load's port carries, resolved at its head, its body a
 * stream that grants the page room for `relayedAheadBytes` ahead of its reader when its head says it
 * sends that way, else pulls one piece at a time from it, as from a page of a release before. The
 * load's signal aborting, as hls.js's abort of its request does where the browser tells its worker,
 * cancels the page's load, its head come or not.
 */
export function responseFromPort(port: PortLike, signal?: AbortSignal): Promise<Response> {
  signal?.addEventListener(
    "abort",
    () => {
      port.postMessage("cancel" satisfies BodyRequestMessage);
    },
    { once: true },
  );
  return new Promise<Response>((resolve, reject) => {
    let deliver: ((reply: ReplyMessage) => void) | undefined;
    port.addEventListener("message", (event) => {
      const reply = asReply(event.data);
      if (reply === undefined) {
        return;
      }
      if (reply.kind === "head") {
        const setDeliver = (next: (reply: ReplyMessage) => void): void => {
          deliver = next;
        };
        let body: ReadableStream<Uint8Array> | null = null;
        if (hasBody(reply.status)) {
          body =
            reply.relay === "credit"
              ? creditedBody(port, setDeliver)
              : pulledBody(port, setDeliver);
        }
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

/**
 * A body pulled one piece at a time over port, from a page of a release before; setDeliver hands it
 * the replies after the head.
 */
function pulledBody(
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
          port.postMessage("pull" satisfies BodyRequestMessage);
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
        port.postMessage("cancel" satisfies BodyRequestMessage);
      },
    },
    { highWaterMark: 0 },
  );
}

/**
 * A body sent over port as the page has room to; setDeliver hands it the replies after the head.
 * Each piece is queued as it comes, and as the reader takes them the page is granted room again, so
 * what is in flight and queued together stays near `relayedAheadBytes`, granted a
 * `grantAtLeastShare` of that or more at a time.
 */
function creditedBody(
  port: PortLike,
  setDeliver: (deliver: (reply: ReplyMessage) => void) => void,
): ReadableStream<Uint8Array> {
  let granted = 0;
  let received = 0;
  let isOver = false;
  let arrived: (() => void) | undefined;
  const wake = (): void => {
    const next = arrived;
    arrived = undefined;
    next?.();
  };
  return new ReadableStream<Uint8Array>(
    {
      start: (controller) => {
        setDeliver((reply) => {
          if (isOver) {
            return;
          }
          if (reply.kind === "chunk") {
            received += reply.data.byteLength;
            controller.enqueue(new Uint8Array(reply.data));
          } else if (reply.kind === "error") {
            isOver = true;
            controller.error(new Error(reply.message));
          } else {
            isOver = true;
            controller.close();
          }
          wake();
        });
      },
      pull: async (controller) => {
        const owed = granted - received;
        const room = (controller.desiredSize ?? 0) - owed;
        // Less than a share is waited out: the reader's next take calls this again, and an empty
        // queue with nothing owed leaves the whole window.
        if (room >= relayedAheadBytes / grantAtLeastShare) {
          granted += room;
          port.postMessage({ kind: "credit", bytes: room } satisfies BodyRequestMessage);
        }
        if (granted - received > 0 && !isOver) {
          await new Promise<void>((resolve) => {
            arrived = resolve;
          });
        }
      },
      cancel: () => {
        isOver = true;
        wake();
        port.postMessage("cancel" satisfies BodyRequestMessage);
      },
    },
    { highWaterMark: relayedAheadBytes, size: (piece) => piece.byteLength },
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
  const head = {
    kind: "head",
    status: data.status,
    headers: headers.filter(isHeaderPair),
  } as const;
  // A page of a release before says nothing of how it sends, and is pulled from.
  return "relay" in data && data.relay === "credit" ? { ...head, relay: "credit" } : head;
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
 * it controls the page, and says in the console that media now goes over name; rejects with
 * PageNotControlled when no live worker controls it within controlWaitMs of asking. It says so again
 * to each worker that takes the page over.
 */
export async function carryMediaOver(
  send: (request: Request) => Promise<Response>,
  credentials: () => MediaCredentials,
  name: string,
): Promise<void> {
  carriesMediaOver(name);
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
  // Said again to each worker that takes the page over: a worker new to it knows no page's path.
  const announce = (): void => {
    workers.controller?.postMessage({ kind: "orpheum-path", path: "webrtc" } satisfies PathMessage);
  };
  workers.addEventListener("controllerchange", announce);
  await workers.register(serviceWorkerUrl);
  const ready = await workers.ready;
  if (!isControlled(workers)) {
    ready.active?.postMessage({ kind: "orpheum-claim" } satisfies ClaimMessage);
  }
  if (!(await controlledWithin(workers, controlWaitMs))) {
    throw new PageNotControlled();
  }
  announce();
  // eslint-disable-next-line no-console -- which path a tab's media takes is read in its console (D273 §7)
  console.info(`Orpheum: media now goes over ${name}`);
}
