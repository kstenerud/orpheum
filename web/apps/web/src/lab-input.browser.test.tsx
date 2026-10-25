// SPDX-License-Identifier: MPL-2.0

/**
 * The button test as a real browser lays it out in the app's shell: it fits a Fire TV's screen,
 * 960 × 540 CSS pixels, with no scrolling, its details closed (the owner, 2026-10-25: the browser's
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

  it("fits a Fire TV's 960 × 540 with no scrolling, in the shell", async () => {
    await page.viewport(960, 540);
    mounted = await mount(withServices(inTheShell));
    expect(mounted.container.querySelector("#lab-input-heading")).not.toBeNull();
    expect(extent().height).toBeLessThanOrEqual(window.innerHeight);
    expect(extent().width).toBeLessThanOrEqual(window.innerWidth);
  });

  it("spills sideways nowhere on a phone", async () => {
    await page.viewport(390, 844);
    mounted = await mount(withServices(inTheShell));
    expect(extent().width).toBeLessThanOrEqual(window.innerWidth);
  });
});
