// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { createNavigation, type Place } from "./navigation.ts";
import { routeHref, type Route } from "./router.ts";
import { fakePlace, testOrigin, type FakePlace } from "./test-services.tsx";

describe("navigation's Up (D367 §7)", () => {
  const isSeries = (route: Route): boolean =>
    route.screen === "series-page" && route.collectionId === 5;

  it("keeps the addresses of this app's entries behind each one it adds, the nearest last", () => {
    const place = fakePlace("/");
    const navigation = createNavigation(place, false);
    navigation.go({ screen: "series-page", collectionId: 5 });
    navigation.replace({ screen: "series-page", collectionId: 5, season: 2 });
    navigation.go({ screen: "title", itemId: 12 });
    expect(navigation.behind()).toEqual([
      { screen: "home" },
      { screen: "series-page", collectionId: 5, season: 2 },
    ]);
    // A reload reads them from the entry's state, as the browser keeps it.
    expect(createNavigation(place, false).behind()).toHaveLength(2);
  });

  it("goes back to the nearest parent behind, however far, so Back after it never comes down", () => {
    const place = fakePlace("/");
    const navigation = createNavigation(place, false);
    navigation.go({ screen: "series-page", collectionId: 5 });
    navigation.go({ screen: "title", itemId: 12 });
    navigation.go({ screen: "play", fileId: 30 });
    navigation.up({ screen: "series-page", collectionId: 5, season: 2 }, isSeries);
    expect(navigation.route()).toEqual({ screen: "series-page", collectionId: 5 });
    expect(navigation.behind()).toEqual([{ screen: "home" }]);
    navigation.back();
    expect(navigation.route()).toEqual({ screen: "home" });
  });

  it("puts the parent in place of the page where none is behind, adding no entry", () => {
    const place = fakePlace("/");
    const navigation = createNavigation(place, false);
    navigation.go({ screen: "play", fileId: 30 });
    navigation.up({ screen: "series-page", collectionId: 5, season: 2 }, isSeries);
    expect(place.addresses).toEqual(["/", "/series/5?season=2"]);
    expect(navigation.behind()).toEqual([{ screen: "home" }]);
    navigation.back();
    expect(navigation.route()).toEqual({ screen: "home" });
  });
});

describe("createNavigation", () => {
  it("reads the address, moves to a route and hears the back button", () => {
    const place = fakePlace("/titles/4");
    const navigation = createNavigation(place);
    const heard: string[] = [];
    navigation.subscribe(() => heard.push(navigation.route().screen));
    expect(navigation.route()).toEqual({ screen: "title", itemId: 4 });
    navigation.go({ screen: "browse", query: "heat" });
    expect(place.location.search).toBe("?query=heat");
    place.back("/series");
    expect(heard).toEqual(["browse", "series"]);
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

describe("navigation's notes on history entries", () => {
  it("goes nowhere back with no up where the entry before is not this app's, as a remote's Back on a page with no Back of its own", () => {
    const place = fakePlace("/films");
    const navigation = createNavigation(place);
    expect(navigation.back()).toBe(false);
    expect(place.addresses).toEqual(["/films"]);
    navigation.go({ screen: "browse", query: "" });
    expect(navigation.back()).toBe(true);
    expect(navigation.route()).toEqual({ screen: "films" });
  });

  it("keeps a note with the entry left, written just before go leaves it, and reads it back on returning", () => {
    const place = fakePlace("/films");
    const navigation = createNavigation(place);
    navigation.beforeLeaving(() => {
      navigation.noteOnEntry({ href: `/left-from${place.location.pathname}`, ordinal: 0 });
    });
    navigation.go({ screen: "title", itemId: 4 });
    expect(navigation.entryNote()).toBeUndefined();
    navigation.back();
    expect(navigation.route()).toEqual({ screen: "films" });
    expect(navigation.entryNote()).toEqual({ href: "/left-from/films", ordinal: 0 });
    // The note rides beside what the entry already kept: it is still this app's.
    expect(navigation.back({ screen: "series" })).toBe(true);
    expect(navigation.route()).toEqual({ screen: "series" });
  });
});

describe("a base under a page a television opened on (the owner's Fire TV, 2026-10-25)", () => {
  it("lays one, so the remote's Back steps to it and not out of the browser, then goes up", () => {
    const place = fakePlace("/play/7");
    const navigation = createNavigation(place, true);
    const went: string[] = [];
    navigation.onBase(() => {
      went.push("up");
      navigation.replace({ screen: "title", itemId: 3 });
    });
    place.history.back();
    expect(went).toEqual(["up"]);
    expect(navigation.route()).toEqual({ screen: "title", itemId: 3 });
  });

  it("goes home from the base when no page says where its Back goes", () => {
    const place = fakePlace("/play/7");
    const navigation = createNavigation(place, true);
    place.history.back();
    expect(navigation.route()).toEqual({ screen: "home" });
  });

  it("keeps no secret after the # in the base (D218 §1), and lays none under home", () => {
    const place = fakePlace("/register#abc_DEF-123");
    createNavigation(place, true);
    expect(place.addresses).toEqual(["/register", "/register#abc_DEF-123"]);
    const home = fakePlace("/");
    createNavigation(home, true);
    expect(home.addresses).toEqual(["/"]);
  });
});

/**
 * A place whose history steps come later, as a browser's do: each back or go waits for settle, and
 * its popstate with it.
 */
function steppingLater(place: FakePlace): { readonly place: Place; readonly settle: () => void } {
  const queued: (() => void)[] = [];
  return {
    place: {
      location: place.location,
      history: {
        get state() {
          return place.history.state;
        },
        back: () => {
          queued.push(() => {
            place.history.back();
          });
        },
        go: (delta) => {
          queued.push(() => {
            place.history.go(delta);
          });
        },
        pushState: (data, unused, url) => {
          place.history.pushState(data, unused, url);
        },
        replaceState: (data, unused, url) => {
          place.history.replaceState(data, unused, url);
        },
      },
      addEventListener: (type, listener) => {
        place.addEventListener(type, listener);
      },
    },
    settle: () => {
      for (let step = queued.shift(); step !== undefined; step = queued.shift()) {
        step();
      }
    },
  };
}

// The owner's Fire TV, 2026-10-28: Silk's remote Back is the history's, never a key, so a list open
// over the page closed only by leaving the page.
describe("an entry held over the page while something is open over it", () => {
  const isSeries = (route: Route): boolean =>
    route.screen === "series-page" && route.collectionId === 5;

  it("is the same address, which the browser's Back steps off, telling the hold, the page staying", () => {
    const place = fakePlace("/");
    const navigation = createNavigation(place, false);
    navigation.go({ screen: "films" });
    let closed = 0;
    navigation.holdBack(() => {
      closed += 1;
    });
    expect(place.addresses).toEqual(["/", "/films", "/films"]);
    expect(navigation.behind()).toEqual([{ screen: "home" }]);
    place.history.back();
    expect(closed).toBe(1);
    expect(navigation.route()).toEqual({ screen: "films" });
    place.history.back();
    expect(closed).toBe(1);
    expect(navigation.route()).toEqual({ screen: "home" });
  });

  it("is taken away again when released, the page not moving, and stepped straight back off by Forward", () => {
    const place = fakePlace("/");
    const navigation = createNavigation(place, false);
    navigation.go({ screen: "films" });
    const heard: string[] = [];
    navigation.subscribe(() => heard.push(routeHref(navigation.route())));
    let closed = 0;
    const release = navigation.holdBack(() => {
      closed += 1;
    });
    release();
    expect(closed).toBe(0);
    expect(navigation.route()).toEqual({ screen: "films" });
    expect(navigation.back()).toBe(true);
    expect(navigation.route()).toEqual({ screen: "home" });
    place.history.go(1);
    expect(navigation.route()).toEqual({ screen: "films" });
    place.history.go(1);
    expect(navigation.route()).toEqual({ screen: "films" });
    expect(heard.every((href) => href === "/films" || href === "/")).toBe(true);
  });

  it("is stepped off by the page's own Back first, as a remote's Back key goes", () => {
    const place = fakePlace("/");
    const navigation = createNavigation(place, false);
    navigation.go({ screen: "films" });
    let closed = 0;
    navigation.holdBack(() => {
      closed += 1;
    });
    expect(navigation.back()).toBe(true);
    expect(closed).toBe(1);
    expect(navigation.route()).toEqual({ screen: "films" });
  });

  it("is gone before a go, whose entry is then the only one added, where the browser's steps come later", () => {
    const later = steppingLater(fakePlace("/"));
    const navigation = createNavigation(later.place, false);
    navigation.go({ screen: "series-page", collectionId: 5 });
    navigation.holdBack(() => undefined);
    navigation.go({ screen: "title", itemId: 12 });
    expect(navigation.route()).toEqual({ screen: "series-page", collectionId: 5 });
    later.settle();
    expect(navigation.route()).toEqual({ screen: "title", itemId: 12 });
    expect(navigation.behind()).toEqual([
      { screen: "home" },
      { screen: "series-page", collectionId: 5 },
    ]);
    navigation.back();
    later.settle();
    expect(navigation.route()).toEqual({ screen: "series-page", collectionId: 5 });
  });

  it("is gone before a replace, which then puts the view in the page's own place", () => {
    const later = steppingLater(fakePlace("/"));
    const navigation = createNavigation(later.place, false);
    navigation.go({ screen: "series-page", collectionId: 5 });
    navigation.holdBack(() => undefined);
    navigation.replace({ screen: "series-page", collectionId: 5, season: 2 });
    later.settle();
    expect(navigation.route()).toEqual({ screen: "series-page", collectionId: 5, season: 2 });
    expect(navigation.behind()).toEqual([{ screen: "home" }]);
    navigation.back();
    later.settle();
    expect(navigation.route()).toEqual({ screen: "home" });
  });

  it("is never a page Up finds behind (D367 §7)", () => {
    const place = fakePlace("/");
    const navigation = createNavigation(place, false);
    navigation.go({ screen: "series-page", collectionId: 5 });
    navigation.go({ screen: "title", itemId: 12 });
    navigation.holdBack(() => undefined);
    expect(navigation.behind()).toEqual([
      { screen: "home" },
      { screen: "series-page", collectionId: 5 },
    ]);
    navigation.up({ screen: "series-page", collectionId: 5 }, isSeries);
    expect(navigation.route()).toEqual({ screen: "series-page", collectionId: 5 });
    expect(navigation.behind()).toEqual([{ screen: "home" }]);
  });

  it("is stepped off when the page is opened again on it, as by a reload with a list open", () => {
    const place = fakePlace("/");
    createNavigation(place, false).go({ screen: "films" });
    createNavigation(place, false).holdBack(() => undefined);
    const reloaded = createNavigation(place, false);
    expect(reloaded.route()).toEqual({ screen: "films" });
    expect(reloaded.back()).toBe(true);
    expect(reloaded.route()).toEqual({ screen: "home" });
  });
});
