// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { parseRoute, routeHref, type Route } from "./router.ts";

describe("parseRoute and routeHref", () => {
  it("read back every route they write", () => {
    const routes: Route[] = [
      { screen: "films" },
      { screen: "series" },
      { screen: "title", itemId: 12 },
      { screen: "series-page", collectionId: 7 },
      { screen: "series-page", collectionId: 7, season: 2 },
      { screen: "series-page", collectionId: 7, season: 0 },
      { screen: "play", fileId: 9 },
      { screen: "play", fileId: 9, atMs: 982_680 },
      { screen: "play", fileId: 9, atMs: 982_680, shouldPlayHere: true },
      { screen: "play", fileId: 9, shouldPlayHere: true },
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
      { screen: "libraries" },
      { screen: "library", libraryId: 3 },
      { screen: "new-library" },
      { screen: "files", libraryId: 3 },
      { screen: "about" },
      { screen: "this-device" },
      { screen: "settings" },
      { screen: "together" },
      { screen: "watch-together" },
      { screen: "casting" },
      { screen: "cast-ready" },
      { screen: "lab" },
      { screen: "lab", probe: "device" },
      { screen: "lab", probe: "input" },
      { screen: "lab", probe: "remote" },
      { screen: "lab", probe: "extend" },
      { screen: "lab", probe: "cast" },
      { screen: "lab-extend-player" },
    ];
    for (const route of routes) {
      const url = new URL(routeHref(route), "http://orpheum.test");
      expect(parseRoute(url.pathname, url.search)).toEqual(route);
    }
  });

  it("names nothing it does not know, and never guesses an id", () => {
    for (const pathname of [
      "/titles/abc",
      "/titles/0",
      "/titles/12/extra",
      "/nowhere",
      "/libraries/1/other",
      "/libraries/0",
      "/libraries/newer",
    ]) {
      expect(parseRoute(pathname, "")).toEqual({ screen: "not-found" });
    }
    expect(parseRoute("/", "")).toEqual({ screen: "home" });
    expect(parseRoute("/films", "")).toEqual({ screen: "films" });
    for (const at of ["?at=-5", "?at=1.5", "?at=soon", "?at="]) {
      expect(parseRoute("/play/9", at)).toEqual({ screen: "play", fileId: 9 });
    }
  });

  it("reaches the device lab by a short address, with or without a slash, and no other probe", () => {
    expect(parseRoute("/lab", "")).toEqual({ screen: "lab" });
    expect(parseRoute("/lab/", "")).toEqual({ screen: "lab" });
    expect(parseRoute("/lab/input/", "")).toEqual({ screen: "lab", probe: "input" });
    expect(parseRoute("/lab/extend/player", "")).toEqual({ screen: "lab-extend-player" });
    expect(parseRoute("/lab/elsewhere", "")).toEqual({ screen: "not-found" });
    expect(parseRoute("/laboratory", "")).toEqual({ screen: "not-found" });
  });
});
