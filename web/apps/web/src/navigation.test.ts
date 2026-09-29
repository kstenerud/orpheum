// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { createNavigation, type Place } from "./navigation.ts";

function fakePlace(start: string): Place & { back(to: string): void } {
  const url = new URL(start, "http://orpheum.test");
  const location = { pathname: url.pathname, search: url.search };
  let popstate = (): void => undefined;
  const move = (to: string): void => {
    const next = new URL(to, "http://orpheum.test");
    location.pathname = next.pathname;
    location.search = next.search;
  };
  return {
    location,
    history: {
      pushState: (_data, _unused, to) => {
        move(to);
      },
    },
    addEventListener: (_type, listener) => {
      popstate = listener;
    },
    back: (to) => {
      move(to);
      popstate();
    },
  };
}

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
});
