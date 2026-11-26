// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { parseRoute, routeHref, type Route } from "./router.ts";

describe("parseRoute and routeHref", () => {
  it("read back every route they write", () => {
    const routes: Route[] = [
      { screen: "films" },
      { screen: "films", sort: "released", order: "ascending", query: "glass runner & co" },
      { screen: "series" },
      { screen: "series", sort: "added", query: "tide" },
      { screen: "films", genre: 18, decade: 1980, isUnwatchedOnly: true },
      { screen: "series", query: "tide", genre: 3 },
      { screen: "title", itemId: 12 },
      { screen: "series-page", collectionId: 7 },
      { screen: "series-page", collectionId: 7, season: 2 },
      { screen: "series-page", collectionId: 7, season: 0 },
      { screen: "play", fileId: 9 },
      { screen: "play", fileId: 9, atMs: 982_680 },
      { screen: "play", fileId: 9, atMs: 982_680, shouldPlayHere: true },
      { screen: "play", fileId: 9, shouldPlayHere: true },
      { screen: "play", fileId: 9, atMs: 982_680, shouldPlayHere: true, isPaused: true },
      { screen: "play", fileId: 9, itemId: 31 },
      { screen: "play", fileId: 9, itemId: 31, atMs: 0 },
      { screen: "browse" },
      { screen: "browse", library: "all" },
      { screen: "browse", library: 3 },
      { screen: "browse", library: 3, sort: "recorded" },
      { screen: "browse", library: 3, sort: "released", order: "ascending" },
      { screen: "browse", library: 3, sort: "name", order: "descending" },
      { screen: "browse", library: 3, view: "folders" },
      { screen: "browse", library: 3, view: "folders", folder: "2024/Summer" },
      { screen: "browse", query: "glass runner & co" },
      { screen: "identify" },
      { screen: "network" },
      { screen: "libraries" },
      { screen: "library", libraryId: 3 },
      { screen: "new-library" },
      { screen: "files", libraryId: 3 },
      { screen: "user", userId: 5 },
      { screen: "person", personId: 41 },
      { screen: "about" },
      { screen: "welcome" },
      { screen: "welcome", step: "discovery" },
      { screen: "this-device" },
      { screen: "settings" },
      { screen: "settings", page: "keys" },
      { screen: "settings", page: "slowLinks" },
      { screen: "settings", page: "remoteAccess", setting: "shouldMapPorts" },
      { screen: "together" },
      { screen: "watch-together" },
      { screen: "casting" },
      { screen: "cast-ready" },
      { screen: "sound-timing" },
    ];
    for (const route of routes) {
      const url = new URL(routeHref(route), "http://orpheum.test");
      expect(parseRoute(url.pathname, url.search)).toEqual(route);
    }
  });

  it("gives each of Settings' pages its own address, landing on a setting where one is named (D538 §11)", () => {
    expect(routeHref({ screen: "settings", page: "keys" })).toBe("/settings/keys");
    expect(routeHref({ screen: "settings", page: "slowLinks", setting: "maxBitrate" })).toBe(
      "/settings/slow-links?setting=maxBitrate",
    );
    expect(parseRoute("/settings/remote-access/", "?setting=")).toEqual({
      screen: "settings",
      page: "remoteAccess",
    });
    expect(parseRoute("/settings/elsewhere", "")).toEqual({ screen: "not-found" });
  });

  it("names nothing it does not know, and never guesses an id", () => {
    for (const pathname of [
      "/titles/abc",
      "/titles/0",
      "/titles/12/extra",
      "/nowhere",
      "/welcome/posters",
      "/libraries/1/other",
      "/libraries/0",
      "/libraries/newer",
    ]) {
      expect(parseRoute(pathname, "")).toEqual({ screen: "not-found" });
    }
    expect(parseRoute("/", "")).toEqual({ screen: "home" });
    expect(parseRoute("/films", "")).toEqual({ screen: "films" });
    for (const chips of [
      "?genre=0",
      "?genre=drama",
      "?decade=1985",
      "?decade=80",
      "?unwatched=yes",
    ]) {
      expect(parseRoute("/films", chips)).toEqual({ screen: "films" });
    }
    for (const at of ["?at=-5", "?at=1.5", "?at=soon", "?at=", "?item=0", "?item=-3", "?item=x"]) {
      expect(parseRoute("/play/9", at)).toEqual({ screen: "play", fileId: 9 });
    }
  });

  // The device lab went (D555 §12): its addresses name nothing; its sound timing test stayed, a page
  // of This device's (the owner, 2026-11-25).
  it("reaches the sound timing test under This device, and the lab's old addresses name nothing", () => {
    expect(parseRoute("/this-device/sound-timing", "")).toEqual({ screen: "sound-timing" });
    expect(parseRoute("/this-device/sound-timing/", "")).toEqual({ screen: "sound-timing" });
    expect(parseRoute("/lab", "")).toEqual({ screen: "not-found" });
    expect(parseRoute("/lab/sound", "")).toEqual({ screen: "not-found" });
    expect(parseRoute("/lab/extend/player", "")).toEqual({ screen: "not-found" });
  });
});
