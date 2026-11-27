// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { forgetFilmVolumeRead, setFilmVolume } from "./film-volume.ts";
import { Play } from "./play.tsx";
import { mount, servedBy, settleUntil, type Mounted } from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  window.localStorage.clear();
  forgetFilmVolumeRead();
});

/** File fileId's Play page, played alone, in no session and cast to nothing; its video. */
async function videoOfFilePlayedAlone(fileId: number): Promise<HTMLVideoElement> {
  const services = servedBy({
    "POST /playback": [200, directDecision({ resumePositionMs: 0 })],
    [`GET /files/${String(fileId)}/waits`]: [200, {}],
    [`GET /files/${String(fileId)}/up-next`]: [200, {}],
    [`GET /files/${String(fileId)}/trickplay`]: [404, {}],
    "POST /playback/t/report": [204, null],
    "PUT /playback/t/wants": [204, null],
    "DELETE /playback/t": [204, null],
  });
  services.session.signIn("token", { deviceId: 5 });
  services.navigation.go({ screen: "play", fileId });
  mounted = await mount(withServices(<Play fileId={fileId} />, services));
  const shown = mounted;
  await settleUntil(() => shown.container.querySelector("video") !== null);
  const video = shown.container.querySelector("video");
  if (video === null) {
    throw new Error("no video");
  }
  return video;
}

describe("the volume a film plays at, alone (the owner, 2026-11-26)", () => {
  it("is the device's, which the film before was turned down to, not full again", async () => {
    setFilmVolume(0.3);
    const first = await videoOfFilePlayedAlone(2);
    expect(first.volume).toBe(0.3);
    first.volume = 0.6; // turned up at the player's own slider
    act(() => {
      first.dispatchEvent(new Event("volumechange"));
    });
    mounted?.unmount();
    const next = await videoOfFilePlayedAlone(4);
    expect(next.volume).toBe(0.6);
  });
});
