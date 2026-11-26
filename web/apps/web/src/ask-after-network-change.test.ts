// SPDX-License-Identifier: MPL-2.0

import { RtcTransport } from "@orpheum/sdk";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { askWhileUnanswered } from "./reopen.ts";

// The owner's phone, its network changed with the Cast panel open: all four of the page's WebRTC
// connections went disconnected together, each held for 5 s more, and the panel's read, asked three
// times, failed on each at once as its channel closed, saying "The server could not be reached"
// before any new connection was up (production, 2026-11-26).

/** What a data channel tells, as the transport listens. */
interface Heard {
  readonly open: () => void;
  readonly message: (data: string | ArrayBuffer) => void;
  readonly close: () => void;
  readonly error: () => void;
}

/** A data channel the test plays the core on: a live connection's answers, a dying one's closes. */
class FakeChannel {
  readonly readyState: RTCDataChannelState = "open";
  private heard: Heard | undefined;

  send(): void {
    // The request's head and end: the test answers whatever was asked.
  }

  close(): void {
    // The client's close of a channel it is done with.
  }

  listen(heard: Heard): void {
    this.heard = heard;
  }

  answer(body: unknown): void {
    this.heard?.message(JSON.stringify({ status: 200 }));
    this.heard?.message(new TextEncoder().encode(JSON.stringify(body)).buffer);
    this.heard?.message(new ArrayBuffer(0));
  }

  closeFromTheCore(): void {
    this.heard?.close();
  }
}

/**
 * A connection whose states the test moves: a disconnected one closes each channel opened on it, and
 * a live one answers it unless it is slow, the core's answer not yet come when the network changes.
 */
class FakePeer {
  isDisconnected = false;
  readonly channels: FakeChannel[] = [];
  private goneListener: ((why: string) => void) | undefined;

  constructor(private readonly isSlow = false) {}

  createDataChannel(): FakeChannel {
    const channel = new FakeChannel();
    this.channels.push(channel);
    setTimeout(() => {
      if (this.isDisconnected) {
        channel.closeFromTheCore();
      } else if (!this.isSlow) {
        channel.answer({ devices: [] });
      }
    }, 0);
    return channel;
  }

  close(): void {
    // Closed by the transport once gone.
  }

  canCarry(): boolean {
    return !this.isDisconnected;
  }

  onJudged(): void {
    // The test moves no connection back, so no judging is told.
  }

  onGone(listener: (why: string) => void): void {
    this.goneListener = listener;
  }

  /** Its network went: disconnected, and each channel open on it closed. */
  disconnects(): void {
    this.isDisconnected = true;
    for (const channel of this.channels) {
      channel.closeFromTheCore();
    }
  }

  /** It stayed disconnected past the grace, as watchGone says. */
  goes(): void {
    this.goneListener?.("it stayed disconnected for 5000 ms");
  }
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("a read asked again after the phone's network changed", () => {
  it("is answered over the connection made after the loss, not failed on the four dying together", async () => {
    const held = [new FakePeer(true), new FakePeer(true), new FakePeer(true), new FakePeer(true)];
    const made: FakePeer[] = [];
    const transport = new RtcTransport(held, {
      size: 4,
      open: async () => {
        // A new connection on the new network, a few seconds in the making.
        await new Promise((resolve) => setTimeout(resolve, 3000));
        const peer = new FakePeer();
        made.push(peer);
        return peer;
      },
    });
    await vi.advanceTimersByTimeAsync(11_000); // the connections have settled
    const failures: unknown[] = [];
    const read = askWhileUnanswered(
      async () => {
        const response = await transport.fetch(
          new Request("https://app.example/api/v1/me/devices"),
        );
        return (await response.json()) as unknown;
      },
      (cause) => failures.push(cause),
    );
    const settled = read.then(
      (answer) => ({ answer }),
      (cause: unknown) => ({ cause }),
    );
    await vi.advanceTimersByTimeAsync(500); // asked, its answer not yet come
    for (const peer of held) {
      peer.disconnects(); // all together, within a second
    }
    await vi.advanceTimersByTimeAsync(5000);
    for (const peer of held) {
      peer.goes(); // the grace over
    }
    await vi.advanceTimersByTimeAsync(5000);
    expect(await settled).toEqual({ answer: { devices: [] } });
    expect(made.length).toBeGreaterThan(0);
    expect(made[0]?.channels).toHaveLength(1);
    expect(failures).toHaveLength(1); // the first ask's, its channel closed under it
    transport.close();
  });
});
