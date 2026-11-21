// SPDX-License-Identifier: MPL-2.0

// @vitest-environment happy-dom
import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AppShell } from "./app-shell.tsx";
import { createNavigation, type Navigation } from "./navigation.ts";
import type { Route } from "./router.ts";
import type { Services } from "./services.tsx";
import { fire, mount, servedBy, settleUntil, type Mounted } from "./test-dom.tsx";
import { fakePlace, type FakePlace, withServices } from "./test-services.tsx";

/** A Fire TV's Silk, whose remote's Back is a step of the history, never a key. */
const silk =
  "Mozilla/5.0 (Linux; Android 9; AFTSSS) AppleWebKit/537.36 (KHTML, like Gecko) Silk/138.19.9 like Chrome/138.0.7204.244 Safari/537.36";

/** A library as the server lists it. */
function library(id: number, name: string): Record<string, unknown> {
  return { id, name, kind: "film", metadataSource: "tmdb", isWritable: true, createdAt: 0 };
}

/** Services whose server lists libraries, over the navigation given, or its own at /films. */
function servicesWith(libraries: readonly number[], navigation?: Navigation): Services {
  const served = servedBy(
    {
      "GET /libraries": [
        200,
        { libraries: libraries.map((id) => library(id, `Library ${String(id)}`)) },
      ],
    },
    [],
    "token",
  );
  return navigation === undefined ? served : { ...served, navigation };
}

/** The shell on screen, its page a list of two cards. */
async function shellOn(screen: Route["screen"], services: Services): Promise<Mounted> {
  const mounted = await mount(
    withServices(
      <AppShell
        isAdmin={false}
        canHostWatchTogether={true}
        currentScreen={screen}
        onSignOut={() => undefined}
        signedInAs={undefined}
      >
        <ul>
          <li>
            <a href="/title/1">First</a>
          </li>
          <li>
            <a href="/title/2">Second</a>
          </li>
        </ul>
      </AppShell>,
      services,
    ),
  );
  // The banner's places draw once the libraries' read has answered.
  await settleUntil(() => mounted.container.querySelector(".app-shell__watching li") !== null);
  return mounted;
}

/** The page's second card, focused. */
function focusCard(mounted: Mounted): HTMLElement {
  const card = mounted.container.querySelector<HTMLElement>('main a[href="/title/2"]');
  if (card === null) {
    throw new Error("the page has no card");
  }
  act(() => {
    card.focus();
  });
  return card;
}

/** A remote's Back, as webOS sends it, at what has focus. */
async function pressBack(): Promise<void> {
  await fire(
    document.activeElement,
    new KeyboardEvent("keydown", { key: "GoBack", keyCode: 461, bubbles: true, cancelable: true }),
  );
}

/** The browser's own Back, as Silk takes a Fire TV remote's. */
async function browsersBack(place: FakePlace): Promise<void> {
  await act(async () => {
    place.history.back();
    await Promise.resolve();
  });
}

/** The control of the banner's place marked current. */
function currentPlace(mounted: Mounted): Element | null {
  return mounted.container.querySelector(
    '.app-shell__watching > li[aria-current="page"] :is(a, [role="button"], button)',
  );
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("Back on a list", () => {
  // The owner, 2026-11-21: "Pressing back while on a film list should bring the cursor to the top
  // bar, over whichever item the view belongs to".
  it("focuses the banner's place for the list first, the page staying; Back from there goes back", async () => {
    const services = servicesWith([1]);
    services.navigation.go({ screen: "series" });
    const mounted = await shellOn("series", services);
    focusCard(mounted);
    await pressBack();
    expect(currentPlace(mounted)).not.toBeNull();
    expect(document.activeElement).toBe(currentPlace(mounted));
    expect(currentPlace(mounted)?.getAttribute("href")).toBe("/series");
    expect(services.navigation.route()).toEqual({ screen: "series" });
    await pressBack();
    expect(services.navigation.route()).toEqual({ screen: "films" });
    mounted.unmount();
  });

  it("focuses Libraries' menu on a library's list, where the person has two", async () => {
    const services = servicesWith([1, 2]);
    services.navigation.go({ screen: "browse", library: 2 });
    const mounted = await shellOn("browse", services);
    await settleUntil(() => currentPlace(mounted)?.getAttribute("role") === "button");
    focusCard(mounted);
    await pressBack();
    expect(document.activeElement).toBe(currentPlace(mounted));
    expect(services.navigation.route()).toEqual({ screen: "browse", library: 2 });
    mounted.unmount();
  });

  it("goes back at once on a page that is no list", async () => {
    const services = servicesWith([1]);
    services.navigation.go({ screen: "about" });
    const mounted = await shellOn("about", services);
    focusCard(mounted);
    await pressBack();
    expect(services.navigation.route()).toEqual({ screen: "films" });
    mounted.unmount();
  });

  // Silk on a Fire TV takes its remote's Back for the history's (idiosyncrasies.md).
  it("focuses the banner's place on a television's browser Back too, each time focus is in the page", async () => {
    vi.spyOn(Navigator.prototype, "userAgent", "get").mockReturnValue(silk);
    const place = fakePlace("/films");
    const navigation = createNavigation(place, false);
    navigation.go({ screen: "series" });
    const mounted = await shellOn("series", servicesWith([1], navigation));
    focusCard(mounted);
    await browsersBack(place);
    expect(document.activeElement).toBe(currentPlace(mounted));
    expect(navigation.route()).toEqual({ screen: "series" });
    // Back into the page, and Back takes focus up again.
    focusCard(mounted);
    await browsersBack(place);
    expect(document.activeElement).toBe(currentPlace(mounted));
    expect(navigation.route()).toEqual({ screen: "series" });
    // From the banner, the browser's Back leaves the page, nothing held in the history over it.
    await browsersBack(place);
    expect(navigation.route()).toEqual({ screen: "films" });
    mounted.unmount();
  });

  // A sort or a search is a view of the list, put in its entry's place (D367 §9), which lets every
  // held entry go (navigation.ts): the list holds one again for the view.
  it("holds the television's Back again for each view of the list", async () => {
    vi.spyOn(Navigator.prototype, "userAgent", "get").mockReturnValue(silk);
    const place = fakePlace("/films");
    const navigation = createNavigation(place, false);
    navigation.go({ screen: "series" });
    const mounted = await shellOn("series", servicesWith([1], navigation));
    focusCard(mounted);
    await act(async () => {
      navigation.replace({ screen: "series", query: "a" });
      await Promise.resolve();
    });
    focusCard(mounted);
    await browsersBack(place);
    expect(document.activeElement).toBe(currentPlace(mounted));
    expect(navigation.route()).toEqual({ screen: "series", query: "a" });
    mounted.unmount();
  });

  it("holds nothing in a computer's history, its browser's Back leaving the list at once", async () => {
    const place = fakePlace("/films");
    const navigation = createNavigation(place, false);
    navigation.go({ screen: "series" });
    const mounted = await shellOn("series", servicesWith([1], navigation));
    focusCard(mounted);
    await browsersBack(place);
    expect(navigation.route()).toEqual({ screen: "films" });
    mounted.unmount();
  });

  it("leaves no entry held in a television's history once the list is gone", async () => {
    vi.spyOn(Navigator.prototype, "userAgent", "get").mockReturnValue(silk);
    const place = fakePlace("/films");
    const navigation = createNavigation(place, false);
    navigation.go({ screen: "series" });
    const mounted = await shellOn("series", servicesWith([1], navigation));
    focusCard(mounted);
    expect(place.history.state).toMatchObject({ heldDepth: 1 });
    mounted.unmount();
    expect(place.history.state).not.toMatchObject({ heldDepth: 1 });
    expect(navigation.route()).toEqual({ screen: "series" });
  });
});
