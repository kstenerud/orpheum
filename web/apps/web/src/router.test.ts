// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { parseRoute, routeHref, type Route } from "./router.ts";

describe("parseRoute and routeHref", () => {
  it("read back every route they write", () => {
    const routes: Route[] = [
      { screen: "films" },
      { screen: "series" },
      { screen: "title", id: 12 },
      { screen: "series-page", id: 7 },
      { screen: "play", fileId: 9 },
      { screen: "search", query: "glass runner & co" },
      { screen: "search", query: "" },
      { screen: "identify" },
      { screen: "libraries" },
      { screen: "files", libraryId: 3 },
      { screen: "about" },
      { screen: "settings" },
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
    ]) {
      expect(parseRoute(pathname, "")).toEqual({ screen: "not-found" });
    }
    expect(parseRoute("/", "")).toEqual({ screen: "films" });
  });
});
