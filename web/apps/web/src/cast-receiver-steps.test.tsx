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

  /** A receiver's controlled mode, and what it was told of leaving. */
  async function controlled(services: Services): Promise<ReturnType<typeof vi.fn>> {
    const onLeave = vi.fn();
    mounted = await mount(
      withServices(<ControlledMode fromName="Pixel" onTakeBack={onLeave} />, services),
    );
    return onLeave;
  }

  it("leaves at a Fire TV's Back, a step back from the entry it keeps", async () => {
    const browser = browserLike("/play/3");
    const pushed = vi.spyOn(browser.services.navigation, "go");
    const onLeave = await controlled(browser.services);
    expect(pushed).toHaveBeenCalledWith({ screen: "play", fileId: 3 });
    act(() => {
      browser.place.history.back();
      browser.settle();
    });
    expect(onLeave).toHaveBeenCalledOnce();
  });

  // Staging, 2026-10-30: the Fire TV gave a cast's control back 16 ms after its Use, the step
  // closing the Use question heard as the person's Back (D384).
  it("keeps control through the step the Use question's close makes, and leaves at the Back after", async () => {
    const browser = browserLike("/play/3");
    const releaseQuestion = browser.services.navigation.holdBack(() => undefined);
    browser.settle();
    releaseQuestion(); // Use pressed: the question's held entry is stepped off, a step still to come
    const onLeave = await controlled(browser.services);
    act(() => {
      browser.settle();
    });
    expect(onLeave).not.toHaveBeenCalled();
    expect(browser.services.navigation.route()).toEqual({ screen: "play", fileId: 3 });
    act(() => {
      browser.place.history.back();
      browser.settle();
    });
    expect(onLeave).toHaveBeenCalledOnce();
  });
});
