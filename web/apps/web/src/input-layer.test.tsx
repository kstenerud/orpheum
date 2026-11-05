// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, useLayoutEffect, useState, type JSX } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { BackButton } from "./back-button.tsx";
import { commandEventName } from "./input-commands.ts";
import { useCommands } from "./input-layer.tsx";
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
    vi.restoreAllMocks();
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

  // The owner's Fire TV, 2026-10-29: a series chosen far down the Series list opened scrolled to
  // its last episodes, the window keeping the list's scroll.
  it("opens a page a link leads to at its top, leaving the scroll to Back and to a view's replace", async () => {
    const scrolledTo = vi.spyOn(window, "scrollTo").mockImplementation(() => undefined);
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
    expect(scrolledTo).toHaveBeenCalledTimes(1);
    expect(scrolledTo).toHaveBeenCalledWith({ top: 0, left: 0 });
    act(() => {
      services.navigation.replace({ screen: "title", itemId: 3 });
    });
    press("Escape", 27);
    await settleUntil(() => document.activeElement?.textContent === "Nosferatu");
    expect(scrolledTo).toHaveBeenCalledTimes(1);
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

/**
 * Controls shown by a press of their button, each command heard named with what showed as it was
 * heard, and Back offered as the shown controls are drawn, before anything after the drawing runs: a
 * Fire TV's Back as soon after the press as it can come.
 */
function ShownControls(props: { readonly heard: string[] }): JSX.Element {
  const [shown, setShown] = useState("hidden");
  return (
    <>
      <button
        type="button"
        onClick={() => {
          setShown("full");
        }}
      >
        {shown}
      </button>
      <ControlsHearing shown={shown} heard={props.heard} />
      <BackOnceShown shown={shown} />
    </>
  );
}

/** Hears the commands, noting each with what showed as it was heard. */
function ControlsHearing(props: { readonly shown: string; readonly heard: string[] }): null {
  useCommands(
    "document",
    (detail) => {
      props.heard.push(`${detail.command} while ${props.shown}`);
    },
    { isCapture: true },
  );
  return null;
}

/** Offers Back as the controls are drawn shown, in the drawing's own layout effects. */
function BackOnceShown(props: { readonly shown: string }): null {
  useLayoutEffect(() => {
    if (props.shown === "full") {
      const detail = { command: "back", source: "key" };
      document.body.dispatchEvent(
        new CustomEvent(commandEventName, { bubbles: true, cancelable: true, detail }),
      );
    }
  }, [props.shown]);
  return null;
}

describe("a component's commands", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  // A Fire TV's Back just after Up showed the player's controls left the film, heard by the handler
  // of the controls still hidden (the overlay's browser test, 2026-11-05).
  it("hears a command offered as soon as the page is drawn by the handler that drew it", async () => {
    const heard: string[] = [];
    mounted = await mount(<ShownControls heard={heard} />);
    act(() => {
      mounted?.container.querySelector("button")?.click();
    });
    expect(mounted.container.textContent).toBe("full");
    expect(heard).toEqual(["back while full"]);
  });
});
