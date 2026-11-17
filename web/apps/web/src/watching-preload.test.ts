// SPDX-License-Identifier: MPL-2.0

/**
 * The hidden preload of a film's start (preloadInHiddenVideo): the player it starts is handed what
 * the start was answered with (preloadOptionsOf).
 */

import { describe, expect, it } from "vitest";

import { startOn, type LadderOffer } from "./moving-down.ts";
import { preloadOptionsOf } from "./watching.ts";

const playlists = {
  master: { url: "/api/v1/bytes/media/chunks/t/master.m3u8", text: "#EXTM3U\n" },
  picture: { url: "/api/v1/bytes/media/chunks/t/v2/index.m3u8", text: "#EXTM3U\n" },
};

const offer: LadderOffer = {
  url: "/api/v1/bytes/media/chunks/t/master.m3u8",
  startAtBps: 1_896_000,
  qualityKbps: 1200,
  height: 480,
  isBelowTheLowest: false,
  soundCarriedGainDb: 0,
  isReady: false,
  filmPictureKbps: 9_900,
  linkSharePercent: 70,
  steadiness: 0,
  playlists,
};

describe("the hidden preload of a film's start", () => {
  it("hands hls.js the playlists the start was answered with, so it need not ask for them (D502 §3)", () => {
    expect(preloadOptionsOf(offer, 0, startOn).loadedPlaylists).toEqual(playlists);
  });
});
