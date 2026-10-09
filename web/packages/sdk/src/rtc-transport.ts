// SPDX-License-Identifier: MPL-2.0

/**
 * The API over WebRTC data channels (D273 §8, plans/webrtc-transport.md): the direct path to a core
 * no HTTPS address reaches. Each request goes on a data channel of its own: a text head (method, path,
 * headers), the body in binary messages of at most `maxMessageBytes`, an empty message ending it;
 * the core answers on the same channel the same way, and the client closes it. A client opens
 * `clientConnections` peer connections and sends each request on the one with the fewest under way.
 */

/** The most a message holds, which every browser carries; the core's rtcapi.MaxMessageBytes. */
export const maxMessageBytes = 64 * 1024;

/**
 * How many peer connections a client opens: one is bound by its congestion window, and four carried
 * three times as much (reachability-approaches-2026-10-09.md §9.4); the core's ClientConnections.
 */
export const clientConnections = 4;

/** How long a connection may take to gather its candidates and connect. */
export const connectTimeoutMs = 10_000;

/** A session description: an offer or an answer, and its SDP. */
export interface Description {
  readonly type: "offer" | "answer";
  readonly sdp: string;
}

/** Takes a client's offer to the core and returns its answer: the offer endpoint, or a discovery. */
export type Signaller = (offer: Description) => Promise<Description>;

/** What a channel says, to the one request it carries. */
export interface ChannelListener {
  readonly open: () => void;
  readonly message: (data: string | ArrayBuffer) => void;
  readonly close: () => void;
  readonly error: () => void;
}

/** What the transport needs of a data channel: the browser's, through channelOf, or a test's. */
export interface ChannelLike {
  readonly readyState: RTCDataChannelState;
  send(data: string | ArrayBuffer): void;
  close(): void;
  listen(listener: ChannelListener): void;
}

/** What the transport needs of a peer connection: opening a channel. */
export interface PeerLike {
  createDataChannel(label: string): ChannelLike;
  close(): void;
}

/** A request's head, as the core reads it: rtcapi.RequestHead. */
interface RequestHead {
  readonly method: string;
  readonly path: string;
  readonly headers: Record<string, string[]>;
}

/** A response's head, as the core sends it: rtcapi.ResponseHead. */
interface ResponseHead {
  readonly status: number;
  readonly headers?: Record<string, string[]>;
}

/** The statuses whose response has no body, which a Response refuses one for. */
const bodylessStatuses = new Set([101, 204, 205, 304]);

/** A browser's data channel as the transport uses it; its binaryType must be "arraybuffer". */
export function channelOf(channel: RTCDataChannel): ChannelLike {
  return {
    get readyState() {
      return channel.readyState;
    },
    send: (data) => {
      // RTCDataChannel.send is overloaded per type, so a union must be narrowed to pick one.
      if (typeof data === "string") {
        channel.send(data);
      } else {
        channel.send(data);
      }
    },
    close: () => {
      channel.close();
    },
    listen: (listener) => {
      channel.addEventListener("open", listener.open);
      channel.addEventListener("message", (event: MessageEvent<unknown>) => {
        const { data } = event;
        if (typeof data === "string" || data instanceof ArrayBuffer) {
          listener.message(data);
        }
      });
      channel.addEventListener("close", listener.close);
      channel.addEventListener("error", listener.error);
    },
  };
}

/** A browser's peer connection as the transport uses it: each channel delivers ArrayBuffers. */
export function peerOf(peer: RTCPeerConnection): PeerLike {
  return {
    createDataChannel: (label) => {
      const channel = peer.createDataChannel(label);
      channel.binaryType = "arraybuffer";
      return channelOf(channel);
    },
    close: () => {
      peer.close();
    },
  };
}

/** A request as the messages that carry it: its head, its body in pieces, and an empty end. */
export async function requestMessages(request: Request): Promise<(string | ArrayBuffer)[]> {
  const body = request.body === null ? undefined : await request.arrayBuffer();
  const url = new URL(request.url);
  const headers: Record<string, string[]> = {};
  request.headers.forEach((value, name) => {
    headers[name] = [...(headers[name] ?? []), value];
  });
  const head: RequestHead = { method: request.method, path: url.pathname + url.search, headers };
  const messages: (string | ArrayBuffer)[] = [JSON.stringify(head)];
  for (let start = 0; body !== undefined && start < body.byteLength; start += maxMessageBytes) {
    messages.push(body.slice(start, start + maxMessageBytes));
  }
  messages.push(new ArrayBuffer(0));
  return messages;
}

/**
 * Sends one request on a channel of its own, and resolves with the response once its head arrives,
 * its body streamed as its messages do. The channel closes when the body ends or is cancelled.
 */
export async function sendOnChannel(channel: ChannelLike, request: Request): Promise<Response> {
  const messages = await requestMessages(request);
  const answered = receiveResponse(channel, new URL(request.url).pathname);
  const sendAll = (): void => {
    for (const message of messages) {
      channel.send(message);
    }
  };
  if (channel.readyState === "open") {
    sendAll();
  } else {
    onceOpen.set(channel, sendAll);
  }
  return await answered;
}

/** What to send once each channel not yet open opens. */
const onceOpen = new WeakMap<ChannelLike, () => void>();

/**
 * How many body messages a reader takes before the core is granted as many more: the core sends
 * rtcapi.InitialCreditMessages before the first grant, so a film asked for from its first byte never
 * sits whole in the browser.
 */
export const grantEvery = 16;

/**
 * A response's body as its messages arrive, handed to its reader only as it reads: each `grantEvery`
 * messages handed out grant the core as many more.
 */
class BodyQueue {
  private readonly queued: Uint8Array[] = [];
  private waiting: (() => void) | undefined;
  private isFinished = false;
  private failure: Error | undefined;
  private handedOut = 0;

  constructor(private readonly grant: (messages: number) => void) {}

  push(chunk: Uint8Array): void {
    this.queued.push(chunk);
    this.wake();
  }

  finish(failure?: Error): void {
    this.isFinished = true;
    this.failure = failure;
    this.wake();
  }

  /** The body as a stream its reader pulls from, each message only when asked for. */
  stream(onCancel: () => void): ReadableStream<Uint8Array> {
    return new ReadableStream<Uint8Array>(
      {
        pull: async (controller) => {
          while (this.queued.length === 0 && !this.isFinished) {
            await new Promise<void>((resolve) => {
              this.waiting = resolve;
            });
          }
          const next = this.queued.shift();
          if (next !== undefined) {
            controller.enqueue(next);
            this.handedOut += 1;
            if (this.handedOut % grantEvery === 0) {
              this.grant(grantEvery);
            }
          } else if (this.failure === undefined) {
            controller.close();
          } else {
            controller.error(this.failure);
          }
        },
        cancel: onCancel,
      },
      { highWaterMark: 0 },
    );
  }

  private wake(): void {
    const waiting = this.waiting;
    this.waiting = undefined;
    waiting?.();
  }
}

/** The response a channel carries: resolved at its head, its body streamed until the empty end. */
function receiveResponse(channel: ChannelLike, path: string): Promise<Response> {
  return new Promise<Response>((resolve, reject) => {
    let isEnded = false;
    const body = new BodyQueue((messages) => {
      if (!isEnded) {
        channel.send(JSON.stringify({ credit: messages }));
      }
    });
    const end = (failure?: Error): void => {
      if (isEnded) {
        return;
      }
      isEnded = true;
      body.finish(failure);
      if (failure !== undefined) {
        reject(failure);
      }
      channel.close();
    };
    const takeHead = (text: string): void => {
      let answered: ResponseHead;
      try {
        answered = readHead(text);
      } catch (error: unknown) {
        end(error instanceof Error ? error : new Error(String(error)));
        return;
      }
      const stream = bodylessStatuses.has(answered.status)
        ? null
        : body.stream(() => {
            end();
          });
      resolve(
        new Response(stream, { status: answered.status, headers: toHeaders(answered.headers) }),
      );
    };
    channel.listen({
      open: () => onceOpen.get(channel)?.(),
      message: (data) => {
        if (typeof data === "string") {
          takeHead(data);
        } else if (data.byteLength === 0) {
          end();
        } else {
          body.push(new Uint8Array(data));
        }
      },
      close: () => {
        end(new Error(`the channel for ${path} closed before its response ended`));
      },
      error: () => {
        end(new Error(`the channel for ${path} failed`));
      },
    });
  });
}

/** A response head as the core sent it, or an error for one that is not. */
function readHead(text: string): ResponseHead {
  const parsed: unknown = JSON.parse(text);
  if (
    typeof parsed !== "object" ||
    parsed === null ||
    !("status" in parsed) ||
    typeof parsed.status !== "number"
  ) {
    throw new Error("the core's response head is not one");
  }
  const headers =
    "headers" in parsed && typeof parsed.headers === "object" && parsed.headers !== null
      ? Object.fromEntries(
          Object.entries(parsed.headers).filter((entry): entry is [string, string[]] =>
            Array.isArray(entry[1]),
          ),
        )
      : {};
  return { status: parsed.status, headers };
}

/** A head's headers as a Headers. */
function toHeaders(headers: Record<string, string[]> | undefined): Headers {
  const result = new Headers();
  for (const [name, values] of Object.entries(headers ?? {})) {
    for (const value of values) {
      result.append(name, value);
    }
  }
  return result;
}

/** One of the transport's connections, and how many requests are under way on it. */
interface Link {
  readonly peer: PeerLike;
  underWay: number;
}

/**
 * A client's WebRTC transport to its core: `clientConnections` peer connections, each request sent
 * on the one with the fewest under way. `fetch` is what the API client sends requests with.
 */
export class RtcTransport {
  private readonly links: readonly Link[];

  constructor(peers: readonly PeerLike[]) {
    if (peers.length === 0) {
      throw new Error("a transport needs at least one connection");
    }
    this.links = peers.map((peer) => ({ peer, underWay: 0 }));
  }

  /** Sends a request to the core and resolves with its response. */
  readonly fetch = async (request: Request): Promise<Response> => {
    const link = this.links.reduce((least, each) =>
      each.underWay < least.underWay ? each : least,
    );
    link.underWay += 1;
    try {
      return await sendOnChannel(link.peer.createDataChannel("request"), request);
    } finally {
      link.underWay -= 1;
    }
  };

  /**
   * A channel of its own on the connection with the fewest under way, for a request that holds it
   * open: a WebSocket's messages (rtc-together.ts).
   */
  openChannel(): ChannelLike {
    const link = this.links.reduce((least, each) =>
      each.underWay < least.underWay ? each : least,
    );
    return link.peer.createDataChannel("upgrade");
  }

  /** Closes every connection. */
  close(): void {
    for (const link of this.links) {
      link.peer.close();
    }
  }
}

/**
 * Opens `count` peer connections to the core through signaller, each with every candidate gathered
 * before its offer is sent, and resolves once all are connected.
 */
export async function connectRtc(
  signaller: Signaller,
  rtcConfig: RTCConfiguration = {},
  count: number = clientConnections,
): Promise<RtcTransport> {
  const peers = await Promise.all(
    Array.from({ length: count }, () => connectOne(signaller, rtcConfig)),
  );
  return new RtcTransport(peers);
}

/** One peer connection, connected through signaller. */
async function connectOne(signaller: Signaller, rtcConfig: RTCConfiguration): Promise<PeerLike> {
  const peer = new RTCPeerConnection(rtcConfig);
  try {
    // An offer needs something to negotiate: a channel opened and never used.
    peer.createDataChannel("hello");
    await peer.setLocalDescription(await peer.createOffer());
    await gathered(peer);
    const local = peer.localDescription;
    if (local === null) {
      throw new Error("the connection made no offer");
    }
    const answer = await signaller({ type: "offer", sdp: local.sdp });
    await peer.setRemoteDescription(answer);
    await connected(peer);
    return peerOf(peer);
  } catch (error: unknown) {
    peer.close();
    throw error;
  }
}

/** Resolves once peer has gathered its candidates, or after connectTimeoutMs with those it has. */
function gathered(peer: RTCPeerConnection): Promise<void> {
  return new Promise((resolve) => {
    if (peer.iceGatheringState === "complete") {
      resolve();
      return;
    }
    const timer = setTimeout(resolve, connectTimeoutMs);
    peer.addEventListener("icegatheringstatechange", () => {
      if (peer.iceGatheringState === "complete") {
        clearTimeout(timer);
        resolve();
      }
    });
  });
}

/** Resolves once peer is connected; rejects when it fails or takes over connectTimeoutMs. */
function connected(peer: RTCPeerConnection): Promise<void> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      reject(new Error("the WebRTC connection did not connect in time"));
    }, connectTimeoutMs);
    const settle = (): void => {
      if (peer.connectionState === "connected") {
        clearTimeout(timer);
        resolve();
      } else if (peer.connectionState === "failed" || peer.connectionState === "closed") {
        clearTimeout(timer);
        reject(new Error(`the WebRTC connection ${peer.connectionState}`));
      }
    };
    peer.addEventListener("connectionstatechange", settle);
    settle();
  });
}

/** Sends a request and resolves with its response: the browser's fetch, or a transport's. */
export type FetchRequest = (request: Request) => Promise<Response>;

/**
 * The fetch the API client sends with, whose path can change while the app runs: the first path to
 * the core that answers is used at once, and a better one answering later takes over (D273 §7).
 * A request already sent finishes on the path it took.
 */
export class SwitchableFetch {
  private current: FetchRequest;

  /** Starts on first: the browser's own fetch when omitted. */
  constructor(first: FetchRequest = (request) => fetch(request)) {
    this.current = first;
  }

  /** Sends a request on the path in use now. */
  readonly fetch: FetchRequest = (request) => this.current(request);

  /** Moves the requests sent from now on to next, the path called name, and says so in the console. */
  use(next: FetchRequest, name: string): void {
    this.current = next;
    // eslint-disable-next-line no-console -- which path a tab's requests take is read in its console (D273 §7)
    console.info(`Orpheum: requests now go over ${name}`);
  }
}
