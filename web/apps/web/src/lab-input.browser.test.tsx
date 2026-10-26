// SPDX-License-Identifier: MPL-2.0

/**
 * The button test as a real browser lays it out in the app's shell: it fits a Fire TV's screen,
 * 960 × 480 CSS pixels below Silk's address bar (its run ONCULPN4), with no scrolling, its details closed (the owner, 2026-10-25: the browser's
 * address bar, the title bar and the test, all at once), and it spills sideways nowhere on a phone.
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";
import { page } from "vitest/browser";

import { AppShell } from "./app-shell.tsx";
import { Lab } from "./lab.tsx";
import { DeviceTestsOfferedContext } from "./lab-offered.ts";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** The button test in the shell, as a signed-in tester with Device tests offered sees it. */
const inTheShell = (
  <DeviceTestsOfferedContext value={true}>
    <AppShell
      isAdmin={false}
      canHostWatchTogether={true}
      currentScreen="lab"
      onSignOut={() => undefined}
      signedInAs={{ name: "tester", deviceName: undefined }}
      showsDeviceTests={true}
    >
      <Lab probe="input" />
    </AppShell>
  </DeviceTestsOfferedContext>
);

/** How far the page reaches down and across, against the window. */
function extent(): { readonly height: number; readonly width: number } {
  const scrolling = document.scrollingElement ?? document.documentElement;
  return { height: scrolling.scrollHeight, width: scrolling.scrollWidth };
}

describe("the button test, laid out by a real browser", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("fits a Fire TV's 960 × 480 under Silk's address bar with no scrolling, in the shell", async () => {
    await page.viewport(960, 480);
    mounted = await mount(withServices(inTheShell));
    expect(mounted.container.querySelector("#lab-input-heading")).not.toBeNull();
    expect(extent().height).toBeLessThanOrEqual(window.innerHeight);
    expect(extent().width).toBeLessThanOrEqual(window.innerWidth);
  });

  it("fits with a controller of unknown layout connected, its choice of OK shown", async () => {
    // An 8BitDo in D-input mode, whose vendor says nothing of its layout.
    const pad = {
      index: 0,
      id: "8Bitdo SF30 Pro (STANDARD GAMEPAD Vendor: 2dc8 Product: 6102)",
      mapping: "standard",
      buttons: Array.from({ length: 17 }, () => ({ pressed: false })),
      axes: [0, 0, 0, 0],
    };
    Object.defineProperty(navigator, "getGamepads", { configurable: true, value: () => [pad] });
    try {
      await page.viewport(960, 480);
      mounted = await mount(withServices(inTheShell));
      globalThis.dispatchEvent(new Event("gamepadconnected"));
      await expect.poll(() => mounted?.container.textContent ?? "").toContain("the right button");
      expect(extent().height).toBeLessThanOrEqual(window.innerHeight);
    } finally {
      Reflect.deleteProperty(navigator, "getGamepads");
    }
  });

  it("moves up from the page's top control into the shell's title bar, by our navigation", async () => {
    // The owner, Silk in spatial mode, 2026-10-25: Up from the page's top reached nothing, our
    // navigation moving within the page alone and preventing Silk's own move.
    await page.viewport(960, 480);
    mounted = await mount(withServices(inTheShell));
    const top = mounted.container.querySelector<HTMLElement>(".lab__back");
    if (top === null) {
      throw new Error("the test has no way back to the lab");
    }
    top.focus();
    const arrow = new KeyboardEvent("keydown", {
      key: "ArrowUp",
      keyCode: 38,
      bubbles: true,
      cancelable: true,
    });
    top.dispatchEvent(arrow);
    expect(arrow.defaultPrevented).toBe(true);
    const banner = mounted.container.querySelector(".app-shell__banner");
    expect(banner?.contains(document.activeElement)).toBe(true);
    expect(document.activeElement?.getClientRects().length).toBeGreaterThan(0);
  });

  it("spills sideways nowhere on a phone", async () => {
    await page.viewport(390, 844);
    mounted = await mount(withServices(inTheShell));
    expect(extent().width).toBeLessThanOrEqual(window.innerWidth);
  });
});
