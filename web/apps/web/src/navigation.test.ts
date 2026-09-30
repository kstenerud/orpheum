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
    expect(place.location.search).toBe("?q=heat");
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
