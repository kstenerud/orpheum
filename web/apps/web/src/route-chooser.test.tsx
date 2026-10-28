// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { createNavigation, type Navigation } from "./navigation.ts";
import { RouteChooser, type Choice } from "./route-chooser.tsx";
import { buttonNamed, click, fire, mount, type Mounted } from "./test-dom.tsx";
import { fakePlace, testServices, withServices, type FakePlace } from "./test-services.tsx";

let shown: Mounted | undefined;

afterEach(() => {
  shown?.unmount();
  shown = undefined;
});

describe("a chooser", () => {
  // The owner, 2026-10-28: it looked "no different from a regular button, so there's no way to know
  // that it would bring up a choice list".
  it("says it opens a list, by a chevron and to a screen reader, and whether it is open", async () => {
    shown = await mount(
      withServices(
        <RouteChooser
          fieldLabel="Season"
          current="Season 1"
          options={[
            {
              key: "1",
              label: "Season 1",
              to: { screen: "series-page", collectionId: 5, season: 1 },
              isCurrent: true,
            },
            {
              key: "2",
              label: "Season 2",
              to: { screen: "series-page", collectionId: 5, season: 2 },
              isCurrent: false,
            },
          ]}
          isAView
        />,
      ),
    );
    const button = shown.container.querySelector(".route-chooser__button");
    expect(button?.getAttribute("aria-haspopup")).toBe("dialog");
    expect(button?.getAttribute("aria-expanded")).toBe("false");
    const chevron = button?.querySelector("svg.icon.route-chooser__chevron");
    expect(chevron?.getAttribute("aria-hidden")).toBe("true");
    // Its name is its choice alone: the chevron says nothing to a screen reader.
    expect(button?.textContent).toBe("Season 1");
    await click(button);
    expect(button?.getAttribute("aria-expanded")).toBe("true");
    expect(shown.container.querySelector("dialog")?.getAttribute("aria-label")).toBe("Season");
  });
});

/** The libraries a chooser offers, each its own page: a choice adds an entry (D367 §9). */
const libraries: readonly Choice[] = [
  { key: "1", label: "Films", to: { screen: "browse", library: 1 }, isCurrent: true },
  { key: "2", label: "Kids", to: { screen: "browse", library: 2 }, isCurrent: false },
];

/** A library chooser on the Libraries page, reached from home, over its own history. */
async function libraryChooser(): Promise<{
  readonly place: FakePlace;
  readonly navigation: Navigation;
  readonly button: HTMLElement;
}> {
  const place = fakePlace("/");
  const navigation = createNavigation(place, false);
  navigation.go({ screen: "browse", library: 1 });
  shown = await mount(
    withServices(<RouteChooser fieldLabel="Library" current="Films" options={libraries} />, {
      ...testServices(),
      navigation,
    }),
  );
  const button = shown.container.querySelector<HTMLElement>(".route-chooser__button");
  if (button === null) {
    throw new Error("the chooser draws no button");
  }
  act(() => {
    button.focus();
  });
  await click(button);
  return { place, navigation, button };
}

/** The open list of the chooser shown, or null while it is closed. */
function openList(): HTMLDialogElement | null {
  return shown?.container.querySelector("dialog") ?? null;
}

// The owner's Fire TV, 2026-10-28: "with the chooser widget I can only exit without choosing by
// pressing the back button, which actually brings me back a page." Silk's Back is the history's.
describe("a chooser's open list", () => {
  it("closes on the browser's own Back, a step of the history, and the page stays", async () => {
    const { place, navigation, button } = await libraryChooser();
    expect(openList()).not.toBeNull();
    await act(async () => {
      place.history.back();
      await Promise.resolve();
    });
    expect(openList()).toBeNull();
    expect(navigation.route()).toEqual({ screen: "browse", library: 1 });
    expect(document.activeElement).toBe(button);
    // The next Back leaves the page, as it would have with the list never opened.
    await act(async () => {
      place.history.back();
      await Promise.resolve();
    });
    expect(navigation.route()).toEqual({ screen: "home" });
  });

  it("leaves exactly one entry for a choice of another page, the list's own taken away", async () => {
    const { place, navigation } = await libraryChooser();
    const kids = [...(openList()?.querySelectorAll("a") ?? [])].find(
      (link) => link.textContent === "Kids",
    );
    await click(kids);
    expect(openList()).toBeNull();
    expect(place.addresses).toEqual(["/", "/browse?library=1", "/browse?library=2"]);
    expect(navigation.behind()).toEqual([{ screen: "home" }, { screen: "browse", library: 1 }]);
    await act(async () => {
      place.history.back();
      await Promise.resolve();
    });
    expect(navigation.route()).toEqual({ screen: "browse", library: 1 });
  });

  it("closes, choosing nothing, at a press outside it, by its Close, and on Left or Right", async () => {
    const { navigation, button } = await libraryChooser();
    await fire(openList(), new MouseEvent("click", { bubbles: true, clientX: 500, clientY: 500 }));
    expect(openList()).toBeNull();
    await click(button);
    await click(buttonNamed(shown?.container ?? document.body, "Close"));
    expect(openList()).toBeNull();
    // The owner, 2026-10-28: "moving left/right while in a chooser or menu closes it".
    for (const key of ["ArrowLeft", "ArrowRight"]) {
      act(() => {
        button.focus();
      });
      await click(button);
      const first = openList()?.querySelector<HTMLElement>("a");
      act(() => {
        first?.focus();
      });
      await fire(first, new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true }));
      expect(openList(), key).toBeNull();
      expect(document.activeElement, key).toBe(button);
    }
    expect(navigation.route()).toEqual({ screen: "browse", library: 1 });
    expect(navigation.behind()).toEqual([{ screen: "home" }]);
  });
});
