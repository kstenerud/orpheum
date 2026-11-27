// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { RtcTransport } from "@orpheum/sdk";
import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { JoinForm } from "./together-join.tsx";
import { buttonWithText, mount, type Mounted } from "./test-dom.tsx";
import { testServices, withServices } from "./test-services.tsx";

/** A data channel on a connection stalled while it still says it is connected: nothing leaves it. */
class StalledChannel {
  readonly readyState: RTCDataChannelState = "open";
  isClosed = false;

  send(): void {
    // Kept in the browser's buffer, never sent.
  }

  close(): void {
    this.isClosed = true;
  }

  listen(): void {
    // The core never hears the request, so never answers it.
  }

  readonly whenSent = (): void => {
    // Its buffered amount never drains.
  };
}

/** A connection stalled while it still says it is connected. */
class StalledPeer {
  isClosed = false;
  readonly channels: StalledChannel[] = [];

  createDataChannel(): StalledChannel {
    const channel = new StalledChannel();
    this.channels.push(channel);
    return channel;
  }

  close(): void {
    this.isClosed = true;
  }

  onGone(): void {
    // It never says it is gone: it says it is connected.
  }
}

afterEach(() => {
  vi.useRealTimers();
});

// A guest's page stayed at "Checking your invitation…" for good: its check of the invitation, a
// POST and the page's only request, was on a connection stalled while it still said it was
// connected, its body never left, and nothing ever gave the request up (staging, 2026-11-27; D565).
describe("a guest's check of their invitation on a stalled connection", () => {
  it("is given up, its connection replaced, and the page shows the form to join by", async () => {
    vi.useFakeTimers();
    const stalled = new StalledPeer();
    const made: StalledPeer[] = [];
    const transport = new RtcTransport([stalled], {
      size: 4,
      open: () => {
        const peer = new StalledPeer();
        made.push(peer);
        return Promise.resolve(peer);
      },
    });
    const services = testServices(undefined, "/together/join", (request) =>
      transport.fetch(request),
    );
    let mounted: Mounted | undefined;
    void mount(
      withServices(
        <JoinForm secret="new-secret" onSubmitted={() => undefined} onJoined={() => undefined} />,
        services,
      ),
    ).then((done) => (mounted = done));
    while (mounted === undefined) {
      // The mount's settling ticks on the faked clock, which the test moves.
      await vi.advanceTimersByTimeAsync(5);
    }
    expect(mounted.container.textContent).toContain("Checking your invitation");
    await act(async () => {
      await vi.advanceTimersByTimeAsync(60_000);
    });
    expect(stalled.isClosed).toBe(true);
    expect(made.length).toBeGreaterThan(0);
    expect(mounted.container.textContent).not.toContain("Checking your invitation");
    expect(mounted.container.querySelector("form input")).not.toBeNull();
    expect(buttonWithText(mounted.container, "Join")).toBeDefined();
    mounted.unmount();
    transport.close();
  });
});
