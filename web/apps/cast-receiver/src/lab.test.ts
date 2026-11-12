// SPDX-License-Identifier: MPL-2.0
// @vitest-environment happy-dom

import { afterEach, describe, expect, it, vi } from "vitest";

import { LabReceiver, labRequestOf, memoryOf } from "./lab.ts";
import { reachOf } from "./reach.ts";

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

  it("ask it to become a cast target, reaching the sender's core by its reach where it sends one", () => {
    const reach = {
      clientApi: "https://api.discovery.example",
      coreId: "k7xq2",
      fingerprint: "SHA256:abc",
      token: "session-token",
      locator: "loc-1",
    };
    expect(labRequestOf({ type: "device", reach })).toEqual({ type: "device", reach });
    expect(labRequestOf({ type: "device" })).toEqual({ type: "device", reach: undefined });
  });

  it("never play an address on another host, nor anything unknown", () => {
    expect(
      labRequestOf({ type: "play", path: "https://elsewhere.example/master.m3u8" }),
    ).toBeUndefined();
    expect(labRequestOf({ type: "eject" })).toBeUndefined();
    expect(labRequestOf("report")).toBeUndefined();
  });
});

describe("how the lab's receiver reaches its sender's core (the 11-23 batch's item 3(d))", () => {
  const reach = {
    clientApi: "https://api.discovery.example",
    coreId: "k7xq2",
    fingerprint: "SHA256:abc",
    token: "session-token",
    locator: "loc-1",
  };

  it("takes the sender's client origin on https alone, and goes over WebRTC where it names none", () => {
    expect(reachOf({ ...reach, clientOrigin: "https://k7xq2.client.example" })).toMatchObject({
      clientOrigin: "https://k7xq2.client.example",
    });
    expect(reachOf({ ...reach, clientOrigin: "http://k7xq2.client.example" })).toMatchObject({
      clientOrigin: undefined,
    });
    expect(reachOf(reach)).toMatchObject({ clientOrigin: undefined });
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

describe("the receiver's log on screen (the owner, 2026-11-12)", () => {
  const reach = {
    clientApi: "https://discovery.example/client/v1",
    coreId: "k7xq2",
    fingerprint: "Vkdap1RjR0wChd9dvyvKtz2mUTWIOem3dIGy6rEHcIw",
    clientOrigin: "https://client.example",
    locator: "the-senders-locator",
  };

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  /** A receiver on a screen of its own, and what it answers a cast under a grant. */
  function receiverOnScreen(): {
    readonly lab: LabReceiver;
    readonly screen: HTMLElement;
    readonly answered: unknown[];
  } {
    // A test reaches no core: the cast under a grant fails, answered as such.
    vi.stubGlobal("fetch", () => Promise.reject(new Error("no network in a test")));
    const screen = document.createElement("pre");
    const lab = new LabReceiver(screen, document.createElement("video"), () => undefined);
    return { lab, screen, answered: [] };
  }

  it("is hidden until a sender asks for it, its notes kept for the report all the while", async () => {
    const { lab, screen, answered } = receiverOnScreen();
    lab.note("CAF started");
    expect(screen.hidden).toBe(true);
    expect(screen.textContent).toContain("CAF started");
    lab.heardGuestLaunch({ type: "guest", grant: "g", reach }, (answer) => answered.push(answer));
    expect(screen.hidden).toBe(true); // an older sender's launch says nothing of the log
    await vi.waitFor(() => {
      expect(answered).toHaveLength(1);
    });
    expect(screen.hidden).toBe(true);
  });

  it("shows once a cast under a grant says to, its sender's person showing debug tools", async () => {
    const { lab, screen, answered } = receiverOnScreen();
    lab.heardGuestLaunch({ type: "guest", grant: "g", reach, showsLog: true }, (answer) =>
      answered.push(answer),
    );
    expect(screen.hidden).toBe(false);
    await vi.waitFor(() => {
      expect(answered).toHaveLength(1);
    });
  });

  it("shows at any request of the lab's sender, a debug tool itself", () => {
    const asked = receiverOnScreen();
    asked.lab.heard({ type: "report" });
    expect(asked.screen.hidden).toBe(false);
    const toggled = receiverOnScreen();
    toggled.lab.heard({ type: "toggle" });
    expect(toggled.screen.hidden).toBe(false);
    const unknown = receiverOnScreen();
    unknown.lab.heard({ type: "something else" });
    expect(unknown.screen.hidden).toBe(true);
  });
});
