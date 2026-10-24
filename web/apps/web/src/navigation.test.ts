// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { createNavigation } from "./navigation.ts";
import { fakePlace, testOrigin } from "./test-services.tsx";

describe("createNavigation", () => {
  it("reads the address, moves to a route and hears the back button", () => {
    const place = fakePlace("/titles/4");
    const navigation = createNavigation(place);
    const heard: string[] = [];
    navigation.subscribe(() => heard.push(navigation.route().screen));
    expect(navigation.route()).toEqual({ screen: "title", itemId: 4 });
    navigation.go({ screen: "search", query: "heat" });
    expect(place.location.search).toBe("?query=heat");
    place.back("/series");
    expect(heard).toEqual(["search", "series"]);
  });

  it("reads the origin and fragment, and replaces the current address in the history", () => {
    const place = fakePlace("/register#abc_DEF-123");
    const navigation = createNavigation(place);
    expect(navigation.origin()).toBe(testOrigin);
    expect(navigation.fragment()).toBe("abc_DEF-123");
    navigation.replace({ screen: "films" });
    expect(place.addresses).toEqual(["/films"]);
    expect(navigation.route()).toEqual({ screen: "films" });
    expect(navigation.fragment()).toBe("");
  });
});

describe("navigation's back", () => {
  it("goes back to the entry before when it is this app's, and up in place when none is", () => {
    const place = fakePlace("/titles/12");
    const navigation = createNavigation(place);
    navigation.back({ screen: "series-page", collectionId: 5, season: 2 });
    expect(place.addresses).toEqual(["/series/5?season=2"]);
    navigation.go({ screen: "title", itemId: 13 });
    navigation.back({ screen: "films" });
    expect(navigation.route()).toEqual({ screen: "series-page", collectionId: 5, season: 2 });
    // Up again never returns to the page it went up from.
    navigation.back({ screen: "series-page", collectionId: 5 });
    navigation.back({ screen: "series" });
    expect(navigation.route()).toEqual({ screen: "series" });
  });

  it("knows an entry this app wrote after the page is loaded again", () => {
    const place = fakePlace("/films");
    createNavigation(place).go({ screen: "title", itemId: 10 });
    const reloaded = createNavigation(place);
    reloaded.back({ screen: "series" });
    expect(reloaded.route()).toEqual({ screen: "films" });
  });
});
