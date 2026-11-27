// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { Play } from "./play.tsx";
import { isPausedHere } from "./playing-here.ts";
import { mount, servedBy, settleUntil, type Mounted } from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** File 2's Play page, played alone, opened paused at 61 s where isPaused says; its video. */
async function filmPlayedHere(
  opened: { readonly isPaused?: boolean } = {},
): Promise<HTMLVideoElement> {
  const services = servedBy({
    "POST /playback": [200, directDecision({ resumePositionMs: 0 })],
    "GET /files/2/waits": [200, {}],
    "GET /files/2/trickplay": [404, {}],
    "GET /files/2/up-next": [200, {}],
    "POST /playback/t/report": [204, null],
    "PUT /playback/t/wants": [204, null],
    "DELETE /playback/t": [204, null],
  });
  services.session.signIn("token", { deviceId: 5 });
  services.navigation.go({ screen: "play", fileId: 2 });
  const isPaused = opened.isPaused === true;
  mounted = await mount(
    withServices(
      isPaused ? <Play fileId={2} atMs={61_000} isPaused shouldPlayHere /> : <Play fileId={2} />,
      services,
    ),
  );
  const shown = mounted;
  await settleUntil(() => shown.container.querySelector("video") !== null);
  const video = shown.container.querySelector("video");
  if (video === null) {
    throw new Error("no video");
  }
  return video;
}

/** The video holding media, as one that has loaded its first frame does. */
function holdsMedia(video: HTMLVideoElement): void {
  Object.defineProperty(video, "readyState", { configurable: true, value: 4 });
}

function said(video: HTMLVideoElement, type: "play" | "pause"): void {
  act(() => {
    video.dispatchEvent(new Event(type));
  });
}

// The owner, 2026-11-08: a cast keeps the pause of the caster. A video still loading reads paused,
// and a cast made before the film started went to the TV paused though no one had paused it.
describe("the pause a cast started from the Play page keeps: the person's, and no other", () => {
  it("is none while the video is still loading, though the video reads paused", async () => {
    const video = await filmPlayedHere();
    expect(video.paused).toBe(true);
    expect(isPausedHere(2)).toBe(false);
  });

  it("is the person's pause of the film playing, until it plays again", async () => {
    const video = await filmPlayedHere();
    holdsMedia(video);
    said(video, "play");
    said(video, "pause");
    expect(isPausedHere(2)).toBe(true);
    said(video, "play");
    expect(isPausedHere(2)).toBe(false);
  });

  it("is the pause carried to a film opened paused where its cast's receiver had it (D564 §1), until it plays", async () => {
    const video = await filmPlayedHere({ isPaused: true });
    expect(isPausedHere(2)).toBe(true);
    said(video, "play");
    expect(isPausedHere(2)).toBe(false);
  });
});
