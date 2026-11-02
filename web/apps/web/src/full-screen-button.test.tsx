// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, type JSX } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { fillTheScreen, setFillsWindow } from "./film-fullscreen.ts";
import { FullScreenButton, useIsFullScreen } from "./full-screen-button.tsx";
import { mount, type Mounted } from "./test-dom.tsx";

/** What the overlay reads: whether the film shows full screen, as text. */
function ShownFullScreen(): JSX.Element {
  return <output>{useIsFullScreen() ? "full screen" : "in the page"}</output>;
}

/** A player's stage on the page, whose browser refuses it full screen. */
function refusedStage(): HTMLElement {
  const stage = document.createElement("div");
  stage.className = "film-player__stage";
  stage.requestFullscreen = (): Promise<void> => Promise.reject(new TypeError("not in a press"));
  document.body.append(stage);
  return stage;
}

describe("whether the film shows full screen (plans/player-overlay.md §2.1, D407)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    Reflect.deleteProperty(document, "fullscreenElement");
    document.body.replaceChildren();
  });

  /** What ShownFullScreen says now. */
  function shown(): string | null | undefined {
    return mounted?.container.querySelector("output")?.textContent;
  }

  it("counts a stage filling the window the browser refused as full screen, until it no longer does", async () => {
    mounted = await mount(<ShownFullScreen />);
    expect(shown()).toBe("in the page");
    const stage = refusedStage();
    await act(async () => {
      fillTheScreen(stage, document);
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(shown()).toBe("full screen");
    // As a cast's receiver lets its stage go back into the page.
    act(() => {
      setFillsWindow(stage, false);
    });
    expect(shown()).toBe("in the page");
  });

  it("follows the browser's full screen as it changes", async () => {
    mounted = await mount(<ShownFullScreen />);
    const stage = refusedStage();
    act(() => {
      Object.defineProperty(document, "fullscreenElement", {
        configurable: true,
        get: () => stage,
      });
      document.dispatchEvent(new Event("fullscreenchange"));
    });
    expect(shown()).toBe("full screen");
    act(() => {
      Reflect.deleteProperty(document, "fullscreenElement");
      document.dispatchEvent(new Event("fullscreenchange"));
    });
    expect(shown()).toBe("in the page");
  });

  it("leaves the button offering full screen while its stage fills the window, as its press asks for it", async () => {
    const stage = refusedStage();
    mounted = await mount(<FullScreenButton />);
    act(() => {
      setFillsWindow(stage, true);
    });
    expect(mounted.container.querySelector("button")?.getAttribute("aria-label")).toBe(
      "Full screen",
    );
  });
});
