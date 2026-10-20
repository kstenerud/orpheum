// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it, vi } from "vitest";

import { reachCore, type Reached, type Reaching, type Target } from "./discovery-start.ts";

const fingerprint = "Vkdap1RjR0wChd9dvyvKtz2mUTWIOem3dIGy6rEHcIw";

describe("the paths a core is reached by after the first (webrtc-transport.md W2c item 6)", () => {
  const lan = "https://192-168-1-2.k7xq2.zone.example:8443";
  const ipv6 = "https://2a00-1.k7xq2.zone.example";
  const target: Target = {
    coreId: "k7xq2",
    fingerprint,
    held: { bootstrapToken: "a-bootstrap-token" },
    key: new Uint8Array(32),
    iceServers: [],
    directUrls: [lan, ipv6],
    lanUrls: [lan],
  };

  it("takes another name when the one in use fails, and the LAN again once it answers", async () => {
    let isLanBlocked = true;
    const fetched: typeof fetch = (url) => {
      const address = url instanceof Request ? url.url : url.toString();
      return address.startsWith(lan) && isLanBlocked
        ? Promise.reject(new TypeError("NetworkError when attempting to fetch resource."))
        : Promise.resolve(new Response("{}", { status: 200 }));
    };
    const reaching = await reachCore(
      { clientApi: "https://discovery.example/client/v1", fetch: fetched },
      target,
    );
    expect(reaching.first).toEqual({ kind: "direct", origin: ipv6 });
    expect(await reaching.better(reaching.first)).toBeUndefined();
    isLanBlocked = false;
    const better = await reaching.better(reaching.first);
    expect(better).toEqual({ kind: "direct", origin: lan });
    expect(await reaching.better({ kind: "direct", origin: lan })).toBeUndefined();
    expect(await reaching.another({ kind: "direct", origin: lan })).toEqual({
      kind: "direct",
      origin: ipv6,
    });
  });

  it("never tries a LAN name the browser was told to refuse, and leaves it out of the look while it asks (W2c item 1)", async () => {
    const tried: string[] = [];
    const fetched: typeof fetch = (url) => {
      tried.push(new URL(url instanceof Request ? url.url : url.toString()).origin);
      return Promise.resolve(new Response("{}", { status: 200 }));
    };
    const client = { clientApi: "https://discovery.example/client/v1", fetch: fetched };
    const asking = (state: "denied" | "prompt") => ({
      state: () => Promise.resolve(state),
      onGranted: () => () => undefined,
      onAsking: (listener: (isAsking: boolean) => void) => {
        listener(state === "prompt");
        return () => undefined;
      },
    });
    const isToldAsking = (reaching: Reaching): boolean => {
      let isAsking = false;
      reaching.onLocalNetworkAsking((isToldAsking) => {
        isAsking = isToldAsking;
      });
      return isAsking;
    };
    const refused = await reachCore(client, target, asking("denied"));
    expect(refused.first).toEqual({ kind: "direct", origin: ipv6 });
    expect(tried).not.toContain(lan);
    tried.length = 0;
    expect(await refused.better(refused.first)).toBeUndefined();
    expect(tried).not.toContain(lan);
    // A LAN name never tried has the browser ask nothing, so the page says nothing of it (W2c item 8).
    expect(isToldAsking(refused)).toBe(false);
    // Still to ask: the first race tries it, which is what has the browser ask; the look does not.
    tried.length = 0;
    const waiting = await reachCore(client, target, asking("prompt"));
    expect(tried).toContain(lan);
    tried.length = 0;
    expect(await waiting.better({ kind: "direct", origin: ipv6 })).toBeUndefined();
    expect(tried).not.toContain(lan);
    expect(isToldAsking(waiting)).toBe(true);
    const noLan = await reachCore(
      client,
      {
        ...target,
        lanUrls: [],
        directUrls: target.directUrls.filter((url) => !url.startsWith(lan)),
      },
      asking("prompt"),
    );
    expect(isToldAsking(noLan)).toBe(false);
  });

  it("tries WebRTC first when the LAN name fails, the public names only after (D300 §4)", async () => {
    const tried: string[] = [];
    const fetched: typeof fetch = (url) => {
      const address = url instanceof Request ? url.url : url.toString();
      tried.push(new URL(address).origin);
      return Promise.resolve(new Response("{}", { status: 200 }));
    };
    const reaching = await reachCore(
      { clientApi: "https://discovery.example/client/v1", fetch: fetched },
      target,
    );
    expect(reaching.first).toEqual({ kind: "direct", origin: lan });
    tried.length = 0;
    // No browser WebRTC here: a connection attempt is heard, and fails as one through no path would.
    vi.stubGlobal("RTCPeerConnection", function noWebRTC(): never {
      tried.push("webrtc");
      throw new Error("no WebRTC in this test");
    });
    try {
      expect(await reaching.another({ kind: "direct", origin: lan })).toEqual({
        kind: "direct",
        origin: ipv6,
      });
    } finally {
      vi.unstubAllGlobals();
    }
    expect(tried[0]).toBe("webrtc");
    expect(tried).toContain(ipv6);
    tried.length = 0;
    expect(await reaching.another({ kind: "direct", origin: ipv6 })).toEqual({
      kind: "direct",
      origin: lan,
    });
    expect(tried).not.toContain("webrtc");
  });

  it("at home, never takes a public name for better than WebRTC, only the LAN name (D300 §4)", async () => {
    let isLanBlocked = false;
    const fetched: typeof fetch = (url) => {
      const address = url instanceof Request ? url.url : url.toString();
      return address.startsWith(lan) && isLanBlocked
        ? Promise.reject(new TypeError("NetworkError when attempting to fetch resource."))
        : Promise.resolve(new Response("{}", { status: 200 }));
    };
    const reaching = await reachCore(
      { clientApi: "https://discovery.example/client/v1", fetch: fetched },
      target,
    );
    expect(reaching.first).toEqual({ kind: "direct", origin: lan });
    // The page went to WebRTC once its LAN name failed: Firefox's prompt, or its service worker.
    isLanBlocked = true;
    const overWebrtc = { kind: "webrtc", transport: {} } as unknown as Reached;
    expect(await reaching.better(overWebrtc)).toBeUndefined();
    isLanBlocked = false;
    expect(await reaching.better(overWebrtc)).toEqual({ kind: "direct", origin: lan });
  });
});
