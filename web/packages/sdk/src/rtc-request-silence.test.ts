// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  sendOnChannel,
  silentAtMostMs,
  type ChannelLike,
  type ChannelListener,
} from "./rtc-request.ts";

/** A data channel that is open, keeps what is sent, and says when it has all left the browser. */
class SendingChannel implements ChannelLike {
  readyState: RTCDataChannelState = "open";
  isClosed = false;
  private listener: ChannelListener | undefined;
  private readonly sentWaiters: (() => void)[] = [];

  send(): void {
    // kept by no one: what was sent is not this test's
  }

  close(): void {
    this.isClosed = true;
  }

  listen(listener: ChannelListener): void {
    this.listener = listener;
  }

  readonly whenSent = (sent: () => void): void => {
    this.sentWaiters.push(sent);
  };

  /** Everything sent has left the browser: its buffered amount drained. */
  drain(): void {
    for (const sent of this.sentWaiters.splice(0)) {
      sent();
    }
  }

  answer(data: string | ArrayBuffer): void {
    this.listener?.message(data);
  }

  /** Opens a channel made not yet open. */
  open(): void {
    this.readyState = "open";
    this.listener?.open();
  }
}

/** A channel the browser has not opened yet, as on a connection that has stopped. */
function unopenedChannel(): SendingChannel {
  const channel = new SendingChannel();
  channel.readyState = "connecting";
  return channel;
}

/** A POST with a body, as the Play page asks for its playback. */
function playbackRequest(): Request {
  return new Request("https://app.example/api/v1/playback", {
    method: "POST",
    body: JSON.stringify({ fileId: 5663, cause: "playPage" }),
  });
}

afterEach(() => {
  vi.useRealTimers();
});

// The Play page's playback is a POST: unbounded, it heard nothing over a channel that had stopped,
// and the page said "Loading…" for good, its host's Watch Together player never drawn (the owner's
// Chrome back from another site, staging, 2026-10-29).
describe("a request with a body over WebRTC", () => {
  it("waits for its head unbounded while its body is on its way, and fails once it has all left and nothing came", async () => {
    vi.useFakeTimers();
    const channel = new SendingChannel();
    let isToldSilent = false;
    const outcome = sendOnChannel(channel, playbackRequest(), () => {
      isToldSilent = true;
    }).then(
      () => "answered",
      (problem: unknown) => (problem instanceof Error ? problem.name : "failed"),
    );
    await vi.advanceTimersByTimeAsync(silentAtMostMs * 2); // a slow upload, still leaving
    expect(channel.isClosed).toBe(false);
    channel.drain();
    await vi.advanceTimersByTimeAsync(silentAtMostMs - 1);
    expect(channel.isClosed).toBe(false);
    await vi.advanceTimersByTimeAsync(1);
    expect(await outcome).toBe("SilentRequestError");
    expect(channel.isClosed).toBe(true);
    expect(isToldSilent).toBe(true); // its transport takes its connection for gone if nothing else came
  });

  it("is answered as ever within the bound once its body has left", async () => {
    vi.useFakeTimers();
    const channel = new SendingChannel();
    const answered = sendOnChannel(channel, playbackRequest());
    await vi.advanceTimersByTimeAsync(0);
    channel.drain();
    await vi.advanceTimersByTimeAsync(silentAtMostMs - 1);
    channel.answer(JSON.stringify({ status: 204 }));
    channel.answer(new ArrayBuffer(0));
    expect((await answered).status).toBe(204);
  });

  // The owner's phone, cast to the laptop, tapped Stop casting, a POST, and nothing happened at all:
  // its channel, on a connection that had stopped, never opened, so its body never left and its
  // wait was never bounded; the core never heard of it (prod, 2026-11-03).
  it("fails as a silence where its channel does not open within the bound, and is sent as ever where it opens", async () => {
    vi.useFakeTimers();
    const late = unopenedChannel();
    const answered = sendOnChannel(late, playbackRequest());
    await vi.advanceTimersByTimeAsync(silentAtMostMs - 1);
    late.open();
    late.drain();
    late.answer(JSON.stringify({ status: 204 }));
    late.answer(new ArrayBuffer(0));
    expect((await answered).status).toBe(204);
    await vi.advanceTimersByTimeAsync(silentAtMostMs);
    expect(late.isClosed).toBe(true); // closed by its end, not failed by the watch

    const never = unopenedChannel();
    let isToldSilent = false;
    const outcome = sendOnChannel(never, playbackRequest(), () => {
      isToldSilent = true;
    }).then(
      () => "answered",
      (problem: unknown) => (problem instanceof Error ? problem.name : "failed"),
    );
    await vi.advanceTimersByTimeAsync(silentAtMostMs - 1);
    expect(never.isClosed).toBe(false);
    await vi.advanceTimersByTimeAsync(1);
    expect(await outcome).toBe("SilentRequestError");
    expect(never.isClosed).toBe(true);
    expect(isToldSilent).toBe(true); // its transport takes its connection for gone
  });
});
