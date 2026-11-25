// SPDX-License-Identifier: MPL-2.0

import type { Target } from "@orpheum/sdk";
import { afterEach, describe, expect, it, vi } from "vitest";

import { coreReachOf, reachThrough, type CoreReach } from "./reach.ts";

/** A sender's reach whose page is the web client's, not the loader's the receiver is served at. */
const reach: CoreReach = {
  clientApi: "https://api.discovery.example",
  coreId: "k7xq2",
  fingerprint: "SHA256:abc",
  locator: "a-locator",
  clientOrigin: "https://app.example",
};

/** The core the discovery names, at one name of its own. */
const target: Target = {
  coreId: "k7xq2",
  fingerprint: "SHA256:abc",
  held: { bootstrapToken: "a-bootstrap-token" },
  key: new Uint8Array(32),
  iceServers: [],
  directUrls: ["https://core.example:8443"],
  lanUrls: [],
};

describe("the receiver reaching its sender's core (D467)", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("goes to the core's own name, served at the loader's origin, where the core lets its page call the API", async () => {
    vi.stubGlobal("location", { origin: "https://loader.example" });
    const asked: string[] = [];
    vi.stubGlobal("fetch", (input: RequestInfo | URL) => {
      const url = input instanceof Request ? input.url : input.toString();
      asked.push(url);
      return Promise.resolve(new Response(null, { status: 200 }));
    });
    const notes: string[] = [];
    const reached = await reachThrough(
      reach,
      () => Promise.resolve(target),
      (line) => notes.push(line),
    );
    expect(notes).toContain("reached the core at https://core.example:8443");
    await reached.fetch(new Request("https://loader.example/api/v1/library?x=1"));
    expect(asked).toEqual([
      "https://core.example:8443/api/v1/health",
      "https://core.example:8443/api/v1/me",
      "https://core.example:8443/api/v1/library?x=1",
    ]);
  });
});

describe("how the receiver reaches its sender's core (the 11-23 batch's item 3(d))", () => {
  const handed = {
    clientApi: "https://api.discovery.example",
    coreId: "k7xq2",
    fingerprint: "SHA256:abc",
    locator: "loc-1",
  };

  it("takes the sender's client origin on https alone, and goes over WebRTC where it names none", () => {
    expect(coreReachOf({ ...handed, clientOrigin: "https://k7xq2.client.example" })).toMatchObject({
      clientOrigin: "https://k7xq2.client.example",
    });
    expect(coreReachOf({ ...handed, clientOrigin: "http://k7xq2.client.example" })).toMatchObject({
      clientOrigin: undefined,
    });
    expect(coreReachOf(handed)).toMatchObject({ clientOrigin: undefined });
  });

  it("carries the way to the sender's core from away, its discovery on https alone (the 11-05 batch's item 10)", () => {
    expect(coreReachOf(handed)).toEqual({ ...handed, clientOrigin: undefined });
    expect(coreReachOf({ ...handed, clientApi: "http://api.discovery.example" })).toBeUndefined();
    expect(coreReachOf({ ...handed, locator: "" })).toBeUndefined();
  });
});
