// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, type JSX } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { BackButton } from "./back-button.tsx";
import { Link } from "./link.tsx";
import { useRoute } from "./services.tsx";
import { mount, settleUntil, type Mounted } from "./test-dom.tsx";
import { testServices, withServices } from "./test-services.tsx";

/** Two screens: a list of two films, and a film's page with its own Back up to the list. */
function TwoScreens(): JSX.Element {
  const route = useRoute();
  if (route.screen === "title") {
    return (
      <main>
        <BackButton up={{ screen: "films" }} />
        <p>{`film ${String(route.itemId)}`}</p>
      </main>
    );
  }
  return (
    <main>
      <Link to={{ screen: "title", itemId: 1 }}>Metropolis</Link>
      <Link to={{ screen: "title", itemId: 2 }}>Nosferatu</Link>
    </main>
  );
}

/** Presses key on what has focus, as a remote's press reaches the page. */
function press(key: string, keyCode = 0): KeyboardEvent {
  const event = new KeyboardEvent("keydown", { key, keyCode, bubbles: true, cancelable: true });
  act(() => {
    (document.activeElement ?? document.body).dispatchEvent(event);
  });
  return event;
}

describe("the input layer in the app", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("gives focus back to the link a page was left by when Back returns to it", async () => {
    const services = testServices("token", "/films");
    mounted = await mount(withServices(<TwoScreens />, services));
    const ronin = [...mounted.container.querySelectorAll("a")].find(
      (link) => link.textContent === "Nosferatu",
    );
    act(() => {
      ronin?.focus();
      ronin?.click();
    });
    expect(mounted.container.textContent).toContain("film 2");
    expect(press("Escape", 27).defaultPrevented).toBe(true);
    await settleUntil(() => document.activeElement?.textContent === "Nosferatu");
    expect(services.navigation.route()).toEqual({ screen: "films" });
  });

  it("goes up where the page's own Back does, from a page opened from outside the app", async () => {
    const services = testServices("token", "/titles/5");
    mounted = await mount(withServices(<TwoScreens />, services));
    expect(mounted.container.textContent).toContain("film 5");
    press("Unidentified", 461);
    await settleUntil(() => mounted?.container.textContent.includes("Nosferatu") === true);
    expect(services.navigation.route()).toEqual({ screen: "films" });
    // Up from the list, with nowhere before it, goes nowhere and leaves the key to the browser.
    expect(press("Escape", 27).defaultPrevented).toBe(false);
  });
});
