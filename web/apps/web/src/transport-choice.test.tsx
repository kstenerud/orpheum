// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { SwitchableFetch } from "@orpheum/sdk";
import { act } from "react";
import { describe, expect, it } from "vitest";

import { createPathMoves } from "./services.tsx";
import { mount, servedBy, settleUntil, type Heard } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { Title, type TitleDetail } from "./title.tsx";
import { moveToWebrtcWhenSignedIn, type WebrtcWays } from "./transport-choice.ts";

const film: TitleDetail = {
  id: 10,
  kind: "movie",
  name: "Metropolis",
  year: 1995,
  runtimeMs: 170 * 60_000,
  tagline: "A Los Angeles crime saga",
  overview: "A crime saga.",
  identificationState: "identified",
  art: { posterSha256: "p1", backdropSha256: "b1" },
  versions: [
    {
      id: 1,
      files: [
        { id: 1, path: "/m/Metropolis (1995).mkv", streams: [{ id: 1, kind: "video", codec: "h264" }] },
      ],
    },
  ],
  extras: [],
};

/** WebRTC ways whose connection is made at once and whose carrying over ends when the test says. */
function heldWebrtcWays(): WebrtcWays & {
  readonly isCarrying: () => boolean;
  readonly endCarrying: () => void;
} {
  let end: (() => void) | undefined;
  return {
    connect: () =>
      Promise.resolve({
        fetch: () => Promise.reject(new Error("the test sends nothing over WebRTC")),
        openChannel: () => {
          throw new Error("the test opens no channel");
        },
        carrierOf: () => undefined,
      }),
    tellMediaCarriers: () => undefined,
    carryMediaOver: () =>
      new Promise<void>((resolve) => {
        end = resolve;
      }),
    isCarrying: () => end !== undefined,
    endCarrying: () => {
      end?.();
    },
  };
}

const onLadder = {
  url: "/api/v1/bytes/media/chunks/a/master.m3u8",
  startAtBps: 4_000_000,
  qualityKbps: 2400,
  height: 1080,
  isBelowTheLowest: false,
  soundCarriedGainDb: 0,
  isReady: false,
  filmPictureKbps: 2400,
};

describe("a tab moved to WebRTC by ?transport=webrtc on the core's own origin", () => {
  it("says its media go over WebRTC once carried over, and its film page then starts no preload", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /items/10": [200, film],
        "POST /playback": [
          200,
          { delivery: "direct", url: "/u", notDirectReasons: [], mediaToken: "a", onLadder },
        ],
        "PUT /interest/page": [204, null],
        "DELETE /interest/page": [204, null],
      },
      heard,
      "token",
    );
    const pathMoves = createPathMoves();
    const ways = heldWebrtcWays();
    moveToWebrtcWhenSignedIn(
      {
        session: services.session,
        api: services.api,
        routed: new SwitchableFetch(),
        switchTogetherSocket: () => undefined,
        pathMoves,
      },
      (problem) => {
        throw problem;
      },
      ways,
    );
    await settleUntil(ways.isCarrying);
    expect(pathMoves.isOverWebrtc()).toBe(false);
    await act(async () => {
      ways.endCarrying();
      await Promise.resolve();
    });
    expect(pathMoves.isOverWebrtc()).toBe(true);

    const page = await mount(withServices(<Title id={10} />, { ...services, pathMoves }));
    await settleUntil(() => heard.some((each) => each.methodAndPath === "POST /playback"));
    // The decision's answer is read, and a preload would start from it, a moment after.
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 50));
    });
    page.unmount();
    expect(document.body.querySelectorAll("video[hidden]")).toHaveLength(0);
  });
});
