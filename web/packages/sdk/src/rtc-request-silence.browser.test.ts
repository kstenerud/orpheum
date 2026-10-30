// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import { silentAtMostMs, type ChannelLike, type ChannelListener } from "./rtc-request.ts";
import { RtcTransport, type PeerLike } from "./rtc-transport.ts";

/**
 * A data channel the test plays the core on, which opens at once: one isAnsweredWhole answers each request
 * whole, as the core does; otherwise it sends only the response's head, then nothing.
 */
class CoreChannel implements ChannelLike {
  readyState: RTCDataChannelState = "connecting";
  isClosed = false;
  private listener: ChannelListener | undefined;

  constructor(private readonly isAnsweredWhole: boolean) {}

  send(data: string | ArrayBuffer): void {
    if (typeof data === "string" || data.byteLength > 0) {
      return;
    }
    // The request ended: its answer's head, and, from a core that answers whole, its body and end.
    this.listener?.message(JSON.stringify({ status: 200 }));
    if (this.isAnsweredWhole) {
      this.listener?.message(new TextEncoder().encode('{"mediaToken":"t2"}').buffer);
      this.listener?.message(new ArrayBuffer(0));
    }
  }

  close(): void {
    this.isClosed = true;
  }

  listen(listener: ChannelListener): void {
    this.listener = listener;
    queueMicrotask(() => {
      this.readyState = "open";
      listener.open();
    });
  }
}

/** A connection whose channels answer as isAnsweredWhole says. */
class CorePeer implements PeerLike {
  isClosed = false;

  constructor(private readonly isAnsweredWhole: boolean) {}

  createDataChannel(): ChannelLike {
    return new CoreChannel(this.isAnsweredWhole);
  }

  close(): void {
    this.isClosed = true;
  }
}

/** The Play page's playback, a POST. */
function playbackRequest(): Request {
  return new Request("https://app.example/api/v1/playback", {
    method: "POST",
    body: JSON.stringify({ fileId: 5671, cause: "playPage" }),
  });
}

/** The browser's own setTimeout, kept before the test fakes it. */
const realSetTimeout = globalThis.setTimeout;

/**
 * Moves the faked clock on by ms a second at a time, the browser given 10 real ms between each for
 * its own tasks: Firefox begins reading a response's body in tasks of its own, which a faked clock
 * neither runs nor waits for, and a clock moved on at once had passed before its read began.
 */
async function elapse(ms: number): Promise<void> {
  for (let left = ms; left > 0; left -= 1000) {
    await new Promise((resolve) => realSetTimeout(resolve, 10));
    await vi.advanceTimersByTimeAsync(Math.min(left, 1000));
  }
  await new Promise((resolve) => realSetTimeout(resolve, 10));
}

afterEach(() => {
  vi.useRealTimers();
});

// The owner's Mac Chrome, back on the Play page from the Watch Together page, had its playback's
// head and nothing after it: the browser read the failed body as "TypeError: Failed to fetch", which
// is not asked again, and the page said "The server could not be reached" (staging, 2026-10-30).
describe("a response whose head came and whose body never did, in a real browser", () => {
  it("fails its reading with a SilentRequestError, its connection replaced, and asked again it is answered on the new one", async () => {
    vi.useFakeTimers();
    const silent = new CorePeer(false);
    const transport = new RtcTransport(
      [silent],
      { open: () => Promise.resolve(new CorePeer(true)), size: 4 },
      () => Date.now(),
    );
    const read = async (): Promise<unknown> =>
      await (await transport.fetch(playbackRequest())).json();
    const first = read().then(
      () => "answered",
      (problem: unknown) => (problem instanceof Error ? problem.name : "failed"),
    );
    await elapse(silentAtMostMs + 1000);
    expect(await first).toBe("SilentRequestError");
    expect(silent.isClosed).toBe(true);
    const again = read();
    await elapse(1000);
    expect(await again).toEqual({ mediaToken: "t2" });
  });
});
