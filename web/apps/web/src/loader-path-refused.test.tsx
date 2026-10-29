// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { KeyRefused, proofRefusal, RtcTransport } from "@orpheum/sdk";
import type { JSX } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { StringStorage } from "./browser-storage.ts";
import type { Reached } from "./discovery-start.ts";
import { keepPath } from "./kept-path.ts";
import { showWhenPathRefused, type AtClientOrigin } from "./loader-at-client-origin.tsx";
import { corePrefix, createSession } from "./session.ts";
import { buttonWithText, click, mount, type Mounted } from "./test-dom.tsx";

// The owner's Firefox, a Watch Together guest through staging's discovery: once the Wi-Fi came back,
// every offer its page made was refused as unproven, every 10 s for good, and its Play page spun,
// saying nothing (2026-10-29).

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  vi.useRealTimers();
  vi.restoreAllMocks();
});

function memoryStorage(): StringStorage {
  const items = new Map<string, string>();
  return {
    getItem: (key) => items.get(key) ?? null,
    setItem: (key, value) => {
      items.set(key, value);
    },
    removeItem: (key) => {
      items.delete(key);
    },
  };
}

/** A peer connection that opens no channel, so the first request the transport sends gives it up. */
function peerOpeningNoChannel(): { createDataChannel: () => never; close: () => void } {
  return {
    createDataChannel: (): never => {
      throw new Error("no channel is opened in this test");
    },
    close: () => undefined,
  };
}

describe("a client origin whose server refused its way back for good", () => {
  it("tells its person over the client, who may reload or sign in again", async () => {
    vi.useFakeTimers();
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    let tries = 0;
    const transport = new RtcTransport([peerOpeningNoChannel()], {
      size: 4,
      // What the device held and what it holds now both refused (the SDK's signaller).
      open: () => {
        tries += 1;
        return Promise.reject(new KeyRefused(`the core refused the offer: ${proofRefusal}`));
      },
    });
    const webrtc: Reached = { kind: "webrtc", transport };
    const realFetch = globalThis.fetch;
    globalThis.fetch = (): Promise<Response> => Promise.resolve(new Response("{}"));
    const kept = keepPath(
      {
        first: webrtc,
        another: () => Promise.resolve(webrtc),
        better: () => Promise.resolve(undefined),
        onLocalNetworkGranted: () => () => undefined,
        onLocalNetworkAsking: () => () => undefined,
      },
      { token: () => undefined },
    );
    kept.carried.catch(() => undefined);
    const storage = memoryStorage();
    createSession(storage, corePrefix("k7xq2")).signIn("a-refused-token", {
      locator: "a-refused-locator",
    });
    const shown: JSX.Element[] = [];
    const at: Pick<AtClientOrigin, "storage" | "coreId" | "loaderOrigin" | "showFailure"> = {
      storage,
      coreId: "k7xq2",
      loaderOrigin: "https://app.example",
      showFailure: (page) => {
        shown.push(page);
      },
    };
    let reloads = 0;
    const core = { coreId: "k7xq2", fingerprint: "a-fingerprint", realm: undefined };
    showWhenPathRefused(kept, { at, core }, () => {
      reloads += 1;
    });
    // The connection goes; its replacement is refused.
    await transport.fetch(new Request("https://core.example/api/v1/me")).catch(() => undefined);
    await vi.advanceTimersByTimeAsync(60_000);
    expect(tries).toBe(1);
    expect(shown).toHaveLength(1);
    vi.useRealTimers();
    mounted = await mount(shown[0]);
    expect(mounted.container.querySelector("[role=alert]")?.textContent).toContain(
      "no longer accepts",
    );
    await click(buttonWithText(mounted.container, "Reload"));
    expect(reloads).toBe(1);
    // Signing in again forgets what was refused; a household reached by a link alone reloads.
    await click(buttonWithText(mounted.container, "Sign in again"));
    expect(createSession(storage, corePrefix("k7xq2")).token()).toBeUndefined();
    expect(reloads).toBe(2);
    kept.close();
    globalThis.fetch = realFetch;
  });
});
