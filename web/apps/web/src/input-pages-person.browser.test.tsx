// SPDX-License-Identifier: MPL-2.0

/**
 * The app's pages walked with a remote's keys in a real browser, as input-pages.browser.test.tsx
 * walks them, here This device, Your account, sign-in and Watch Together: each arrived at, every
 * control on it reached by arrows, one chosen, and Back finding focus where it was.
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { chooseOk, okChoice } from "./controller-choice.ts";
import { closeTrayDialog } from "./tray-dialog.ts";
import { createSession } from "./session.ts";
import { screens, signedIn, devices, remoteWalk } from "./test-walk.tsx";
import { focusedName, isFocusIn, mountApp, walkServices, type Walking } from "./test-walk.tsx";

const { press, pressUntil, drawn, unreachedInMain, unreachedIn } = remoteWalk({
  expect,
  keyboard: async (keys) => {
    await userEvent.keyboard(keys);
  },
});

describe.each(screens)("the pages by a remote's keys, on $name", ({ width, height }) => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
  });

  it("chooses a controller's OK on This device by arrows and OK, every control reached", async () => {
    chooseOk("automatic");
    await page.viewport(width, height);
    walking = await mountApp(
      walkServices({ ...signedIn, "GET /me/devices": { devices: [] } }, "/this-device"),
    );
    await drawn('input[value="right"]');
    try {
      // No device listed, so no Rename to arrive at.
      expect(await unreachedInMain({ hasArrival: false })).toEqual([]);
      document.querySelector<HTMLElement>('input[value="automatic"]')?.focus();
      expect(
        await pressUntil(
          "{ArrowDown}",
          () => document.activeElement?.getAttribute("value") === "right",
        ),
      ).toBe(true);
      await press("{Enter}");
      expect(okChoice()).toBe("right");
    } finally {
      chooseOk("automatic");
    }
  });

  it("goes Down from This device's Rename, its arrival, to the controller's choices", async () => {
    await page.viewport(width, height);
    walking = await mountApp(
      walkServices({ ...signedIn, "GET /me/devices": { devices: devices(1) } }, "/this-device"),
    );
    await expect.poll(focusedName).toBe("Rename");
    expect(await unreachedInMain()).toEqual([]);
    document.querySelector<HTMLElement>(".your-devices__name button")?.focus();
    expect(
      await pressUntil(
        "{ArrowDown}",
        () => document.activeElement?.getAttribute("type") === "radio",
      ),
    ).toBe(true);
  });

  it("goes Right on Your account from the code, its arrival, to the name and Activate, Down to every control, the devices past five behind All devices", async () => {
    await page.viewport(width, height);
    walking = await mountApp(
      walkServices({ ...signedIn, "GET /me/devices": { devices: devices(7) } }, "/account"),
    );
    await drawn("details.your-devices__all");
    await expect
      .poll(() => document.activeElement?.classList.contains("device-activation__code-field"))
      .toBe(true);
    expect(await unreachedInMain()).toEqual([]);
    document.querySelector<HTMLElement>(".device-activation__code-field")?.focus();
    await press("{ArrowRight}");
    // The device's name: the field beside the code.
    expect(document.activeElement?.getAttribute("maxlength")).toBe("100");
    await press("{ArrowRight}");
    expect(focusedName()).toBe("Activate");
    expect(document.querySelectorAll(".your-devices__device")).toHaveLength(5);
    expect(await pressUntil("{ArrowDown}", () => focusedName() === "All devices (7)", 30)).toBe(
      true,
    );
  });

  it("reaches every control of the sign-in page and types in its fields", async () => {
    await page.viewport(width, height);
    const services = walkServices({ "GET /client-settings": {} });
    const signedOut = { ...services, session: createSession(undefined) };
    walking = await mountApp(signedOut);
    await drawn("input[name=name]");
    expect(await unreachedInMain()).toEqual([]);
    document.querySelector<HTMLElement>("input[name=name]")?.focus();
    await press("karl");
    expect(document.querySelector<HTMLInputElement>("input[name=name]")?.value).toBe("karl");
    // Down leaves the one-line field for the next, as on a TV's form.
    await press("{ArrowDown}");
    expect(document.activeElement?.getAttribute("type")).toBe("password");
  });

  it("starts Watch Together by OK in its dialog, opened over home from its old address, reaches every control of its session, and Back closes its invitation (D384 §2)", async () => {
    window.sessionStorage.clear();
    await page.viewport(width, height);
    walking = await mountApp(
      walkServices(
        {
          ...signedIn,
          "GET /me/together": { sessions: [] },
          "POST /together": { sessionId: 3, membershipCredential: "host-credential", isNew: true },
          "GET /together/3": {
            sessionId: 3,
            nextRights: { canPlayPause: true, canSeek: true },
            links: [{ id: 4, rights: { canPlayPause: true, canSeek: false }, hostNote: "Grandma" }],
            members: [{ id: 1, name: "Karl", isHost: true }],
          },
        },
        "/watch-together",
      ),
    );
    // Its code loads as the dialog is first shown (later-screens.tsx).
    await drawn('dialog[aria-label="Watch Together"][open]');
    expect(walking.services.navigation.route()).toEqual({ screen: "home" });
    try {
      expect(await pressUntil("{ArrowDown}", () => focusedName() === "Start a session")).toBe(true);
      await press("{Enter}");
      await expect.poll(() => document.body.textContent).toContain("Grandma");
      await press("{ArrowDown}");
      expect(await unreachedIn('dialog[aria-label="Watch Together"]')).toEqual([]);
      const invite = [
        ...document.querySelectorAll<HTMLElement>('dialog[aria-label="Watch Together"] button'),
      ].find((button) => button.textContent === "Invite someone");
      invite?.focus();
      await press("{Enter}");
      await expect.poll(() => document.querySelectorAll("dialog[open]").length).toBe(2);
      await press("{ArrowDown}{ArrowDown}");
      expect(isFocusIn(".watch-together__dialog")).toBe(true);
      await press("{Escape}");
      await expect.poll(() => document.querySelectorAll("dialog[open]").length).toBe(1);
      expect(walking.services.navigation.route()).toEqual({ screen: "home" });
    } finally {
      window.sessionStorage.clear();
      closeTrayDialog(); // the next test's app opens with none
    }
  });
});
