// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { tellNextChunkLoadMs } from "@orpheum/sdk";
import { act, type JSX } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { Screen } from "./app.tsx";
import { firstAskOf, Play } from "./play.tsx";
import {
  bodyField,
  mount,
  playbackCauses,
  servedBy,
  settleUntil,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";

describe("playing here from where a cast stood (§9 Q5)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  /** How often the video of film 2's Play page, page, was played once loaded far ahead of it. */
  async function playsOf(page: JSX.Element): Promise<number> {
    const services = servedBy({
      "POST /playback": [200, directDecision({ resumePositionMs: 0 })],
      "PUT /files/2/preferences": [204, undefined],
      "GET /files/2/waits": [200, {}],
      "GET /files/2/up-next": [200, {}],
      "GET /files/2/trickplay": [404, {}],
      "POST /playback/t/report": [204, undefined],
      "PUT /playback/t/wants": [204, null],
      "DELETE /playback/t": [204, null],
    });
    services.session.signIn("token", { deviceId: 5 });
    mounted = await mount(withServices(page, services));
    await settleUntil(() => mounted?.container.querySelector("video") !== null);
    const video = mounted.container.querySelector("video");
    if (video === null) {
      throw new Error("no video");
    }
    let plays = 0;
    Object.defineProperty(video, "readyState", { value: 4 });
    Object.defineProperty(video, "networkState", { value: 2 }); // loading
    Object.defineProperty(video, "buffered", {
      get: () => ({ length: 1, start: () => 0, end: () => 7200 }),
    });
    Object.defineProperty(video, "play", {
      value: async (): Promise<void> => {
        plays += 1;
        await Promise.resolve();
      },
    });
    tellNextChunkLoadMs(video, () => ({ loadMs: 100, from: "confident" }));
    for (const event of ["loadstart", "loadedmetadata", "canplay", "progress"]) {
      await act(async () => {
        video.dispatchEvent(new Event(event));
        await new Promise((resolve) => setTimeout(resolve, 0));
      });
    }
    mounted.unmount();
    mounted = undefined;
    return plays;
  }

  it("waits paused there where the receiver had the film paused as its sender took it over, and plays at once where it played (D385 §2)", async () => {
    expect(await playsOf(<Play fileId={2} atMs={982_680} isPaused />)).toBe(0);
    expect(await playsOf(<Play fileId={2} atMs={982_680} />)).toBeGreaterThan(0);
  });

  it("opens a cast received paused there where its sender had the film paused, and playing where it played (the owner, 2026-11-08)", () => {
    const cast = {
      castId: 9,
      membershipCredential: "receiver",
      fileId: 2,
      itemName: "Metropolis",
      positionMs: 61_000,
      fromDeviceId: 1,
      fromDeviceName: "Pixel",
    };
    const nothingGiven = { atMs: undefined, round: 0 };
    const receiving = (isPaused?: boolean) => ({
      cast: isPaused === undefined ? cast : { ...cast, isPaused },
      onLeave: () => undefined,
    });
    expect(firstAskOf(receiving(true), nothingGiven)).toEqual({
      cause: "playPage",
      atMs: 61_000,
      isPaused: true,
    });
    expect(firstAskOf(receiving(), nothingGiven)).toEqual({
      cause: "playPage",
      atMs: 61_000,
      isPaused: false,
    });
  });

  it("waits paused there from its address, as on another film's page the cast had moved on to (D385 §2)", async () => {
    const route = { screen: "play", fileId: 2, atMs: 982_680, shouldPlayHere: true } as const;
    expect(await playsOf(<Screen route={{ ...route, isPaused: true }} />)).toBe(0);
    expect(await playsOf(<Screen route={route} />)).toBeGreaterThan(0);
  });

  it("starts the film there, not where the server last heard it was, and asks saying so", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "POST /playback": [200, directDecision({ resumePositionMs: 0 })],
        "PUT /files/2/preferences": [204, undefined],
        "GET /files/2/waits": [200, {}],
        "GET /files/2/up-next": [200, {}],
        "GET /files/2/trickplay": [404, {}],
        "DELETE /playback/t": [204, null],
      },
      heard,
    );
    services.session.signIn("token", { deviceId: 5 });
    // A Play here mid-film played from 0:00 (staging, 2026-10-27).
    mounted = await mount(withServices(<Play fileId={2} atMs={982_680} />, services));
    const video = mounted.container.querySelector("video");
    if (video === null) {
      throw new Error("no video");
    }
    Object.defineProperty(video, "readyState", { value: 1, configurable: true });
    video.dispatchEvent(new Event("loadedmetadata"));
    expect(video.currentTime).toBeCloseTo(982.68);
    const asked = heard.filter((each) => each.methodAndPath === "POST /playback");
    expect(playbackCauses(heard)).toEqual(["castOver"]);
    expect(asked.map((each) => bodyField(each, "startsAtMs"))).toEqual([982_680]);
  });
});
