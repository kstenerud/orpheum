// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { PlayView } from "./play.tsx";
import { fire, mount, servedBy, type Mounted } from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const onLadder = {
  url: "/api/v1/bytes/media/chunks/t/master.m3u8",
  startAtBps: 1_896_000,
  qualityKbps: 1200,
  height: 480,
  isBelowTheLowest: false,
  soundCarriedGainDb: 0,
  isReady: true,
  filmPictureKbps: 4000,
  linkSharePercent: 70,
};

describe("the original file, chosen over the ladder (web-client.md: playback is never a black box)", () => {
  it("is offered under a film on its ladder, and asks how the film plays again when pressed", async () => {
    const asked: string[] = [];
    mounted = await mount(
      withServices(
        <PlayView
          fileId={2}
          decision={directDecision({ onLadder })}
          onDecideAgain={(cause) => asked.push(cause)}
        />,
        servedBy({
          "GET /files/2/waits": [200, {}],
          "GET /files/2/availability": [200, {}],
          "GET /files/2/up-next": [200, {}],
          "POST /playback/t/report": [200, {}],
          "DELETE /playback/t": [204, null],
        }),
      ),
    );
    const button = [...mounted.container.querySelectorAll("button")].find(
      (each) => each.textContent === "Play the original file",
    );
    expect(button).toBeDefined();
    await fire(button ?? null, new MouseEvent("click", { bubbles: true }));
    expect(asked).toEqual(["originalChosen"]);
  });

  it("is not offered for a film already playing its own file, which may play in the bare player", async () => {
    mounted = await mount(
      withServices(
        <PlayView fileId={2} decision={directDecision()} onDecideAgain={() => undefined} />,
        servedBy({
          "GET /files/2/waits": [200, {}],
          "GET /files/2/up-next": [200, {}],
          "DELETE /playback/t": [204, null],
        }),
      ),
    );
    expect(mounted.container.textContent).not.toContain("Play the original file");
    const bare = [...mounted.container.querySelectorAll("button")].find(
      (each) => each.textContent === "Play in a bare player",
    );
    await fire(bare ?? null, new MouseEvent("click", { bubbles: true }));
    // The video alone, the browser's own controls, nothing over it (bare-player.tsx).
    const video = mounted.container.querySelector("video.bare-player");
    expect(video?.getAttribute("src")).toBe("/api/v1/bytes/media/direct/abc");
    expect(video?.hasAttribute("controls")).toBe(true);
    expect(mounted.container.querySelector(".film-player__stage")).toBeNull();
  });
});
