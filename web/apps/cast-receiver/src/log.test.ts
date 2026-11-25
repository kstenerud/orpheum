// SPDX-License-Identifier: MPL-2.0
// @vitest-environment happy-dom

import { afterEach, describe, expect, it, vi } from "vitest";

import { memoryOf, ReceiverLog } from "./log.ts";
import { RegularCasts } from "./regular-cast.ts";

describe("the receiver's memory", () => {
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
    readonly log: ReceiverLog;
    readonly casts: RegularCasts;
    readonly screen: HTMLElement;
    readonly answered: unknown[];
  } {
    // A test reaches no core: the cast under a grant fails, answered as such.
    vi.stubGlobal("fetch", () => Promise.reject(new Error("no network in a test")));
    const screen = document.createElement("pre");
    const video = document.createElement("video");
    const log = new ReceiverLog(screen, video);
    return { log, casts: new RegularCasts(video, log), screen, answered: [] };
  }

  it("is hidden until a sender asks for it, its notes kept all the while", async () => {
    const { log, casts, screen, answered } = receiverOnScreen();
    log.note("CAF started");
    expect(screen.hidden).toBe(true);
    expect(screen.textContent).toContain("CAF started");
    casts.heard(
      { type: "guest", grant: "g", reach },
      (answer) => answered.push(answer),
      () => undefined,
    );
    expect(screen.hidden).toBe(true); // an older sender's launch says nothing of the log
    await vi.waitFor(() => {
      expect(answered).toHaveLength(1);
    });
    expect(screen.hidden).toBe(true);
  });

  it("shows once a cast under a grant says to, its sender's person showing debug tools", async () => {
    const { casts, screen, answered } = receiverOnScreen();
    casts.heard(
      { type: "guest", grant: "g", reach, showsLog: true },
      (answer) => answered.push(answer),
      () => undefined,
    );
    expect(screen.hidden).toBe(false);
    await vi.waitFor(() => {
      expect(answered).toHaveLength(1);
    });
  });

  it("notes a launch it does not know, showing nothing and answering nothing", () => {
    const { casts, screen, answered } = receiverOnScreen();
    casts.heard(
      { type: "device" },
      (answer) => answered.push(answer),
      () => undefined,
    );
    expect(screen.hidden).toBe(true);
    expect(screen.textContent).toContain("a cast it does not know");
    expect(answered).toEqual([]);
  });
});
