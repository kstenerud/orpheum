// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { labRequestOf, memoryOf } from "./lab.ts";

describe("the lab receiver's requests", () => {
  it("are a report asked for, a pause or play, or a film to play by its path on this core, signed in or not", () => {
    expect(labRequestOf({ type: "report" })).toEqual({ type: "report" });
    expect(labRequestOf({ type: "toggle" })).toEqual({ type: "toggle" });
    expect(
      labRequestOf({
        type: "play",
        path: "/api/v1/bytes/media/chunks/t/master.m3u8",
        startAtBps: 2_400_000,
        authorization: "Bearer x",
      }),
    ).toEqual({
      type: "play",
      path: "/api/v1/bytes/media/chunks/t/master.m3u8",
      startAtBps: 2_400_000,
      authorization: "Bearer x",
      reach: undefined,
    });
    expect(labRequestOf({ type: "play", path: "/a/master.m3u8" })).toEqual({
      type: "play",
      path: "/a/master.m3u8",
      startAtBps: 0,
      authorization: undefined,
      reach: undefined,
    });
  });

  it("carry the way to the sender's core from away, its discovery on https alone (the 11-05 batch's item 10)", () => {
    const reach = {
      clientApi: "https://api.discovery.example",
      coreId: "k7xq2",
      fingerprint: "SHA256:abc",
      token: "session-token",
      locator: "loc-1",
    };
    expect(labRequestOf({ type: "play", path: "/a/master.m3u8", reach })).toMatchObject({
      reach,
    });
    expect(
      labRequestOf({
        type: "play",
        path: "/a/master.m3u8",
        reach: { ...reach, clientApi: "http://api.discovery.example" },
      }),
    ).toMatchObject({ reach: undefined });
    expect(
      labRequestOf({ type: "play", path: "/a/master.m3u8", reach: { ...reach, locator: "" } }),
    ).toMatchObject({ reach: undefined });
  });

  it("never play an address on another host, nor anything unknown", () => {
    expect(
      labRequestOf({ type: "play", path: "https://elsewhere.example/master.m3u8" }),
    ).toBeUndefined();
    expect(labRequestOf({ type: "eject" })).toBeUndefined();
    expect(labRequestOf("report")).toBeUndefined();
  });
});

describe("the lab receiver's memory", () => {
  it("is Chromium's heap in MiB, or nothing where the browser says nothing", () => {
    expect(
      memoryOf(
        { usedJSHeapSize: 52_428_800, totalJSHeapSize: 62_914_560, jsHeapSizeLimit: 536_870_912 },
        60,
      ),
    ).toEqual({ atS: 60, usedMb: 50, totalMb: 60, limitMb: 512 });
    expect(memoryOf(undefined, 60)).toBeUndefined();
    expect(memoryOf({ usedJSHeapSize: 1 }, 60)).toBeUndefined();
  });
});
