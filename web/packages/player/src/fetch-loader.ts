// SPDX-License-Identifier: MPL-2.0

/**
 * hls.js's loads made by a fetch the page is given, not by XMLHttpRequest: a page that reaches its
 * core over a transport of its own, the lab's cast receiver over WebRTC (plans/tv-device-tests.md),
 * where no service worker carries the requests the browser makes (the 11-05 batch's item 10). hls.js
 * calls the browser's fetch itself in its fetch loader, so it cannot be handed another.
 */

import { LoadStats } from "hls.js";
import type {
  Loader,
  LoaderCallbacks,
  LoaderConfiguration,
  LoaderContext,
  LoaderStats,
} from "hls.js";
import type { FetchRequest } from "@orpheum/sdk";

import type { ProgressingRequest } from "./first-piece.ts";
import type { LoaderClass } from "./link-shaping.ts";

/** What each of a playback's requests carries, and who hears of its answer and its arrival. */
export interface RequestSetup {
  /** The headers each request carries. */
  readonly headersOf: () => readonly (readonly [string, string])[];
  /** Told the answer's status and headers, before its body. */
  readonly answered: (status: number, header: (name: string) => string | null) => void;
  /** Told of the load of url beginning, on a request whose progress events say its bytes. */
  readonly follow: (request: ProgressingRequest, url: string) => void;
}

/** A wait of ms as a timer can be set: none for a policy's 0 or infinity, which say no limit. */
function isLimit(ms: number): boolean {
  return Number.isFinite(ms) && ms > 0;
}

/** The body of answer, its bytes counted to onBytes as each piece arrives. */
async function bodyOf(answer: Response, onBytes: (loaded: number) => void): Promise<Uint8Array> {
  if (answer.body === null) {
    const whole = new Uint8Array(await answer.arrayBuffer());
    onBytes(whole.byteLength);
    return whole;
  }
  const reader = answer.body.getReader();
  const pieces: Uint8Array[] = [];
  let loaded = 0;
  for (;;) {
    const { done: isDone, value } = await reader.read();
    if (isDone) {
      break;
    }
    pieces.push(value);
    loaded += value.byteLength;
    onBytes(loaded);
  }
  const whole = new Uint8Array(loaded);
  let at = 0;
  for (const piece of pieces) {
    whole.set(piece, at);
    at += piece.byteLength;
  }
  return whole;
}

/**
 * The class of hls.js loader that makes each load by send, its request carrying setup's headers and
 * a Range for a byte range, and its stats timed by performance.now(), hls.js's clock: a load past
 * its policy's wait for the first byte, or for the whole, times out, as hls.js's own loaders do.
 */
export function fetchLoader(send: FetchRequest, setup: RequestSetup): LoaderClass {
  // hls.js constructs each loader with its config, which this one needs none of.
  return class extends FetchLoader {
    constructor() {
      super(send, setup);
    }
  };
}

/** A load made by send, as fetchLoader's class makes each. */
class FetchLoader implements Loader<LoaderContext> {
  context: LoaderContext | null = null;
  stats: LoaderStats = new LoadStats();
  readonly #send: FetchRequest;
  readonly #setup: RequestSetup;
  #stopping: AbortController | undefined;
  #timer: ReturnType<typeof setTimeout> | undefined;
  #callbacks: LoaderCallbacks<LoaderContext> | undefined;

  constructor(send: FetchRequest, setup: RequestSetup) {
    this.#send = send;
    this.#setup = setup;
  }

  load(
    context: LoaderContext,
    config: LoaderConfiguration,
    callbacks: LoaderCallbacks<LoaderContext>,
  ): void {
    this.context = context;
    this.#callbacks = callbacks;
    this.stats = new LoadStats();
    this.stats.loading.start = performance.now();
    const stopping = new AbortController();
    this.#stopping = stopping;
    const { maxTimeToFirstByteMs, maxLoadTimeMs } = config.loadPolicy;
    this.#timeOutAfter(isLimit(maxTimeToFirstByteMs) ? maxTimeToFirstByteMs : maxLoadTimeMs);
    void this.#run(context, stopping, maxLoadTimeMs);
  }

  abort(): void {
    const callbacks = this.#callbacks;
    if (this.#stop() && this.context !== null) {
      this.stats.aborted = true;
      callbacks?.onAbort?.(this.stats, this.context, null);
    }
  }

  destroy(): void {
    this.#stop();
    this.#callbacks = undefined;
    this.context = null;
  }

  /** Stops the load on its way, if one is; whether one was. */
  #stop(): boolean {
    clearTimeout(this.#timer);
    const stopping = this.#stopping;
    this.#stopping = undefined;
    stopping?.abort();
    return stopping !== undefined;
  }

  /** Times the load out after ms from now, none for no limit. */
  #timeOutAfter(ms: number): void {
    clearTimeout(this.#timer);
    if (!isLimit(ms)) {
      return;
    }
    this.#timer = setTimeout(() => {
      const callbacks = this.#callbacks;
      if (this.#stop() && this.context !== null) {
        callbacks?.onTimeout(this.stats, this.context, null);
      }
    }, ms);
  }

  async #run(
    context: LoaderContext,
    stopping: AbortController,
    maxLoadTimeMs: number,
  ): Promise<void> {
    const headers = new Headers(
      this.#setup.headersOf().map(([name, value]): [string, string] => [name, value]),
    );
    if (context.rangeEnd !== undefined && context.rangeEnd > 0) {
      headers.set(
        "Range",
        `bytes=${String(context.rangeStart ?? 0)}-${String(context.rangeEnd - 1)}`,
      );
    }
    const progress = new EventTarget();
    this.#setup.follow(progress, context.url);
    try {
      const answer = await this.#send(
        new Request(context.url, { headers, signal: stopping.signal }),
      );
      const { stats } = this;
      stats.loading.first = performance.now();
      this.#timeOutAfter(maxLoadTimeMs - (stats.loading.first - stats.loading.start));
      this.#setup.answered(answer.status, (name) => answer.headers.get(name));
      if (!answer.ok) {
        this.#ended(stopping, (callbacks) => {
          callbacks.onError(
            { code: answer.status, text: answer.statusText },
            context,
            answer,
            stats,
          );
        });
        return;
      }
      const body = await bodyOf(answer, (loaded) => {
        stats.loaded = loaded;
        // An Event saying its loaded, as first-piece reads it: Node, where tests run, has no ProgressEvent.
        progress.dispatchEvent(Object.assign(new Event("progress"), { loaded }));
      });
      stats.loading.end = performance.now();
      stats.total = body.byteLength;
      const data =
        context.responseType === "arraybuffer"
          ? body.buffer.slice(body.byteOffset, body.byteOffset + body.byteLength)
          : new TextDecoder().decode(body);
      this.#ended(stopping, (callbacks) => {
        callbacks.onSuccess(
          { url: context.url, data, code: answer.status },
          stats,
          context,
          answer,
        );
      });
    } catch (error) {
      this.#ended(stopping, (callbacks) => {
        callbacks.onError({ code: 0, text: String(error) }, context, null, this.stats);
      });
    }
  }

  /** Tells the load's end to its callbacks, unless it was stopped (aborted or timed out) first. */
  #ended(
    stopping: AbortController,
    tell: (callbacks: LoaderCallbacks<LoaderContext>) => void,
  ): void {
    if (this.#stopping !== stopping) {
      return;
    }
    clearTimeout(this.#timer);
    this.#stopping = undefined;
    const callbacks = this.#callbacks;
    if (callbacks !== undefined) {
      tell(callbacks);
    }
  }
}
