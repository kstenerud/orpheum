// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ControlledMode } from "./cast-receiver.tsx";
import { createNavigation, type Place } from "./navigation.ts";
import type { Services } from "./services.tsx";
import { mount, type Mounted } from "./test-dom.tsx";
import { fakePlace, steppingLater, testServices, withServices } from "./test-services.tsx";

/** Takes away the window's popstate listeners a browser-like place added. */
const windowListenersAdded: (() => void)[] = [];

/**
 * Services at path over a place that behaves as a browser's: its history steps come later
 * (steppingLater), each told by a popstate on the window, which every listener of the page hears.
 */
function browserLike(path: string): {
  readonly services: Services;
  readonly place: Place;
  readonly settle: () => void;
} {
  const inner = fakePlace(path);
  inner.addEventListener("popstate", () => {
    window.dispatchEvent(new PopStateEvent("popstate"));
  });
  const later = steppingLater(inner);
  const place: Place = {
    ...later.place,
    addEventListener: (type, listener) => {
      window.addEventListener(type, listener);
      windowListenersAdded.push(() => {
        window.removeEventListener(type, listener);
      });
    },
  };
  const services = { ...testServices("token", path), navigation: createNavigation(place, false) };
  return { services, place, settle: later.settle };
}

// A Fire TV's remote Back in Silk is a step of the history, never a key (D338 §6).
describe("the receiver's controlled mode at a step through the history", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    for (const takeAway of windowListenersAdded.splice(0)) {
      takeAway();
    }
  });

  /** A receiver's controlled mode over services. */
  async function controlled(services: Services): Promise<void> {
    mounted = await mount(withServices(<ControlledMode fromName="Pixel" />, services));
  }

  /** How many held entries the history stands above the page's own. */
  function heldDepth(place: Place): unknown {
    const state: unknown = place.history.state;
    return typeof state === "object" && state !== null
      ? Reflect.get(state, "heldDepth")
      : undefined;
  }

  // The owner, 2026-11-07 (the 11-20 batch, answer 4): nothing on the receiver ends the cast.
  it("stays on its page at a Fire TV's Back, each step taking an entry it holds again", async () => {
    const browser = browserLike("/play/3");
    const pushed = vi.spyOn(browser.services.navigation, "go");
    await controlled(browser.services);
    act(() => {
      browser.settle();
    });
    expect(heldDepth(browser.place)).toBe(1);
    for (const step of ["the first Back", "the second"]) {
      act(() => {
        browser.place.history.back();
        browser.settle();
      });
      expect(browser.services.navigation.route(), step).toEqual({ screen: "play", fileId: 3 });
      expect(heldDepth(browser.place), step).toBe(1);
    }
    expect(pushed).not.toHaveBeenCalled();
  });

  // Staging, 2026-10-30: the Fire TV gave a cast's control back 16 ms after its Use, the step
  // closing the Use question heard as the person's Back (D384).
  it("stays on its page through the step the Use question's close makes, and at the Back after", async () => {
    const browser = browserLike("/play/3");
    const releaseQuestion = browser.services.navigation.holdBack(() => undefined);
    browser.settle();
    releaseQuestion(); // Use pressed: the question's held entry is stepped off, a step still to come
    await controlled(browser.services);
    act(() => {
      browser.settle();
    });
    expect(browser.services.navigation.route()).toEqual({ screen: "play", fileId: 3 });
    expect(heldDepth(browser.place)).toBe(1);
    act(() => {
      browser.place.history.back();
      browser.settle();
    });
    expect(browser.services.navigation.route()).toEqual({ screen: "play", fileId: 3 });
    expect(heldDepth(browser.place)).toBe(1);
  });
});
