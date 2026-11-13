// SPDX-License-Identifier: MPL-2.0

/**
 * One request over WebRTC, on a data channel of its own (D273 §8, plans/webrtc-transport.md): a text
 * head (method, path, headers), the body in binary messages of at most `maxMessageBytes`, an empty
 * message ending it; the core answers on the same channel the same way, its body handed to its reader
 * only as it reads. Which connection a channel is opened on is rtc-transport.ts.
 *
 * Chrome before 92, a Chromecast's Chrome 90 among them, neither sends an empty message on a data
 * channel nor takes one it is sent (idiosyncrasies.md), so the head also declares the body's length,
 * by which the core takes the request as whole, and asks for the response to end in `endMessage`,
 * text; an empty message still ends a request and a response, for a core before that (rtcapi's
 * EndHeader).
 */

/** The most a message holds, which every browser carries; the core's rtcapi.MaxMessageBytes. */
export const maxMessageBytes = 64 * 1024;

/** What a request's head names, with its value, to have its response end in endMessage. */
const endHeader = "x-orpheum-end";
const endInText = "text";

/**
 * The text message that ends a response its head asked to end in it: rtcapi.End, as the golden file
 * internal/core/rtcapi/testdata/end.json both sides' tests read has it.
 */
export const endMessage = '{"end":true}';

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
  /**
   * Calls sent once everything given to send so far has left the browser for the core: its buffered
   * amount drained. Absent, everything sent is taken as gone at once.
   */
  readonly whenSent?: ((sent: () => void) => void) | undefined;
  /**
   * Its SCTP stream's id, which the core's log names its channel by too: null until the browser
   * gives it one, and absent from a test's.
   */
  readonly id?: number | null | undefined;
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
    get id() {
      return channel.id;
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
    whenSent: (sent) => {
      if (channel.bufferedAmount === 0) {
        sent();
        return;
      }
      // Its threshold is left at the browser's 0, so this comes as the last byte leaves.
      channel.addEventListener("bufferedamountlow", sent, { once: true });
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

/**
 * A request as the messages that carry it: its head, its body in pieces, and an empty end. The head
 * declares the body's length and asks for its response to end in endMessage, for a browser that
 * cannot send or take an empty message; and it carries the browser's user agent, as a request over HTTP does: a Request's headers never hold it,
 * and every playback over WebRTC was logged with userAgent="" (local-path-2026-10-27.md §4 item 7).
 */
export async function requestMessages(
  request: Request,
  userAgent: string = globalThis.navigator.userAgent,
): Promise<(string | ArrayBuffer)[]> {
  const body = request.body === null ? undefined : await request.arrayBuffer();
  const url = new URL(request.url);
  const headers: Record<string, string[]> = {};
  request.headers.forEach((value, name) => {
    headers[name] = [...(headers[name] ?? []), value];
  });
  if (headers["user-agent"] === undefined && userAgent !== "") {
    headers["user-agent"] = [userAgent];
  }
  headers["content-length"] = [String(body?.byteLength ?? 0)];
  headers[endHeader] = [endInText];
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
 * How long a request over WebRTC may hear nothing while it waits on the core, a justified constant:
 * a request with no body waiting for its head, or a reader waiting for the next of a body; longer
 * than the core holds a chunk being made at most (chunkHoldMs, 30 s). Unbounded, a chunk whose
 * body stopped after 19 bytes, and every request after it on the same connection, waited for good:
 * the owner's Chrome, throttled until its film stalled and unthrottled, never loaded again
 * (staging, 2026-10-29). A request with a body waits for its head unbounded while its body is on its
 * way, since the core answers only once it has read it all, however slowly it travels, and bounded
 * once all of it has left the browser: unbounded until then, the Play page's playback, a POST, heard
 * nothing over a channel that had stopped, and the page said "Loading…" for good, its host's player
 * never drawn (the owner's Chrome back from another site, staging, 2026-10-29).
 */
export const silentAtMostMs = 40_000;

/** What a request over WebRTC fails with when it heard nothing for silentAtMostMs. */
export class SilentRequestError extends Error {
  constructor(path: string) {
    super(`nothing came for ${path} in ${String(silentAtMostMs)} ms`);
    this.name = "SilentRequestError";
  }
}

/**
 * Sends one request on a channel of its own, and resolves with the response once its head arrives,
 * its body streamed as its messages do. The channel closes when the body ends or is cancelled, or
 * when the request's signal aborts: the request then fails, or its body does, with the signal's
 * reason, and the core, its channel closed, ends the request's work (rtcapi's
 * TestChannel_AChannelTheClientClosesEndsItsRequest). One that hears nothing for silentAtMostMs while
 * it waits on the core fails, or its body does, with a SilentRequestError, its channel closed, and
 * tells silent first; so does one whose channel is not open within silentAtMostMs.
 */
export async function sendOnChannel(
  channel: ChannelLike,
  request: Request,
  silent?: () => void,
): Promise<Response> {
  const messages = await requestMessages(request);
  const { signal } = request;
  const path = new URL(request.url).pathname;
  const headWait = new HeadWait();
  const isHeadBounded = request.body === null;
  const opening =
    channel.readyState === "open"
      ? undefined
      : untilOpened(signal, () => {
          silent?.();
          return new SilentRequestError(path);
        });
  const answered = receiveResponse(channel, {
    path,
    signal: opening?.signal ?? signal,
    headWait,
    silent,
  });
  answered.catch(() => opening?.opened());
  if (isHeadBounded) {
    headWait.bound();
  }
  const sendAll = (): void => {
    if (signal.aborted) {
      return;
    }
    for (const message of messages) {
      channel.send(message);
    }
    if (!isHeadBounded) {
      // Its body all sent, the core has it to answer: its head is waited for as any other's.
      const bound = (): void => {
        headWait.bound();
      };
      if (channel.whenSent === undefined) {
        bound();
      } else {
        channel.whenSent(bound);
      }
    }
  };
  if (opening === undefined) {
    sendAll();
  } else {
    onceOpen.set(channel, () => {
      opening.opened();
      sendAll();
    });
  }
  return await answered;
}

/**
 * A signal for a request whose channel is not open yet: it aborts as signal does, or with what
 * unopened makes once silentAtMostMs pass before opened is called. A channel on a connection that
 * had stopped never opened, and a request with a body, its head's wait bound only once the body has
 * left, waited for good: the owner's Stop casting did nothing at all (prod, 2026-11-03).
 */
function untilOpened(
  signal: AbortSignal,
  unopened: () => Error,
): { readonly signal: AbortSignal; readonly opened: () => void } {
  const either = new AbortController();
  const relay = (): void => {
    either.abort(signal.reason);
  };
  if (signal.aborted) {
    relay();
  } else {
    signal.addEventListener("abort", relay, { once: true });
  }
  const timer = setTimeout(() => {
    either.abort(unopened());
  }, silentAtMostMs);
  return {
    signal: either.signal,
    opened: () => {
      clearTimeout(timer);
    },
  };
}

/** What to send once each channel not yet open opens. */
const onceOpen = new WeakMap<ChannelLike, () => void>();

/**
 * The wait for a response's head: bounded at once for a request with no body, and for one with a body
 * once all of it has left the browser, bound then starting the watch on the core's silence its
 * response named (watchWith).
 */
class HeadWait {
  private start: (() => void) | undefined;
  private isBound = false;

  /** Names what starts the watch; the wait bounded already, it starts now. */
  watchWith(start: () => void): void {
    this.start = start;
    if (this.isBound) {
      start();
    }
  }

  bound(): void {
    this.isBound = true;
    this.start?.();
  }
}

/**
 * How many body messages a reader takes before the core is granted as many more: the core sends
 * rtcapi.InitialCreditMessages before the first grant, so a film asked for from its first byte never
 * sits whole in the browser.
 */
export const grantEvery = 16;

/**
 * A request's watch on the core's silence: started, it calls heardNothing after silentAtMostMs,
 * unless it is stopped, or started again, first.
 */
class SilenceWatch {
  private timer: ReturnType<typeof setTimeout> | undefined;

  constructor(private readonly heardNothing: () => void) {}

  start(): void {
    this.stop();
    this.timer = setTimeout(this.heardNothing, silentAtMostMs);
  }

  stop(): void {
    clearTimeout(this.timer);
    this.timer = undefined;
  }
}

/**
 * A response's body as its messages arrive, handed to its reader only as it reads: each `grantEvery`
 * messages handed out grant the core as many more. While its reader waits for a message, its
 * silence is watched.
 */
class BodyQueue {
  private readonly queued: Uint8Array[] = [];
  private waiting: (() => void) | undefined;
  private isFinished = false;
  private failure: Error | undefined;
  private handedOut = 0;

  constructor(
    private readonly grant: (messages: number) => void,
    private readonly silence: SilenceWatch,
  ) {}

  push(chunk: Uint8Array): void {
    this.queued.push(chunk);
    this.wake();
  }

  finish(failure?: Error): void {
    this.isFinished = true;
    this.failure = failure;
    this.wake();
  }

  /** Why the body failed, once it has; undefined while it has not. */
  get failedWith(): Error | undefined {
    return this.failure;
  }

  /** The body as a stream its reader pulls from, each message only when asked for. */
  stream(onCancel: () => void): ReadableStream<Uint8Array> {
    return new ReadableStream<Uint8Array>(
      {
        pull: async (controller) => {
          const isWaiting = this.queued.length === 0 && !this.isFinished;
          if (isWaiting) {
            this.silence.start();
          }
          while (this.queued.length === 0 && !this.isFinished) {
            await new Promise<void>((resolve) => {
              this.waiting = resolve;
            });
          }
          if (isWaiting) {
            this.silence.stop();
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

/**
 * The response a channel carries: resolved at its head, its body streamed until the empty end; failed
 * once it hears nothing for silentAtMostMs while it waits on the core: for its head once headWait is
 * bound, for the end of a response with no body, and while its reader waits for its body.
 */
function receiveResponse(
  channel: ChannelLike,
  request: {
    readonly path: string;
    readonly signal: AbortSignal;
    readonly headWait: HeadWait;
    readonly silent: (() => void) | undefined;
  },
): Promise<Response> {
  const { path, signal } = request;
  return new Promise<Response>((resolve, reject) => {
    let isEnded = false;
    let isHeadHeard = false;
    const silence = new SilenceWatch(() => {
      request.silent?.();
      end(new SilentRequestError(path));
    });
    request.headWait.watchWith(() => {
      if (!isEnded && !isHeadHeard) {
        silence.start();
      }
    });
    const body = new BodyQueue((messages) => {
      if (!isEnded) {
        channel.send(JSON.stringify({ credit: messages }));
      }
    }, silence);
    const end = (failure?: Error): void => {
      if (isEnded) {
        return;
      }
      isEnded = true;
      silence.stop();
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
    const head = (text: string): void => {
      isHeadHeard = true;
      const response = responseOf(text, body, end);
      if (response !== undefined) {
        // A response with no body is waited on for its end; one with a body only as it is read.
        silence[response.body === null ? "start" : "stop"]();
        resolve(response);
      }
    };
    listenForAnswer(channel, path, { head, body, end });
    if (signal.aborted) {
      aborted();
    } else {
      signal.addEventListener("abort", aborted);
    }
  });
}

/**
 * Listens on channel for the answer to its request for path: its head to head, its body's pieces
 * into body, and its end, an empty message or endMessage after its head, or the channel closing or
 * failing first, to end.
 */
function listenForAnswer(
  channel: ChannelLike,
  path: string,
  answer: {
    readonly head: (text: string) => void;
    readonly body: BodyQueue;
    readonly end: (failure?: Error) => void;
  },
): void {
  let isHeadHeard = false;
  channel.listen({
    open: () => onceOpen.get(channel)?.(),
    message: (data) => {
      if (typeof data === "string" && isHeadHeard && data === endMessage) {
        answer.end();
      } else if (typeof data === "string") {
        isHeadHeard = true;
        answer.head(data);
      } else if (data.byteLength === 0) {
        answer.end();
      } else {
        answer.body.push(new Uint8Array(data));
      }
    },
    close: () => {
      answer.end(new Error(`the channel for ${path} closed before its response ended`));
    },
    error: () => {
      answer.end(new Error(`the channel for ${path} failed`));
    },
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
  return new ChannelResponse(
    stream,
    { status: answered.status, headers: toHeaders(answered.headers) },
    () => body.failedWith,
  );
}

/**
 * A response over a channel whose body, read whole, fails with why its body failed: a
 * SilentRequestError, or its channel closing first. The browser's own reading says neither:
 * Chromium reads a body whose stream failed as a bare "TypeError: Failed to fetch", and Firefox as an
 * AbortError. A Play page whose playback's head came and whose body never did was told "TypeError:
 * Failed to fetch" after 40 s, which is not asked again as a request the core answered nothing for,
 * and it said "The server could not be reached" while the core was there (the owner's Mac Chrome
 * back from the Watch Together page, staging, 2026-10-30; idiosyncrasies.md).
 */
class ChannelResponse extends Response {
  constructor(
    body: ReadableStream<Uint8Array> | null,
    init: ResponseInit,
    private readonly bodyFailure: () => Error | undefined,
  ) {
    super(body, init);
  }

  override async arrayBuffer(): Promise<ArrayBuffer> {
    return await this.read(() => super.arrayBuffer());
  }

  override async blob(): Promise<Blob> {
    return await this.read(() => super.blob());
  }

  override async json(): Promise<unknown> {
    return await this.read(() => super.json());
  }

  override async text(): Promise<string> {
    return await this.read(() => super.text());
  }

  /** What reading gives, its failure the body's own when the body failed. */
  private async read<T>(reading: () => Promise<T>): Promise<T> {
    try {
      return await reading();
    } catch (problem: unknown) {
      throw this.bodyFailure() ?? problem;
    }
  }
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
