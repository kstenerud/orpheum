// SPDX-License-Identifier: MPL-2.0

/**
 * One request over WebRTC, on a data channel of its own (D273 §8, plans/webrtc-transport.md): a text
 * head (method, path, headers), the body in binary messages of at most `maxMessageBytes`, an empty
 * message ending it; the core answers on the same channel the same way, its body handed to its reader
 * only as it reads. Which connection a channel is opened on is rtc-transport.ts.
 */

/** The most a message holds, which every browser carries; the core's rtcapi.MaxMessageBytes. */
export const maxMessageBytes = 64 * 1024;

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
 * What a request whose signal aborted fails with, as fetch fails: the signal's reason, an AbortError
 * unless the one who aborted it gave another.
 */
export function abortReason(signal: AbortSignal): Error {
  const reason: unknown = signal.reason;
  return reason instanceof Error
    ? reason
    : new DOMException("the request was aborted", "AbortError");
}

/**
 * Sends one request on a channel of its own, and resolves with the response once its head arrives,
 * its body streamed as its messages do. The channel closes when the body ends or is cancelled, or
 * when the request's signal aborts: the request then fails, or its body does, with the signal's
 * reason, and the core, its channel closed, ends the request's work (rtcapi's
 * TestChannel_AChannelTheClientClosesEndsItsRequest).
 */
export async function sendOnChannel(channel: ChannelLike, request: Request): Promise<Response> {
  const messages = await requestMessages(request);
  const { signal } = request;
  const answered = receiveResponse(channel, new URL(request.url).pathname, signal);
  const sendAll = (): void => {
    if (signal.aborted) {
      return;
    }
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
function receiveResponse(
  channel: ChannelLike,
  path: string,
  signal: AbortSignal,
): Promise<Response> {
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
      signal.removeEventListener("abort", aborted);
      body.finish(failure);
      if (failure !== undefined) {
        reject(failure);
      }
      channel.close();
    };
    const aborted = (): void => {
      end(abortReason(signal));
    };
    channel.listen({
      open: () => onceOpen.get(channel)?.(),
      message: (data) => {
        if (typeof data === "string") {
          const response = responseOf(data, body, end);
          if (response !== undefined) {
            resolve(response);
          }
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
    if (signal.aborted) {
      aborted();
    } else {
      signal.addEventListener("abort", aborted);
    }
  });
}

/**
 * The response a head begins, its body read from body, or undefined for a head that is not one,
 * which ends the request with why.
 */
function responseOf(
  text: string,
  body: BodyQueue,
  end: (failure?: Error) => void,
): Response | undefined {
  let answered: ResponseHead;
  try {
    answered = readHead(text);
  } catch (error: unknown) {
    end(error instanceof Error ? error : new Error(String(error)));
    return undefined;
  }
  const stream = bodylessStatuses.has(answered.status)
    ? null
    : body.stream(() => {
        end();
      });
  return new Response(stream, { status: answered.status, headers: toHeaders(answered.headers) });
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
